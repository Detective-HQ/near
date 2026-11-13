// TEST DATA for endpoints the Go API does not serve yet. Only used when
// NEXT_PUBLIC_USE_MOCKS=true, and every view that renders it shows a "TEST DATA" tag.
// Values are deterministic placeholders for interface testing, not observations.
import type {
  AssetDetail,
  BacktestsResponse,
  HistoryItem,
  HistoryResponse,
  LiquidityResponse,
  PortfolioBalance,
  ScenarioResponse,
  Signal,
  SignalsResponse,
  StructureResponse,
  TradingOverview,
} from "./trading-contracts";

const AS_OF = "2026-11-13T12:00:00Z";
const SOURCE = "TEST DATA";
const P = { source: SOURCE, asOf: AS_OF, quality: "live" as const };

const UNIVERSE = [
  ["NEAR", "near"], ["ETH", "ethereum"], ["SOL", "solana"], ["XLM", "stellar"], ["CELO", "celo"],
  ["APT", "aptos"], ["ARB", "arbitrum"], ["OP", "ethereum"], ["AVAX", "avalanche"], ["LINK", "ethereum"],
  ["UNI", "ethereum"], ["AAVE", "ethereum"], ["MKR", "ethereum"], ["LDO", "ethereum"], ["JUP", "solana"],
  ["RAY", "solana"], ["PYTH", "solana"], ["REF", "near"], ["AURORA", "near"], ["BNB", "bnb"],
  ["POL", "polygon"], ["GMX", "arbitrum"], ["PENDLE", "ethereum"], ["ENA", "ethereum"], ["WIF", "solana"],
  ["BONK", "solana"], ["TIA", "ethereum"], ["INJ", "ethereum"], ["SEI", "ethereum"], ["SUI", "ethereum"],
  ["TON", "ethereum"], ["DOT", "ethereum"], ["ATOM", "ethereum"], ["ADA", "ethereum"], ["ALGO", "ethereum"],
  ["HBAR", "ethereum"], ["FIL", "ethereum"], ["RNDR", "ethereum"], ["GRT", "ethereum"], ["IMX", "ethereum"],
] as const;

const TIMEFRAMES = ["1h", "4h", "1d"];
const VENUES = ["Binance", "Hyperliquid"];
const QUOTES = ["USDT", "USDC"];
const TIERS = ["Tier 1", "Tier 2", "Tier 3"];

function idOf(symbol: string, chain: string) {
  return `${chain}:${symbol.toLowerCase()}`;
}

function signal(i: number, direction: "bullish" | "bearish"): Signal {
  const [symbol, chain] = UNIVERSE[(direction === "bullish" ? i : i + 20) % UNIVERSE.length];
  const rank = i + 1;
  const score = String(92 - i * 2.5 > 30 ? (92 - i * 2.5).toFixed(1) : "31.0");
  return {
    ...P,
    assetId: idOf(symbol, chain),
    symbol,
    chain,
    venue: VENUES[i % 2],
    quote: QUOTES[i % 2],
    liquidityTier: TIERS[i % 3],
    direction,
    rank,
    rankChange: [2, 0, -1, 3, 0, 1, -2][i % 7],
    score,
    change: `${direction === "bullish" ? "" : "-"}${(1.2 + (i % 5) * 0.8).toFixed(2)}`,
    coverage: String(95 - (i % 6) * 5),
    timeframe: TIMEFRAMES[i % 3],
    suppressed: i === 17,
    quality: i === 12 ? "partial" : "live",
  };
}

export function mockOverview(): TradingOverview {
  return {
    ...P,
    regime: "mixed",
    regimeMethodologyUrl: "https://github.com/Detective-HQ/near/blob/main/docs/research-methodology.md",
    breadth: { bullish: 14, neutral: 19, bearish: 7 },
    universe: { scanned: 40, total: 44 },
    coverage: "90.9",
    modelVersion: "test-0.1",
  };
}

export function mockSignals(params: URLSearchParams): SignalsResponse {
  const match = (s: Signal) =>
    (!params.get("timeframe") || s.timeframe === params.get("timeframe"))
    && (!params.get("chain") || s.chain === params.get("chain"))
    && (!params.get("venue") || s.venue === params.get("venue"))
    && (!params.get("quote") || s.quote === params.get("quote"))
    && (!params.get("liquidityTier") || s.liquidityTier === params.get("liquidityTier"))
    && (!params.get("asset") || s.symbol.toLowerCase().includes((params.get("asset") ?? "").toLowerCase()));
  const rerank = (list: Signal[]) => list.filter(match).map((s, i) => ({ ...s, rank: i + 1 }));
  return {
    ...P,
    modelVersion: "test-0.1",
    bullish: rerank(Array.from({ length: 20 }, (_, i) => signal(i, "bullish"))),
    bearish: rerank(Array.from({ length: 20 }, (_, i) => signal(i, "bearish"))),
    notRanked: [
      { assetId: "ethereum:fil", symbol: "FIL", chain: "ethereum", reason: "Coverage below the minimum for ranking." },
      { assetId: "ethereum:hbar", symbol: "HBAR", chain: "ethereum", reason: "No derivatives data from any venue." },
      { assetId: "ethereum:algo", symbol: "ALGO", chain: "ethereum", reason: "Price data is stale." },
      { assetId: "ethereum:grt", symbol: "GRT", chain: "ethereum", reason: "Insufficient candle history." },
    ],
    filters: { timeframe: TIMEFRAMES, chain: ["near", "ethereum", "solana", "stellar"], venue: VENUES, quote: QUOTES, liquidityTier: TIERS },
  };
}

