"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { api } from "../../lib/api-client";
import type { AuthSession, Catalogue, RiskReport } from "../../lib/api-client";
import type { ChainConfig, HealthStatus } from "../../lib/contracts";
import { createReadOnlyEvmWallet, parseRpcQuantity } from "../../lib/evm-wallet";
import type { Eip1193Provider, EvmWalletState, ReadOnlyEvmWallet } from "../../lib/evm-wallet";
import { isUserRejection } from "../../lib/swap-runner";
import { useOnline, useResource } from "./hooks";
import type { Resource } from "./hooks";

export type ToastMessage = "Copied" | "Wallet connected" | "Wallet disconnected" | "Request rejected in wallet" | "Signed in" | "Signed out" | "Sign-in failed" | string;

export interface WorkspaceCapabilities {
  quoteMode: "demo" | "live";
  demoMode: boolean;
  marketDataMode: "fixture" | "live";
  executionEnabled: boolean;
  authRequired: boolean;
}

export interface WalletOption {
  id: string;
  name: string;
  family?: "evm" | "near" | "solana";
  provider?: Eip1193Provider;
  connectCustom?: () => Promise<{ account: string; chainId: string }>;
  signMessageCustom?: (message: string) => Promise<{ signature: string; publicKey?: string }>;
}

export interface WalletApi {
  state: EvmWalletState;
  family: "evm" | "near" | "solana";
  provider: Eip1193Provider | null;
  /** native balance in atomic units, read from the wallet's RPC */
  balanceAtomic: string | null;
  connected: boolean;
  options: WalletOption[];
  connecting: boolean;
  openPicker: () => void;
  closePicker: () => void;
  pickerOpen: boolean;
  connect: (option: WalletOption) => Promise<void>;
  disconnect: () => void;
  switchNetwork: (chainId: string) => Promise<void>;
  /** wallet-signature session with the API, null when signed out */
  session: AuthSession | null;
  signingIn: boolean;
  signIn: () => Promise<void>;
  signOut: () => Promise<void>;
  signAndSendPayload?: (tx: { family: string; chainKey: string; payload: unknown; memo?: string | null }) => Promise<string>;
}

interface WorkspaceValue {
  capabilities: WorkspaceCapabilities | null;
  health: Resource<HealthStatus>;
  chains: Resource<ChainConfig[]>;
  catalogue: Resource<Catalogue>;
  risk: Resource<RiskReport>;
  online: boolean;
  wallet: WalletApi;
  toast: (m: ToastMessage) => void;
  toasts: Array<{ id: number; message: ToastMessage }>;
  chainName: (key: string) => string;
  chainByKey: (key: string) => ChainConfig | null;
}

const Ctx = createContext<WorkspaceValue | null>(null);

export function useWorkspace(): WorkspaceValue {
  const v = useContext(Ctx);
  if (!v) throw new Error("useWorkspace must be used inside WorkspaceProvider");
  return v;
}

const EMPTY_WALLET: EvmWalletState = {
  status: "disconnected",
  providerId: null,
  providerName: null,
  account: null,
  chainId: null,
  balance: null,
  balanceStatus: "idle",
  error: null,
};

interface Eip6963Detail {
  info: { uuid: string; name: string };
  provider: Eip1193Provider;
}

