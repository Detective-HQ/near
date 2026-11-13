package httpapi

import (
	"universal-defi-intelligence/internal/swap/domain"
)

const (
	FixtureDate   = "2026-11-10"
	FixtureNotice = "Illustrative fixture values only. Not live. No market feed, provider catalogue, or chain indexer is connected."
)

type ChainResponse struct {
	Key                       string                `json:"key"`
	Name                      string                `json:"name"`
	Family                    string                `json:"family"`
	Status                    string                `json:"status"`
	ChainID                   *string               `json:"chainId"`
	NativeCurrency            *NativeCurrencyConfig `json:"nativeCurrency"`
	WalletSupported           bool                  `json:"walletSupported"`
	PortfolioBalanceSupported bool                  `json:"portfolioBalanceSupported"`
	SwapEnabled               bool                  `json:"swapEnabled"`
	Source                    string                `json:"source"`
	WalletConfigSourceURL     *string               `json:"walletConfigSourceUrl"`
	ObservedAt                *string               `json:"observedAt"`
}

type NativeCurrencyConfig struct {
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
}

type AssetResponse struct {
	ID                 string  `json:"id"`
	Symbol             string  `json:"symbol"`
	Name               string  `json:"name"`
	Chain              string  `json:"chain"`
	Decimals           int     `json:"decimals"`
	FixturePriceMicros string  `json:"fixturePriceMicros"`
	Source             string  `json:"source"`
	ObservedAt         *string `json:"observedAt"`
	EnabledForLiveSwap bool    `json:"enabledForLiveSwap"`
}

type MetricItem struct {
	Key                string  `json:"key"`
	Label              string  `json:"label"`
	Value              float64 `json:"value"`
	Unit               string  `json:"unit"`
	Scope              string  `json:"scope"`
	Definition         string  `json:"definition"`
	Source             string  `json:"source"`
	ObservedAt         *string `json:"observedAt"`
	CapturedAt         string  `json:"capturedAt"`
	MethodologyVersion string  `json:"methodologyVersion"`
	Classification     string  `json:"classification"`
}

type SeriesItem struct {
	Date           string  `json:"date"`
	ActiveAccounts float64 `json:"activeAccounts"`
	Transactions   float64 `json:"transactions"`
}

type ChainComparison struct {
	Chain          string  `json:"chain"`
	TVLUSD         float64 `json:"tvlUsd"`
	DailyVolumeUSD float64 `json:"dailyVolumeUsd"`
	Source         string  `json:"source"`
}

type MarketOverviewResponse struct {
	Mode             string            `json:"mode"`
	Notice           string            `json:"notice"`
	CapturedAt       string            `json:"capturedAt"`
	Metrics          []MetricItem      `json:"metrics"`
	Series           []SeriesItem      `json:"series"`
	ChainComparisons []ChainComparison `json:"chainComparisons"`
}

type ScenarioItem struct {
	Key               string  `json:"key"`
	Label             string  `json:"label"`
	DailyVolumeUSD    float64 `json:"dailyVolumeUsd"`
	FeeRatePct        float64 `json:"feeRatePct"`
	CapturePct        float64 `json:"capturePct"`
	ActivityGrowthPct float64 `json:"activityGrowthPct"`
}

type ResearchScenariosResponse struct {
	Unit      string         `json:"unit"`
	Note      string         `json:"note"`
	Scenarios []ScenarioItem `json:"scenarios"`
	Formula   string         `json:"formula"`
}

func stringPtr(s string) *string {
	return &s
}

