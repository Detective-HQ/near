import test from "node:test";
import assert from "node:assert/strict";
import { createAppServer } from "../server.mjs";
import { createRepository } from "../lib/repository.mjs";

async function withServer(options, run) {
  const server = createAppServer({
    demoMode: true,
    quoteMode: "demo",
    marketDataMode: "fixture",
    executionEnabled: false,
    ...options
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  try {
    await run(`http://127.0.0.1:${address.port}`);
  } finally {
    await new Promise((resolve, reject) => server.close((error) => error ? reject(error) : resolve()));
  }
}

async function getJson(base, path) {
  const response = await fetch(`${base}${path}`);
  return { response, body: await response.json() };
}

test("health and catalogue endpoints identify demo-only capability", async () => {
  await withServer({ repository: createRepository() }, async (base) => {
    const health = await getJson(base, "/api/v1/health");
    const chains = await getJson(base, "/api/v1/chains");
    const assets = await getJson(base, "/api/v1/assets");
    assert.equal(health.body.mode, "local-demo");
    assert.equal(health.body.marketDataMode, "fixture");
    assert.equal(chains.body.chains.every((chain) => chain.swapEnabled === false), true);
    assert.equal(assets.body.mode, "fixture");
    assert.match(assets.body.notice, /not live/i);
    assert.equal(health.response.headers.get("x-content-type-options"), "nosniff");
  });
});

test("live market mode serves the same cached read-only snapshot to Markets and Intelligence", async () => {
  const refreshOptions = [];
  const snapshot = {
    mode: "live",
    provider: "DeFiLlama",
    status: "partial",
    retrievedAt: "2026-11-10T12:00:00.000Z",
    notice: "Some sources are unavailable.",
    metrics: [{ key: "tvl", value: 100, source: "DeFiLlama", observedAt: null }],
    series: [{ date: "2026-11-10T00:00:00.000Z", tvl: 100 }]
  };
  const defiLlama = {
    async getNearSnapshot(options = {}) {
      refreshOptions.push(options);
      return snapshot;
    }
  };

  await withServer({
    repository: createRepository(),
    marketDataMode: "live",
    defiLlama
  }, async (base) => {
    const health = await getJson(base, "/api/v1/health");
    const markets = await getJson(base, "/api/v1/markets/overview?refresh=true");
    const intelligence = await getJson(base, "/api/v1/intelligence/near");

    assert.equal(health.body.marketDataMode, "live");
    assert.equal(markets.body.mode, "live");
    assert.equal(markets.body.metrics[0].source, "DeFiLlama");
    assert.equal(intelligence.body.status, "partial");
    assert.equal(refreshOptions[0].force, true);
    assert.equal(refreshOptions[1].force, undefined);
  });
});

test("quote, idempotent creation, and simulation lifecycle work through the API", async () => {
  await withServer({ repository: createRepository() }, async (base) => {
    const quoteResponse = await fetch(`${base}/api/v1/quotes`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        originAssetId: "demo:near:near",
        destinationAssetId: "demo:base:usdc",
        amountIn: "10"
      })
    });
    assert.equal(quoteResponse.status, 201);
    const { quote } = await quoteResponse.json();
    assert.equal(quote.mode, "simulation");

    const createOptions = {
      method: "POST",
      headers: { "content-type": "application/json", "idempotency-key": "simulation-key-001" },
      body: JSON.stringify({ quoteId: quote.id })
    };
    const firstResponse = await fetch(`${base}/api/v1/swaps`, createOptions);
    const first = await firstResponse.json();
    assert.equal(firstResponse.status, 201);
    assert.equal(first.swap.state, "QUOTE_READY");

    const duplicateResponse = await fetch(`${base}/api/v1/swaps`, createOptions);
    const duplicate = await duplicateResponse.json();
    assert.equal(duplicateResponse.status, 200);
    assert.equal(duplicate.created, false);
    assert.equal(duplicate.swap.id, first.swap.id);

    let state = first.swap;
    for (const expected of ["SIMULATED_DEPOSIT", "SIMULATED_PROCESSING", "SIMULATION_COMPLETE"]) {
      const result = await fetch(`${base}/api/v1/swaps/${state.id}/simulate`, { method: "POST" });
      state = (await result.json()).swap;
      assert.equal(state.state, expected);
      assert.equal(state.realSettlement, false);
    }
    assert.equal(state.transactions.source, null);
    assert.equal(state.providerStatus, null);
  });
});

