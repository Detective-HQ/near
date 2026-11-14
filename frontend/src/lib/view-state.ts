// Shared view states and their copy. Every workspace view resolves to one of these.
// No imports so it can be unit-tested directly.

export type ViewState = "loading" | "offline" | "error" | "unavailable" | "empty" | "stale" | "partial" | "ready" | "unauthorized";

export type ViewKey =
  | "explore"
  | "swap"
  | "activity"
  | "portfolio"
  | "markets"
  | "trading"
  | "trading-asset"
  | "trading-history"
  | "trading-backtest"
  | "intelligence"
  | "research"
  | "risk";

export const STATE_COPY = {
  partial: "Some data is missing. Showing what's available.",
  stale: (time: string) => `This data is out of date. Last updated ${time}.`,
  unavailable: "Data unavailable right now.",
  error: "Something went wrong. Please try again.",
  offline: "You're offline. Reconnect to update.",
} as const;

export const EMPTY_COPY: Record<ViewKey, { message: string; action?: string }> = {
  explore: { message: "No assets match these filters.", action: "Clear filters" },
  swap: { message: "Data unavailable right now.", action: "Retry" },
  activity: { message: "No swaps yet.", action: "Make your first swap" },
  portfolio: { message: "No balances found on supported chains." },
  markets: { message: "Data unavailable right now.", action: "Retry" },
  trading: { message: "No setups meet the criteria right now." },
  "trading-asset": { message: "Asset not found.", action: "Back to scanner" },
  "trading-history": { message: "No signals recorded yet." },
  "trading-backtest": { message: "No reproducible backtests available yet." },
  intelligence: { message: "No data for this chain yet." },
  research: { message: "Data unavailable right now.", action: "Retry" },
  risk: { message: "Data unavailable right now.", action: "Retry" },
};

export interface ErrorLike {
  status?: number;
  code?: string | null;
  name?: string;
}

/** Missing endpoints, upstream outages, network failures and gated fixtures are "unavailable"; 401 is unauthorized; everything else is an error. */
export function classifyError(error: unknown): "unavailable" | "error" | "unauthorized" {
  const e = (error ?? {}) as ErrorLike;
  if (e.status === 401 || e.name === "UnauthorizedError" || e.code === "UNAUTHORIZED") return "unauthorized";
  if (e.name === "UnavailableError" || e.name === "TypeError") return "unavailable";
  if (e.status === 404 || e.status === 502 || e.status === 503 || e.status === 504) return "unavailable";
  if (e.code === "API_UNAVAILABLE" || e.code === "NOT_FOUND" || e.code === "UPSTREAM_UNAVAILABLE") return "unavailable";
  return "error";
}

export interface ViewInput {
  loading: boolean;
  online: boolean;
  error: unknown;
  hasData: boolean;
  isEmpty?: boolean;
  stale?: boolean;
  partial?: boolean;
}

export function deriveViewState(input: ViewInput): ViewState {
  if (!input.hasData) {
    if (input.loading) return "loading";
    if (!input.online) return "offline";
    if (input.error) return classifyError(input.error);
    return "loading";
  }
  if (input.isEmpty) return "empty";
  if (input.stale) return "stale";
  if (input.partial) return "partial";
  return "ready";
}

/** The visible message and action label for a view in a given state. */
export function viewMessage(view: ViewKey, state: ViewState, staleTime = "unknown"): { message: string; action?: string } | null {
  switch (state) {
    case "empty":
      return EMPTY_COPY[view];
    case "partial":
      return { message: STATE_COPY.partial };
    case "stale":
      return { message: STATE_COPY.stale(staleTime), action: "Refresh" };
    case "unavailable":
      return { message: STATE_COPY.unavailable, action: "Retry" };
    case "error":
      return { message: STATE_COPY.error, action: "Retry" };
    case "unauthorized":
      return { message: "Sign in with your wallet to continue.", action: "Sign in" };
    case "offline":
      return { message: STATE_COPY.offline };
    default:
      return null;
  }
}