var DefaultChains = []ChainResponse{
	{
		Key:                       "ethereum",
		Name:                      "Ethereum",
		Family:                    "EVM",
		Status:                    "Planned",
		ChainID:                   stringPtr("1"),
		NativeCurrency:            &NativeCurrencyConfig{Symbol: "ETH", Decimals: 18},
		WalletSupported:           true,
		PortfolioBalanceSupported: true,
		SwapEnabled:               false,
		Source:                    "Project configuration",
		WalletConfigSourceURL:     stringPtr("https://eips.ethereum.org/EIPS/eip-2228"),
	},
	{
		Key:                       "base",
		Name:                      "Base",
		Family:                    "EVM",
		Status:                    "Planned",
		ChainID:                   stringPtr("8453"),
		NativeCurrency:            &NativeCurrencyConfig{Symbol: "ETH", Decimals: 18},
		WalletSupported:           true,
		PortfolioBalanceSupported: true,
		SwapEnabled:               false,
		Source:                    "Project configuration",
		WalletConfigSourceURL:     stringPtr("https://docs.base.org/base-chain/api-reference/ethereum-json-rpc-api/eth_chainId"),
	},
	{
		Key:                       "arbitrum",
		Name:                      "Arbitrum",
		Family:                    "EVM",
		Status:                    "Planned",
		ChainID:                   stringPtr("42161"),
		NativeCurrency:            &NativeCurrencyConfig{Symbol: "ETH", Decimals: 18},
		WalletSupported:           true,
		PortfolioBalanceSupported: true,
		SwapEnabled:               false,
		Source:                    "Project configuration",
		WalletConfigSourceURL:     stringPtr("https://docs.arbitrum.io/chain-info"),
	},
	{
		Key:                       "polygon",
		Name:                      "Polygon",
		Family:                    "EVM",
		Status:                    "Planned",
		ChainID:                   stringPtr("137"),
		NativeCurrency:            &NativeCurrencyConfig{Symbol: "POL", Decimals: 18},
		WalletSupported:           true,
		PortfolioBalanceSupported: true,
		SwapEnabled:               false,
		Source:                    "Project configuration",
		WalletConfigSourceURL:     stringPtr("https://docs.polygon.technology/pos/reference/rpc-endpoints"),
	},
	{
		Key:                       "avalanche",
		Name:                      "Avalanche",
		Family:                    "EVM",
		Status:                    "Planned",
		ChainID:                   stringPtr("43114"),
		NativeCurrency:            &NativeCurrencyConfig{Symbol: "AVAX", Decimals: 18},
		WalletSupported:           true,
		PortfolioBalanceSupported: true,
		SwapEnabled:               false,
		Source:                    "Project configuration",
		WalletConfigSourceURL:     stringPtr("https://build.avax.network/docs/primary-network"),
	},
	{
		Key:                       "bnb",
		Name:                      "BNB Chain",
		Family:                    "EVM",
		Status:                    "Planned",
		ChainID:                   stringPtr("56"),
		NativeCurrency:            &NativeCurrencyConfig{Symbol: "BNB", Decimals: 18},
		WalletSupported:           true,
		PortfolioBalanceSupported: true,
		SwapEnabled:               false,
		Source:                    "Project configuration",
		WalletConfigSourceURL:     stringPtr("https://docs.bnbchain.org/bnb-smart-chain/validator/create-val/"),
	},
	{
		Key:                       "solana",
		Name:                      "Solana",
		Family:                    "Solana",
		Status:                    "Planned",
		WalletSupported:           false,
		PortfolioBalanceSupported: false,
		SwapEnabled:               false,
		Source:                    "Project configuration",
	},
	{
		Key:                       "stellar",
		Name:                      "Stellar",
		Family:                    "Stellar",
		Status:                    "Planned",
		WalletSupported:           false,
		PortfolioBalanceSupported: false,
		SwapEnabled:               false,
		Source:                    "Project configuration",
	},
	{
		Key:                       "celo",
		Name:                      "Celo",
		Family:                    "EVM",
		Status:                    "Feature-flagged",
		ChainID:                   stringPtr("42220"),
		WalletSupported:           true,
		PortfolioBalanceSupported: false,
		SwapEnabled:               false,
		Source:                    "Project configuration",
		WalletConfigSourceURL:     stringPtr("https://github.com/celo-org/agent-skills"),
	},
	{
		Key:                       "aptos",
		Name:                      "Aptos",
		Family:                    "Move",
		Status:                    "Planned",
		WalletSupported:           false,
		PortfolioBalanceSupported: false,
		SwapEnabled:               false,
		Source:                    "Project configuration",
	},
	{
		Key:                       "near",
		Name:                      "NEAR Protocol",
		Family:                    "NEAR",
		Status:                    "Planned",
		WalletSupported:           false,
		PortfolioBalanceSupported: false,
		SwapEnabled:               false,
		Source:                    "Project configuration",
	},
}

