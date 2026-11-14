package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"universal-defi-intelligence/internal/config"
	"universal-defi-intelligence/internal/swap/domain"
)

// catalogueSnapshot is an immutable, indexed view of the asset catalogue. It
// is swapped atomically, so readers never lock while serving a request.
type catalogueSnapshot struct {
	assets   []domain.Asset
	byID     map[string]domain.Asset
	loadedAt time.Time
	source   string
}

func newSnapshot(assets []domain.Asset, source string, loadedAt time.Time) *catalogueSnapshot {
	sort.SliceStable(assets, func(i, j int) bool {
		if assets[i].Chain != assets[j].Chain {
			return assets[i].Chain < assets[j].Chain
		}
		return assets[i].Symbol < assets[j].Symbol
	})
	byID := make(map[string]domain.Asset, len(assets)*2)
	for _, asset := range assets {
		byID[strings.ToLower(asset.ID)] = asset
		if asset.ProviderAssetID != "" {
			byID[strings.ToLower(asset.ProviderAssetID)] = asset
		}
	}
	return &catalogueSnapshot{assets: assets, byID: byID, loadedAt: loadedAt, source: source}
}

// CatalogueInfo describes the catalogue that served a request.
type CatalogueInfo struct {
	Source   string
	LoadedAt time.Time
}

// canonicalChain maps provider chain names (eth, arb, sol, ...) to registry
// keys. Unknown chains are kept lower-cased so they stay visible but can never
// pass route checks.
func (s *Service) canonicalChain(chain string) string {
	if key, ok := s.registry.Canonical(chain); ok {
		return key
	}
	return strings.ToLower(strings.TrimSpace(chain))
}

func (s *Service) normalizeAssets(assets []domain.Asset) []domain.Asset {
	out := make([]domain.Asset, 0, len(assets))
	for _, asset := range assets {
		asset.Chain = s.canonicalChain(asset.Chain)
		out = append(out, asset)
	}
	return out
}

// loadCatalogue returns the current snapshot, refreshing it at most once per
// TTL. Concurrent callers share one refresh (singleflight), and a failed
// refresh keeps serving the previous snapshot (stale-on-error).
func (s *Service) loadCatalogue(ctx context.Context) (*catalogueSnapshot, error) {
	s.catalogueMu.RLock()
	current := s.catalogue
	s.catalogueMu.RUnlock()
	if current != nil && s.now().Sub(current.loadedAt) < s.catalogueTTL {
		return current, nil
	}

	result, err, _ := s.catalogueFlight.Do("catalogue", func() (any, error) {
		snapshot, refreshErr := s.refreshCatalogue(ctx)
		if refreshErr != nil {
			return nil, refreshErr
		}
		s.catalogueMu.Lock()
		s.catalogue = snapshot
		s.catalogueMu.Unlock()
		return snapshot, nil
	})
	if err != nil {
		if current != nil {
			s.logger.Warn("asset catalogue refresh failed; serving stale catalogue", "error", err)
			return current, nil
		}
		return nil, err
	}
	return result.(*catalogueSnapshot), nil
}

func (s *Service) refreshCatalogue(ctx context.Context) (*catalogueSnapshot, error) {
	now := s.now()
	if s.quoteMode == config.QuoteModeDemo {
		// The demo catalogue is built in, so every load is a fresh one. Report it,
		// otherwise the risk engine marks the catalogue stale and blocks demo swaps.
		if s.observer != nil {
			s.observer.ObserveCatalogueRefresh(now)
		}
		return newSnapshot(DemoAssets(), "demo_fixture", now), nil
	}

	stored, err := s.store.ListAssets(ctx, "")
	if err == nil && len(stored) > 0 {
		if s.observer != nil {
			s.observer.ObserveCatalogueRefresh(now)
		}
		return newSnapshot(s.normalizeAssets(stored), "provider_catalogue", now), nil
	}

	// Cold store (no worker refresh yet): fetch once from the provider and
	// persist it so the next API instance does not need to.
	tokens, tokErr := s.provider.GetTokens(ctx)
	s.observe(tokErr)
	if tokErr != nil {
		if err != nil {
			return nil, err
		}
		return nil, tokErr
	}
	normalized := s.normalizeAssets(tokens)
	if _, upsertErr := s.store.UpsertAssets(ctx, normalized, s.provider.Name(), now); upsertErr != nil {
		s.logger.Warn("could not persist provider catalogue", "error", upsertErr)
	}
	if s.observer != nil {
		s.observer.ObserveCatalogueRefresh(now)
	}
	return newSnapshot(normalized, "provider_catalogue", now), nil
}

// ListAssets returns the enabled catalogue, optionally filtered by chain (key
// or provider alias).
func (s *Service) ListAssets(ctx context.Context, chain string) ([]domain.Asset, CatalogueInfo, error) {
	snapshot, err := s.loadCatalogue(ctx)
	if err != nil {
		return nil, CatalogueInfo{}, err
	}
	info := CatalogueInfo{Source: snapshot.source, LoadedAt: snapshot.loadedAt}
	chain = strings.TrimSpace(chain)
	if chain == "" {
		return snapshot.assets, info, nil
	}
	want := s.canonicalChain(chain)
	filtered := make([]domain.Asset, 0, 16)
	for _, asset := range snapshot.assets {
		if asset.Chain == want {
			filtered = append(filtered, asset)
		}
	}
	return filtered, info, nil
}

// LookupAsset resolves an asset id from the cached catalogue, falling back to
// the store for assets that were disabled after a quote was made.
func (s *Service) LookupAsset(ctx context.Context, id string) (domain.Asset, bool) {
	if snapshot, err := s.loadCatalogue(ctx); err == nil {
		if asset, ok := snapshot.byID[strings.ToLower(strings.TrimSpace(id))]; ok {
			return asset, true
		}
	}
	if s.quoteMode == config.QuoteModeDemo {
		return domain.Asset{}, false
	}
	asset, err := s.store.GetAssetByID(ctx, id)
	if err != nil {
		return domain.Asset{}, false
	}
	asset.Chain = s.canonicalChain(asset.Chain)
	return asset, true
}

func (s *Service) resolveAsset(ctx context.Context, id string) (domain.Asset, error) {
	asset, ok := s.LookupAsset(ctx, id)
	if !ok {
		return domain.Asset{}, domain.Newf(domain.CodeUnknownAsset, "Asset %q was not found in the catalogue.", id)
	}
	return asset, nil
}

// AssetLiveSwapEnabled reports whether an asset's chain supports executable swaps.
func (s *Service) AssetLiveSwapEnabled(asset domain.Asset) (swapEnabled, depositSupported bool) {
	chain, ok := s.registry.Get(asset.Chain)
	if !ok {
		return false, false
	}
	live := s.quoteMode == config.QuoteModeLive && s.enableLiveExecution
	return live && chain.Features.SwapEnabled, chain.Features.DepositSupported
}
