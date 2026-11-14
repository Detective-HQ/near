// Swap UX state: the call-to-action machine, status pills and progress steps.
// Pure and import-free so the whole flow can be unit-tested with a mocked wallet.

export type SwapCta =
  | { label: "Connect wallet"; action: "connect"; disabled: false }
  | { label: "Select assets"; action: "none"; disabled: true }
  | { label: "Enter amount"; action: "none"; disabled: true }
  | { label: "Insufficient balance"; action: "none"; disabled: true }
  | { label: "Route not supported"; action: "none"; disabled: true }
  | { label: "Getting quote..."; action: "none"; disabled: true }
  | { label: "Quote expired - Refresh"; action: "refresh"; disabled: false }
  | { label: "Review swap"; action: "review"; disabled: boolean };

export interface CtaInput {
  walletConnected: boolean;
  fromSelected: boolean;
  toSelected: boolean;
  amountValid: boolean;
  /** true when a known balance is lower than the amount */
  exceedsBalance: boolean;
  routeSupported: boolean;
  quoteStatus: "idle" | "loading" | "ready" | "error";
  quoteExpired: boolean;
  /** price data or catalogue is stale */
  stale: boolean;
}

export function swapCta(i: CtaInput): SwapCta {
  if (!i.walletConnected) return { label: "Connect wallet", action: "connect", disabled: false };
  if (!i.fromSelected || !i.toSelected) return { label: "Select assets", action: "none", disabled: true };
  if (!i.amountValid) return { label: "Enter amount", action: "none", disabled: true };
  if (i.exceedsBalance) return { label: "Insufficient balance", action: "none", disabled: true };
  if (!i.routeSupported) return { label: "Route not supported", action: "none", disabled: true };
  if (i.quoteStatus === "loading") return { label: "Getting quote...", action: "none", disabled: true };
  if (i.quoteExpired) return { label: "Quote expired - Refresh", action: "refresh", disabled: false };
  // No usable quote yet (debouncing, or the quote panel is showing an error with Retry).
  if (i.quoteStatus !== "ready") return { label: "Review swap", action: "review", disabled: true };
  return { label: "Review swap", action: "review", disabled: i.stale };
}

export type SwapPill = "Pending" | "Processing" | "Completed" | "Failed" | "Refunded";

const PILL: Record<string, SwapPill> = {
  CREATED: "Pending",
  QUOTING: "Pending",
  QUOTED: "Pending",
  QUOTE_UNAVAILABLE: "Failed",
  QUOTE_READY: "Pending",
  AWAITING_USER_DEPOSIT: "Pending",
  SIMULATED_DEPOSIT: "Pending",
  DEPOSIT_SUBMITTED: "Processing",
  INCOMPLETE_DEPOSIT: "Processing",
  PROCESSING: "Processing",
  SIMULATED_PROCESSING: "Processing",
  UNKNOWN_PROVIDER_STATUS: "Processing",
  SUCCEEDED: "Completed",
  SIMULATION_COMPLETE: "Completed",
  FAILED: "Failed",
  EXPIRED: "Failed",
  CANCELLED: "Failed",
  REFUNDED: "Refunded",
};

/** API state -> pill. Unknown states are never shown as success. */
export function swapPill(state: string): SwapPill {
  return PILL[state] ?? "Processing";
}

export function swapPillTone(p: SwapPill): "" | "good" | "warn" | "bad" {
  if (p === "Completed") return "good";
  if (p === "Failed") return "bad";
  if (p === "Refunded" || p === "Processing") return "warn";
  return "";
}

// Terminal states of the Go API lifecycle (plus legacy simulation names).
const FINAL = new Set(["SUCCEEDED", "SIMULATION_COMPLETE", "FAILED", "EXPIRED", "CANCELLED", "REFUNDED", "QUOTE_UNAVAILABLE"]);
export function isFinalSwapState(state: string): boolean {
  return FINAL.has(state);
}

export type SwapOutcome = "completed" | "failed" | "refunded";
export function swapOutcome(state: string): SwapOutcome | null {
  const p = swapPill(state);
  if (!isFinalSwapState(state)) return null;
  return p === "Completed" ? "completed" : p === "Refunded" ? "refunded" : "failed";
}

export const PROGRESS_STEPS = ["Confirm in your wallet", "Swap submitted", "Processing"] as const;
export type ProgressPhase = "confirm" | "submitted" | "processing" | SwapOutcome;

export function finalLabel(o: SwapOutcome): "Completed" | "Failed" | "Refunded" {
  return o === "completed" ? "Completed" : o === "refunded" ? "Refunded" : "Failed";
}

/** Whole seconds until expiry, never negative. Null when the quote has no expiry. */
export function secondsLeft(expiresAt: string | null | undefined, now: number): number | null {
  if (!expiresAt) return null;
  const t = Date.parse(expiresAt);
  if (Number.isNaN(t)) return null;
  return Math.max(0, Math.floor((t - now) / 1000));
}

export const EXPIRY_WARNING_SECONDS = 15;

/** Amount input: digits with an optional fraction, no sign, no exponent. */
export function isValidAmount(value: string): boolean {
  return /^\d+(\.\d+)?$/.test(value.trim()) && /[1-9]/.test(value);
}
