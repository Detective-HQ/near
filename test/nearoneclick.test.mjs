import test from "node:test";
import assert from "node:assert/strict";
import { createNearOneClickProvider } from "../lib/providers/nearoneclick.mjs";

const tokenFixture = {
  assetId: "nep141:wrap.near",
  blockchain: "near",
  symbol: "wNEAR",
  decimals: 24,
  contractAddress: "wrap.near",
  price: 5.1,
  priceUpdatedAt: "2026-11-10T12:00:00.000Z"
};

const quoteFixture = {
  correlationId: "quote-correlation-1",
  timestamp: "2026-11-10T12:00:00.000Z",
  signature: "provider-signature",
  quote: {
    amountOut: "9900000",
    amountOutFormatted: "9.9",
    minAmountOut: "9800000",
    minAmountOutFormatted: "9.8"
  }
};

function jsonResponse(payload, status = 200) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "content-type": "application/json" }
  });
}

test("catalogue preserves provider asset IDs and caches validated entries", async () => {
  let calls = 0;
  const provider = createNearOneClickProvider({
    fetchImpl: async () => {
      calls += 1;
      return jsonResponse([tokenFixture]);
    },
    now: () => Date.parse("2026-11-10T12:01:00.000Z")
  });

  const first = await provider.getTokens();
  const second = await provider.getTokens();
  assert.equal(calls, 1);
  assert.equal(first, second);
  assert.equal(first[0].id, "nep141:wrap.near");
  assert.equal(first[0].providerAssetId, "nep141:wrap.near");
  assert.equal(first[0].decimals, 24);
  assert.equal(first[0].enabledForLiveSwap, false);
});

test("dry quote authenticates server-side, sends atomic exact-input fields, and returns no deposit instructions", async () => {
  let captured;
  const provider = createNearOneClickProvider({
    apiKey: "server-only-secret",
    fetchImpl: async (url, options) => {
      captured = { url, options, body: JSON.parse(options.body) };
      return jsonResponse(quoteFixture);
    }
  });
  const originAsset = {
    id: "nep141:wrap.near",
    symbol: "wNEAR",
    chain: "near",
    decimals: 24
  };
  const destinationAsset = {
    id: "nep141:usdc.near",
    symbol: "USDC",
    chain: "near",
    decimals: 6
  };

  const quote = await provider.getDryQuote({
    originAsset,
    destinationAsset,
    amountAtomic: "1000000000000000000000000",
    recipient: "recipient.near",
    refundTo: "refund.near",
    slippageBps: 100
  });

  assert.equal(captured.url, "https://1click.chaindefuser.com/v0/quote");
  assert.equal(captured.options.headers["X-API-Key"], "server-only-secret");
  assert.equal(captured.body.dry, true);
  assert.equal(captured.body.swapType, "EXACT_INPUT");
  assert.equal(captured.body.amount, "1000000000000000000000000");
  assert.equal(captured.body.originAsset, originAsset.id);
  assert.equal(captured.body.destinationAsset, destinationAsset.id);
  assert.equal(captured.body.refundTo, "refund.near");
  assert.equal(captured.body.recipient, "recipient.near");
  assert.equal(quote.mode, "provider_dry_quote");
  assert.equal(quote.amountOutEstimate, "9.9");
  assert.equal(quote.minimumAmountOut, "9.8");
  assert.equal(quote.expiresAt, null);
  assert.equal(quote.depositAddress, null);
  assert.equal(quote.depositMemo, null);
});

test("provider errors are sanitized and never expose upstream bodies", async () => {
  const provider = createNearOneClickProvider({
    apiKey: "server-only-secret",
    fetchImpl: async () => jsonResponse({ message: "sensitive upstream detail" }, 429)
  });

  await assert.rejects(
    provider.getDryQuote({
      originAsset: { id: "a", symbol: "A", chain: "near", decimals: 0 },
      destinationAsset: { id: "b", symbol: "B", chain: "near", decimals: 0 },
      amountAtomic: "1",
      recipient: "recipient",
      refundTo: "refund"
    }),
    (error) => {
      assert.equal(error.code, "PROVIDER_RATE_LIMITED");
      assert.equal(error.status, 503);
      assert.doesNotMatch(error.message, /sensitive|secret/i);
      return true;
    }
  );
});

test("invalid catalogue and malformed provider quotes fail closed", async () => {
  const badCatalogue = createNearOneClickProvider({
    fetchImpl: async () => jsonResponse([{ ...tokenFixture, decimals: 40 }])
  });
  await assert.rejects(badCatalogue.getTokens(), { code: "PROVIDER_INVALID_CATALOGUE" });

  const malformedQuote = createNearOneClickProvider({
    apiKey: "configured",
    fetchImpl: async () => jsonResponse({
      correlationId: "quote-correlation-2",
      quote: { amountOut: "not-an-integer", depositAddress: "unexpected" }
    })
  });
  await assert.rejects(
    malformedQuote.getDryQuote({
      originAsset: { id: "a", symbol: "A", chain: "near", decimals: 0 },
      destinationAsset: { id: "b", symbol: "B", chain: "near", decimals: 0 },
      amountAtomic: "1",
      recipient: "recipient",
      refundTo: "refund"
    }),
    { code: "PROVIDER_INVALID_QUOTE" }
  );
});

test("dry quote refuses unexpected executable deposit data", async () => {
  const provider = createNearOneClickProvider({
    apiKey: "configured",
    fetchImpl: async () => jsonResponse({
      correlationId: "quote-correlation-3",
      quote: { amountOut: "10", depositAddress: "must-not-escape" }
    })
  });

  await assert.rejects(
    provider.getDryQuote({
      originAsset: { id: "a", symbol: "A", chain: "near", decimals: 0 },
      destinationAsset: { id: "b", symbol: "B", chain: "near", decimals: 0 },
      amountAtomic: "1",
      recipient: "recipient",
      refundTo: "refund"
    }),
    { code: "PROVIDER_UNEXPECTED_DEPOSIT_DATA" }
  );
});
