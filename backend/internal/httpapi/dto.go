package httpapi

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	analytics "universal-defi-intelligence/internal/analytics/domain"
	"universal-defi-intelligence/internal/chains/registry"
	"universal-defi-intelligence/internal/swap/domain"
	"universal-defi-intelligence/internal/swap/service"
)

// The DTOs in this file mirror frontend/src/lib/contracts.ts field for field.

type ChainDTO struct {
	Key                       string             `json:"key"`
	Name                      string             `json:"name"`
	Family                    string             `json:"family"`
	Status                    string             `json:"status"`
	ChainID                   *string            `json:"chainId"`
	NativeCurrency            *NativeCurrencyDTO `json:"nativeCurrency"`
	WalletSupported           bool               `json:"walletSupported"`
	PortfolioBalanceSupported bool               `json:"portfolioBalanceSupported"`
	SwapEnabled               bool               `json:"swapEnabled"`
	Source                    string             `json:"source"`
	WalletConfigSourceURL     *string            `json:"walletConfigSourceUrl"`
	ObservedAt                *string            `json:"observedAt"`
}

type NativeCurrencyDTO struct {
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
}

type AssetDTO struct {
	ID                     string   `json:"id"`
	ProviderAssetID        string   `json:"providerAssetId,omitempty"`
	Symbol                 string   `json:"symbol"`
	Name                   string   `json:"name"`
	Chain                  string   `json:"chain"`
	Decimals               int      `json:"decimals"`
	ContractAddress        *string  `json:"contractAddress"`
	PriceUSD               *float64 `json:"priceUsd"`
	Source                 string   `json:"source"`
	ObservedAt             *string  `json:"observedAt"`
	EnabledForLiveSwap     bool     `json:"enabledForLiveSwap"`
	SourceDepositSupported bool     `json:"sourceDepositSupported"`
}

type QuoteAssetDTO struct {
	ID              string  `json:"id"`
	Symbol          string  `json:"symbol"`
	Chain           string  `json:"chain"`
	Decimals        int     `json:"decimals"`
	ContractAddress *string `json:"contractAddress"`
}

type SwapQuoteDTO struct {
	ID                      string        `json:"id"`
	Mode                    string        `json:"mode"`
	Provider                string        `json:"provider"`
	ProviderQuoteID         string        `json:"providerQuoteId,omitempty"`
	ProviderSignature       *string       `json:"providerSignature"`
	ProviderTimestamp       *string       `json:"providerTimestamp"`
	SignatureVerified       bool          `json:"signatureVerified"`
	OriginAssetID           string        `json:"originAssetId"`
	DestinationAssetID      string        `json:"destinationAssetId"`
	OriginAsset             QuoteAssetDTO `json:"originAsset"`
	DestinationAsset        QuoteAssetDTO `json:"destinationAsset"`
	AmountInAtomic          string        `json:"amountInAtomic"`
	AmountIn                string        `json:"amountIn"`
	AmountOutEstimateAtomic string        `json:"amountOutEstimateAtomic"`
	AmountOutEstimate       string        `json:"amountOutEstimate"`
	MinimumAmountOutAtomic  *string       `json:"minimumAmountOutAtomic"`
	MinimumAmountOut        *string       `json:"minimumAmountOut"`
	FeeBps                  int           `json:"feeBps"`
	SlippageBps             int           `json:"slippageBps"`
	ExpiresAt               *string       `json:"expiresAt"`
	Deadline                *string       `json:"deadline"`
	TimeWhenInactive        *string       `json:"timeWhenInactive"`
	TimeEstimateSeconds     *int          `json:"timeEstimateSeconds"`
	DepositAddress          *string       `json:"depositAddress"`
	DepositMemo             *string       `json:"depositMemo"`
	Recipient               *string       `json:"recipient"`
	RefundTo                *string       `json:"refundTo"`
	Route                   []string      `json:"route"`
	Warning                 string        `json:"warning"`
}

