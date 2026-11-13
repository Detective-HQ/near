import test from "node:test";
import assert from "node:assert/strict";
import {
  createReadOnlyEvmWallet,
  formatNativeBalance,
  normalizeEvmAddress,
  normalizeEvmChainId,
  parseRpcQuantity,
  sendEvmDeposit,
  shortenEvmAddress
} from "../src/lib/evm-wallet.ts";

const validAddress = "0x1234567890AbCdEf1234567890aBCdef12345678";

class MockProvider {
  listeners = new Map();
  requests = [];
  chainId = "0x1";
  accounts = [validAddress];
  balance = "0xde0b6b3a7640000";
  sendResult = `0x${"a".repeat(64)}`;

  async request(request) {
    this.requests.push(request);
    if (request.method === "eth_requestAccounts") return this.accounts;
    if (request.method === "eth_accounts") return this.accounts;
    if (request.method === "eth_chainId") return this.chainId;
    if (request.method === "eth_getBalance") return this.balance;
    if (request.method === "eth_sendTransaction") return this.sendResult;
    throw new Error(`Unexpected method: ${request.method}`);
  }

  on(event, listener) {
    const listeners = this.listeners.get(event) ?? new Set();
    listeners.add(listener);
    this.listeners.set(event, listeners);
  }

  removeListener(event, listener) {
    this.listeners.get(event)?.delete(listener);
  }

  emit(event, value) {
    for (const listener of this.listeners.get(event) ?? []) listener(value);
  }
}

function supportedChain(chainId, options = {}) {
  return {
    chainId,
    family: "EVM",
    walletSupported: true,
    portfolioBalanceSupported: options.balanceSupported !== false,
    nativeCurrency: options.balanceSupported === false ? null : { symbol: "ETH", decimals: 18 }
  };
}

test("normalizes EVM accounts and quantity-encoded chain IDs without Number precision loss", () => {
  assert.equal(normalizeEvmAddress(validAddress), validAddress.toLowerCase());
  assert.equal(normalizeEvmAddress("0x1234"), null);
  assert.equal(normalizeEvmAddress(null), null);
  assert.equal(normalizeEvmChainId("0x1"), "1");
  assert.equal(normalizeEvmChainId("0x2105"), "8453");
  assert.equal(normalizeEvmChainId("0x20000000000001"), "9007199254740993");
  assert.equal(normalizeEvmChainId("0x01"), null);
  assert.equal(normalizeEvmChainId("0x"), null);
  assert.equal(parseRpcQuantity("0x0"), 0n);
  assert.equal(parseRpcQuantity("0x00"), null);
  assert.equal(shortenEvmAddress(validAddress), "0x1234…5678");
});

test("formats native balances exactly and makes truncation visible", () => {
  assert.equal(formatNativeBalance("0x0"), "0");
  assert.equal(formatNativeBalance("0xde0b6b3a7640000"), "1");
  assert.equal(formatNativeBalance("0x1"), "<0.000001");
  assert.equal(formatNativeBalance("0x11c37937e08000"), "0.005");
  assert.throws(() => formatNativeBalance("0x00"), /invalid balance quantity/i);
  assert.throws(() => formatNativeBalance("0x1", 37), /precision is invalid/i);
});

test("connects only on request and calls read-only account, chain, and balance methods", async () => {
  const provider = new MockProvider();
  const wallet = createReadOnlyEvmWallet({
    resolveChain: (chainId) => chainId === "1" ? supportedChain("1") : null
  });

  await wallet.connect({ id: "test-wallet", name: "Test Wallet", provider });
  const state = wallet.getState();
  assert.equal(state.status, "connected");
  assert.equal(state.account, validAddress.toLowerCase());
  assert.equal(state.chainId, "1");
  assert.equal(state.balance, "1");
  assert.equal(state.balanceStatus, "available");
  assert.deepEqual(provider.requests.map((request) => request.method), [
    "eth_requestAccounts",
    "eth_chainId",
    "eth_getBalance"
  ]);
});

