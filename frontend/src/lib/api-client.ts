import type {
  ApiErrorBody,
  Asset,
  ChainConfig,
  DataSnapshot,
  HealthStatus,
  ResearchInputs,
  SwapQuote,
  SwapRecord
} from "./contracts";
import type {
  AssetDetail,
  BacktestsResponse,
  HistoryResponse,
  LiquidityResponse,
  PortfolioBalance,
  ScenarioResponse,
  SignalsResponse,
  StructureResponse,
  TradingOverview
} from "./trading-contracts";

/** Test data is only ever used when this flag is set, and is always labelled "TEST DATA". */
export const USE_MOCKS = process.env.NEXT_PUBLIC_USE_MOCKS === "true";

export class ApiRequestError extends Error {
  readonly status: number;
  readonly code: string | null;
  readonly requestId: string | null;

  constructor(message: string, status: number, code: string | null, requestId: string | null) {
    super(message);
    this.name = "ApiRequestError";
    this.status = status;
    this.code = code;
    this.requestId = requestId;
  }
}

/** Data that cannot be shown: a missing endpoint, or fixture values while mocks are off. */
export class UnavailableError extends Error {
  constructor(message = "Data unavailable right now.") {
    super(message);
    this.name = "UnavailableError";
  }
}

export async function apiRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: {
      accept: "application/json",
      ...(init.body ? { "content-type": "application/json" } : {}),
      ...init.headers
    },
    cache: "no-store"
  });

  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    throw new ApiRequestError(
      "The API returned a response that was not valid JSON.",
      response.status,
      "INVALID_RESPONSE",
      response.headers.get("x-request-id")
    );
  }

  if (!response.ok) {
    const body = payload as ApiErrorBody;
    throw new ApiRequestError(
      body.error?.message ?? `The API request failed (${response.status}).`,
      response.status,
      body.error?.code ?? null,
      body.requestId ?? response.headers.get("x-request-id")
    );
  }

  return payload as T;
}

/** Some endpoints return a bare value, others wrap it ({ quote }, { swap }). Accept both. */
function unwrap<T>(payload: unknown, key: string): T {
  if (payload && typeof payload === "object" && !Array.isArray(payload) && key in payload) {
    return (payload as Record<string, T>)[key];
  }
  return payload as T;
}

/** Backend fixtures are illustrative values. They are only shown when mocks are enabled. */
export function isFixtureAsset(a: Pick<Asset, "id" | "source">): boolean {
  return a.id.startsWith("demo:") || /fixture/i.test(a.source);
}

function gateSnapshot(s: DataSnapshot): DataSnapshot & { testData: boolean } {
  const testData = s.mode === "fixture";
  if (testData && !USE_MOCKS) throw new UnavailableError();
  return { ...s, testData };
}

async function mockOr<T>(load: () => T | null, real: () => Promise<T>): Promise<T & { testData?: boolean }> {
  if (!USE_MOCKS) return (await real()) as T & { testData?: boolean };
  await new Promise((r) => setTimeout(r, 250));
  const value = load();
  if (value === null) throw new ApiRequestError("Not found", 404, "NOT_FOUND", null);
  return { ...value, testData: true };
}

const qs = (params?: URLSearchParams) => {
  const s = params?.toString();
  return s ? `?${s}` : "";
};

const enc = encodeURIComponent;

export interface Catalogue {
  assets: Array<Asset & { fixturePriceMicros?: string }>;
  testData: boolean;
  servedAt: string | null;
}

