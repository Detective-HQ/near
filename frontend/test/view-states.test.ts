import { test } from "node:test";
import assert from "node:assert/strict";
import { classifyError, deriveViewState, EMPTY_COPY, viewMessage } from "../src/lib/view-state.ts";
import type { ViewKey } from "../src/lib/view-state.ts";

const VIEWS = Object.keys(EMPTY_COPY) as ViewKey[];
const base = { loading: false, online: true, error: null, hasData: true };

for (const view of VIEWS) {
  test(`${view}: six shared states plus offline`, () => {
    // loading
    assert.equal(deriveViewState({ ...base, loading: true, hasData: false }), "loading");
    assert.equal(viewMessage(view, "loading"), null);
    // empty
    assert.equal(deriveViewState({ ...base, isEmpty: true }), "empty");
    assert.deepEqual(viewMessage(view, "empty"), EMPTY_COPY[view]);
    // partial
    assert.equal(deriveViewState({ ...base, partial: true }), "partial");
    assert.equal(viewMessage(view, "partial")?.message, "Some data is missing. Showing what's available.");
    // stale
    assert.equal(deriveViewState({ ...base, stale: true }), "stale");
    assert.deepEqual(viewMessage(view, "stale", "12:00"), { message: "This data is out of date. Last updated 12:00.", action: "Refresh" });
    // unavailable (missing endpoint)
    assert.equal(deriveViewState({ ...base, hasData: false, error: { status: 404, code: "NOT_FOUND" } }), "unavailable");
    assert.deepEqual(viewMessage(view, "unavailable"), { message: "Data unavailable right now.", action: "Retry" });
    // error
    assert.equal(deriveViewState({ ...base, hasData: false, error: { status: 500, code: "INTERNAL" } }), "error");
    assert.deepEqual(viewMessage(view, "error"), { message: "Something went wrong. Please try again.", action: "Retry" });
    // offline
    assert.equal(deriveViewState({ ...base, hasData: false, online: false, error: new Error("x") }), "offline");
    assert.equal(viewMessage(view, "offline")?.message, "You're offline. Reconnect to update.");
  });
}

test("empty-state copy matches the spec", () => {
  assert.equal(EMPTY_COPY.explore.message, "No assets match these filters.");
  assert.equal(EMPTY_COPY.activity.message, "No swaps yet.");
  assert.equal(EMPTY_COPY.portfolio.message, "No balances found on supported chains.");
  assert.equal(EMPTY_COPY.trading.message, "No setups meet the criteria right now.");
  assert.equal(EMPTY_COPY["trading-history"].message, "No signals recorded yet.");
  assert.equal(EMPTY_COPY["trading-backtest"].message, "No reproducible backtests available yet.");
  assert.equal(EMPTY_COPY.intelligence.message, "No data for this chain yet.");
});

test("gated fixtures and outages are unavailable, not errors", () => {
  assert.equal(classifyError({ name: "UnavailableError" }), "unavailable");
  assert.equal(classifyError({ status: 503 }), "unavailable");
  assert.equal(classifyError({ status: 400, code: "VALIDATION_FAILED" }), "error");
});
