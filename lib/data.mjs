export const fixtureDate = "2026-11-10";
export const fixtureNotice =
  "Illustrative fixture values only. Not live. No market feed, provider catalogue, or chain indexer is connected.";

const evmWalletNetworks = {
  ethereum: {
    chainId: "1",
    nativeCurrency: { symbol: "ETH", decimals: 18 },
    sourceUrl: "https://eips.ethereum.org/EIPS/eip-2228"
  },
  base: {
    chainId: "8453",
    nativeCurrency: { symbol: "ETH", decimals: 18 },
    sourceUrl: "https://docs.base.org/base-chain/api-reference/ethereum-json-rpc-api/eth_chainId"
  },
  arbitrum: {
    chainId: "42161",
    nativeCurrency: { symbol: "ETH", decimals: 18 },
    sourceUrl: "https://docs.arbitrum.io/chain-info"
  },
  polygon: {
    chainId: "137",
    nativeCurrency: { symbol: "POL", decimals: 18 },
    sourceUrl: "https://docs.polygon.technology/pos/reference/rpc-endpoints"
  },
  avalanche: {
    chainId: "43114",
    nativeCurrency: { symbol: "AVAX", decimals: 18 },
    sourceUrl: "https://build.avax.network/docs/primary-network"
  },
  bnb: {
    chainId: "56",
    nativeCurrency: { symbol: "BNB", decimals: 18 },
    sourceUrl: "https://docs.bnbchain.org/bnb-smart-chain/validator/create-val/"
  },
  celo: {
    chainId: "42220",
    nativeCurrency: null,
    sourceUrl: "https://github.com/celo-org/agent-skills"
  }
};

export const chains = [
  { key: "ethereum", name: "Ethereum", family: "EVM", status: "Planned" },
  { key: "base", name: "Base", family: "EVM", status: "Planned" },
  { key: "arbitrum", name: "Arbitrum", family: "EVM", status: "Planned" },
  { key: "polygon", name: "Polygon", family: "EVM", status: "Planned" },
  { key: "avalanche", name: "Avalanche", family: "EVM", status: "Planned" },
  { key: "bnb", name: "BNB Chain", family: "EVM", status: "Planned" },
  { key: "solana", name: "Solana", family: "Solana", status: "Planned" },
  { key: "stellar", name: "Stellar", family: "Stellar", status: "Planned" },
  { key: "celo", name: "Celo", family: "EVM", status: "Feature-flagged" },
  { key: "aptos", name: "Aptos", family: "Move", status: "Planned" },
  { key: "near", name: "NEAR Protocol", family: "NEAR", status: "Planned" }
].map((chain) => ({
  ...chain,
  chainId: evmWalletNetworks[chain.key]?.chainId ?? null,
  nativeCurrency: evmWalletNetworks[chain.key]?.nativeCurrency ?? null,
  walletSupported: Boolean(evmWalletNetworks[chain.key]),
  portfolioBalanceSupported: Boolean(evmWalletNetworks[chain.key]?.nativeCurrency),
  swapEnabled: false,
  source: "Project configuration",
  walletConfigSourceUrl: evmWalletNetworks[chain.key]?.sourceUrl ?? null,
  observedAt: null
}));

export const assets = [
  { id: "demo:ethereum:eth", symbol: "ETH", name: "Ether", chain: "ethereum", decimals: 18, fixturePriceMicros: "3400000000" },
  { id: "demo:base:usdc", symbol: "USDC", name: "USD Coin", chain: "base", decimals: 6, fixturePriceMicros: "1000000" },
  { id: "demo:near:near", symbol: "NEAR", name: "NEAR", chain: "near", decimals: 24, fixturePriceMicros: "5100000" },
  { id: "demo:solana:sol", symbol: "SOL", name: "Solana", chain: "solana", decimals: 9, fixturePriceMicros: "165000000" },
  { id: "demo:stellar:xlm", symbol: "XLM", name: "Stellar Lumens", chain: "stellar", decimals: 7, fixturePriceMicros: "340000" },
  { id: "demo:ethereum:usdc", symbol: "USDC", name: "USD Coin", chain: "ethereum", decimals: 6, fixturePriceMicros: "1000000" },
  { id: "demo:arbitrum:eth", symbol: "ETH", name: "Ether", chain: "arbitrum", decimals: 18, fixturePriceMicros: "3400000000" },
  { id: "demo:polygon:usdc", symbol: "USDC", name: "USD Coin", chain: "polygon", decimals: 6, fixturePriceMicros: "1000000" }
].map((asset) => ({
  ...asset,
  source: "Demo fixture",
  observedAt: null,
  enabledForLiveSwap: false
}));

