"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import type { ReactNode } from "react";
import { createProvider } from "./index";
import type {
  ChainFamily,
  ConnectionState,
  WalletAccount,
  WalletProvider,
} from "./types";

const STORAGE_KEY_FAMILY = "zonyx_wallet_family";

export interface WalletContextValue {
  account: WalletAccount | null;
  state: ConnectionState;
  error: string | null;
  provider: WalletProvider | null;
  isConnected: boolean;
  connect: (family: ChainFamily, providerName?: string) => Promise<WalletAccount>;
  disconnect: () => Promise<void>;
  switchChain: (chainId: string) => Promise<void>;
  signMessage: (message: string) => Promise<string>;
}

const WalletContext = createContext<WalletContextValue | null>(null);

export function useWallet(): WalletContextValue {
  const context = useContext(WalletContext);
  if (!context) {
    throw new Error("useWallet must be used within a WalletContextProvider");
  }
  return context;
}

export function WalletContextProvider({ children }: { children: ReactNode }) {
  const [account, setAccount] = useState<WalletAccount | null>(null);
  const [connectionState, setConnectionState] = useState<ConnectionState>("disconnected");
  const [error, setError] = useState<string | null>(null);
  const [activeProvider, setActiveProvider] = useState<WalletProvider | null>(null);

  const connect = useCallback(async (family: ChainFamily, providerName?: string): Promise<WalletAccount> => {
    setConnectionState("connecting");
    setError(null);

    try {
      const p = createProvider(family, providerName);
      setConnectionState("confirming");
      const acc = await p.connect();

      setAccount(acc);
      setActiveProvider(p);
      setConnectionState("connected");
      setError(null);

      if (typeof window !== "undefined") {
        try {
          window.localStorage.setItem(STORAGE_KEY_FAMILY, family);
        } catch {
          // Ignore localStorage errors (private mode, etc.)
        }
      }

      return acc;
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Failed to connect wallet.";
      setError(msg);
      setConnectionState("error");
      throw err;
    }
  }, []);

  const disconnect = useCallback(async (): Promise<void> => {
    try {
      if (activeProvider) {
        await activeProvider.disconnect();
      }
    } catch {
      // Ignore disconnect errors
    } finally {
      setAccount(null);
      setActiveProvider(null);
      setConnectionState("disconnected");
      setError(null);

      if (typeof window !== "undefined") {
        try {
          window.localStorage.removeItem(STORAGE_KEY_FAMILY);
        } catch {
          // Ignore
        }
      }
    }
  }, [activeProvider]);

  const switchChain = useCallback(async (chainId: string): Promise<void> => {
    if (!activeProvider || !activeProvider.switchChain) {
      throw new Error("Current wallet does not support network switching.");
    }
    setConnectionState("switching");
    try {
      await activeProvider.switchChain(chainId);
      if (account) {
        setAccount({ ...account, chainId });
      }
      setConnectionState("connected");
    } catch (err: unknown) {
      setConnectionState("connected");
      throw err;
    }
  }, [activeProvider, account]);

  const signMessage = useCallback(async (message: string): Promise<string> => {
    if (!activeProvider || !activeProvider.signMessage) {
      throw new Error("Current wallet does not support message signing.");
    }
    setConnectionState("confirming");
    try {
      const sig = await activeProvider.signMessage(message);
      setConnectionState("connected");
      return sig;
    } catch (err: unknown) {
      setConnectionState("connected");
      throw err;
    }
  }, [activeProvider]);

  // Silent auto-reconnect on mount for EVM if previously connected
  useEffect(() => {
    if (typeof window === "undefined") return;
    let savedFamily: string | null = null;
    try {
      savedFamily = window.localStorage.getItem(STORAGE_KEY_FAMILY);
    } catch {
      return;
    }

    if (savedFamily === "evm") {
      const eth = (window as unknown as { ethereum?: { request: (args: { method: string }) => Promise<string[]> } }).ethereum;
      if (eth && typeof eth.request === "function") {
        // eth_accounts checks authorization silently without opening a prompt
        eth
          .request({ method: "eth_accounts" })
          .then((accounts) => {
            if (Array.isArray(accounts) && accounts.length > 0) {
              void connect("evm", "Injected EVM");
            }
          })
          .catch(() => {
            // Ignore auto-connect failures
          });
      }
    }
  }, [connect]);

  const value = useMemo<WalletContextValue>(
    () => ({
      account,
      state: connectionState,
      error,
      provider: activeProvider,
      isConnected: connectionState === "connected" && account !== null,
      connect,
      disconnect,
      switchChain,
      signMessage,
    }),
    [account, connectionState, error, activeProvider, connect, disconnect, switchChain, signMessage]
  );

  return <WalletContext.Provider value={value}>{children}</WalletContext.Provider>;
}
