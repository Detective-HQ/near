"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { api } from "../../lib/api-client";
import type { Catalogue, RiskReport } from "../../lib/api-client";
import type { ChainConfig } from "../../lib/contracts";
import { createReadOnlyEvmWallet, parseRpcQuantity } from "../../lib/evm-wallet";
import type { Eip1193Provider, EvmWalletState, ReadOnlyEvmWallet } from "../../lib/evm-wallet";
import { isUserRejection } from "../../lib/swap-runner";
import { useOnline, useResource } from "./hooks";
import type { Resource } from "./hooks";

export type ToastMessage = "Copied" | "Wallet connected" | "Wallet disconnected" | "Request rejected in wallet";

export interface WalletOption {
  id: string;
  name: string;
  provider: Eip1193Provider;
}

export interface WalletApi {
  state: EvmWalletState;
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
}

interface WorkspaceValue {
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
  const chains = useResource("chains", api.chains);
  const catalogue = useResource("catalogue", () => api.assets());
  const risk = useResource("risk", api.risk);

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
        found.set("injected", { id: "injected", name: "Browser wallet", provider: legacy });
        publish();
      }
    }, 300);
    return () => {
      clearTimeout(t);
      window.removeEventListener("eip6963:announceProvider", onAnnounce);
    };
  }, []);

  const [walletState, setWalletState] = useState<EvmWalletState>(EMPTY_WALLET);
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
    const w = walletRef.current;
    if (!w) return;
    setConnecting(true);
    try {
      const s = await w.connect({ id: option.id, name: option.name, provider: option.provider });
      if (s.account) {
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

  const disconnect = useCallback(() => {
    walletRef.current?.forget();
    setWalletState(EMPTY_WALLET);
    setProvider(null);
    toast("Wallet disconnected");
  }, [toast]);

  const switchNetwork = useCallback(async (chainId: string) => {
    if (!provider) return;
    try {
      await provider.request({ method: "wallet_switchEthereumChain", params: [{ chainId: `0x${BigInt(chainId).toString(16)}` }] });
    } catch (e) {
      if (isUserRejection(e)) toast("Request rejected in wallet");
    }
  }, [provider, toast]);

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
  };

  const chainByKey = useCallback((key: string) => chainList?.find((c) => c.key === key) ?? null, [chainList]);
  const chainName = useCallback((key: string) => chainByKey(key)?.name ?? key, [chainByKey]);

  const value = useMemo(
    () => ({ chains, catalogue, risk, online, wallet, toast, toasts, chainName, chainByKey }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [chains, catalogue, risk, online, walletState, provider, balanceAtomic, options, connecting, pickerOpen, toast, toasts, chainName, chainByKey],
  );

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
