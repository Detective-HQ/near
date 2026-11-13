const EVM_ADDRESS_PATTERN = /^0x[a-f0-9]{40}$/i;
const RPC_QUANTITY_PATTERN = /^0x(?:0|[1-9a-f][0-9a-f]*)$/i;

export interface Eip1193Provider {
  request(args: { method: string; params?: unknown[] }): Promise<unknown>;
  on(event: string, listener: (value: unknown) => void): void;
  removeListener?(event: string, listener: (value: unknown) => void): void;
  off?(event: string, listener: (value: unknown) => void): void;
  isMetaMask?: boolean;
  isCoinbaseWallet?: boolean;
  isBraveWallet?: boolean;
  [key: string]: unknown;
}

export interface EvmDepositRequest {
  provider: Eip1193Provider;
  account: string;
  chainId: string;
  depositAddress: string;
  amountAtomic: string;
  tokenAddress?: string | null;
  depositMemo?: string | null;
}

export interface EvmWalletChain {
  chainId: string;
  family: string;
  walletSupported: boolean;
  portfolioBalanceSupported: boolean;
  nativeCurrency: { symbol: string; decimals: number } | null;
}

export interface EvmWalletState {
  status: "disconnected" | "connecting" | "connected" | "unsupported-network" | "account-unavailable" | "error";
  providerId: string | null;
  providerName: string | null;
  account: string | null;
  chainId: string | null;
  balance: string | null;
  balanceStatus: "idle" | "loading" | "available" | "unsupported" | "error";
  error: string | null;
}

interface WalletEntry {
  id?: string;
  name?: string;
  provider: Eip1193Provider;
}

interface ReadOnlyWalletOptions {
  resolveChain: (chainId: string) => EvmWalletChain | null | undefined;
  onChange?: (state: EvmWalletState) => void;
  requestTimeoutMs?: number;
}

export interface ReadOnlyEvmWallet {
  getState: () => EvmWalletState;
  connect: (entry: WalletEntry) => Promise<EvmWalletState>;
  refreshBalance: () => Promise<EvmWalletState>;
  forget: () => void;
}

export function normalizeEvmAddress(value: unknown): string | null {
  return typeof value === "string" && EVM_ADDRESS_PATTERN.test(value)
    ? value.toLowerCase()
    : null;
}

export function normalizeEvmChainId(value: unknown): string | null {
  const quantity = parseRpcQuantity(value);
  return quantity === null || quantity === 0n ? null : quantity.toString();
}

export function parseRpcQuantity(value: unknown): bigint | null {
  if (
    typeof value !== "string"
    || value.length > 66
    || !RPC_QUANTITY_PATTERN.test(value)
  ) {
    return null;
  }
  try {
    return BigInt(value);
  } catch {
    return null;
  }
}

export function formatNativeBalance(value: unknown, decimals = 18, maxFractionDigits = 6): string {
  const atomic = parseRpcQuantity(value);
  if (atomic === null) throw new TypeError("Wallet returned an invalid balance quantity.");
  if (
    !Number.isInteger(decimals)
    || decimals < 0
    || decimals > 36
    || !Number.isInteger(maxFractionDigits)
    || maxFractionDigits < 0
  ) {
    throw new TypeError("Native currency precision is invalid.");
  }
  if (atomic === 0n || decimals === 0) return atomic.toString();

  const scale = 10n ** BigInt(decimals);
  const whole = atomic / scale;
  const fraction = (atomic % scale).toString().padStart(decimals, "0");
  const precision = Math.min(decimals, maxFractionDigits);
  const visibleFraction = fraction.slice(0, precision);
  const trimmedFraction = visibleFraction.replace(/0+$/, "");
  const hiddenDigits = fraction.slice(precision);

  if (whole === 0n && !trimmedFraction && /[1-9]/.test(hiddenDigits)) {
    return `<0.${"0".repeat(Math.max(0, precision - 1))}1`;
  }

  const formatted = trimmedFraction ? `${whole}.${trimmedFraction}` : whole.toString();
  return /[1-9]/.test(hiddenDigits) ? `${formatted}…` : formatted;
}

