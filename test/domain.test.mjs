import test from "node:test";
import assert from "node:assert/strict";
import { assets } from "../lib/data.mjs";
import {
  advanceSimulation,
  createQuote,
  createSwap,
  DomainError,
  formatUnits,
  parseUnits
} from "../lib/domain.mjs";

const near = assets.find((asset) => asset.id === "demo:near:near");
const usdc = assets.find((asset) => asset.id === "demo:base:usdc");

test("parseUnits keeps decimal amounts exact", () => {
  assert.equal(parseUnits("12.340001", 6), 12_340_001n);
  assert.equal(parseUnits("0.00000001", 8), 1n);
  assert.equal(formatUnits(12_340_001n, 6), "12.340001");
});

test("parseUnits rejects zero, exponent notation, negatives, and excess precision", () => {
  for (const value of ["0", "0.0", "-1", "1e3", "1.0000001", ""]) {
    assert.throws(() => parseUnits(value, 6), DomainError, value);
  }
});

test("demo quote returns an exact estimate and a smaller minimum", () => {
  const quote = createQuote({
    originAsset: near,
    destinationAsset: usdc,
    amountIn: "10",
    now: 1_800_000_000_000
  });
  assert.equal(quote.mode, "simulation");
  assert.equal(quote.amountInAtomic, "10000000000000000000000000");
  assert.equal(quote.amountOutEstimate, "50.847");
  assert.ok(BigInt(quote.minimumAmountOutAtomic) < BigInt(quote.amountOutEstimateAtomic));
  assert.equal(Date.parse(quote.expiresAt), 1_800_000_120_000);
});

test("quote rejects duplicate assets and expired executable records", () => {
  assert.throws(
    () => createQuote({ originAsset: near, destinationAsset: near, amountIn: "1" }),
    (error) => error.code === "SAME_ASSET"
  );
  const quote = createQuote({ originAsset: near, destinationAsset: usdc, amountIn: "1", now: 1000 });
  assert.throws(
    () => createSwap(quote, { now: 123_001 }),
    (error) => error.code === "QUOTE_EXPIRED"
  );
});

test("simulation lifecycle is explicit, monotonic, and never represents settlement", () => {
  const quote = createQuote({ originAsset: near, destinationAsset: usdc, amountIn: "2" });
  let swap = createSwap(quote, { id: "test", now: 1000 });
  assert.equal(swap.state, "QUOTE_READY");
  assert.equal(swap.realSettlement, false);
  for (const expected of ["SIMULATED_DEPOSIT", "SIMULATED_PROCESSING", "SIMULATION_COMPLETE"]) {
    swap = advanceSimulation(swap, { now: Date.parse(swap.updatedAt) + 1000 });
    assert.equal(swap.state, expected);
    assert.equal(swap.realSettlement, false);
    assert.equal(swap.providerStatus, null);
  }
  assert.equal(swap.transactions.source, null);
  assert.throws(
    () => advanceSimulation(swap),
    (error) => error.code === "TERMINAL_STATE"
  );
});