export const marketSeries = [
  { date: "Nov 02", activeAccounts: 7800, transactions: 1120000 },
  { date: "Nov 03", activeAccounts: 8120, transactions: 1190000 },
  { date: "Nov 04", activeAccounts: 7950, transactions: 1150000 },
  { date: "Nov 05", activeAccounts: 8460, transactions: 1280000 },
  { date: "Nov 06", activeAccounts: 8710, transactions: 1320000 },
  { date: "Nov 07", activeAccounts: 8290, transactions: 1240000 },
  { date: "Nov 08", activeAccounts: 9030, transactions: 1390000 },
  { date: "Nov 09", activeAccounts: 8870, transactions: 1360000 },
  { date: "Nov 10", activeAccounts: 9250, transactions: 1440000 }
];

const provenance = {
  source: "Demo fixture",
  observedAt: null,
  capturedAt: fixtureDate,
  methodologyVersion: "fixture-v1",
  classification: "illustrative"
};

export const overviewMetrics = [
  {
    key: "near_active_accounts",
    label: "Active accounts",
    value: 9250,
    unit: "accounts / day",
    scope: "NEAR Protocol",
    definition: "Illustrative daily active-account series for interface testing.",
    ...provenance
  },
  {
    key: "near_transactions",
    label: "Transactions",
    value: 1440000,
    unit: "transactions / day",
    scope: "NEAR Protocol",
    definition: "Illustrative daily transaction count; not an indexed chain observation.",
    ...provenance
  },
  {
    key: "near_tvl",
    label: "TVL",
    value: 125000000,
    unit: "USD",
    scope: "NEAR ecosystem",
    definition: "Illustrative TVL value shown to demonstrate units and provenance labels.",
    ...provenance
  },
  {
    key: "near_stablecoin_supply",
    label: "Stablecoin supply",
    value: 82000000,
    unit: "USD",
    scope: "NEAR ecosystem",
    definition: "Illustrative stablecoin-supply value; supply is not the same as available trading liquidity.",
    ...provenance
  }
];

export const nearMetrics = [
  {
    key: "active_accounts",
    label: "Daily active accounts",
    value: 9250,
    unit: "accounts / day",
    scope: "NEAR Protocol",
    definition: "Example daily active-account count. Production methodology must specify account filtering and bot handling.",
    ...provenance
  },
  {
    key: "transactions",
    label: "Daily transactions",
    value: 1440000,
    unit: "transactions / day",
    scope: "NEAR Protocol",
    definition: "Example transaction count. Production ingestion must define included transaction and receipt types.",
    ...provenance
  },
  {
    key: "tvl",
    label: "Total value locked",
    value: 125000000,
    unit: "USD",
    scope: "NEAR DeFi",
    definition: "Example aggregate TVL. Protocol coverage, pricing source, and double-counting rules are not configured.",
    ...provenance
  },
  {
    key: "dex_volume",
    label: "DEX volume",
    value: 17600000,
    unit: "USD / day",
    scope: "NEAR DeFi",
    definition: "Example gross trading volume. Volume is not protocol revenue.",
    ...provenance
  },
  {
    key: "stablecoin_supply",
    label: "Stablecoin supply",
    value: 82000000,
    unit: "USD",
    scope: "NEAR ecosystem",
    definition: "Example issued supply. It does not measure immediately available liquidity.",
    ...provenance
  },
  {
    key: "fees",
    label: "Protocol fees",
    value: 18400,
    unit: "USD / day",
    scope: "NEAR DeFi",
    definition: "Example fee estimate. Production methodology must distinguish user fees from protocol revenue.",
    ...provenance
  }
];

export const marketOverview = {
  mode: "fixture",
  notice: fixtureNotice,
  capturedAt: fixtureDate,
  metrics: overviewMetrics,
  series: marketSeries,
  chainComparisons: [
    { chain: "NEAR Protocol", tvlUsd: 125000000, dailyVolumeUsd: 17600000, source: "Demo fixture" },
    { chain: "Base", tvlUsd: 4200000000, dailyVolumeUsd: 610000000, source: "Demo fixture" },
    { chain: "Arbitrum", tvlUsd: 2600000000, dailyVolumeUsd: 380000000, source: "Demo fixture" }
  ]
};

export const researchInputs = {
  unit: "USD / year",
  note: "Scenario arithmetic only. Inputs are fictional and do not imply token value or investment return.",
  scenarios: [
    { key: "bear", label: "Bear", dailyVolumeUsd: 8000000, feeRatePct: 0.12, capturePct: 18, activityGrowthPct: -15 },
    { key: "base", label: "Base", dailyVolumeUsd: 17600000, feeRatePct: 0.18, capturePct: 28, activityGrowthPct: 12 },
    { key: "bull", label: "Bull", dailyVolumeUsd: 32000000, feeRatePct: 0.24, capturePct: 40, activityGrowthPct: 35 }
  ],
  formula: "Annual captured fees = daily volume × (1 + activity growth) × fee rate × capture share × 365"
};
