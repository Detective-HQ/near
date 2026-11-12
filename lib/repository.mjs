import { existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { randomUUID } from "node:crypto";

function emptyState() {
  return { quotes: {}, swaps: {}, idempotency: {} };
}

export function createRepository(filePath = null) {
  const resolvedPath = filePath ? resolve(filePath) : null;
  let state = emptyState();

  if (resolvedPath && existsSync(resolvedPath)) {
    const loaded = JSON.parse(readFileSync(resolvedPath, "utf8"));
    if (!loaded || typeof loaded !== "object" || !loaded.quotes || !loaded.swaps || !loaded.idempotency) {
      throw new Error("Persistence file has an invalid shape.");
    }
    state = loaded;
  }

  function persist() {
    if (!resolvedPath) return;
    mkdirSync(dirname(resolvedPath), { recursive: true });
    const temporaryPath = `${resolvedPath}.${randomUUID()}.tmp`;
    writeFileSync(temporaryPath, JSON.stringify(state, null, 2), { mode: 0o600 });
    renameSync(temporaryPath, resolvedPath);
  }

  return {
    saveQuote(quote) {
      state.quotes[quote.id] = quote;
      persist();
      return quote;
    },
    getQuote(id) {
      return state.quotes[id] ?? null;
    },
    createSwap(swap, idempotencyKey) {
      const existingId = idempotencyKey ? state.idempotency[idempotencyKey] : null;
      if (existingId) return { swap: state.swaps[existingId], created: false };

      state.swaps[swap.id] = swap;
      if (idempotencyKey) state.idempotency[idempotencyKey] = swap.id;
      persist();
      return { swap, created: true };
    },
    getSwap(id) {
      return state.swaps[id] ?? null;
    },
    updateSwap(swap) {
      state.swaps[swap.id] = swap;
      persist();
      return swap;
    },
    listSwaps() {
      return Object.values(state.swaps).sort((a, b) => b.createdAt.localeCompare(a.createdAt));
    }
  };
}
