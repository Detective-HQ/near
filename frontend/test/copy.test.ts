import { test } from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

const ROOTS = ["src/components/workspace", "src/lib", "src/app/workspace"];
const BANNED = [/guaranteed/i, /will go up/i, /\bprofit\b/i, /win rate/i, /institutional buying/i];

function files(dir: string): string[] {
  return readdirSync(dir).flatMap((f) => {
    const p = join(dir, f);
    return statSync(p).isDirectory() ? files(p) : /\.(tsx?|css)$/.test(f) ? [p] : [];
  });
}

test("no banned words in workspace copy", () => {
  for (const f of ROOTS.flatMap(files)) {
    const text = readFileSync(f, "utf8");
    for (const re of BANNED) assert.ok(!re.test(text), `${f} contains ${re}`);
  }
});

test("scores are never described as a probability", () => {
  for (const f of ROOTS.flatMap(files)) {
    for (const line of readFileSync(f, "utf8").split("\n")) {
      if (/probabilit/i.test(line)) assert.ok(/not (a )?probabilit|not probabilities/i.test(line), `${f}: ${line.trim()}`);
    }
  }
});

test("trading pages have no trade or swap actions", () => {
  for (const f of files("src/components/workspace/pages").filter((p) => /trading/.test(p))) {
    const text = readFileSync(f, "utf8");
    assert.ok(!/\/workspace\/swap/.test(text), `${f} links to swap`);
    assert.ok(!/>\s*(Buy|Sell|Swap)\s*</.test(text), `${f} has a trade button`);
  }
});

test("no secrets or seed phrases are referenced client-side", () => {
  for (const f of ROOTS.flatMap(files)) {
    const text = readFileSync(f, "utf8");
    assert.ok(!/ONECLICK_JWT|seed phrase input|privateKey\s*[:=]/i.test(text), `${f} references a secret`);
  }
});
