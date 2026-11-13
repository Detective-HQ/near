import { test } from "node:test";
import assert from "node:assert/strict";
import { add, bpsToPercent, cmp, div, formatDecimal, formatPercent, formatUsd, fromAtomic, mul, numberToDecimalString, percentOf, percentToBps, sub, toAtomic } from "../src/lib/decimal.ts";

test("exact arithmetic without float drift", () => {
  assert.equal(add("0.1", "0.2"), "0.3");
  assert.equal(sub("1", "0.9"), "0.1");
  assert.equal(mul("1.1", "1.1"), "1.21");
  assert.equal(div("1", "3", 6), "0.333333");
  assert.equal(div("2", "3", 4), "0.6667");
  assert.equal(div("1", "0"), null);
  assert.equal(cmp("10.0", "9.99"), 1);
});

test("atomic conversion keeps every digit", () => {
  assert.equal(fromAtomic("100000000000000005", 18), "0.100000000000000005");
  assert.equal(toAtomic("0.1", 18), "100000000000000000");
  assert.equal(toAtomic("0.1234567", 6), null, "rejects more precision than the asset has");
});

test("display formatting", () => {
  assert.equal(formatDecimal("1234567.891", 2), "1,234,567.89");
  assert.equal(formatUsd("125000000"), "$125.00M");
  assert.equal(formatUsd("1234.5", false), "$1,234.50");
  assert.equal(formatPercent("2.31", 2, true), "+2.31%");
  assert.equal(numberToDecimalString(1.2e-7), "0.00000012");
  assert.equal(bpsToPercent(50), "0.5");
  assert.equal(percentToBps("0.5"), 50);
  assert.equal(percentToBps("0.555"), null);
  assert.equal(percentOf("25", "200", 1), "12.5");
});
