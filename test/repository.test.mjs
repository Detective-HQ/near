import test from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createRepository } from "../lib/repository.mjs";

test("repository persists simulation data and returns the same record for a repeated idempotency key", () => {
  const directory = mkdtempSync(join(tmpdir(), "udi-test-"));
  const filePath = join(directory, "udi.json");
  try {
    const repository = createRepository(filePath);
    const quote = { id: "qte_test", expiresAt: new Date(Date.now() + 60_000).toISOString() };
    repository.saveQuote(quote);
    const first = { id: "swp_first", quote, state: "QUOTE_READY", createdAt: new Date().toISOString() };
    assert.equal(repository.createSwap(first, "idempotency-123").created, true);

    const reloaded = createRepository(filePath);
    assert.deepEqual(reloaded.getQuote("qte_test"), quote);
    const duplicate = { id: "swp_second", quote, state: "QUOTE_READY", createdAt: new Date().toISOString() };
    const result = reloaded.createSwap(duplicate, "idempotency-123");
    assert.equal(result.created, false);
    assert.equal(result.swap.id, "swp_first");
    assert.equal(JSON.parse(readFileSync(filePath, "utf8")).swaps.swp_first.id, "swp_first");
  } finally {
    rmSync(directory, { recursive: true, force: true });
  }
});
