import test from "node:test";
import assert from "node:assert/strict";
import { displaySource, formatMetric, formatTimestamp } from "../src/lib/format.ts";

test("formatMetric returns an em dash for missing or non-finite values", () => {
  assert.equal(formatMetric(null), "—");
  assert.equal(formatMetric(undefined), "—");
  assert.equal(formatMetric(Number.NaN), "—");
  assert.equal(formatMetric(Number.POSITIVE_INFINITY), "—");
});

test("formatMetric formats USD with currency and adaptive precision", () => {
  assert.equal(formatMetric(0, "USD"), "$0.00");
  assert.equal(formatMetric(12.5, "USD"), "$12.50");
  assert.equal(formatMetric(1234.5, "USD"), "$1,235");
  assert.equal(formatMetric(1234.5, "USD / rolling 24 hours"), "$1,235");
});

test("formatMetric formats plain numbers with adaptive precision", () => {
  assert.equal(formatMetric(12.5), "12.5");
  assert.equal(formatMetric(12.567), "12.57");
  assert.equal(formatMetric(1234.5), "1,235");
});

test("formatTimestamp renders UTC and passes through unknown values", () => {
  assert.equal(formatTimestamp(null), "Not provided");
  assert.equal(formatTimestamp(""), "Not provided");

  const formatted = formatTimestamp("2026-11-10T12:30:00.000Z");
  assert.match(formatted, /2026/);
  assert.equal(formatted.endsWith("UTC"), true);

  assert.equal(formatTimestamp("not-a-date"), "not-a-date");
});

test("displaySource distinguishes live and demo data", () => {
  assert.equal(displaySource("live", "DeFiLlama"), "DeFiLlama");
  assert.equal(displaySource("live"), "Live source");
  assert.equal(displaySource("demo", "DeFiLlama"), "Demo fixture");
  assert.equal(displaySource("partial"), "Demo fixture");
});
