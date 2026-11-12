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
  quoteRequest: {
    dry: true,
    swapType: "EXACT_INPUT",
    slippageTolerance: 100,
    originAsset: "nep141:wrap.near",
    depositType: "ORIGIN_CHAIN",
    destinationAsset: "nep141:usdc.near",
    amount: "1000000000000000000000000",
    refundTo: "refund.near",
    refundType: "ORIGIN_CHAIN",
    recipient: "recipient.near",
    recipientType: "DESTINATION_CHAIN"
  },
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
    verifySignature: () => true,
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
  assert.equal(captured.options.headers.authorization, "Bearer server-only-secret");
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

test("executable quote checks the signed request and uses deadline as expiry, not timeWhenInactive", async () => {
  const now = Date.parse("2026-11-10T12:00:00.000Z");
  let captured;
  const provider = createNearOneClickProvider({
    apiKey: "server-only-secret",
    verifySignature: () => true,
    now: () => now,
    fetchImpl: async (url, options) => {
      captured = { url, options, body: JSON.parse(options.body) };
      return jsonResponse({
        correlationId: "executable-reference",
        timestamp: new Date(now).toISOString(),
        signature: "verified-by-test-double",
        quoteRequest: captured.body,
        quote: {
          amountOut: "9900000",
          amountOutFormatted: "9.9",
          minAmountOut: "9800000",
          minAmountOutFormatted: "9.8",
          depositAddress: "0x1234567890abcdef1234567890abcdef12345678",
          deadline: "2026-11-10T12:03:00.000Z",
          timeWhenInactive: "2026-11-10T12:01:00.000Z"
        }
      });
    }
  });
  const originAsset = {
    id: "eip155:1/erc20:0x1111111111111111111111111111111111111111",
    symbol: "USDC",
    chain: "ethereum",
    decimals: 6,
    contractAddress: "0x1111111111111111111111111111111111111111"
  };
  const destinationAsset = {
    id: "eip155:8453/erc20:0x2222222222222222222222222222222222222222",
    symbol: "USDC",
    chain: "base",
    decimals: 6,
    contractAddress: "0x2222222222222222222222222222222222222222"
  };
  const quote = await provider.getExecutableQuote({
    originAsset,
    destinationAsset,
    amountAtomic: "1000000",
    recipient: "0x3333333333333333333333333333333333333333",
    refundTo: "0x4444444444444444444444444444444444444444",
    slippageBps: 100,
    deadline: "2026-11-10T12:03:00.000Z",
    depositMode: "SIMPLE"
  });

  assert.equal(captured.url, "https://1click.chaindefuser.com/v0/quote");
  assert.equal(captured.body.dry, false);
  assert.equal(captured.body.depositMode, "SIMPLE");
  assert.equal(quote.mode, "provider_executable_quote");
  assert.equal(quote.depositAddress, "0x1234567890abcdef1234567890abcdef12345678");
  assert.equal(quote.expiresAt, "2026-11-10T12:03:00.000Z");
  assert.equal(quote.timeWhenInactive, "2026-11-10T12:01:00.000Z");
  assert.equal(quote.originAsset.contractAddress, originAsset.contractAddress);
});