type SwapEventDTO struct {
	State  string `json:"state"`
	Detail string `json:"detail"`
	At     string `json:"at"`
}

type TxRefsDTO struct {
	Source      *string `json:"source"`
	Destination *string `json:"destination"`
	Refund      *string `json:"refund"`
}

type SwapRecordDTO struct {
	ID               string         `json:"id"`
	QuoteID          string         `json:"quoteId"`
	State            string         `json:"state"`
	ProviderStatus   *string        `json:"providerStatus"`
	Mode             string         `json:"mode"`
	RealSettlement   bool           `json:"realSettlement"`
	Simulated        bool           `json:"simulated"`
	ProviderUpdated  *string        `json:"providerUpdatedAt,omitempty"`
	Transactions     TxRefsDTO      `json:"transactions"`
	TransactionLinks TxRefsDTO      `json:"transactionLinks"`
	DepositAddress   *string        `json:"depositAddress"`
	DepositMemo      *string        `json:"depositMemo"`
	ExpiresAt        *string        `json:"expiresAt"`
	CreatedAt        string         `json:"createdAt"`
	UpdatedAt        string         `json:"updatedAt"`
	Events           []SwapEventDTO `json:"events"`
	Quote            SwapQuoteDTO   `json:"quote"`
}

type MetricDTO struct {
	Key                string   `json:"key"`
	Label              string   `json:"label"`
	Value              *float64 `json:"value"`
	Unit               string   `json:"unit"`
	Scope              string   `json:"scope"`
	Definition         string   `json:"definition"`
	Source             string   `json:"source"`
	SourceURL          *string  `json:"sourceUrl"`
	ObservedAt         *string  `json:"observedAt"`
	RetrievedAt        *string  `json:"retrievedAt"`
	CapturedAt         *string  `json:"capturedAt"`
	MethodologyVersion string   `json:"methodologyVersion"`
	Classification     string   `json:"classification,omitempty"`
	Status             string   `json:"status"`
	Error              *string  `json:"error"`
}

type ChainComparisonDTO struct {
	Chain          string  `json:"chain"`
	TVLUSD         float64 `json:"tvlUsd"`
	DailyVolumeUSD float64 `json:"dailyVolumeUsd"`
	Source         string  `json:"source"`
}

type DataSnapshotDTO struct {
	Mode               string               `json:"mode"`
	Provider           string               `json:"provider,omitempty"`
	Chain              string               `json:"chain,omitempty"`
	Status             string               `json:"status,omitempty"`
	RetrievedAt        *string              `json:"retrievedAt"`
	CapturedAt         *string              `json:"capturedAt"`
	MethodologyVersion string               `json:"methodologyVersion,omitempty"`
	Notice             string               `json:"notice"`
	Metrics            []MetricDTO          `json:"metrics"`
	Series             []map[string]any     `json:"series"`
	ChainComparisons   []ChainComparisonDTO `json:"chainComparisons"`
}

type ResearchScenarioDTO struct {
	Key                    string  `json:"key"`
	Label                  string  `json:"label"`
	DailyVolumeUSD         float64 `json:"dailyVolumeUsd"`
	FeeRatePct             float64 `json:"feeRatePct"`
	CapturePct             float64 `json:"capturePct"`
	ActivityGrowthPct      float64 `json:"activityGrowthPct"`
	ProjectedAnnualFeesUSD float64 `json:"projectedAnnualFeesUsd"`
}

type ResearchDTO struct {
	Unit      string                `json:"unit"`
	Note      string                `json:"note"`
	Formula   string                `json:"formula"`
	Scenarios []ResearchScenarioDTO `json:"scenarios"`
}

func strPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func timePtr(value time.Time) *string {
	if value.IsZero() {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}

func atomicString(value *big.Int) string {
	if value == nil {
		return "0"
	}
	return value.String()
}

func chainStatus(features registry.Features, liveSwaps bool) string {
	switch {
	case features.SwapEnabled && liveSwaps:
		return "Live swaps"
	case features.SwapEnabled:
		return "Swap ready (execution disabled)"
	case features.WalletOnly:
		return "Wallet only"
	case features.Planned:
		return "Planned"
	default:
		return "Validation only"
	}
}

func chainDTO(chain registry.Chain, liveSwaps bool) ChainDTO {
	dto := ChainDTO{
		Key:                       chain.Key,
		Name:                      chain.Name,
		Family:                    strings.ToUpper(string(chain.Family)),
		Status:                    chainStatus(chain.Features, liveSwaps),
		NativeCurrency:            &NativeCurrencyDTO{Symbol: chain.NativeCurrency.Symbol, Decimals: chain.NativeCurrency.Decimals},
		WalletSupported:           chain.Family == registry.FamilyEVM,
		PortfolioBalanceSupported: chain.Family == registry.FamilyEVM,
		SwapEnabled:               liveSwaps && chain.Features.SwapEnabled,
		Source:                    "Chain registry",
	}
	if chain.ChainID != "" {
		id := chain.ChainID
		dto.ChainID = &id
	}
	return dto
}

func (h *Handler) assetDTO(asset domain.Asset) AssetDTO {
	swapEnabled, depositSupported := h.swaps.AssetLiveSwapEnabled(asset)
	dto := AssetDTO{
		ID:                     asset.ID,
		ProviderAssetID:        asset.ProviderAssetID,
		Symbol:                 asset.Symbol,
		Name:                   asset.Name,
		Chain:                  asset.Chain,
		Decimals:               asset.Decimals,
		ContractAddress:        strPtr(asset.ContractAddress),
		Source:                 asset.Source,
		ObservedAt:             timePtr(asset.ObservedAt),
		EnabledForLiveSwap:     swapEnabled,
		SourceDepositSupported: depositSupported,
	}
	if micros, ok := service.DemoPriceMicros(asset.ID); ok {
		price := float64(micros) / 1e6
		dto.PriceUSD = &price
	}
	return dto
}

func quoteAssetDTO(asset domain.Asset, fallbackID string) QuoteAssetDTO {
	if asset.ID == "" {
		return QuoteAssetDTO{ID: fallbackID, Symbol: "?", Chain: "unknown"}
	}
	return QuoteAssetDTO{
		ID:              asset.ID,
		Symbol:          asset.Symbol,
		Chain:           asset.Chain,
		Decimals:        asset.Decimals,
		ContractAddress: strPtr(asset.ContractAddress),
	}
}

func feeString(data map[string]any, key string) *string {
	if value, ok := data[key].(string); ok && value != "" {
		return &value
	}
	return nil
}

func (h *Handler) quoteDTO(ctx context.Context, quote domain.Quote) SwapQuoteDTO {
	origin, _ := h.swaps.LookupAsset(ctx, quote.OriginAssetID)
	dest, _ := h.swaps.LookupAsset(ctx, quote.DestinationAssetID)

	dto := SwapQuoteDTO{
		ID:                      quote.ID,
		Mode:                    string(quote.Mode),
		Provider:                quote.ProviderName,
		ProviderQuoteID:         quote.ProviderQuoteID,
		ProviderTimestamp:       feeString(quote.FeeData, "providerTimestamp"),
		OriginAssetID:           quote.OriginAssetID,
		DestinationAssetID:      quote.DestinationAssetID,
		OriginAsset:             quoteAssetDTO(origin, quote.OriginAssetID),
		DestinationAsset:        quoteAssetDTO(dest, quote.DestinationAssetID),
		AmountInAtomic:          atomicString(quote.AmountInAtomic),
		AmountIn:                quote.AmountInDecimal,
		AmountOutEstimateAtomic: atomicString(quote.AmountOutEstimateAtomic),
		AmountOutEstimate:       domain.FormatUnits(quote.AmountOutEstimateAtomic, dest.Decimals),
		FeeBps:                  quote.FeeBPS,
		SlippageBps:             quote.SlippageBPS,
		ExpiresAt:               timePtr(quote.ExpiresAt),
		DepositAddress:          strPtr(quote.DepositAddress),
		DepositMemo:             strPtr(quote.DepositMemo),
		Recipient:               strPtr(quote.Recipient),
		RefundTo:                strPtr(quote.RefundTo),
		Route:                   []string{dto0(origin.Symbol), dto0(dest.Symbol)},
	}
	if verified, ok := quote.FeeData["signatureVerified"].(bool); ok {
		dto.SignatureVerified = verified
	}
	if quote.MinAmountOutAtomic != nil {
		minAtomic := quote.MinAmountOutAtomic.String()
		minDecimal := domain.FormatUnits(quote.MinAmountOutAtomic, dest.Decimals)
		dto.MinimumAmountOutAtomic = &minAtomic
		dto.MinimumAmountOut = &minDecimal
	}
	if quote.Mode == domain.QuoteModeProviderExecutable {
		dto.Deadline = timePtr(quote.ExpiresAt)
	}
	dto.TimeWhenInactive = feeString(quote.FeeData, "timeWhenInactive")
	if seconds, ok := quote.FeeData["timeEstimateSeconds"].(float64); ok {
		value := int(seconds)
		dto.TimeEstimateSeconds = &value
	}
	switch quote.Mode {
	case domain.QuoteModeSimulation:
		dto.Warning = "Illustrative simulation estimate from fixture prices. No on-chain funds will move."
	case domain.QuoteModeProviderDry:
		dto.Warning = "Indicative provider quote. Review it, then request executable deposit instructions."
	default:
		dto.Warning = "Send exactly the quoted amount to the deposit address before the deadline."
	}
	if warning := feeString(quote.FeeData, "warning"); warning != nil {
		dto.Warning = *warning
	}
	return dto
}

func dto0(symbol string) string {
	if symbol == "" {
		return "?"
	}
	return symbol
}

func eventDetail(event domain.Event) string {
	if detail, ok := event.Payload["detail"].(string); ok && detail != "" {
		return detail
	}
	if inferred, ok := event.Payload["inferred"].(bool); ok && inferred {
		return fmt.Sprintf("Inferred from provider status %v.", event.Payload["observedProviderStatus"])
	}
	if reason, ok := event.Payload["reason"].(string); ok && reason != "" {
		return "Reason: " + strings.ReplaceAll(reason, "_", " ") + "."
	}
	if hash, ok := event.Payload["txHash"].(string); ok && hash != "" {
		return "Source transaction " + hash + " recorded."
	}
	if event.OldState == "" {
		return "Swap record created."
	}
	if event.ProviderStatus != "" {
		return "Provider reported " + string(event.ProviderStatus) + "."
	}
	return "State changed."
}

func (h *Handler) swapDTO(ctx context.Context, view service.SwapView) SwapRecordDTO {
	swap := view.Swap
	refs := TxRefsDTO{Source: strPtr(swap.SourceTxHash)}
	for _, tx := range view.Transactions {
		hash := tx.TxHash
		switch tx.Role {
		case domain.TransactionRoleDeposit:
			if refs.Source == nil {
				refs.Source = &hash
			}
		case domain.TransactionRoleDestination:
			refs.Destination = &hash
		case domain.TransactionRoleRefund:
			refs.Refund = &hash
		}
	}
	// Destination and refund hashes reported by the provider live in events.
	for _, event := range view.Events {
		if hash, ok := event.Payload["destinationTxHash"].(string); ok && hash != "" {
			refs.Destination = &hash
		}
		if hash, ok := event.Payload["refundTxHash"].(string); ok && hash != "" {
			refs.Refund = &hash
		}
	}

	links := TxRefsDTO{}
	if !swap.Simulated {
		reg := h.swaps.Registry()
		link := func(chain string, hash *string) *string {
			if hash == nil {
				return nil
			}
			if url, ok := reg.ExplorerTx(chain, *hash); ok {
				return &url
			}
			return nil
		}
		links.Source = link(swap.OriginChain, refs.Source)
		links.Destination = link(swap.DestinationChain, refs.Destination)
		links.Refund = link(swap.OriginChain, refs.Refund)
	}

	events := make([]SwapEventDTO, 0, len(view.Events))
	for _, event := range view.Events {
		events = append(events, SwapEventDTO{
			State:  string(event.NewState),
			Detail: eventDetail(event),
			At:     event.CreatedAt.UTC().Format(time.RFC3339),
		})
	}

	mode := "provider"
	if swap.Simulated {
		mode = "simulation"
	}
	var providerUpdated *string
	if !swap.LastPolledAt.IsZero() {
		providerUpdated = timePtr(swap.LastPolledAt)
	}
	return SwapRecordDTO{
		ID:               swap.ID,
		QuoteID:          swap.QuoteID,
		State:            string(swap.State),
		ProviderStatus:   strPtr(string(swap.ProviderStatus)),
		Mode:             mode,
		RealSettlement:   swap.RealSettlement,
		Simulated:        swap.Simulated,
		ProviderUpdated:  providerUpdated,
		Transactions:     refs,
		TransactionLinks: links,
		DepositAddress:   strPtr(swap.DepositAddress),
		DepositMemo:      strPtr(swap.DepositMemo),
		ExpiresAt:        timePtr(swap.ExpiresAt),
		CreatedAt:        swap.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        swap.UpdatedAt.UTC().Format(time.RFC3339),
		Events:           events,
		Quote:            h.quoteDTO(ctx, view.Quote),
	}
}

func liveSnapshotDTO(snapshot analytics.Snapshot) DataSnapshotDTO {
	metrics := make([]MetricDTO, 0, len(snapshot.Metrics))
	for _, metric := range snapshot.Metrics {
		metrics = append(metrics, MetricDTO{
			Key:                metric.MetricKey,
			Label:              metric.Label,
			Value:              metric.Value,
			Unit:               metric.Unit,
			Scope:              metric.Scope,
			Definition:         metric.Definition,
			Source:             metric.SourceName,
			SourceURL:          strPtr(metric.SourceURL),
			ObservedAt:         timePtr(metric.ObservedAt),
			RetrievedAt:        timePtr(metric.RetrievedAt),
			CapturedAt:         timePtr(snapshot.CapturedAt),
			MethodologyVersion: metric.MethodologyVersion,
			Classification:     "Public market data",
			Status:             string(metric.Status),
			Error:              strPtr(metric.Error),
		})
	}
	series := make([]map[string]any, 0, len(snapshot.TVLSeries))
	for _, point := range snapshot.TVLSeries {
		series = append(series, map[string]any{"date": point.Date, "tvlUsd": point.Value})
	}
	comparisons := make([]ChainComparisonDTO, 0, len(snapshot.ChainComparisons))
	for _, c := range snapshot.ChainComparisons {
		comparisons = append(comparisons, ChainComparisonDTO(c))
	}
	return DataSnapshotDTO{
		Mode:               "live",
		Provider:           snapshot.Provider,
		Chain:              snapshot.Chain,
		Status:             string(snapshot.Status),
		RetrievedAt:        timePtr(snapshot.RetrievedAt),
		CapturedAt:         timePtr(snapshot.CapturedAt),
		MethodologyVersion: snapshot.MethodologyVersion,
		Notice:             snapshot.Notice,
		Metrics:            metrics,
		Series:             series,
		ChainComparisons:   comparisons,
	}
}

func metricValue(snapshot analytics.Snapshot, key string) float64 {
	for _, metric := range snapshot.Metrics {
		if metric.MetricKey == key && metric.Value != nil {
			return *metric.Value
		}
	}
	return 0
}