test("manual balance refresh keeps the last value visible while loading and after a failed read", async () => {
  const provider = new MockProvider();
  const wallet = createReadOnlyEvmWallet({
    resolveChain: (chainId) => chainId === "1" ? supportedChain("1") : null
  });
  await wallet.connect({ id: "test-wallet", name: "Test Wallet", provider });

  const originalRequest = provider.request.bind(provider);
  let nextBalanceRead = "deferred";
  let finishBalanceRead;
  provider.request = async (request) => {
    if (request.method === "eth_getBalance" && nextBalanceRead === "deferred") {
      provider.requests.push(request);
      nextBalanceRead = "normal";
      return new Promise((resolve) => {
        finishBalanceRead = resolve;
      });
    }
    if (request.method === "eth_getBalance" && nextBalanceRead === "reject") {
      provider.requests.push(request);
      nextBalanceRead = "normal";
      throw Object.assign(new Error("sensitive provider detail"), { code: -32000 });
    }
    return originalRequest(request);
  };

  const refresh = wallet.refreshBalance();
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(wallet.getState().balance, "1");
  assert.equal(wallet.getState().balanceStatus, "loading");
  assert.equal(typeof finishBalanceRead, "function");
  finishBalanceRead("0x1bc16d674ec80000");
  await refresh;
  assert.equal(wallet.getState().balance, "2");
  assert.equal(wallet.getState().balanceStatus, "available");

  nextBalanceRead = "reject";
  await wallet.refreshBalance();
  assert.equal(wallet.getState().balance, "2");
  assert.equal(wallet.getState().balanceStatus, "error");
  assert.doesNotMatch(wallet.getState().error, /sensitive provider detail/i);
});

test("unknown chains do not trigger balance reads", async () => {
  const provider = new MockProvider();
  provider.chainId = "0x539";
  const wallet = createReadOnlyEvmWallet({
    resolveChain: (chainId) => chainId === "1" ? supportedChain("1") : null
  });

  await wallet.connect({ id: "test-wallet", name: "Test Wallet", provider });
  assert.equal(wallet.getState().status, "unsupported-network");
  assert.equal(wallet.getState().balanceStatus, "unsupported");
  assert.deepEqual(provider.requests.map((request) => request.method), [
    "eth_requestAccounts",
    "eth_chainId"
  ]);
});

test("wallet networks without verified native-currency metadata are connection-only", async () => {
  const provider = new MockProvider();
  provider.chainId = "0xa4ec";
  const wallet = createReadOnlyEvmWallet({
    resolveChain: (chainId) => chainId === "42220"
      ? supportedChain("42220", { balanceSupported: false })
      : null
  });

  await wallet.connect({ id: "celo-wallet", name: "Celo wallet", provider });
  assert.equal(wallet.getState().status, "connected");
  assert.equal(wallet.getState().balanceStatus, "unsupported");
  assert.deepEqual(provider.requests.map((request) => request.method), [
    "eth_requestAccounts",
    "eth_chainId"
  ]);
});

