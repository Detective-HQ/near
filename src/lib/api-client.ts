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

export async function apiRequest<T>(
  path: string,
  init: RequestInit = {}
): Promise<T> {
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

export const api = {
  health: () => apiRequest<HealthStatus>("/api/v1/health"),
  chains: () => apiRequest<{ chains: ChainConfig[] }>("/api/v1/chains"),
  assets: (chain?: string) => {
    const query = chain ? `?chain=${encodeURIComponent(chain)}` : "";
    return apiRequest<{ mode: string; notice?: string; servedAt?: string; assets: Asset[] }>(
      `/api/v1/assets${query}`
    );
  },
  markets: (refresh = false) =>
    apiRequest<DataSnapshot>(`/api/v1/markets/overview${refresh ? "?refresh=true" : ""}`),
  intelligence: () => apiRequest<DataSnapshot>("/api/v1/intelligence/near"),
  research: () => apiRequest<ResearchInputs>("/api/v1/research/scenarios"),
  activity: () => apiRequest<{ swaps: SwapRecord[] }>("/api/v1/activity"),
  quote: (input: {
    originAssetId: string;
    destinationAssetId: string;
    amountIn: string;
    recipient?: string;
    refundTo?: string;
    slippageBps?: number;
  }) => apiRequest<{ quote: SwapQuote }>("/api/v1/quotes", {
    method: "POST",
    body: JSON.stringify(input)
  }),
  createLiveSwap: (
    input: { quoteId: string; recipient: string; refundTo: string },
    idempotencyKey: string
  ) =>
    apiRequest<{ swap: SwapRecord; created: boolean }>("/api/v1/swaps", {
      method: "POST",
      headers: { "Idempotency-Key": idempotencyKey },
      body: JSON.stringify(input)
    }),
  recordDeposit: (swapId: string, txHash: string, chainId: string) =>
    apiRequest<{ swap: SwapRecord; duplicate: boolean }>(
      `/api/v1/swaps/${encodeURIComponent(swapId)}/deposit`,
      {
        method: "POST",
        body: JSON.stringify({ txHash, chainId })
      }
    ),
  cancelSwap: (swapId: string) =>
    apiRequest<{ swap: SwapRecord }>(
      `/api/v1/swaps/${encodeURIComponent(swapId)}/cancel`,
      { method: "POST", body: "{}" }
    ),
  swap: (swapId: string) =>
    apiRequest<{ swap: SwapRecord }>(`/api/v1/swaps/${encodeURIComponent(swapId)}`),
  createSimulation: (quoteId: string, idempotencyKey: string) =>
    apiRequest<{ swap: SwapRecord; created: boolean }>("/api/v1/swaps", {
      method: "POST",
      headers: { "Idempotency-Key": idempotencyKey },
      body: JSON.stringify({ quoteId })
    }),
  advanceSimulation: (swapId: string) =>
    apiRequest<{ swap: SwapRecord }>(`/api/v1/swaps/${encodeURIComponent(swapId)}/simulate`, {
      method: "POST",
      body: "{}"
    })
};
