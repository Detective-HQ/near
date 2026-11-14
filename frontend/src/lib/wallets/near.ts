import type { WalletAccount, WalletProvider } from "./types";

/**
 * NEAR Protocol wallet adapter stub.
 *
 * Production integration requires:
 * - `@near-wallet-selector/core`
 * - `@near-wallet-selector/modal-ui`
 * - Wallets: MyNearWallet, Meteor Wallet, HERE Wallet, Ledger
 * - `near-api-js` for account balance queries and transaction signing.
 */
export function createNearProvider(providerName = "MyNearWallet / Meteor"): WalletProvider {
  return {
    family: "near",
    name: providerName,
    icon: "/images/wallets/near.svg",

    async connect(): Promise<WalletAccount> {
      throw new Error(
        "NEAR wallet support requires installing @near-wallet-selector. Please install the selector packages to connect NEAR accounts."
      );
    },

    async disconnect(): Promise<void> {
      // No-op for stub
    },

    async signMessage(_msg: string): Promise<string> {
      throw new Error("NEAR wallet support requires @near-wallet-selector.");
    },

    async signTransaction(_tx: unknown): Promise<string> {
      throw new Error("NEAR wallet support requires @near-wallet-selector.");
    },

    async getBalance(_address: string): Promise<string> {
      throw new Error("NEAR wallet support requires @near-wallet-selector.");
    },
  };
}
