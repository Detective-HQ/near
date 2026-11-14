// Contract for the read-only trading endpoints served by the Go API
// (backend/internal/trading, documented in docs/openapi.yaml and
// docs/research-methodology.md#trading-signals). They require MARKET_DATA_MODE=live;
// otherwise the API answers 503 and the frontend shows "Data unavailable right now."
// TEST DATA is served instead when NEXT_PUBLIC_USE_MOCKS=true.
// All numeric values are decimal strings. Timestamps are ISO-8601 UTC.

export type ApiQuality = "live" | "delayed" | "stale" | "partial" | "unavailable";

export interface Provenance {
  source: string;
  asOf: string | null;
  quality: ApiQuality;
}

export interface MetricValue extends Provenance {
  value: string | null;
  unit: string;
}

export type Direction = "bullish" | "bearish";

export interface TradingOverview extends Provenance {
  regime: Direction | "mixed" | null;
  regimeMethodologyUrl: string | null;
  breadth: { bullish: number; neutral: number; bearish: number } | null;
  universe: { scanned: number; total: number } | null;
  /** percent, e.g. "92.5" */
  coverage: string | null;
  modelVersion: string | null;
}

export interface Signal extends Provenance {
  assetId: string;
  symbol: string;
  chain: string;
  venue: string;
  quote: string;
  liquidityTier: string;
  direction: Direction;
  rank: number;
  /** change in rank since the previous run; positive = moved up */
  rankChange: number | null;
  /** ranking score 0-100 */
  score: string;
  /** 24h price change, percent */
  change: string | null;
  /** share of evidence available, percent */
  coverage: string | null;
  timeframe: string;
  /** true when the signal was hidden because its data is stale */
  suppressed: boolean;
}

export interface SignalsResponse extends Provenance {
  modelVersion: string | null;
  bullish: Signal[];
  bearish: Signal[];
  notRanked: Array<{ assetId: string; symbol: string; chain: string; reason: string }>;
  /** filter values the API accepts; only these are shown */
  filters: {
    timeframe: string[];
    chain: string[];
    venue: string[];
    quote: string[];
    liquidityTier: string[];
  };
}

export interface Evidence {
  type: string;
  value: string;
  source: string;
  observedAt: string | null;
}

export interface AssetDetail extends Provenance {
  assetId: string;
  symbol: string;
  chain: string;
  venues: string[];
  lastPrice: string | null;
  change24h: string | null;
  scores: { bullish: string | null; bearish: string | null; coverage: string | null; modelVersion: string | null };
  evidence: Evidence[];
  derivatives: {
    openInterest: MetricValue | null;
    fundingRate: MetricValue | null;
    liquidations: MetricValue | null;
    futuresVolume: MetricValue | null;
  };
  risk: {
    volatility: MetricValue | null;
    spreadDepth: MetricValue | null;
    dataFreshness: MetricValue | null;
    venueConcentration: MetricValue | null;
  };
  timeframes: string[];
}

export type ZoneStatus = "fresh" | "tested" | "mitigated" | "invalidated" | "expired";

export interface Candle {
  t: string;
  o: string;
  h: string;
  l: string;
  c: string;
}

export interface StructureResponse extends Provenance {
  timeframe: string;
  rulesVersion: string;
  candles: Candle[];
  zones: Array<{
    id: string;
    kind: "order_block" | "fair_value_gap";
    direction: Direction;
    status: ZoneStatus;
    top: string;
    bottom: string;
    from: string;
    to: string | null;
  }>;
  structure: Array<{ kind: "BOS" | "CHoCH"; direction: Direction; price: string; at: string }>;
  swings: Array<{ kind: "high" | "low"; price: string; at: string }>;
}

export interface LiquidityResponse extends Provenance {
  levels: Array<{ price: string; side: "bid" | "ask"; label: string }>;
  spotVolume: MetricValue | null;
  cvd: (MetricValue & { method: string }) | null;
  depth: (Provenance & { bid: string | null; ask: string | null; range: string }) | null;
}

export interface ScenarioResponse extends Provenance {
  status: "available" | "stale" | "none";
  modelVersion: string | null;
  referencePrice: string | null;
  entryArea: { low: string; high: string } | null;
  target1: string | null;
  target2: string | null;
  invalidation: string | null;
  riskReward: string | null;
  /** percent distance from reference price to entry area */
  distance: string | null;
  expiresAt: string | null;
}

export type Outcome = "target_hit" | "stop_hit" | "expired" | "open";

export interface HistoryItem {
  id: string;
  generatedAt: string;
  assetId: string;
  symbol: string;
  chain: string;
  direction: Direction;
  score: string;
  timeframe: string;
  modelVersion: string;
  outcome: Outcome;
  /** maximum favourable / adverse excursion, percent */
  mfe: string | null;
  mae: string | null;
  timeline: Array<{ at: string; label: string }>;
}

export interface HistoryResponse extends Provenance {
  items: HistoryItem[];
  page: number;
  pageSize: number;
  total: number;
  /** filter values the API accepts; only these are shown */
  filters: { timeframe: string[] };
  /** only present with methodology and sample size; never shown otherwise */
  aggregate: { label: string; value: string; methodology: string; sampleSize: number } | null;
}

export interface Backtest {
  id: string;
  model: string;
  period: { from: string; to: string };
  universe: string;
  timeframe: string;
  sampleSize: number;
  costs: string;
  benchmark: string;
  maxDrawdown: string | null;
  metrics: Array<{ label: string; value: string }>;
  methodology: string;
  costsAssumed: string;
  methodologyUrl: string | null;
}

export interface BacktestsResponse extends Provenance {
  items: Backtest[];
}

export interface TopMoversResponse extends Provenance {
  items: Signal[];
}

export interface PortfolioBalance {
  assetId: string;
  symbol: string;
  chain: string;
  balance: string;
  valueUsd: string | null;
  source: string;
  asOf: string | null;
}

export interface PortfolioResponse extends Provenance {
  address: string | null;
  chain: string | null;
  totalValueUsd: string | null;
  balances: PortfolioBalance[];
  notice?: string;
}
