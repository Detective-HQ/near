import { test } from "node:test";
import assert from "node:assert/strict";
import { qualityFromStatus, qualityTone, regimeLabel, scoreBand } from "../src/lib/quality.ts";

test("score bands", () => {
  assert.equal(scoreBand("85"), "Strong");
  assert.equal(scoreBand("84.9"), "Setup");
  assert.equal(scoreBand("70"), "Setup");
  assert.equal(scoreBand("55"), "Moderate");
  assert.equal(scoreBand("40"), "Neutral");
  assert.equal(scoreBand("39.9"), "Weak");
  assert.equal(scoreBand(null), null);
});

test("quality badges; Unavailable is neutral, never bearish", () => {
  assert.equal(qualityFromStatus("available"), "Live");
  assert.equal(qualityFromStatus("stale"), "Stale");
  assert.equal(qualityFromStatus("partial"), "Partial");
  assert.equal(qualityFromStatus("delayed"), "Delayed");
  assert.equal(qualityFromStatus(undefined), "Unavailable");
  assert.equal(qualityFromStatus("available", false), "Unavailable");
  assert.equal(qualityTone("Unavailable"), "");
});

test("regime only reflects the API value", () => {
  assert.equal(regimeLabel("bullish"), "Bullish");
  assert.equal(regimeLabel("mixed"), "Mixed");
  assert.equal(regimeLabel(null), "Unavailable");
  assert.equal(regimeLabel("green"), "Unavailable");
});