test("account and chain changes invalidate previous balances and refresh read-only state", async () => {
  const provider = new MockProvider();
  const wallet = createReadOnlyEvmWallet({
    resolveChain: (chainId) => chainId === "1" || chainId === "8453"
      ? supportedChain(chainId)
      : null
  });
  await wallet.connect({ id: "test-wallet", name: "Test Wallet", provider });

  provider.balance = "0x0";
  provider.emit("accountsChanged", ["0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]);
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(wallet.getState().account, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa");
  assert.equal(wallet.getState().balance, "0");

  provider.chainId = "0x2105";
  provider.emit("chainChanged", "0x2105");
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(wallet.getState().chainId, "8453");
  assert.equal(wallet.getState().balanceStatus, "available");
  assert.equal(provider.requests.at(-1).method, "eth_getBalance");

  provider.emit("accountsChanged", []);
  assert.equal(wallet.getState().account, null);
  assert.equal(wallet.getState().balance, null);
  assert.equal(wallet.getState().status, "account-unavailable");
});

test("forgetting a wallet clears local state and unsubscribes without a wallet RPC", async () => {
  const provider = new MockProvider();
  const wallet = createReadOnlyEvmWallet({
    resolveChain: (chainId) => chainId === "1" ? supportedChain("1") : null
  });
  await wallet.connect({ id: "test-wallet", name: "Test Wallet", provider });
  const requestCount = provider.requests.length;

  wallet.forget();
  assert.equal(wallet.getState().status, "disconnected");
  assert.equal(wallet.getState().account, null);
  assert.equal(provider.requests.length, requestCount);
  assert.equal([...provider.listeners.values()].every((listeners) => listeners.size === 0), true);
});

test("declined wallet permission is reported without exposing provider error text", async () => {
  const provider = new MockProvider();
  provider.request = async (request) => {
    provider.requests.push(request);
    throw Object.assign(new Error("private wallet payload"), { code: 4001 });
  };
  const wallet = createReadOnlyEvmWallet({
    resolveChain: () => supportedChain("1")
  });

  await wallet.connect({ id: "test-wallet", name: "Test Wallet", provider });
  assert.equal(wallet.getState().status, "error");
  assert.equal(wallet.getState().error, "Connection request was declined in your wallet.");
  assert.doesNotMatch(wallet.getState().error, /private wallet payload/i);
});

test("native EVM deposit builds an exact-value transaction after network and account checks", async () => {
  const provider = new MockProvider();
  const txHash = await sendEvmDeposit({
    provider,
    account: validAddress,
    chainId: "1",
    depositAddress: "0x5555555555555555555555555555555555555555",
    amountAtomic: "1000000000000000000"
  });

  assert.equal(txHash, provider.sendResult);
  assert.deepEqual(provider.requests.at(-1), {
    method: "eth_sendTransaction",
    params: [{
      from: validAddress.toLowerCase(),
      to: "0x5555555555555555555555555555555555555555",
      value: "0xde0b6b3a7640000"
    }]
  });
});

test("ERC-20 deposit encodes transfer to the provider address without native value", async () => {
  const provider = new MockProvider();
  await sendEvmDeposit({
    provider,
    account: validAddress,
    chainId: "1",
    depositAddress: "0x5555555555555555555555555555555555555555",
    tokenAddress: "0x6666666666666666666666666666666666666666",
    amountAtomic: "1000000"
  });

  assert.deepEqual(provider.requests.at(-1), {
    method: "eth_sendTransaction",
    params: [{
      from: validAddress.toLowerCase(),
      to: "0x6666666666666666666666666666666666666666",
      value: "0x0",
      data: `0xa9059cbb${"0".repeat(24)}${"5".repeat(40)}${"0".repeat(59)}f4240`
    }]
  });
});

test("deposit refuses a changed network or account before asking the wallet to send", async () => {
  const wrongNetwork = new MockProvider();
  wrongNetwork.chainId = "0x2105";
  await assert.rejects(
    sendEvmDeposit({
      provider: wrongNetwork,
      account: validAddress,
      chainId: "1",
      depositAddress: "0x5555555555555555555555555555555555555555",
      amountAtomic: "1"
    }),
    { code: "WRONG_NETWORK" }
  );
  assert.equal(wrongNetwork.requests.some((request) => request.method === "eth_sendTransaction"), false);

  const changedAccount = new MockProvider();
  changedAccount.accounts = ["0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"];
  await assert.rejects(
    sendEvmDeposit({
      provider: changedAccount,
      account: validAddress,
      chainId: "1",
      depositAddress: "0x5555555555555555555555555555555555555555",
      amountAtomic: "1"
    }),
    { code: "WALLET_ACCOUNT_CHANGED" }
  );
  assert.equal(changedAccount.requests.some((request) => request.method === "eth_sendTransaction"), false);
});

test("wallet rejection is surfaced as a typed non-broadcast failure", async () => {
  const provider = new MockProvider();
  provider.request = async (request) => {
    provider.requests.push(request);
    if (request.method === "eth_chainId") return "0x1";
    if (request.method === "eth_accounts") return [validAddress];
    if (request.method === "eth_sendTransaction") {
      throw Object.assign(new Error("private wallet detail"), { code: 4001 });
    }
    throw new Error(`Unexpected method: ${request.method}`);
  };

  await assert.rejects(
    sendEvmDeposit({
      provider,
      account: validAddress,
      chainId: "1",
      depositAddress: "0x5555555555555555555555555555555555555555",
      amountAtomic: "1"
    }),
    { code: "WALLET_REJECTED", message: "The wallet transaction was rejected. No deposit was recorded." }
  );
  assert.doesNotMatch(provider.requests.at(-1).params[0].from, /private wallet detail/);
});
