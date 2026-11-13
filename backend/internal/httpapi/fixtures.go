package httpapi

const (
	FixtureDate   = "2026-11-10"
	FixtureNotice = "Illustrative fixture values only. Not live. No market feed, provider catalogue, or chain indexer is connected."
)

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
