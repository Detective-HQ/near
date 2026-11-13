import { test } from "node:test";
import assert from "node:assert/strict";
import { runSwap } from "../src/lib/swap-runner.ts";
import type { SwapRunnerApi } from "../src/lib/swap-runner.ts";
import type { SwapQuote, SwapRecord } from "../src/lib/contracts.ts";

const quote = { id: "q1", mode: "provider_dry_quote", amountInAtomic: "1000", originAsset: { contractAddress: null } } as unknown as SwapQuote;
const rec = (state: string): SwapRecord => ({ id: "s1", state, quote: { ...quote, depositAddress: "0xdep" } } as unknown as SwapRecord);
const noSleep = async () => undefined;

function mockApi(states: string[], calls: string[]): SwapRunnerApi {
  let i = 0;
  return {
    createLiveSwap: async () => { calls.push("create"); return rec("AWAITING_USER_DEPOSIT"); },
    createSimulation: async () => { calls.push("createSim"); return rec("QUOTE_READY"); },
    advanceSimulation: async () => { calls.push("advance"); return rec(["SIMULATED_DEPOSIT", "SIMULATED_PROCESSING", "SIMULATION_COMPLETE"][i++]); },
    recordDeposit: async () => { calls.push("deposit"); return rec("DEPOSIT_SUBMITTED"); },
    cancelSwap: async () => { calls.push("cancel"); return rec("CANCELLED"); },
    getSwap: async () => { calls.push("poll"); return rec(states[Math.min(i++, states.length - 1)]); },
  };
}

test("live swap: wallet confirms, deposit is recorded, polling stops at Completed", async () => {
  const calls: string[] = [];
  const phases: string[] = [];
  const wallet = { account: "0xabc", chainId: "1", sendDeposit: async () => { calls.push("wallet"); return "0xhash"; } };
  const result = await runSwap({
    quote, api: mockApi(["PROCESSING", "SUCCEEDED"], calls), wallet, idempotencyKey: "k", sleep: noSleep,
    onProgress: (p) => phases.push(p),
  });
  assert.equal(result.kind, "final");
  assert.deepEqual(calls, ["create", "wallet", "deposit", "poll", "poll"]);
  assert.deepEqual(phases, ["confirm", "submitted", "processing", "completed"]);
});

test("live swap: rejection in wallet cancels the unfunded instruction", async () => {
  const calls: string[] = [];
  const wallet = { account: "0xabc", chainId: "1", sendDeposit: async () => { throw Object.assign(new Error("User rejected"), { code: 4001 }); } };
  const result = await runSwap({ quote, api: mockApi([], calls), wallet, idempotencyKey: "k", sleep: noSleep, onProgress: () => undefined });
  assert.equal(result.kind, "rejected");
  assert.deepEqual(calls, ["create", "cancel"]);
});

test("live swap: refund is reported as Refunded", async () => {
  const phases: string[] = [];
  const wallet = { account: "0xabc", chainId: "1", sendDeposit: async () => "0xhash" };
  await runSwap({ quote, api: mockApi(["REFUNDED"], []), wallet, idempotencyKey: "k", sleep: noSleep, onProgress: (p) => phases.push(p) });
  assert.equal(phases.at(-1), "refunded");
});

test("live swap: provider failure is reported as Failed", async () => {
  const phases: string[] = [];
  const wallet = { account: "0xabc", chainId: "1", sendDeposit: async () => "0xhash" };
  await runSwap({ quote, api: mockApi(["FAILED"], []), wallet, idempotencyKey: "k", sleep: noSleep, onProgress: (p) => phases.push(p) });
  assert.equal(phases.at(-1), "failed");
});

test("live swap requires a wallet and never submits without one", async () => {
  const calls: string[] = [];
  await assert.rejects(runSwap({ quote, api: mockApi([], calls), wallet: null, idempotencyKey: "k", sleep: noSleep, onProgress: () => undefined }));
  assert.deepEqual(calls, []);
});

test("simulation (TEST DATA) steps through to Completed without a wallet", async () => {
  const phases: string[] = [];
  const sim = { ...quote, mode: "simulation" } as SwapQuote;
  const result = await runSwap({ quote: sim, api: mockApi([], []), wallet: null, idempotencyKey: "k", sleep: noSleep, onProgress: (p) => phases.push(p) });
  assert.equal(result.kind, "final");
  assert.equal(phases.at(-1), "completed");
});