var DefaultAssets = []AssetResponse{
	{ID: "demo:ethereum:eth", Symbol: "ETH", Name: "Ether", Chain: "ethereum", Decimals: 18, FixturePriceMicros: "3400000000", Source: "Demo fixture"},
	{ID: "demo:base:usdc", Symbol: "USDC", Name: "USD Coin", Chain: "base", Decimals: 6, FixturePriceMicros: "1000000", Source: "Demo fixture"},
	{ID: "demo:near:near", Symbol: "NEAR", Name: "NEAR", Chain: "near", Decimals: 24, FixturePriceMicros: "5100000", Source: "Demo fixture"},
	{ID: "demo:solana:sol", Symbol: "SOL", Name: "Solana", Chain: "solana", Decimals: 9, FixturePriceMicros: "165000000", Source: "Demo fixture"},
	{ID: "demo:stellar:xlm", Symbol: "XLM", Name: "Stellar Lumens", Chain: "stellar", Decimals: 7, FixturePriceMicros: "340000", Source: "Demo fixture"},
	{ID: "demo:ethereum:usdc", Symbol: "USDC", Name: "USD Coin", Chain: "ethereum", Decimals: 6, FixturePriceMicros: "1000000", Source: "Demo fixture"},
	{ID: "demo:arbitrum:eth", Symbol: "ETH", Name: "Ether", Chain: "arbitrum", Decimals: 18, FixturePriceMicros: "3400000000", Source: "Demo fixture"},
	{ID: "demo:polygon:usdc", Symbol: "USDC", Name: "USD Coin", Chain: "polygon", Decimals: 6, FixturePriceMicros: "1000000", Source: "Demo fixture"},
}

var DefaultSeries = []SeriesItem{
	{Date: "Nov 02", ActiveAccounts: 7800, Transactions: 1120000},
	{Date: "Nov 03", ActiveAccounts: 8120, Transactions: 1190000},
	{Date: "Nov 04", ActiveAccounts: 7950, Transactions: 1150000},
	{Date: "Nov 05", ActiveAccounts: 8460, Transactions: 1280000},
	{Date: "Nov 06", ActiveAccounts: 8710, Transactions: 1320000},
	{Date: "Nov 07", ActiveAccounts: 8290, Transactions: 1240000},
	{Date: "Nov 08", ActiveAccounts: 9030, Transactions: 1390000},
	{Date: "Nov 09", ActiveAccounts: 8870, Transactions: 1360000},
	{Date: "Nov 10", ActiveAccounts: 9250, Transactions: 1440000},
}

var DefaultOverviewMetrics = []MetricItem{
	{
		Key:                "near_active_accounts",
		Label:              "Active accounts",
		Value:              9250,
		Unit:               "accounts / day",
		Scope:              "NEAR Protocol",
		Definition:         "Illustrative daily active-account series for interface testing.",
		Source:             "Demo fixture",
		CapturedAt:         FixtureDate,
		MethodologyVersion: "fixture-v1",
		Classification:     "illustrative",
	},
	{
		Key:                "near_transactions",
		Label:              "Transactions",
		Value:              1440000,
		Unit:               "transactions / day",
		Scope:              "NEAR Protocol",
		Definition:         "Illustrative daily transaction count; not an indexed chain observation.",
		Source:             "Demo fixture",
		CapturedAt:         FixtureDate,
		MethodologyVersion: "fixture-v1",
		Classification:     "illustrative",
	},
	{
		Key:                "near_tvl",
		Label:              "TVL",
		Value:              125000000,
		Unit:               "USD",
		Scope:              "NEAR ecosystem",
		Definition:         "Illustrative TVL value shown to demonstrate units and provenance labels.",
		Source:             "Demo fixture",
		CapturedAt:         FixtureDate,
		MethodologyVersion: "fixture-v1",
		Classification:     "illustrative",
	},
	{
		Key:                "near_stablecoin_supply",
		Label:              "Stablecoin supply",
		Value:              82000000,
		Unit:               "USD",
		Scope:              "NEAR ecosystem",
		Definition:         "Illustrative stablecoin-supply value; supply is not the same as available trading liquidity.",
		Source:             "Demo fixture",
		CapturedAt:         FixtureDate,
		MethodologyVersion: "fixture-v1",
		Classification:     "illustrative",
	},
}

