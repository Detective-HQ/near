import type { WalletAccount, WalletProvider } from "./types";

/**
 * Solana wallet adapter stub.
 *
 * Production integration requires:
 * - `@solana/wallet-adapter-base`
 * - `@solana/wallet-adapter-react`
 * - `@solana/wallet-adapter-wallets` (Phantom, Solflare, etc.)
 * - `@solana/web3.js` for SPL token and SOL balance/transfer encoding.
 */
export function createSolanaProvider(providerName = "Phantom / Solflare"): WalletProvider {
  return {
    family: "solana",
    name: providerName,
    icon: "/images/wallets/solana.svg",

    async connect(): Promise<WalletAccount> {
      throw new Error(
        "Solana wallet support requires installing @solana/wallet-adapter. Please install the adapter package to connect Solana wallets."
      );
    },

    async disconnect(): Promise<void> {
      // No-op for stub
    },

    async signMessage(_msg: string): Promise<string> {
      throw new Error("Solana wallet support requires @solana/wallet-adapter.");
    },

    async signTransaction(_tx: unknown): Promise<string> {
      throw new Error("Solana wallet support requires @solana/wallet-adapter.");
    },

    async getBalance(_address: string): Promise<string> {
      throw new Error("Solana wallet support requires @solana/wallet-adapter.");
    },
  };
}
