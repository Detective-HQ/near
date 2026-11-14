// Submits a reviewed quote and follows it to a final state. Wallet and API are injected,
// so the flow runs the same in the browser and in tests with a mocked wallet.
// The wallet signs and broadcasts; this code never sees keys or signatures, only the tx hash.
import type { SwapQuote, SwapRecord } from "./contracts";
import { isFinalSwapState, swapOutcome } from "./swap-flow.ts";
import type { ProgressPhase } from "./swap-flow.ts";

export interface SwapRunnerApi {
  createLiveSwap(input: { quoteId: string; recipient: string; refundTo: string }, key: string): Promise<SwapRecord>;
  createSimulation(quoteId: string, key: string): Promise<SwapRecord>;
  advanceSimulation(id: string): Promise<SwapRecord>;
  recordDeposit(id: string, txHash: string, chainId: string): Promise<SwapRecord>;
  cancelSwap(id: string): Promise<SwapRecord>;
  getSwap(id: string): Promise<SwapRecord>;
  depositTx?(id: string, from?: string): Promise<{ family: string; chainKey: string; payload: unknown; memo?: string | null; expiresAt?: string | null }>;
}

export interface SwapRunnerWallet {
  account: string;
  chainId: string;
  /** Asks the wallet to sign and broadcast the deposit; resolves with the tx hash. */
  sendDeposit(swap: SwapRecord): Promise<string>;
  signAndSendPayload?(tx: { family: string; chainKey: string; payload: unknown; memo?: string | null }): Promise<string>;
}

export interface RunOptions {
  quote: SwapQuote;
  api: SwapRunnerApi;
  wallet: SwapRunnerWallet | null;
  idempotencyKey: string;
  onProgress: (phase: ProgressPhase, swap: SwapRecord | null) => void;
  pollMs?: number;
  sleep?: (ms: number) => Promise<void>;
  /** stop polling when this returns true (e.g. component unmounted) */
  cancelled?: () => boolean;
  maxPolls?: number;
}

export type RunResult =
  | { kind: "final"; swap: SwapRecord }
  | { kind: "rejected" }
  | { kind: "stopped"; swap: SwapRecord | null };

const defaultSleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

export function isUserRejection(error: unknown): boolean {
  const e = error as { code?: unknown; message?: unknown } | null;
  if (!e) return false;
  if (e.code === 4001 || e.code === "ACTION_REJECTED") return true;
  return typeof e.message === "string" && /user (rejected|denied)/i.test(e.message);
}

export async function runSwap(opts: RunOptions): Promise<RunResult> {
  const { quote, api, wallet, onProgress } = opts;
  const sleep = opts.sleep ?? defaultSleep;
  const pollMs = opts.pollMs ?? 5000;
  const cancelled = opts.cancelled ?? (() => false);
  const maxPolls = opts.maxPolls ?? 720;

  onProgress("confirm", null);
  let swap: SwapRecord;

  if (quote.mode === "simulation") {
    swap = await api.createSimulation(quote.id, opts.idempotencyKey);
    onProgress("submitted", swap);
    for (let i = 0; i < 6 && !isFinalSwapState(swap.state); i++) {
      if (cancelled()) return { kind: "stopped", swap };
      await sleep(Math.min(pollMs, 1200));
      swap = await api.advanceSimulation(swap.id);
      if (!isFinalSwapState(swap.state)) onProgress("processing", swap);
    }
  } else {
    if (!wallet) throw new Error("A connected wallet is required.");
    swap = await api.createLiveSwap(
      { quoteId: quote.id, recipient: wallet.account, refundTo: wallet.account },
      opts.idempotencyKey,
    );
    let txHash: string;
    try {
      if (api.depositTx && wallet.signAndSendPayload) {
        const tx = await api.depositTx(swap.id, wallet.account);
        txHash = await wallet.signAndSendPayload(tx);
      } else {
        txHash = await wallet.sendDeposit(swap);
      }
    } catch (error) {
      // Nothing was sent: discard the unfunded instruction.
      await api.cancelSwap(swap.id).catch(() => undefined);
      if (isUserRejection(error)) return { kind: "rejected" };
      throw error;
    }
    swap = await api.recordDeposit(swap.id, txHash, wallet.chainId);
    onProgress("submitted", swap);

    // Stream updates via Server-Sent Events with fallback to polling
    if (typeof EventSource !== "undefined") {
      try {
        const es = new EventSource(`/api/v1/swaps/${encodeURIComponent(swap.id)}/events`);
        await new Promise<void>((resolve) => {
          es.addEventListener("swap", (e: MessageEvent) => {
            try {
              const updated = JSON.parse(e.data) as SwapRecord;
              swap = updated;
              if (!isFinalSwapState(swap.state)) {
                onProgress("processing", swap);
              } else {
                es.close();
                resolve();
              }
            } catch {
              // ignore parse errors
            }
          });
          es.onerror = () => {
            es.close();
            resolve();
          };
        });
      } catch {
        // EventSource unavailable or failed; fall back to polling below
      }
    }

    for (let i = 0; i < maxPolls && !isFinalSwapState(swap.state); i++) {
      if (cancelled()) return { kind: "stopped", swap };
      await sleep(pollMs);
      swap = await api.getSwap(swap.id);
      if (!isFinalSwapState(swap.state)) onProgress("processing", swap);
    }
  }

  const outcome = swapOutcome(swap.state);
  if (!outcome) return { kind: "stopped", swap };
  onProgress(outcome, swap);
  return { kind: "final", swap };
}