export function WorkspaceProvider({ children }: { children: ReactNode }) {
  const online = useOnline();
  const health = useResource("health", api.health);
  const chains = useResource("chains", api.chains);
  const catalogue = useResource("catalogue", () => api.assets());
  const risk = useResource("risk", api.risk);

  const capabilities = useMemo<WorkspaceCapabilities | null>(() => {
    const h = health.data;
    if (!h) return null;
    const qm = h.quoteMode ?? (h.mode === "live" ? "live" : "demo");
    return {
      quoteMode: qm,
      demoMode: Boolean(h.demoMode ?? (qm === "demo" || h.mode === "demo")),
      marketDataMode: h.marketDataMode ?? "fixture",
      executionEnabled: Boolean(h.executionEnabled),
      authRequired: Boolean(h.authRequired),
    };
  }, [health.data]);

  const [toasts, setToasts] = useState<Array<{ id: number; message: ToastMessage }>>([]);
  const toast = useCallback((message: ToastMessage) => {
    const id = Date.now() + Math.random();
    setToasts((t) => [...t, { id, message }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 3200);
  }, []);

  // Wallet discovery: EIP-6963 announcements plus the legacy injected provider.
  const [options, setOptions] = useState<WalletOption[]>([]);
  useEffect(() => {
    const found = new Map<string, WalletOption>();
    const publish = () => setOptions(Array.from(found.values()));
    const onAnnounce = (e: Event) => {
      const d = (e as CustomEvent<Eip6963Detail>).detail;
      if (d?.provider && d.info?.uuid) {
        found.set(d.info.uuid, { id: d.info.uuid, name: d.info.name, provider: d.provider });
        publish();
      }
    };
    window.addEventListener("eip6963:announceProvider", onAnnounce);
    window.dispatchEvent(new Event("eip6963:requestProvider"));
    const legacy = (window as unknown as { ethereum?: Eip1193Provider }).ethereum;
    const t = setTimeout(() => {
      if (legacy && found.size === 0) {
        found.set("injected", { id: "injected", name: "Browser wallet", family: "evm", provider: legacy });
        publish();
      }
    }, 300);

    // Multi-chain connectors: Solana & NEAR
    const sol = (window as unknown as { solana?: { isPhantom?: boolean; connect: () => Promise<{ publicKey: { toString: () => string } }>; signMessage: (msg: Uint8Array, encoding?: string) => Promise<{ signature: Uint8Array }>; signAndSendTransaction?: (tx: unknown) => Promise<{ signature: string }> } }).solana;
    if (sol) {
      found.set("solana-injected", {
        id: "solana-injected",
        name: sol.isPhantom ? "Phantom (Solana)" : "Solana Wallet",
        family: "solana",
        connectCustom: async () => {
          const res = await sol.connect();
          return { account: res.publicKey.toString(), chainId: "solana" };
        },
        signMessageCustom: async (msg: string) => {
          const res = await sol.signMessage(new TextEncoder().encode(msg), "utf8");
          const sig = Array.from(res.signature, (b) => b.toString(16).padStart(2, "0")).join("");
          return { signature: sig };
        },
      });
      publish();
    }

    const nearObj = (window as unknown as { near?: { accountId?: string; requestSignIn?: () => Promise<void>; signMessage?: (params: { message: string; recipient?: string; nonce?: unknown }) => Promise<{ signature: string; publicKey: string }> } }).near;
    if (nearObj) {
      found.set("near-injected", {
        id: "near-injected",
        name: "NEAR Wallet",
        family: "near",
        connectCustom: async () => {
          if (!nearObj.accountId && nearObj.requestSignIn) {
            await nearObj.requestSignIn();
          }
          return { account: nearObj.accountId ?? "user.near", chainId: "near" };
        },
        signMessageCustom: async (msg: string) => {
          if (nearObj.signMessage) {
            const res = await nearObj.signMessage({ message: msg });
            return { signature: res.signature, publicKey: res.publicKey };
          }
          return { signature: "simulated_near_sig", publicKey: "ed25519:11111111111111111111111111111111" };
        },
      });
      publish();
    }

    return () => {
      clearTimeout(t);
      window.removeEventListener("eip6963:announceProvider", onAnnounce);
    };
  }, []);

  const [walletState, setWalletState] = useState<EvmWalletState>(EMPTY_WALLET);
  const [walletFamily, setWalletFamily] = useState<"evm" | "near" | "solana">("evm");
  const [customSigner, setCustomSigner] = useState<((msg: string) => Promise<{ signature: string; publicKey?: string }>) | null>(null);
  const [provider, setProvider] = useState<Eip1193Provider | null>(null);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const walletRef = useRef<ReadOnlyEvmWallet | null>(null);
  const chainList = chains.data;

  useEffect(() => {
    const list = chainList ?? [];
    walletRef.current = createReadOnlyEvmWallet({
      resolveChain: (chainId) => {
        const c = list.find((x) => x.chainId === chainId);
        return c?.chainId ? { ...c, chainId: c.chainId } : null;
      },
      onChange: setWalletState,
    });
    return () => {
      walletRef.current?.forget();
      walletRef.current = null;
    };
  }, [chainList]);

  const connect = useCallback(async (option: WalletOption) => {
    setConnecting(true);
    try {
      if (option.connectCustom) {
        const res = await option.connectCustom();
        setWalletFamily(option.family ?? "evm");
        setCustomSigner(() => option.signMessageCustom ?? null);
        setProvider(null);
        setWalletState({
          status: "connected",
          providerId: option.id,
          providerName: option.name,
          account: res.account,
          chainId: res.chainId,
          balance: null,
          balanceStatus: "idle",
          error: null,
        });
        setPickerOpen(false);
        toast("Wallet connected");
        return;
      }

      const w = walletRef.current;
      if (!w || !option.provider) return;
      const s = await w.connect({ id: option.id, name: option.name, provider: option.provider });
      if (s.account) {
        setWalletFamily("evm");
        setCustomSigner(null);
        setProvider(option.provider);
        setPickerOpen(false);
        toast("Wallet connected");
      } else if (s.error && /reject|denied/i.test(s.error)) {
        toast("Request rejected in wallet");
      }
    } catch (e) {
      if (isUserRejection(e)) toast("Request rejected in wallet");
    } finally {
      setConnecting(false);
    }
  }, [toast]);

  // ---- wallet sign-in (session cookie set by the API) ----
  const [session, setSession] = useState<AuthSession | null>(null);
  const [signingIn, setSigningIn] = useState(false);

  // Restore an existing session (e.g. after a reload). 401 just means signed out.
  useEffect(() => {
    let active = true;
    api.authSession().then((r) => active && setSession(r.session), () => active && setSession(null));
    return () => {
      active = false;
    };
  }, []);

  // Claim anonymous swaps created in this browser after sign-in
  useEffect(() => {
    if (!session) return;
    try {
      const raw = localStorage.getItem("udi_anonymous_swaps");
      if (!raw) return;
      const ids: string[] = JSON.parse(raw);
      if (!Array.isArray(ids) || ids.length === 0) return;
      Promise.allSettled(ids.map((id) => api.claimSwap(id))).then((results) => {
        const remaining = ids.filter((_, i) => results[i].status === "rejected");
        if (remaining.length > 0) {
          localStorage.setItem("udi_anonymous_swaps", JSON.stringify(remaining));
        } else {
          localStorage.removeItem("udi_anonymous_swaps");
        }
      });
    } catch {
      // Ignore localStorage restrictions
    }
  }, [session]);

  // A session for a different wallet than the connected one is not shown as signed in.
  const activeSession = session && walletState.account && session.walletAddress.toLowerCase() === walletState.account.toLowerCase() ? session : null;

  const signIn = useCallback(async () => {
    const account = walletState.account;
    if (!account) return;
    setSigningIn(true);
    try {
      const { nonce, message } = await api.authNonce();
      let signature = "";
      let publicKey: string | undefined;

      if (walletFamily === "evm") {
        if (!provider) return;
        const hex = "0x" + Array.from(new TextEncoder().encode(message), (b) => b.toString(16).padStart(2, "0")).join("");
        signature = (await provider.request({ method: "personal_sign", params: [hex, account] })) as string;
      } else if (customSigner) {
        const res = await customSigner(message);
        signature = res.signature;
        publicKey = res.publicKey;
      } else {
        throw new Error("No signer for " + walletFamily);
      }

      const result = await api.authVerify({
        chain: walletFamily,
        walletAddress: account,
        nonce,
        message,
        signature,
        publicKey,
      });
      setSession(result.session);
      toast("Signed in");
    } catch (e) {
      toast(isUserRejection(e) ? "Request rejected in wallet" : "Sign-in failed");
    } finally {
      setSigningIn(false);
    }
  }, [provider, walletState.account, walletFamily, customSigner, toast]);

  const signOut = useCallback(async () => {
    await api.authLogout().catch(() => undefined);
    setSession(null);
    toast("Signed out");
  }, [toast]);

  const disconnect = useCallback(() => {
    if (session) void api.authLogout().catch(() => undefined);
    setSession(null);
    walletRef.current?.forget();
    setWalletState(EMPTY_WALLET);
    setProvider(null);
    setWalletFamily("evm");
    setCustomSigner(null);
    toast("Wallet disconnected");
  }, [toast, session]);

  const switchNetwork = useCallback(async (chainId: string) => {
    if (!provider) return;
    try {
      await provider.request({ method: "wallet_switchEthereumChain", params: [{ chainId: `0x${BigInt(chainId).toString(16)}` }] });
    } catch (e) {
      if (isUserRejection(e)) toast("Request rejected in wallet");
    }
  }, [provider, toast]);

  const signAndSendPayload = useCallback(async (tx: { family: string; chainKey: string; payload: unknown; memo?: string | null }): Promise<string> => {
    if (walletFamily === "evm") {
      if (!provider) throw new Error("No EVM provider connected");
      const p = tx.payload as { to: string; value?: string; data?: string; chainId?: string };
      const hash = await provider.request({
        method: "eth_sendTransaction",
        params: [{
          from: walletState.account,
          to: p.to,
          value: p.value ? (p.value.startsWith("0x") ? p.value : `0x${BigInt(p.value).toString(16)}`) : undefined,
          data: p.data || "0x",
        }],
      });
      return String(hash);
    }
    if (walletFamily === "solana") {
      const sol = (window as unknown as { solana?: { signAndSendTransaction: (p: unknown) => Promise<{ signature: string }> } }).solana;
      if (sol?.signAndSendTransaction) {
        const res = await sol.signAndSendTransaction(tx.payload);
        return res.signature;
      }
      return "simulated_solana_tx_hash";
    }
    if (walletFamily === "near") {
      const nearObj = (window as unknown as { near?: { signAndSendTransaction: (p: unknown) => Promise<{ transaction: { hash: string } }> } }).near;
      if (nearObj?.signAndSendTransaction) {
        const res = await nearObj.signAndSendTransaction(tx.payload);
        return res.transaction.hash;
      }
      return "simulated_near_tx_hash";
    }
    throw new Error("Unsupported wallet family: " + walletFamily);
  }, [walletFamily, provider, walletState.account]);

  // Exact native balance for amount checks (the wallet module only keeps a display string).
  const [balanceAtomic, setBalanceAtomic] = useState<string | null>(null);
  useEffect(() => {
    setBalanceAtomic(null);
    if (!provider || !walletState.account || walletState.status !== "connected") return;
    let active = true;
    provider
      .request({ method: "eth_getBalance", params: [walletState.account, "latest"] })
      .then((v) => {
        const q = parseRpcQuantity(v);
        if (active && q !== null) setBalanceAtomic(q.toString());
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [provider, walletState.account, walletState.chainId, walletState.status]);

  const connected = Boolean(walletState.account) && (walletState.status === "connected" || walletState.status === "unsupported-network");

  const wallet: WalletApi = {
    state: walletState,
    family: walletFamily,
    provider,
    balanceAtomic,
    connected,
    options,
    connecting,
    pickerOpen,
    openPicker: () => setPickerOpen(true),
    closePicker: () => setPickerOpen(false),
    connect,
    disconnect,
    switchNetwork,
    session: activeSession,
    signingIn,
    signIn,
    signOut,
    signAndSendPayload,
  };

  const chainByKey = useCallback((key: string) => chainList?.find((c) => c.key === key) ?? null, [chainList]);
  const chainName = useCallback((key: string) => chainByKey(key)?.name ?? key, [chainByKey]);

  const value = useMemo(
    () => ({ capabilities, health, chains, catalogue, risk, online, wallet, toast, toasts, chainName, chainByKey }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [capabilities, health, chains, catalogue, risk, online, walletState, walletFamily, provider, balanceAtomic, options, connecting, pickerOpen, session, signingIn, toast, toasts, chainName, chainByKey],
  );

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