test("executable quote fails closed for a mismatched request, invalid signature, or expired deadline", async (t) => {
  const now = Date.parse("2026-11-10T12:00:00.000Z");
  const originAsset = { id: "asset-in", symbol: "IN", chain: "ethereum", decimals: 18 };
  const destinationAsset = { id: "asset-out", symbol: "OUT", chain: "base", decimals: 6 };
  const expectedRequest = {
    dry: false,
    swapType: "EXACT_INPUT",
    slippageTolerance: 100,
    originAsset: originAsset.id,
    depositType: "ORIGIN_CHAIN",
    destinationAsset: destinationAsset.id,
    amount: "1000000000000000000",
    refundTo: "0x4444444444444444444444444444444444444444",
    refundType: "ORIGIN_CHAIN",
    recipient: "0x3333333333333333333333333333333333333333",
    recipientType: "DESTINATION_CHAIN",
    deadline: "2026-11-10T12:03:00.000Z",
    depositMode: "SIMPLE"
  };
  const basePayload = {
    correlationId: "executable-reference",
    timestamp: new Date(now).toISOString(),
    signature: "signature",
    quoteRequest: expectedRequest,
    quote: {
      amountOut: "9900000",
      minAmountOut: "9800000",
      depositAddress: "0x1234567890abcdef1234567890abcdef12345678",
      deadline: expectedRequest.deadline
    }
  };
  const run = (payload, verifySignature = () => true) => {
    const provider = createNearOneClickProvider({
      apiKey: "server-only-secret",
      verifySignature,
      now: () => now,
      fetchImpl: async () => jsonResponse(payload)
    });
    return provider.getExecutableQuote({
      originAsset,
      destinationAsset,
      amountAtomic: expectedRequest.amount,
      recipient: expectedRequest.recipient,
      refundTo: expectedRequest.refundTo,
      slippageBps: expectedRequest.slippageTolerance,
      deadline: expectedRequest.deadline,
      depositMode: expectedRequest.depositMode
    });
  };

  await t.test("does not accept a signed quote for different request inputs", async () => {
    await assert.rejects(
      run({ ...basePayload, quoteRequest: { ...expectedRequest, amount: "2" } }),
      { code: "PROVIDER_QUOTE_MISMATCH" }
    );
  });
  await t.test("does not accept an invalid provider signature", async () => {
    await assert.rejects(run(basePayload, () => false), { code: "PROVIDER_SIGNATURE_INVALID" });
  });
  await t.test("does not issue deposit instructions without a future deadline", async () => {
    await assert.rejects(
      run({ ...basePayload, quote: { ...basePayload.quote, deadline: "2026-11-10T11:59:59.000Z" } }),
      { code: "PROVIDER_INVALID_DEADLINE" }
    );
  });
});

test("status lookup and deposit notification use the provider's address, memo, and transaction fields", async () => {
  const calls = [];
  const provider = createNearOneClickProvider({
    apiKey: "server-only-secret",
    fetchImpl: async (url, options) => {
      calls.push({ url: new URL(url), options });
      if (new URL(url).pathname.endsWith("/status")) {
        return jsonResponse({
          correlationId: "status-reference",
          status: "PROCESSING",
          updatedAt: "2026-11-10T12:02:00.000Z",
          swapDetails: { destinationChainTxHashes: [] }
        });
      }
      return jsonResponse({
        correlationId: "submit-reference",
        status: "KNOWN_DEPOSIT_TX",
        updatedAt: "2026-11-10T12:01:00.000Z",
        swapDetails: {}
      });
    }
  });
  const status = await provider.getStatus({
    depositAddress: "0x1234567890abcdef1234567890abcdef12345678",
    depositMemo: "memo / 1"
  });
  const acknowledgement = await provider.submitDepositTx({
    txHash: `0x${"a".repeat(64)}`,
    depositAddress: "0x1234567890abcdef1234567890abcdef12345678"
  });

  assert.equal(status.providerStatus, "PROCESSING");
  assert.equal(calls[0].url.searchParams.get("depositAddress"), "0x1234567890abcdef1234567890abcdef12345678");
  assert.equal(calls[0].url.searchParams.get("depositMemo"), "memo / 1");
  assert.equal(calls[0].options.headers.authorization, "Bearer server-only-secret");
  assert.equal(calls[1].url.pathname, "/v0/deposit/submit");
  assert.equal(JSON.parse(calls[1].options.body).txHash, `0x${"a".repeat(64)}`);
  assert.equal(acknowledgement.providerStatus, "KNOWN_DEPOSIT_TX");
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
    verifySignature: () => true,
    fetchImpl: async () => jsonResponse({
      correlationId: "quote-correlation-3",
      timestamp: new Date().toISOString(),
      signature: "verified-by-test-double",
      quoteRequest: {
        dry: true,
        swapType: "EXACT_INPUT",
        slippageTolerance: 100,
        originAsset: "a",
        depositType: "ORIGIN_CHAIN",
        destinationAsset: "b",
        amount: "1",
        refundTo: "refund",
        refundType: "ORIGIN_CHAIN",
        recipient: "recipient",
        recipientType: "DESTINATION_CHAIN"
      },
      quote: {
        amountOut: "10",
        minAmountOut: "9",
        depositAddress: "must-not-escape"
      }
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
