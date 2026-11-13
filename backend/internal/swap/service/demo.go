package service

import (
	"math/big"
	"strings"
	"time"

	"universal-defi-intelligence/internal/swap/domain"
)

// DemoFixtureDate is the capture date shown for demo catalogue entries.
var DemoFixtureDate = time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)

// demoSimulationFeeBPS is the illustrative fee applied to simulation quotes.
const demoSimulationFeeBPS = 30

type demoAsset struct {
	asset       domain.Asset
	priceMicros int64 // USD price × 1e6, illustrative only
}

var demoCatalogue = []demoAsset{
	{domain.Asset{ID: "demo:ethereum:eth", Symbol: "ETH", Name: "Ether", Chain: "ethereum", Decimals: 18}, 3_400_000_000},
	{domain.Asset{ID: "demo:base:usdc", Symbol: "USDC", Name: "USD Coin", Chain: "base", Decimals: 6}, 1_000_000},
	{domain.Asset{ID: "demo:near:near", Symbol: "NEAR", Name: "NEAR", Chain: "near", Decimals: 24}, 5_100_000},
	{domain.Asset{ID: "demo:solana:sol", Symbol: "SOL", Name: "Solana", Chain: "solana", Decimals: 9}, 165_000_000},
	{domain.Asset{ID: "demo:stellar:xlm", Symbol: "XLM", Name: "Stellar Lumens", Chain: "stellar", Decimals: 7}, 340_000},
	{domain.Asset{ID: "demo:ethereum:usdc", Symbol: "USDC", Name: "USD Coin", Chain: "ethereum", Decimals: 6}, 1_000_000},
	{domain.Asset{ID: "demo:arbitrum:eth", Symbol: "ETH", Name: "Ether", Chain: "arbitrum", Decimals: 18}, 3_400_000_000},
	{domain.Asset{ID: "demo:polygon:usdc", Symbol: "USDC", Name: "USD Coin", Chain: "polygon", Decimals: 6}, 1_000_000},
}

// DemoAssets returns the illustrative demo catalogue.
func DemoAssets() []domain.Asset {
	out := make([]domain.Asset, 0, len(demoCatalogue))
	for _, entry := range demoCatalogue {
		asset := entry.asset
		asset.ProviderAssetID = asset.ID
		asset.Source = "Demo fixture"
		asset.Enabled = true
		asset.ObservedAt = DemoFixtureDate
		out = append(out, asset)
	}
	return out
}

// DemoPriceMicros returns the illustrative USD price × 1e6 of a demo asset.
func DemoPriceMicros(id string) (int64, bool) {
	for _, entry := range demoCatalogue {
		if strings.EqualFold(entry.asset.ID, id) {
			return entry.priceMicros, true
		}
	}
	return 0, false
}

// demoAmountOut converts amountIn atomic units of origin into destination
// atomic units with exact integer math:
//
//	out = in × priceIn × 10^destDecimals / (priceOut × 10^originDecimals)
//
// then subtracts the fee in basis points. Truncation only ever rounds down.
func demoAmountOut(amountIn *big.Int, origin, dest domain.Asset, feeBPS int) *big.Int {
	priceIn, okIn := DemoPriceMicros(origin.ID)
	priceOut, okOut := DemoPriceMicros(dest.ID)
	if !okIn || !okOut || priceOut == 0 {
		priceIn, priceOut = 1, 1
	}
	pow := func(exp int) *big.Int {
		return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp)), nil)
	}
	numerator := new(big.Int).Mul(amountIn, big.NewInt(priceIn))
	numerator.Mul(numerator, pow(dest.Decimals))
	denominator := new(big.Int).Mul(big.NewInt(priceOut), pow(origin.Decimals))
	gross := numerator.Quo(numerator, denominator)
	return applyBPS(gross, feeBPS)
}

// applyBPS returns value × (10000 − bps) / 10000, rounded down.
func applyBPS(value *big.Int, bps int) *big.Int {
	out := new(big.Int).Mul(value, big.NewInt(int64(10_000-bps)))
	return out.Quo(out, big.NewInt(10_000))
}