export const api = {
  health: () => apiRequest<HealthStatus>("/api/v1/health"),

  chains: async () => {
    const payload = await apiRequest<ChainConfig[] | { chains: ChainConfig[] }>("/api/v1/chains");
    return unwrap<ChainConfig[]>(payload, "chains");
  },

  /** Catalogue for Explore and Swap. Fixture assets are dropped unless mocks are on. */
  assets: async (chain?: string): Promise<Catalogue> => {
    const payload = await apiRequest<unknown>(`/api/v1/assets${chain ? `?chain=${enc(chain)}` : ""}`);
    const list = unwrap<Catalogue["assets"]>(payload, "assets") ?? [];
    const servedAt = (payload as { servedAt?: string }).servedAt ?? null;
    const fixtures = list.filter(isFixtureAsset);
    if (fixtures.length === list.length && list.length > 0) {
      if (!USE_MOCKS) throw new UnavailableError();
      return { assets: list, testData: true, servedAt };
    }
    return { assets: list.filter((a) => !isFixtureAsset(a)), testData: false, servedAt };
  },

  markets: async (refresh = false) =>
    gateSnapshot(await apiRequest<DataSnapshot>(`/api/v1/markets/overview${refresh ? "?refresh=true" : ""}`)),

  intelligence: async () => gateSnapshot(await apiRequest<DataSnapshot>("/api/v1/intelligence/near")),

  /** Scenario inputs are labelled fictional by the API, so they are treated as test data. */
  research: async () => {
    const inputs = await apiRequest<ResearchInputs>("/api/v1/research/scenarios");
    if (!USE_MOCKS) throw new UnavailableError();
    return { ...inputs, testData: true };
  },

  risk: () => apiRequest<RiskReport>("/api/v1/risk/status"),

  activity: async () => {
    const payload = await apiRequest<SwapRecord[] | { swaps: SwapRecord[] }>("/api/v1/activity");
    return unwrap<SwapRecord[]>(payload, "swaps") ?? [];
  },

  quote: async (input: {
    originAssetId: string;
    destinationAssetId: string;
    amountIn: string;
    recipient?: string;
    refundTo?: string;
    slippageBps?: number;
  }) => unwrap<SwapQuote>(await apiRequest<unknown>("/api/v1/quotes", { method: "POST", body: JSON.stringify(input) }), "quote"),

  createLiveSwap: async (input: { quoteId: string; recipient: string; refundTo: string }, idempotencyKey: string) =>
    unwrap<SwapRecord>(await apiRequest<unknown>("/api/v1/swaps", {
      method: "POST",
      headers: { "Idempotency-Key": idempotencyKey },
      body: JSON.stringify(input)
    }), "swap"),

  createSimulation: async (quoteId: string, idempotencyKey: string) =>
    unwrap<SwapRecord>(await apiRequest<unknown>("/api/v1/swaps", {
      method: "POST",
      headers: { "Idempotency-Key": idempotencyKey },
      body: JSON.stringify({ quoteId })
    }), "swap"),

  advanceSimulation: async (swapId: string) =>
    unwrap<SwapRecord>(await apiRequest<unknown>(`/api/v1/swaps/${enc(swapId)}/simulate`, { method: "POST", body: "{}" }), "swap"),

  recordDeposit: async (swapId: string, txHash: string, chainId: string) =>
    unwrap<SwapRecord>(await apiRequest<unknown>(`/api/v1/swaps/${enc(swapId)}/deposit`, {
      method: "POST",
      body: JSON.stringify({ txHash, chainId })
    }), "swap"),

  cancelSwap: async (swapId: string) =>
    unwrap<SwapRecord>(await apiRequest<unknown>(`/api/v1/swaps/${enc(swapId)}/cancel`, { method: "POST", body: "{}" }), "swap"),

  swap: async (swapId: string) => unwrap<SwapRecord>(await apiRequest<unknown>(`/api/v1/swaps/${enc(swapId)}`), "swap"),

  // Trading intelligence: read-only. Not yet served by the Go API (see trading-contracts.ts).
  tradingOverview: async () => {
    const m = await import("./mocks");
    return mockOr(m.mockOverview, () => apiRequest<TradingOverview>("/api/v1/trading/overview"));
  },
  tradingSignals: async (params: URLSearchParams) => {
    const m = await import("./mocks");
    return mockOr(() => m.mockSignals(params), () => apiRequest<SignalsResponse>(`/api/v1/trading/signals${qs(params)}`));
  },
  tradingAsset: async (assetId: string) => {
    const m = await import("./mocks");
    return mockOr(() => m.mockAsset(assetId), () => apiRequest<AssetDetail>(`/api/v1/trading/assets/${enc(assetId)}`));
  },
  tradingStructure: async (assetId: string, timeframe: string) => {
    const m = await import("./mocks");
    const p = new URLSearchParams({ timeframe });
    return mockOr(() => m.mockStructure(assetId, timeframe), () => apiRequest<StructureResponse>(`/api/v1/trading/orderblocks/${enc(assetId)}${qs(p)}`));
  },
  tradingLiquidity: async (assetId: string) => {
    const m = await import("./mocks");
    return mockOr(() => m.mockLiquidity(assetId), () => apiRequest<LiquidityResponse>(`/api/v1/trading/liquidity/${enc(assetId)}`));
  },
  tradingScenario: async (assetId: string) => {
    const m = await import("./mocks");
    return mockOr(() => m.mockScenario(assetId), () => apiRequest<ScenarioResponse>(`/api/v1/trading/scenarios/${enc(assetId)}`));
  },
  tradingHistory: async (params: URLSearchParams) => {
    const m = await import("./mocks");
    return mockOr(() => m.mockHistory(params), () => apiRequest<HistoryResponse>(`/api/v1/trading/history${qs(params)}`));
  },
  tradingBacktests: async () => {
    const m = await import("./mocks");
    return mockOr(m.mockBacktests, () => apiRequest<BacktestsResponse>("/api/v1/trading/backtests"));
  },

  /** No portfolio endpoint exists; only wallet-read native balances are real. */
  portfolioTestData: async (): Promise<PortfolioBalance[] | null> => {
    if (!USE_MOCKS) return null;
    const m = await import("./mocks");
    return m.mockPortfolio();
  }
};

export interface RiskReport {
  status: "HEALTHY" | "DEGRADED" | "UNHEALTHY" | string;
  circuitBreaker: string;
  providerErrorRate: number;
  lastSuccessfulPoll: string | null;
  catalogueStale: boolean;
  catalogueLastRefreshed: string | null;
  maxSlippageBps: number;
  warningSlippageBps: number;
  activeAlerts: string[] | null;
}
