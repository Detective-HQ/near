package repository

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"universal-defi-intelligence/internal/storage/postgres"
	"universal-defi-intelligence/internal/swap/domain"
)

const assetColumns = `id,
	provider_asset_id,
	chain_key,
	symbol,
	name,
	decimals,
	COALESCE(contract_address, ''),
	COALESCE(source, ''),
	is_enabled,
	observed_at`

// UpsertAssets refreshes a provider catalogue.
//
// Assets already present for the same (provider_asset_id, chain_key) are
// updated; new ones are inserted. Assets from the same source that the provider
// no longer returns are soft-disabled (is_enabled = FALSE) rather than deleted,
// so historical quotes and swaps keep a resolvable reference. All writes happen
// in one transaction.
func (s *Store) UpsertAssets(
	ctx context.Context,
	assets []domain.Asset,
	source string,
	observedAt time.Time,
) (int, error) {
	if err := s.ready(); err != nil {
		return 0, err
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return 0, domain.New(domain.CodeValidationFailed, "Catalogue source is required.")
	}
	if len(assets) == 0 {
		return 0, nil
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}

	for index := range assets {
		if err := validateAsset(assets[index]); err != nil {
			return 0, err
		}
	}

	count := 0
	err := postgres.WithTransaction(ctx, s.pool, func(tx pgx.Tx) error {
		// Disable the whole source first, then re-enable whatever the provider
		// still returns. A single failed refresh cannot silently drop assets
		// because the caller keeps the previous enabled set on error.
		if _, err := tx.Exec(ctx, `
			UPDATE assets
			SET is_enabled = FALSE, updated_at = $2
			WHERE source = $1 AND is_enabled`, source, observedAt); err != nil {
			return postgres.MapError(err)
		}

		batch := &pgx.Batch{}
		for _, asset := range assets {
			batch.Queue(`
				INSERT INTO assets (
					id, provider_asset_id, chain_key, symbol, name, decimals,
					contract_address, is_enabled, source, observed_at, created_at, updated_at
				) VALUES (
					$1, $2, $3, $4, $5, $6,
					NULLIF($7, ''), TRUE, $8, $9, $9, $9
				)
				ON CONFLICT (provider_asset_id, chain_key) DO UPDATE SET
					symbol = EXCLUDED.symbol,
					name = EXCLUDED.name,
					decimals = EXCLUDED.decimals,
					contract_address = EXCLUDED.contract_address,
					is_enabled = TRUE,
					source = EXCLUDED.source,
					observed_at = EXCLUDED.observed_at,
					updated_at = EXCLUDED.updated_at`,
				asset.ID,
				asset.ProviderAssetID,
				asset.Chain,
				asset.Symbol,
				asset.Name,
				asset.Decimals,
				asset.ContractAddress,
				source,
				observedAt,
			)
		}

		results := tx.SendBatch(ctx, batch)
		for range assets {
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return postgres.MapError(err)
			}
		}
		if err := results.Close(); err != nil {
			return postgres.MapError(err)
		}
		count = len(assets)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ListAssets returns enabled assets, optionally filtered by chain key. An empty
// chain returns every enabled asset.
func (s *Store) ListAssets(ctx context.Context, chain string) ([]domain.Asset, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT `+assetColumns+`
		FROM assets
		WHERE is_enabled AND ($1 = '' OR chain_key = $1)
		ORDER BY chain_key ASC, symbol ASC`, strings.TrimSpace(chain))
	if err != nil {
		return nil, postgres.MapError(err)
	}
	defer rows.Close()

	assets := make([]domain.Asset, 0, 64)
	for rows.Next() {
		asset, scanErr := scanAsset(rows)
		if scanErr != nil {
			return nil, postgres.MapError(scanErr)
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, postgres.MapError(err)
	}
	return assets, nil
}

// GetAssetByID returns an enabled asset by its internal id or provider asset id.
func (s *Store) GetAssetByID(ctx context.Context, id string) (domain.Asset, error) {
	if err := s.ready(); err != nil {
		return domain.Asset{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return domain.Asset{}, domain.New(domain.CodeValidationFailed, "Asset id is required.")
	}
	row := s.pool.QueryRow(ctx, `
		SELECT `+assetColumns+`
		FROM assets
		WHERE is_enabled AND (id = $1 OR provider_asset_id = $1)
		LIMIT 1`, id)
	asset, err := scanAsset(row)
	if err != nil {
		return domain.Asset{}, notFound(err, domain.CodeUnknownAsset, "Asset not found.")
	}
	return asset, nil
}

func scanAsset(row rowScanner) (domain.Asset, error) {
	var asset domain.Asset
	err := row.Scan(
		&asset.ID,
		&asset.ProviderAssetID,
		&asset.Chain,
		&asset.Symbol,
		&asset.Name,
		&asset.Decimals,
		&asset.ContractAddress,
		&asset.Source,
		&asset.Enabled,
		&asset.ObservedAt,
	)
	if err != nil {
		return domain.Asset{}, err
	}
	asset.ObservedAt = asset.ObservedAt.UTC()
	return asset, nil
}

func validateAsset(asset domain.Asset) error {
	if strings.TrimSpace(asset.ID) == "" {
		return domain.New(domain.CodeValidationFailed, "Asset id is required.")
	}
	if strings.TrimSpace(asset.ProviderAssetID) == "" {
		return domain.New(domain.CodeValidationFailed, "Asset provider id is required.")
	}
	if strings.TrimSpace(asset.Chain) == "" {
		return domain.New(domain.CodeValidationFailed, "Asset chain is required.")
	}
	if strings.TrimSpace(asset.Symbol) == "" {
		return domain.New(domain.CodeValidationFailed, "Asset symbol is required.")
	}
	if asset.Decimals < 0 || asset.Decimals > 255 {
		return domain.New(domain.CodeValidationFailed, "Asset decimals must be between 0 and 255.")
	}
	return nil
}
