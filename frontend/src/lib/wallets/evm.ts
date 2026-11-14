import {
  formatNativeBalance,
  normalizeEvmAddress,
  normalizeEvmChainId,
  type Eip1193Provider,
} from "../evm-wallet";
import type { WalletAccount, WalletProvider } from "./types";

export const EVM_CHAINS: Record<string, { chainId: string; name: string }> = {
  "1": { chainId: "1", name: "Ethereum" },
  "8453": { chainId: "8453", name: "Base" },
  "42161": { chainId: "42161", name: "Arbitrum" },
  "137": { chainId: "137", name: "Polygon" },
  "43114": { chainId: "43114", name: "Avalanche" },
  "56": { chainId: "56", name: "BNB Chain" },
  "42220": { chainId: "42220", name: "Celo" },
};

function getInjectedProvider(): Eip1193Provider | null {
  if (typeof window === "undefined") return null;
  const eth = (window as unknown as { ethereum?: Eip1193Provider }).ethereum;
  return eth ?? null;
}

export function createEvmProvider(
  explicitProvider?: Eip1193Provider,
  providerName = "Browser Wallet"
): WalletProvider {
  let activeAccount: WalletAccount | null = null;

  const getProvider = (): Eip1193Provider => {
    const p = explicitProvider ?? getInjectedProvider();
    if (!p || typeof p.request !== "function") {
      throw new Error("No EVM wallet detected. Install MetaMask, Coinbase Wallet, or another EIP-1193 wallet.");
    }
    return p;
  };

  return {
    family: "evm",
    name: providerName,
    icon: "/images/wallets/evm.svg",

    async connect(): Promise<WalletAccount> {
      const p = getProvider();
      const accounts = (await p.request({
        method: "eth_requestAccounts",
      })) as string[];

      if (!Array.isArray(accounts) || accounts.length === 0) {
        throw new Error("No accounts returned from wallet.");
      }

      const address = normalizeEvmAddress(accounts[0]);
      if (!address) {
        throw new Error("Wallet returned an invalid EVM address.");
      }

      const rawChainId = await p.request({ method: "eth_chainId" });
      const chainId = normalizeEvmChainId(rawChainId) ?? "1";

      activeAccount = {
        address,
        chainFamily: "evm",
        chainId,
        label: EVM_CHAINS[chainId]?.name ?? `EVM (${chainId})`,
      };

      return activeAccount;
    },

    async disconnect(): Promise<void> {
      activeAccount = null;
    },

    async signMessage(msg: string): Promise<string> {
      const p = getProvider();
      if (!activeAccount) {
        throw new Error("Wallet must be connected before signing.");
      }
      const signature = (await p.request({
        method: "personal_sign",
        params: [msg, activeAccount.address],
      })) as string;
      return signature;
    },

    async signTransaction(tx: unknown): Promise<string> {
      const p = getProvider();
      const hash = (await p.request({
        method: "eth_sendTransaction",
        params: [tx],
      })) as string;
      return hash;
    },

    async switchChain(targetChainId: string): Promise<void> {
      const p = getProvider();
      const chainInfo = EVM_CHAINS[targetChainId];
      const chainName = chainInfo?.name ?? `Chain ${targetChainId}`;
      const hexChainId = `0x${BigInt(targetChainId).toString(16)}`;

      try {
        await p.request({
          method: "wallet_switchEthereumChain",
          params: [{ chainId: hexChainId }],
        });
        if (activeAccount) {
          activeAccount.chainId = targetChainId;
          activeAccount.label = chainName;
        }
      } catch (err: unknown) {
        const error = err as { code?: number; message?: string };
        // 4902 means the chain has not been added to MetaMask
        if (error?.code === 4902) {
          throw new Error(`Switch to ${chainName} to continue. Network is not configured in your wallet.`);
        }
        throw new Error(`Switch to ${chainName} to continue.`);
      }
    },

    async getBalance(address: string): Promise<string> {
      const p = getProvider();
      const rawBalance = await p.request({
        method: "eth_getBalance",
        params: [address, "latest"],
      });
      return formatNativeBalance(rawBalance);
    },
  };
}