function findAsset(assetId: string): readonly [string, string] | null {
  return UNIVERSE.find(([s, c]) => idOf(s, c) === assetId) ?? null;
}

const metric = (value: string | null, unit: string, quality: "live" | "unavailable" = "live") => ({
  ...P, value, unit, quality: value === null ? ("unavailable" as const) : quality,
});

export function mockAsset(assetId: string): AssetDetail | null {
  const a = findAsset(assetId);
  if (!a) return null;
  const [symbol, chain] = a;
  const hasDerivs = symbol !== "XLM";
  return {
    ...P,
    assetId,
    symbol,
    chain,
    venues: VENUES,
    lastPrice: "5.1024",
    change24h: "2.31",
    scores: { bullish: "86.5", bearish: "41.0", coverage: "92", modelVersion: "test-0.1" },
    evidence: [
      { type: "Market structure", value: "Bullish BOS on 4h", source: SOURCE, observedAt: AS_OF },
      { type: "Order block", value: "Fresh demand zone below price", source: SOURCE, observedAt: AS_OF },
      { type: "Funding", value: "Neutral", source: SOURCE, observedAt: AS_OF },
      { type: "Liquidity", value: "Resting bids above 4.90", source: SOURCE, observedAt: AS_OF },
    ],
    derivatives: {
      openInterest: hasDerivs ? metric("182400000", "USD") : null,
      fundingRate: hasDerivs ? metric("0.0081", "%") : null,
      liquidations: hasDerivs ? metric("2350000", "USD") : null,
      futuresVolume: null,
    },
    risk: {
      volatility: metric("3.4", "% daily"),
      spreadDepth: metric("0.04", "% spread"),
      dataFreshness: metric("45", "seconds"),
      venueConcentration: metric("61", "% top venue"),
    },
    timeframes: TIMEFRAMES,
  };
}

export function mockStructure(assetId: string, timeframe: string): StructureResponse | null {
  if (!findAsset(assetId)) return null;
  const candles = Array.from({ length: 48 }, (_, i) => {
    const base = 5 + Math.sin(i / 5) * 0.35 + i * 0.006;
    const o = base;
    const c = base + Math.cos(i * 1.7) * 0.08;
    const h = Math.max(o, c) + 0.05 + (i % 3) * 0.02;
    const l = Math.min(o, c) - 0.05 - (i % 4) * 0.015;
    return { t: new Date(Date.parse(AS_OF) - (48 - i) * 3600_000).toISOString(), o: o.toFixed(4), h: h.toFixed(4), l: l.toFixed(4), c: c.toFixed(4) };
  });
  const at = (i: number) => candles[i].t;
  return {
    ...P,
    timeframe,
    rulesVersion: "rules-test-0.1",
    candles,
    zones: [
      { id: "z1", kind: "order_block", direction: "bullish", status: "fresh", top: "4.92", bottom: "4.84", from: at(18), to: null },
      { id: "z2", kind: "order_block", direction: "bearish", status: "tested", top: "5.44", bottom: "5.36", from: at(8), to: null },
      { id: "z3", kind: "fair_value_gap", direction: "bullish", status: "mitigated", top: "5.10", bottom: "5.04", from: at(30), to: at(38) },
      { id: "z4", kind: "order_block", direction: "bearish", status: "invalidated", top: "5.62", bottom: "5.55", from: at(2), to: at(14) },
      { id: "z5", kind: "fair_value_gap", direction: "bearish", status: "expired", top: "5.30", bottom: "5.26", from: at(22), to: at(28) },
    ],
    structure: [
      { kind: "BOS", direction: "bullish", price: "5.22", at: at(40) },
      { kind: "CHoCH", direction: "bullish", price: "4.98", at: at(26) },
    ],
    swings: [
      { kind: "high", price: "5.41", at: at(9) },
      { kind: "low", price: "4.80", at: at(20) },
      { kind: "high", price: "5.33", at: at(44) },
    ],
  };
}

export function mockLiquidity(assetId: string): LiquidityResponse | null {
  if (!findAsset(assetId)) return null;
  return {
    ...P,
    levels: [
      { price: "4.90", side: "bid", label: "Resting bids" },
      { price: "5.45", side: "ask", label: "Equal highs" },
    ],
    spotVolume: metric("48200000", "USD 24h"),
    cvd: { ...metric("-1250000", "USD"), method: "Taker buy minus taker sell volume, spot venues" },
    depth: { ...P, bid: "1840000", ask: "1610000", range: "±2%" },
  };
}