test("malformed amounts and missing idempotency keys fail with typed errors", async () => {
  await withServer({ repository: createRepository() }, async (base) => {
    const badQuote = await fetch(`${base}/api/v1/quotes`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        originAssetId: "demo:near:near",
        destinationAssetId: "demo:base:usdc",
        amountIn: "1e4"
      })
    });
    assert.equal(badQuote.status, 400);
    assert.equal((await badQuote.json()).error.code, "INVALID_AMOUNT");

    const missingKey = await fetch(`${base}/api/v1/swaps`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ quoteId: "not-found" })
    });
    assert.equal(missingKey.status, 400);
    assert.equal((await missingKey.json()).error.code, "IDEMPOTENCY_KEY_REQUIRED");
  });
});

test("simulation routes fail closed when demo mode is disabled", async () => {
  await withServer({ repository: createRepository(), demoMode: false }, async (base) => {
    const response = await fetch(`${base}/api/v1/quotes`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({})
    });
    assert.equal(response.status, 503);
    assert.equal((await response.json()).error.code, "DEMO_DISABLED");
  });
});

test("live mode serves provider assets and only requests dry quotes", async () => {
  let receivedQuoteInput;
  const originAsset = {
    id: "nep141:eth.omft.near",
    providerAssetId: "nep141:eth.omft.near",
    symbol: "ETH",
    name: "Ether",
    chain: "ethereum",
    providerBlockchain: "eth",
    decimals: 18,
    contractAddress: null,
    source: "NEAR Intents 1Click token catalogue",
    enabledForLiveSwap: false
  };
  const destinationAsset = {
    id: "nep141:usdc.base.near",
    providerAssetId: "nep141:usdc.base.near",
    symbol: "USDC",
    name: "USD Coin",
    chain: "base",
    providerBlockchain: "base",
    decimals: 6,
    contractAddress: "0x2222222222222222222222222222222222222222",
    source: "NEAR Intents 1Click token catalogue",
    enabledForLiveSwap: false
  };
  const nearOneClick = {
    configured: true,
    async getTokens() {
      return [originAsset, destinationAsset];
    },
    async getDryQuote(input) {
      receivedQuoteInput = input;
      return {
        id: "qte_provider",
        mode: "provider_dry_quote",
        provider: "near_1click",
        providerQuoteId: "provider-reference",
        originAssetId: input.originAsset.id,
        destinationAssetId: input.destinationAsset.id,
        amountInAtomic: input.amountAtomic,
        amountIn: "1",
        amountOutEstimateAtomic: "9900000",
        amountOutEstimate: "9.9",
        minimumAmountOutAtomic: "9800000",
        minimumAmountOut: "9.8",
        slippageBps: input.slippageBps,
        expiresAt: null,
        depositAddress: null,
        depositMemo: null,
        warning: "Dry quote only."
      };
    }
  };

  await withServer({ repository: createRepository(), quoteMode: "live", nearOneClick }, async (base) => {
    const health = await getJson(base, "/api/v1/health");
    const catalogue = await getJson(base, "/api/v1/assets");
    assert.equal(health.body.quoteMode, "live");
    assert.equal(health.body.liveQuotesConfigured, true);
    assert.equal(health.body.executionEnabled, false);
    assert.equal(catalogue.body.mode, "provider_catalogue");
    assert.equal(catalogue.body.assets[0].id, "nep141:eth.omft.near");
    assert.equal(catalogue.body.assets[0].enabledForLiveSwap, false);

    const quoteResponse = await fetch(`${base}/api/v1/quotes`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        originAssetId: "nep141:eth.omft.near",
        destinationAssetId: "nep141:usdc.base.near",
        amountIn: "1.25",
        recipient: "0x3333333333333333333333333333333333333333",
        refundTo: "0x4444444444444444444444444444444444444444"
      })
    });
    const result = await quoteResponse.json();
    assert.equal(quoteResponse.status, 200);
    assert.equal(result.quote.mode, "provider_dry_quote");
    assert.equal(result.quote.depositAddress, null);
    assert.equal(receivedQuoteInput.amountAtomic, "1250000000000000000");
    assert.equal(receivedQuoteInput.slippageBps, 100);
    assert.equal(receivedQuoteInput.recipient, "0x3333333333333333333333333333333333333333");

    const createSwap = await fetch(`${base}/api/v1/swaps`, {
      method: "POST",
      headers: { "content-type": "application/json", "idempotency-key": "live-quote-key-01" },
      body: JSON.stringify({ quoteId: result.quote.id })
    });
    assert.equal(createSwap.status, 503);
    assert.equal((await createSwap.json()).error.code, "LIVE_EXECUTION_DISABLED");
  });
});

