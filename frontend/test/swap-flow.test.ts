import { test } from "node:test";
import assert from "node:assert/strict";
import { isFinalSwapState, isValidAmount, secondsLeft, swapCta, swapOutcome, swapPill } from "../src/lib/swap-flow.ts";
import type { CtaInput } from "../src/lib/swap-flow.ts";

const ready: CtaInput = {
  walletConnected: true, fromSelected: true, toSelected: true, amountValid: true, exceedsBalance: false,
  routeSupported: true, quoteStatus: "ready", quoteExpired: false, stale: false,
};

test("CTA walks through every state in order", () => {
  assert.equal(swapCta({ ...ready, walletConnected: false }).label, "Connect wallet");
  assert.equal(swapCta({ ...ready, toSelected: false }).label, "Select assets");
  assert.equal(swapCta({ ...ready, amountValid: false }).label, "Enter amount");
  assert.equal(swapCta({ ...ready, exceedsBalance: true }).label, "Insufficient balance");
  assert.equal(swapCta({ ...ready, routeSupported: false }).label, "Route not supported");
  assert.equal(swapCta({ ...ready, quoteStatus: "loading" }).label, "Getting quote...");
  const expired = swapCta({ ...ready, quoteExpired: true });
  assert.equal(expired.label, "Quote expired - Refresh");
  assert.equal(expired.disabled, false);
  assert.deepEqual(swapCta(ready), { label: "Review swap", action: "review", disabled: false });
});

test("CTA is disabled when price data is stale", () => {
  assert.equal(swapCta({ ...ready, stale: true }).disabled, true);
});

test("status pills follow the API mapping and never invent success", () => {
  assert.equal(swapPill("AWAITING_USER_DEPOSIT"), "Pending");
  assert.equal(swapPill("PROCESSING"), "Processing");
  assert.equal(swapPill("SUCCEEDED"), "Completed");
  assert.equal(swapPill("FAILED"), "Failed");
  assert.equal(swapPill("REFUNDED"), "Refunded");
  assert.equal(swapPill("UNKNOWN_PROVIDER_STATUS"), "Processing");
  assert.equal(isFinalSwapState("PROCESSING"), false);
  assert.equal(swapOutcome("REFUNDED"), "refunded");
});

test("quote expiry and amount validation", () => {
  const now = Date.parse("2026-11-13T12:00:00Z");
  assert.equal(secondsLeft("2026-11-13T12:00:30Z", now), 30);
  assert.equal(secondsLeft("2026-11-13T11:59:00Z", now), 0);
  assert.equal(secondsLeft(null, now), null);
  assert.equal(isValidAmount("0.00"), false);
  assert.equal(isValidAmount("1.5"), true);
  assert.equal(isValidAmount("-1"), false);
  assert.equal(isValidAmount("1e3"), false);
});