export function shortenEvmAddress(value: unknown): string {
  const address = normalizeEvmAddress(value);
  return address ? `${address.slice(0, 6)}…${address.slice(-4)}` : "Address unavailable";
}

export async function sendEvmDeposit({
  provider,
  account,
  chainId,
  depositAddress,
  amountAtomic,
  tokenAddress = null,
  depositMemo = null
}: EvmDepositRequest): Promise<string> {
  const normalizedAccount = normalizeEvmAddress(account);
  const normalizedDeposit = normalizeEvmAddress(depositAddress);
  const normalizedToken = tokenAddress === null ? null : normalizeEvmAddress(tokenAddress);
  if (!provider || typeof provider.request !== "function") {
    throw new TypeError("An injected EVM wallet is required.");
  }
  if (!normalizedAccount || !normalizedDeposit || (tokenAddress !== null && !normalizedToken)) {
    throw new TypeError("The wallet, deposit or token address is invalid.");
  }
  if (typeof depositMemo === "string" && depositMemo.length > 0) {
    throw new TypeError("This EVM transaction path cannot attach a provider memo.");
  }
  if (!/^\d+$/.test(amountAtomic) || BigInt(amountAtomic) <= 0n || BigInt(amountAtomic) >= 1n << 256n) {
    throw new TypeError("The deposit amount must be a positive EVM-sized integer.");
  }
  if (!/^\d+$/.test(chainId) || BigInt(chainId) <= 0n) {
    throw new TypeError("The expected EVM network identifier is invalid.");
  }

  const currentChainId = normalizeEvmChainId(await provider.request({ method: "eth_chainId" }));
  if (currentChainId !== chainId) {
    throw Object.assign(new Error("Switch the wallet to the quoted source network before sending."), {
      code: "WRONG_NETWORK"
    });
  }
  const accounts = await provider.request({ method: "eth_accounts" });
  const currentAccount = Array.isArray(accounts) ? normalizeEvmAddress(accounts[0]) : null;
  if (!currentAccount || currentAccount !== normalizedAccount) {
    throw Object.assign(new Error("The selected wallet account changed. Review the swap again."), {
      code: "WALLET_ACCOUNT_CHANGED"
    });
  }

  const transaction: Record<string, string> = {
    from: normalizedAccount,
    to: normalizedToken ?? normalizedDeposit,
    value: normalizedToken ? "0x0" : `0x${BigInt(amountAtomic).toString(16)}`
  };
  if (normalizedToken) {
    const paddedAddress = normalizedDeposit.slice(2).padStart(64, "0");
    const paddedAmount = BigInt(amountAtomic).toString(16).padStart(64, "0");
    transaction.data = `0xa9059cbb${paddedAddress}${paddedAmount}`;
  }

  let result: unknown;
  try {
    result = await provider.request({
      method: "eth_sendTransaction",
      params: [transaction]
    });
  } catch (error) {
    const walletError = error as { code?: string | number };
    if (walletError?.code === 4001) {
      throw Object.assign(new Error("The wallet transaction was rejected. No deposit was recorded."), {
        code: "WALLET_REJECTED"
      });
    }
    throw Object.assign(new Error("The wallet could not submit the deposit transaction."), {
      code: "WALLET_SEND_FAILED"
    });
  }

  if (typeof result !== "string" || !/^0x[a-f0-9]{64}$/i.test(result)) {
    throw new TypeError("The wallet returned an invalid transaction hash.");
  }
  return result;
}

function providerErrorMessage(error: unknown): string {
  const walletError = error as { code?: string | number; name?: string };
  if (walletError?.code === 4001) return "Connection request was declined in your wallet.";
  if (walletError?.code === 4100) return "This site is not authorized in the selected wallet.";
  if (walletError?.code === 4900 || walletError?.code === 4901) {
    return "The selected wallet is disconnected from this network.";
  }
  if (walletError?.code === "WALLET_TIMEOUT") return "The wallet did not respond. Check the extension and try again.";
  return "The wallet request failed. Check the selected wallet and try again.";
}

function hasErrorCode(error: unknown, code: string): boolean {
  return typeof error === "object"
    && error !== null
    && "code" in error
    && error.code === code;
}

