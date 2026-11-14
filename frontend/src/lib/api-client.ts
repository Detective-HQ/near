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
  PortfolioResponse,
  ScenarioResponse,
  SignalsResponse,
  StructureResponse,
  TopMoversResponse,
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

/** 401 Unauthorized error requiring wallet sign-in. */
export class UnauthorizedError extends ApiRequestError {
  constructor(message = "Sign in with your wallet to continue.", code: string | null = "UNAUTHORIZED", requestId: string | null = null) {
    super(message, 401, code, requestId);
    this.name = "UnauthorizedError";
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
    const msg = body.error?.message ?? `The API request failed (${response.status}).`;
    const code = body.error?.code ?? null;
    const reqId = body.requestId ?? response.headers.get("x-request-id");
    if (response.status === 401) {
      throw new UnauthorizedError(msg, code, reqId);
    }
    throw new ApiRequestError(
      msg,
      response.status,
      code,
      reqId
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
  simulation?: boolean;
  servedAt: string | null;
}

export interface ActivityResponse {
  swaps: SwapRecord[];
  nextCursor?: string;
}

export const api = {
  health: () => apiRequest<HealthStatus>("/api/v1/health"),

  chains: async () => {
    const payload = await apiRequest<ChainConfig[] | { chains: ChainConfig[] }>("/api/v1/chains");
    return unwrap<ChainConfig[]>(payload, "chains");
  },

  /** Catalogue for Explore and Swap. Fixture assets are allowed in simulation/test data mode without throwing. */
  assets: async (chain?: string): Promise<Catalogue> => {
    const payload = await apiRequest<unknown>(`/api/v1/assets${chain ? `?chain=${enc(chain)}` : ""}`);
    const list = unwrap<Catalogue["assets"]>(payload, "assets") ?? [];
    const servedAt = (payload as { servedAt?: string }).servedAt ?? null;
    const fixtures = list.filter(isFixtureAsset);
    if (fixtures.length === list.length && list.length > 0) {
      return { assets: list, testData: USE_MOCKS, simulation: true, servedAt };
    }
    return { assets: list.filter((a) => !isFixtureAsset(a)), testData: false, simulation: false, servedAt };
  },

  markets: async (refresh = false) =>
    gateSnapshot(await apiRequest<DataSnapshot>(`/api/v1/markets/overview${refresh ? "?refresh=true" : ""}`)),

  intelligence: async () => gateSnapshot(await apiRequest<DataSnapshot>("/api/v1/intelligence/near")),

  /** Scenario inputs are illustrative model output. */
  research: async () => {
    const inputs = await apiRequest<ResearchInputs>("/api/v1/research/scenarios");
    return { ...inputs, testData: USE_MOCKS, illustrative: true };
  },

  risk: () => apiRequest<RiskReport>("/api/v1/risk/status"),

  activity: async (cursor?: string, limit = 50): Promise<ActivityResponse> => {
    const p = new URLSearchParams();
    if (cursor) p.set("cursor", cursor);
    if (limit) p.set("limit", String(limit));
    const qs = p.toString() ? `?${p.toString()}` : "";
    const payload = await apiRequest<SwapRecord[] | { swaps: SwapRecord[]; nextCursor?: string }>(`/api/v1/activity${qs}`);
    if (Array.isArray(payload)) {
      return { swaps: payload };
    }
    return {
      swaps: payload.swaps ?? [],
      nextCursor: payload.nextCursor,
    };
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

  claimSwap: async (swapId: string) =>
    unwrap<SwapRecord>(await apiRequest<unknown>(`/api/v1/swaps/${enc(swapId)}/claim`, { method: "POST", body: "{}" }), "swap"),

  depositTx: async (swapId: string, from?: string) => {
    const qs = from ? `?from=${encodeURIComponent(from)}` : "";
    return apiRequest<{
      family: string;
      chainKey: string;
      payload: unknown;
      memo?: string | null;
      expiresAt?: string | null;
    }>(`/api/v1/swaps/${enc(swapId)}/deposit-tx${qs}`);
  },

  swap: async (swapId: string) => unwrap<SwapRecord>(await apiRequest<unknown>(`/api/v1/swaps/${enc(swapId)}`), "swap"),

  // Trading intelligence: read-only, served by the Go API when MARKET_DATA_MODE=live (see trading-contracts.ts).
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
  tradingTopMovers: async (timeframe?: string) => {
    const m = await import("./mocks");
    const p = new URLSearchParams(timeframe ? { timeframe } : {});
    return mockOr(
      () => {
        const sigs = m.mockSignals(p);
        const all = [...sigs.bullish, ...sigs.bearish];
        return { source: sigs.source, asOf: sigs.asOf, quality: sigs.quality, items: all.slice(0, 10) };
      },
      () => apiRequest<TopMoversResponse>(`/api/v1/trading/top-movers${qs(p)}`)
    );
  },

  // Wallet sign-in. The API sets an HttpOnly session cookie on verify; the
  // browser sends it automatically, so the token never needs to be stored here.
  authNonce: async () => apiRequest<{ nonce: string; message: string }>("/api/v1/auth/nonce", { method: "POST", body: "{}" }),
  authVerify: async (input: { chain: string; walletAddress: string; signature: string; nonce: string; message: string; publicKey?: string }) =>
    apiRequest<{ token: string; session: AuthSession }>("/api/v1/auth/verify", {
      method: "POST",
      body: JSON.stringify(input)
    }),
  authSession: async () => apiRequest<{ session: AuthSession; authRequired: boolean }>("/api/v1/auth/session"),
  authLogout: async () => apiRequest<{ status: string }>("/api/v1/auth/logout", { method: "POST", body: "{}" }),

  portfolio: async (address?: string, chain?: string, tokens: string = "catalogue") => {
    const m = await import("./mocks");
    const p = new URLSearchParams();
    if (address) p.set("address", address);
    if (chain) p.set("chain", chain);
    if (tokens) p.set("tokens", tokens);
    return mockOr(
      () => {
        const balances = m.mockPortfolio().filter((b) => !chain || b.chain === chain);
        return {
          source: "TEST DATA",
          asOf: "2026-11-13T12:00:00Z",
          quality: "live" as const,
          address: address ?? null,
          chain: chain ?? null,
          totalValueUsd: "6745.51",
          balances,
        };
      },
      () => apiRequest<PortfolioResponse>(`/api/v1/portfolio${qs(p)}`)
    );
  },

  /** TEST DATA balances for interface testing (mocks only). Real balances come from api.portfolio. */
  portfolioTestData: async (): Promise<PortfolioBalance[] | null> => {
    if (!USE_MOCKS) return null;
    const m = await import("./mocks");
    return m.mockPortfolio();
  }
};

export interface AuthSession {
  id: string;
  walletAddress: string;
  chain: string;
  createdAt: string;
  expiresAt: string;
}

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
