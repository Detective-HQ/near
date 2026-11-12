export type DataMode = "fixture" | "live";

export interface ChainConfig {
  key: string;
  name: string;
  family: string;
  status: string;
  chainId: string | null;
  nativeCurrency: { symbol: string; decimals: number } | null;
  walletSupported: boolean;
  portfolioBalanceSupported: boolean;
  swapEnabled: boolean;
  source: string;
  walletConfigSourceUrl: string | null;
  observedAt: string | null;
}

export interface Asset {
  id: string;
  providerAssetId?: string;
  symbol: string;
  name: string;
  chain: string;
  decimals: number;
  contractAddress?: string | null;
  priceUsd?: number | null;
  source: string;
  observedAt: string | null;
  enabledForLiveSwap: boolean;
  sourceDepositSupported?: boolean;
}

export interface MetricObservation {
  key: string;
  label: string;
  value: number | null;
  unit: string;
  scope: string;
  definition: string;
  source: string;
  sourceUrl?: string | null;
  observedAt: string | null;
  retrievedAt?: string | null;
  capturedAt?: string | null;
  methodologyVersion?: string | null;
  classification?: string | null;
  status?: "available" | "unavailable" | "stale" | "partial" | string;
  error?: string | null;
}

export interface SeriesPoint {
  date: string;
  [key: string]: string | number | null | undefined;
}

export interface DataSnapshot {
  mode: DataMode;
  provider?: string;
  chain?: string;
  status?: string;
  retrievedAt?: string | null;
  capturedAt?: string | null;
  methodologyVersion?: string | null;
  notice?: string | null;
  metrics: MetricObservation[];
  series: SeriesPoint[];
  chainComparisons?: Array<{
    chain: string;
    tvlUsd: number;
    dailyVolumeUsd: number;
    source: string;
  }>;
}

export interface QuoteAsset {
  id: string;
  symbol: string;
  chain: string;
  decimals: number;
  contractAddress?: string | null;
}

export interface SwapQuote {
  id: string;
  mode: "simulation" | "provider_dry_quote" | "provider_executable_quote" | string;
  provider: string;
  providerQuoteId?: string;
  providerSignature?: string | null;
  providerTimestamp?: string | null;
  originAssetId: string;
  destinationAssetId: string;
  originAsset: QuoteAsset;
  destinationAsset: QuoteAsset;
  amountInAtomic: string;
  amountIn: string;
  amountOutEstimateAtomic: string;
  amountOutEstimate: string;
  minimumAmountOutAtomic: string | null;
  minimumAmountOut: string | null;
  feeBps?: number;
  slippageBps: number;
  expiresAt: string | null;
  deadline?: string | null;
  timeWhenInactive?: string | null;
  timeEstimateSeconds?: number | null;
  depositAddress: string | null;
  depositMemo: string | null;
  refundFeeAtomic?: string | null;
  withdrawFeeAtomic?: string | null;
  route?: string[];
  warning: string;
}

export interface SwapEvent {
  state: string;
  detail: string;
  at: string;
}

export interface SwapRecord {
  id: string;
  quoteId: string;
  state: string;
  providerStatus: string | null;
  mode: string;
  realSettlement: boolean;
  reviewedQuoteId?: string;
  providerUpdatedAt?: string;
  providerDetails?: Record<string, unknown>;
  transactions: {
    source: string | null;
    destination: string | null;
    refund: string | null;
  };
  transactionLinks?: {
    source: string | null;
    destination: string | null;
    refund: string | null;
  };
  createdAt: string;
  updatedAt: string;
  events: SwapEvent[];
  quote: SwapQuote;
}

export interface ResearchScenario {
  key: "bear" | "base" | "bull" | string;
  label: string;
  dailyVolumeUsd: number;
  feeRatePct: number;
  capturePct: number;
  activityGrowthPct: number;
}

export interface ResearchInputs {
  unit: string;
  note: string;
  scenarios: ResearchScenario[];
  formula: string;
}

export interface ApiErrorBody {
  error?: {
    code?: string;
    message?: string;
  };
  requestId?: string;
}

export interface HealthStatus {
  status: string;
  mode: string;
  quoteMode: "demo" | "live";
  demoMode: boolean;
  liveQuotesConfigured: boolean;
  marketDataMode: "fixture" | "live";
  executionEnabled: boolean;
  capturedAt: string;
}