export function mockScenario(assetId: string): ScenarioResponse | null {
  if (!findAsset(assetId)) return null;
  return {
    ...P,
    status: "available",
    modelVersion: "test-0.1",
    referencePrice: "5.1024",
    entryArea: { low: "4.84", high: "4.92" },
    target1: "5.36",
    target2: "5.62",
    invalidation: "4.71",
    riskReward: "2.4",
    distance: "-3.6",
    expiresAt: "2026-11-15T12:00:00Z",
  };
}

const OUTCOMES = ["target_hit", "stop_hit", "expired", "open"] as const;

function historyItem(i: number): HistoryItem {
  const [symbol, chain] = UNIVERSE[i % UNIVERSE.length];
  const generatedAt = new Date(Date.parse(AS_OF) - i * 9 * 3600_000).toISOString();
  return {
    id: `sig_${i}`,
    generatedAt,
    assetId: idOf(symbol, chain),
    symbol,
    chain,
    direction: i % 3 === 0 ? "bearish" : "bullish",
    score: String(70 + (i % 25)),
    timeframe: TIMEFRAMES[i % 3],
    modelVersion: "test-0.1",
    outcome: OUTCOMES[i % 4],
    mfe: (1 + (i % 7) * 0.6).toFixed(2),
    mae: (-0.4 - (i % 5) * 0.3).toFixed(2),
    timeline: [
      { at: generatedAt, label: "Signal generated" },
      { at: new Date(Date.parse(generatedAt) + 4 * 3600_000).toISOString(), label: "Entry area reached" },
      { at: new Date(Date.parse(generatedAt) + 20 * 3600_000).toISOString(), label: `Outcome: ${OUTCOMES[i % 4].replace("_", " ")}` },
    ],
  };
}

export function mockHistory(params: URLSearchParams): HistoryResponse {
  const all = Array.from({ length: 64 }, (_, i) => historyItem(i)).filter((h) =>
    (!params.get("asset") || h.symbol.toLowerCase().includes((params.get("asset") ?? "").toLowerCase()) || h.assetId === params.get("asset"))
    && (!params.get("direction") || h.direction === params.get("direction"))
    && (!params.get("timeframe") || h.timeframe === params.get("timeframe"))
    && (!params.get("outcome") || h.outcome === params.get("outcome"))
    && (!params.get("from") || h.generatedAt >= `${params.get("from")}T00:00:00Z`)
    && (!params.get("to") || h.generatedAt <= `${params.get("to")}T23:59:59Z`));
  const pageSize = 20;
  const page = Math.max(1, Number(params.get("page") ?? "1") || 1);
  return { ...P, items: all.slice((page - 1) * pageSize, page * pageSize), page, pageSize, total: all.length, filters: { timeframe: TIMEFRAMES }, aggregate: null };
}

export function mockBacktests(): BacktestsResponse {
  return {
    ...P,
    items: [
      {
        id: "bt_1",
        model: "test-0.1",
        period: { from: "2025-01-01", to: "2025-12-31" },
        universe: "40 assets, Tier 1-3",
        timeframe: "4h",
        sampleSize: 412,
        costs: "0.10% per side",
        benchmark: "Equal-weight universe",
        maxDrawdown: "-14.2",
        metrics: [
          { label: "Signals evaluated", value: "412" },
          { label: "Median MFE", value: "2.8%" },
          { label: "Median MAE", value: "-1.1%" },
        ],
        methodology: "Walk-forward evaluation on 4h candles. Each signal is entered at the next candle open and closed at target, invalidation or expiry.",
        costsAssumed: "Taker fee 0.05% and slippage 0.05% on entry and exit.",
        methodologyUrl: "https://github.com/Detective-HQ/near/blob/main/docs/research-methodology.md",
      },
    ],
  };
}

export function mockPortfolio(): PortfolioBalance[] {
  return [
    { assetId: "ethereum:eth", symbol: "ETH", chain: "ethereum", balance: "0.8421", valueUsd: "2863.14", source: SOURCE, asOf: AS_OF },
    { assetId: "near:near", symbol: "NEAR", chain: "near", balance: "412.5", valueUsd: "2103.75", source: SOURCE, asOf: AS_OF },
    { assetId: "base:usdc", symbol: "USDC", chain: "base", balance: "1250", valueUsd: "1250", source: SOURCE, asOf: AS_OF },
    { assetId: "solana:sol", symbol: "SOL", chain: "solana", balance: "3.2", valueUsd: null, source: SOURCE, asOf: AS_OF },
    { assetId: "ethereum:link", symbol: "LINK", chain: "ethereum", balance: "0.04", valueUsd: "0.62", source: SOURCE, asOf: AS_OF },
  ];
}