var DefaultNearMetrics = []MetricItem{
	{
		Key:                "active_accounts",
		Label:              "Daily active accounts",
		Value:              9250,
		Unit:               "accounts / day",
		Scope:              "NEAR Protocol",
		Definition:         "Example daily active-account count. Production methodology must specify account filtering and bot handling.",
		Source:             "Demo fixture",
		CapturedAt:         FixtureDate,
		MethodologyVersion: "fixture-v1",
		Classification:     "illustrative",
	},
	{
		Key:                "transactions",
		Label:              "Daily transactions",
		Value:              1440000,
		Unit:               "transactions / day",
		Scope:              "NEAR Protocol",
		Definition:         "Example transaction count. Production ingestion must define included transaction and receipt types.",
		Source:             "Demo fixture",
		CapturedAt:         FixtureDate,
		MethodologyVersion: "fixture-v1",
		Classification:     "illustrative",
	},
	{
		Key:                "tvl",
		Label:              "Total value locked",
		Value:              125000000,
		Unit:               "USD",
		Scope:              "NEAR DeFi",
		Definition:         "Example aggregate TVL. Protocol coverage, pricing source, and double-counting rules are not configured.",
		Source:             "Demo fixture",
		CapturedAt:         FixtureDate,
		MethodologyVersion: "fixture-v1",
		Classification:     "illustrative",
	},
	{
		Key:                "dex_volume",
		Label:              "DEX volume",
		Value:              17600000,
		Unit:               "USD / day",
		Scope:              "NEAR DeFi",
		Definition:         "Example gross trading volume. Volume is not protocol revenue.",
		Source:             "Demo fixture",
		CapturedAt:         FixtureDate,
		MethodologyVersion: "fixture-v1",
		Classification:     "illustrative",
	},
	{
		Key:                "stablecoin_supply",
		Label:              "Stablecoin supply",
		Value:              82000000,
		Unit:               "USD",
		Scope:              "NEAR ecosystem",
		Definition:         "Example issued supply. It does not measure immediately available liquidity.",
		Source:             "Demo fixture",
		CapturedAt:         FixtureDate,
		MethodologyVersion: "fixture-v1",
		Classification:     "illustrative",
	},
	{
		Key:                "fees",
		Label:              "Protocol fees",
		Value:              18400,
		Unit:               "USD / day",
		Scope:              "NEAR DeFi",
		Definition:         "Example fee estimate. Production methodology must distinguish user fees from protocol revenue.",
		Source:             "Demo fixture",
		CapturedAt:         FixtureDate,
		MethodologyVersion: "fixture-v1",
		Classification:     "illustrative",
	},
}

var DefaultMarketOverview = MarketOverviewResponse{
	Mode:       "fixture",
	Notice:     FixtureNotice,
	CapturedAt: FixtureDate,
	Metrics:    DefaultOverviewMetrics,
	Series:     DefaultSeries,
	ChainComparisons: []ChainComparison{
		{Chain: "NEAR Protocol", TVLUSD: 125000000, DailyVolumeUSD: 17600000, Source: "Demo fixture"},
		{Chain: "Base", TVLUSD: 4200000000, DailyVolumeUSD: 610000000, Source: "Demo fixture"},
		{Chain: "Arbitrum", TVLUSD: 2600000000, DailyVolumeUSD: 380000000, Source: "Demo fixture"},
	},
}

var DefaultResearchScenarios = ResearchScenariosResponse{
	Unit: "USD / year",
	Note: "Scenario arithmetic only. Inputs are fictional and do not imply token value or investment return.",
	Scenarios: []ScenarioItem{
		{Key: "bear", Label: "Bear", DailyVolumeUSD: 8000000, FeeRatePct: 0.12, CapturePct: 18, ActivityGrowthPct: -15},
		{Key: "base", Label: "Base", DailyVolumeUSD: 17600000, FeeRatePct: 0.18, CapturePct: 28, ActivityGrowthPct: 12},
		{Key: "bull", Label: "Bull", DailyVolumeUSD: 32000000, FeeRatePct: 0.24, CapturePct: 40, ActivityGrowthPct: 35},
	},
	Formula: "Annual captured fees = daily volume × (1 + activity growth) × fee rate × capture share × 365",
}

func AssetMap() map[string]AssetResponse {
	m := make(map[string]AssetResponse, len(DefaultAssets))
	for _, a := range DefaultAssets {
		m[a.ID] = a
	}
	return m
}

func ConvertAssetToDomain(a AssetResponse) domain.Asset {
	return domain.Asset{
		ID:              a.ID,
		ProviderAssetID: a.ID,
		Chain:           a.Chain,
		Symbol:          a.Symbol,
		Name:            a.Name,
		Decimals:        a.Decimals,
		Enabled:         true,
	}
}
