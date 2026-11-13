// Package trading computes read-only market-structure analytics (signals,
// order blocks, liquidity levels, trade scenarios, a signal journal and
// walk-forward backtests) from public aggregate price history.
//
// Data provenance and limits are explicit by design:
//   - Prices come from the DeFiLlama coins API (an aggregate of DEX/CEX
//     sources). Candles are approximated from sampled prices: open/close are
//     the first/last sample in the bucket and high/low the sample extremes,
//     so intrabar wicks between samples are not captured.
//   - No volume, order-book depth or derivatives data is available from this
//     source; those fields are returned as null with quality "unavailable"
//     rather than estimated.
//   - Signals are deterministic rule outputs (documented in model.go), not
//     predictions or investment advice.
package trading

import "strings"

// Asset is one member of the scanned universe.
type Asset struct {
	ID       string // "<chain>:<symbol>" as used by the frontend
	Symbol   string
	Chain    string // chain registry key
	CoinKey  string // DeFiLlama coins key
	Tier     string // static liquidity classification, see LiquidityTiers
	Category string
}

// Venue and Quote describe where the prices come from. The source is an
// aggregate, so there is exactly one venue and quote currency.
const (
	Venue = "DeFiLlama aggregate"
	Quote = "USD"
)

// LiquidityTiers documents the static tier classification of the universe:
// Tier 1 = large-cap majors with deep multi-venue liquidity, Tier 2 =
// established mid-caps, Tier 3 = smaller ecosystem tokens. Tiers are reviewed
// manually and are not derived from live depth data.
var LiquidityTiers = []string{"Tier 1", "Tier 2", "Tier 3"}

// Universe is the scanned asset list.
var Universe = []Asset{
	{Symbol: "ETH", Chain: "ethereum", CoinKey: "coingecko:ethereum", Tier: "Tier 1", Category: "L1"},
	{Symbol: "SOL", Chain: "solana", CoinKey: "coingecko:solana", Tier: "Tier 1", Category: "L1"},
	{Symbol: "BNB", Chain: "bnb", CoinKey: "coingecko:binancecoin", Tier: "Tier 1", Category: "L1"},
	{Symbol: "AVAX", Chain: "avalanche", CoinKey: "coingecko:avalanche-2", Tier: "Tier 1", Category: "L1"},
	{Symbol: "LINK", Chain: "ethereum", CoinKey: "coingecko:chainlink", Tier: "Tier 1", Category: "Oracle"},
	{Symbol: "NEAR", Chain: "near", CoinKey: "coingecko:near", Tier: "Tier 2", Category: "L1"},
	{Symbol: "XLM", Chain: "stellar", CoinKey: "coingecko:stellar", Tier: "Tier 2", Category: "L1"},
	{Symbol: "APT", Chain: "aptos", CoinKey: "coingecko:aptos", Tier: "Tier 2", Category: "L1"},
	{Symbol: "ARB", Chain: "arbitrum", CoinKey: "coingecko:arbitrum", Tier: "Tier 2", Category: "L2"},
	{Symbol: "POL", Chain: "polygon", CoinKey: "coingecko:polygon-ecosystem-token", Tier: "Tier 2", Category: "L2"},
	{Symbol: "OP", Chain: "ethereum", CoinKey: "coingecko:optimism", Tier: "Tier 2", Category: "L2"},
	{Symbol: "UNI", Chain: "ethereum", CoinKey: "coingecko:uniswap", Tier: "Tier 2", Category: "DeFi"},
	{Symbol: "AAVE", Chain: "ethereum", CoinKey: "coingecko:aave", Tier: "Tier 2", Category: "DeFi"},
	{Symbol: "LDO", Chain: "ethereum", CoinKey: "coingecko:lido-dao", Tier: "Tier 2", Category: "DeFi"},
	{Symbol: "JUP", Chain: "solana", CoinKey: "coingecko:jupiter-exchange-solana", Tier: "Tier 2", Category: "DeFi"},
	{Symbol: "CELO", Chain: "celo", CoinKey: "coingecko:celo", Tier: "Tier 3", Category: "L1"},
	{Symbol: "RAY", Chain: "solana", CoinKey: "coingecko:raydium", Tier: "Tier 3", Category: "DeFi"},
	{Symbol: "REF", Chain: "near", CoinKey: "coingecko:ref-finance", Tier: "Tier 3", Category: "DeFi"},
	{Symbol: "GMX", Chain: "arbitrum", CoinKey: "coingecko:gmx", Tier: "Tier 3", Category: "DeFi"},
	{Symbol: "PENDLE", Chain: "ethereum", CoinKey: "coingecko:pendle", Tier: "Tier 3", Category: "DeFi"},
}

func init() {
	for i := range Universe {
		Universe[i].ID = Universe[i].Chain + ":" + strings.ToLower(Universe[i].Symbol)
	}
}

// FindAsset looks an asset up by id (case-insensitive).
func FindAsset(id string) (Asset, bool) {
	for _, asset := range Universe {
		if strings.EqualFold(asset.ID, strings.TrimSpace(id)) {
			return asset, true
		}
	}
	return Asset{}, false
}