export function createReadOnlyEvmWallet({
  resolveChain,
  onChange = () => {},
  requestTimeoutMs = 15_000
}: ReadOnlyWalletOptions): ReadOnlyEvmWallet {
  let provider: Eip1193Provider | null = null;
  let listeners: Array<{
    event: string;
    listener: (value: unknown) => void;
    remove: (event: string, listener: (value: unknown) => void) => void;
  }> = [];
  let sessionVersion = 0;
  let balanceRequestVersion = 0;
  let accountEventVersion = 0;
  let chainEventVersion = 0;
  let state: EvmWalletState = {
    status: "disconnected",
    providerId: null,
    providerName: null,
    account: null,
    chainId: null,
    balance: null,
    balanceStatus: "idle",
    error: null
  };

  const snapshot = (): EvmWalletState => ({ ...state });
  const publish = (update: Partial<EvmWalletState>) => {
    state = { ...state, ...update };
    onChange(snapshot());
  };
  const clearListeners = () => {
    for (const { event, listener, remove } of listeners) remove.call(provider, event, listener);
    listeners = [];
  };
  const request = async (
    target: Eip1193Provider | null,
    method: string,
    params?: unknown[]
  ): Promise<unknown> => {
    if (!target || typeof target.request !== "function") {
      throw Object.assign(new Error("Unsupported wallet provider."), { code: "UNSUPPORTED_WALLET" });
    }
    let timer;
    const timeout = new Promise((resolve, reject) => {
      timer = setTimeout(() => reject(Object.assign(new Error("Wallet timeout."), { code: "WALLET_TIMEOUT" })), requestTimeoutMs);
    });
    try {
      const requestOptions = params === undefined ? { method } : { method, params };
      return await Promise.race([
        Promise.resolve().then(() => target.request(requestOptions)),
        timeout
      ]);
    } finally {
      clearTimeout(timer);
    }
  };
  const resolveCurrentChain = (chainId: string | null): EvmWalletChain | null => {
    if (!chainId || typeof resolveChain !== "function") return null;
    try {
      const chain = resolveChain(chainId);
      return chain?.family === "EVM" && chain.walletSupported ? chain : null;
    } catch {
      return null;
    }
  };

  async function refreshBalance(): Promise<EvmWalletState> {
    const activeProvider = provider;
    const activeSession = sessionVersion;
    const activeAccount = state.account;
    const activeChainId = state.chainId;
    const chain = resolveCurrentChain(activeChainId);
    if (!activeProvider || !activeAccount || !chain) {
      publish({ balance: null, balanceStatus: "unsupported", error: null });
      return snapshot();
    }
    if (
      chain.portfolioBalanceSupported !== true
      || !chain.nativeCurrency
      || typeof chain.nativeCurrency.symbol !== "string"
      || !Number.isInteger(chain.nativeCurrency.decimals)
    ) {
      publish({ balance: null, balanceStatus: "unsupported", error: null });
      return snapshot();
    }

    const requestVersion = ++balanceRequestVersion;
    publish({ balanceStatus: "loading", error: null });
    try {
      const result = await request(activeProvider, "eth_getBalance", [activeAccount, "latest"]);
      const formatted = formatNativeBalance(result, chain.nativeCurrency.decimals);
      if (
        requestVersion !== balanceRequestVersion
        || activeSession !== sessionVersion
        || activeProvider !== provider
        || activeAccount !== state.account
        || activeChainId !== state.chainId
      ) {
        return snapshot();
      }
      publish({ balance: formatted, balanceStatus: "available", error: null });
    } catch (error) {
      if (
        requestVersion === balanceRequestVersion
        && activeSession === sessionVersion
        && activeProvider === provider
        && activeAccount === state.account
        && activeChainId === state.chainId
      ) {
        publish({
          balanceStatus: "error",
          error: providerErrorMessage(error)
        });
      }
    }
    return snapshot();
  }

  function resetSession(
    status: EvmWalletState["status"] = "disconnected",
    error: string | null = null
  ) {
    sessionVersion += 1;
    balanceRequestVersion += 1;
    clearListeners();
    provider = null;
    publish({
      status,
      providerId: null,
      providerName: null,
      account: null,
      chainId: null,
      balance: null,
      balanceStatus: "idle",
      error
    });
  }

  function bind(
    target: Eip1193Provider,
    event: string,
    listener: (value: unknown) => void
  ) {
    target.on(event, listener);
    listeners.push({
      event,
      listener,
      remove: (typeof target.removeListener === "function"
        ? target.removeListener
        : target.off)!.bind(target)
    });
  }

  async function connect(entry: WalletEntry): Promise<EvmWalletState> {
    const target = entry?.provider;
    if (
      !target
      || typeof target.request !== "function"
      || typeof target.on !== "function"
      || (typeof target.removeListener !== "function" && typeof target.off !== "function")
    ) {
      publish({
        status: "error",
        error: "This wallet does not expose the required account and network change events."
      });
      return snapshot();
    }

    resetSession();
    const activeSession = sessionVersion;
    provider = target;
    accountEventVersion = 0;
    chainEventVersion = 0;
    publish({
      status: "connecting",
      providerId: String(entry.id ?? "").slice(0, 100),
      providerName: String(entry.name ?? "Injected EVM wallet").slice(0, 80),
      error: null
    });

    bind(target, "accountsChanged", (accounts) => {
      if (activeSession !== sessionVersion) return;
      accountEventVersion += 1;
      const account = Array.isArray(accounts) ? normalizeEvmAddress(accounts[0]) : null;
      balanceRequestVersion += 1;
      if (!account) {
        publish({
          status: "account-unavailable",
          account: null,
          balance: null,
          balanceStatus: "idle",
          error: null
        });
        return;
      }
      publish({
        status: resolveCurrentChain(state.chainId) ? "connected" : "unsupported-network",
        account,
        balance: null,
        balanceStatus: "idle",
        error: null
      });
      void refreshBalance();
    });

    bind(target, "chainChanged", (rawChainId) => {
      if (activeSession !== sessionVersion) return;
      chainEventVersion += 1;
      balanceRequestVersion += 1;
      const chainId = normalizeEvmChainId(rawChainId);
      publish({
        status: chainId && resolveCurrentChain(chainId) ? "connected" : "unsupported-network",
        chainId,
        balance: null,
        balanceStatus: chainId ? "idle" : "unsupported",
        error: null
      });
      if (chainId && state.account) void refreshBalance();
    });

    bind(target, "disconnect", () => {
      if (activeSession === sessionVersion) {
        resetSession("disconnected", "The selected wallet disconnected.");
      }
    });

    const accountVersionBeforeRequest = accountEventVersion;
    const chainVersionBeforeRequest = chainEventVersion;
    try {
      const accounts = await request(target, "eth_requestAccounts");
      if (activeSession !== sessionVersion) return snapshot();
      if (!Array.isArray(accounts)) throw new TypeError("Invalid account response.");
      if (accountVersionBeforeRequest === accountEventVersion) {
        const account = normalizeEvmAddress(accounts[0]);
        if (!account) throw new TypeError("No valid EVM account was returned.");
        publish({ account });
      }

      const rawChainId = await request(target, "eth_chainId");
      if (activeSession !== sessionVersion) return snapshot();
      if (chainVersionBeforeRequest === chainEventVersion) {
        const chainId = normalizeEvmChainId(rawChainId);
        if (!chainId) throw new TypeError("The wallet returned an invalid network identifier.");
        publish({ chainId });
      }

      const supported = resolveCurrentChain(state.chainId);
      publish({
        status: supported ? "connected" : "unsupported-network",
        balance: null,
        balanceStatus: supported ? "idle" : "unsupported",
        error: null
      });
      await refreshBalance();
    } catch (error) {
      if (activeSession === sessionVersion) {
        resetSession("error", hasErrorCode(error, "UNSUPPORTED_WALLET")
          ? "This wallet does not expose the required read-only request method."
          : providerErrorMessage(error));
      }
    }
    return snapshot();
  }

  return {
    getState: snapshot,
    connect,
    refreshBalance,
    forget() {
      resetSession();
    }
  };
}