test("mocked EVM lifecycle preserves source and destination hashes and prevents status regression", async () => {
  const originAsset = {
    id: "nep141:eth.omft.near",
    providerAssetId: "nep141:eth.omft.near",
    symbol: "ETH",
    name: "Ether",
    chain: "ethereum",
    providerBlockchain: "eth",
    decimals: 18,
    contractAddress: null,
    source: "mock catalogue",
    enabledForLiveSwap: false
  };
  const destinationAsset = {
    id: "nep141:usdc.base.near",
    providerAssetId: "nep141:usdc.base.near",
    symbol: "USDC",
    name: "USD Coin",
    chain: "base",
    providerBlockchain: "base",
    decimals: 6,
    contractAddress: "0x2222222222222222222222222222222222222222",
    source: "mock catalogue",
    enabledForLiveSwap: false
  };
  const providerStatuses = ["PROCESSING", "KNOWN_DEPOSIT_TX", "SUCCESS"];
  let statusCalls = 0;
  let submittedHash = null;
  const nearOneClick = {
    configured: true,
    async getTokens() {
      return [originAsset, destinationAsset];
    },
    makeQuote(input, mode) {
      const executable = mode === "provider_executable_quote";
      return {
        id: executable ? "qte_mock_executable" : "qte_mock_dry",
        mode,
        provider: "near_1click",
        providerQuoteId: `mock-${mode}`,
        originAssetId: input.originAsset.id,
        destinationAssetId: input.destinationAsset.id,
        originAsset: {
          id: input.originAsset.id,
          symbol: input.originAsset.symbol,
          chain: input.originAsset.chain,
          decimals: input.originAsset.decimals,
          contractAddress: input.originAsset.contractAddress
        },
        destinationAsset: {
          id: input.destinationAsset.id,
          symbol: input.destinationAsset.symbol,
          chain: input.destinationAsset.chain,
          decimals: input.destinationAsset.decimals,
          contractAddress: input.destinationAsset.contractAddress
        },
        amountInAtomic: input.amountAtomic,
        amountIn: "1",
        amountOutEstimateAtomic: "3000000000",
        amountOutEstimate: "3000",
        minimumAmountOutAtomic: "2970000000",
        minimumAmountOut: "2970",
        slippageBps: input.slippageBps,
        expiresAt: executable ? new Date(Date.now() + 60_000).toISOString() : null,
        depositAddress: executable ? "0x5555555555555555555555555555555555555555" : null,
        depositMemo: null,
        warning: "Mock provider quote."
      };
    },
    async getDryQuote(input) {
      return this.makeQuote(input, "provider_dry_quote");
    },
    async getExecutableQuote(input) {
      return this.makeQuote(input, "provider_executable_quote");
    },
    async submitDepositTx({ txHash }) {
      submittedHash = txHash;
      return {
        providerStatus: "KNOWN_DEPOSIT_TX",
        updatedAt: new Date().toISOString(),
        swapDetails: { originChainTxHashes: [{ hash: txHash }] }
      };
    },
    async getStatus() {
      const providerStatus = providerStatuses[Math.min(statusCalls++, providerStatuses.length - 1)];
      return {
        providerStatus,
        updatedAt: new Date().toISOString(),
        swapDetails: {
          originChainTxHashes: submittedHash ? [{ hash: submittedHash }] : [],
          destinationChainTxHashes: providerStatus === "SUCCESS"
            ? [{
                hash: `0x${"b".repeat(64)}`,
                explorerUrl: `https://basescan.org/tx/${"b".repeat(64)}`
              }]
            : []
        }
      };
    }
  };

  await withServer({
    repository: createRepository(),
    quoteMode: "live",
    executionEnabled: true,
    nearOneClick,
    statusPollIntervalMs: 1_000
  }, async (base) => {
    const recipient = "0x3333333333333333333333333333333333333333";
    const refundTo = "0x4444444444444444444444444444444444444444";
    const quoteResponse = await fetch(`${base}/api/v1/quotes`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        originAssetId: originAsset.id,
        destinationAssetId: destinationAsset.id,
        amountIn: "1",
        recipient,
        refundTo
      })
    });
    assert.equal(quoteResponse.status, 200);
    const dryQuote = (await quoteResponse.json()).quote;

    const createResponse = await fetch(`${base}/api/v1/swaps`, {
      method: "POST",
      headers: { "content-type": "application/json", "idempotency-key": "mock-live-swap-001" },
      body: JSON.stringify({ quoteId: dryQuote.id, recipient, refundTo })
    });
    assert.equal(createResponse.status, 201);
    const created = (await createResponse.json()).swap;
    assert.equal(created.state, "AWAITING_USER_DEPOSIT");
    assert.equal(created.transactions.source, null);
    assert.equal(created.transactions.destination, null);

    const sourceHash = `0x${"a".repeat(64)}`;
    const depositResponse = await fetch(`${base}/api/v1/swaps/${created.id}/deposit`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ txHash: sourceHash, chainId: "1" })
    });
    assert.equal(depositResponse.status, 202);
    const deposit = (await depositResponse.json()).swap;
    assert.equal(deposit.state, "DEPOSIT_SUBMITTED");
    assert.equal(deposit.transactions.source, sourceHash);
    assert.equal(deposit.transactions.destination, null);
    assert.equal(submittedHash, sourceHash);

    const waitForState = async (expected, timeoutMs = 5_000) => {
      const deadline = Date.now() + timeoutMs;
      while (Date.now() < deadline) {
        const response = await fetch(`${base}/api/v1/swaps/${created.id}`);
        const current = (await response.json()).swap;
        if (current.state === expected) return current;
        await new Promise((resolve) => setTimeout(resolve, 25));
      }
      assert.fail(`Timed out waiting for provider state ${expected}`);
    };

    const processing = await waitForState("PROCESSING");
    assert.equal(processing.transactions.source, sourceHash);
    assert.equal(processing.transactions.destination, null);
    await new Promise((resolve) => setTimeout(resolve, 1_050));
    const afterRegressiveProviderStatus = (await getJson(base, `/api/v1/swaps/${created.id}`)).body.swap;
    assert.equal(afterRegressiveProviderStatus.state, "PROCESSING");

    const succeeded = await waitForState("SUCCEEDED");
    assert.equal(succeeded.realSettlement, true);
    assert.equal(succeeded.transactions.source, sourceHash);
    assert.equal(succeeded.transactions.destination, `0x${"b".repeat(64)}`);
    assert.notEqual(succeeded.transactions.source, succeeded.transactions.destination);
    assert.equal(succeeded.transactionLinks.destination, `https://basescan.org/tx/${"b".repeat(64)}`);
    assert.ok(statusCalls >= 3);

    const duplicateDeposit = await fetch(`${base}/api/v1/swaps/${created.id}/deposit`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ txHash: sourceHash, chainId: "1" })
    });
    assert.equal(duplicateDeposit.status, 200);
    assert.equal((await duplicateDeposit.json()).duplicate, true);
  });
});

test("live quote mode fails closed when provider credentials are missing", async () => {
  const nearOneClick = {
    configured: false,
    async getTokens() {
      return [];
    }
  };

  await withServer({ repository: createRepository(), quoteMode: "live", nearOneClick }, async (base) => {
    const response = await fetch(`${base}/api/v1/quotes`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({})
    });
    assert.equal(response.status, 503);
    assert.equal((await response.json()).error.code, "PROVIDER_NOT_CONFIGURED");
  });
});
