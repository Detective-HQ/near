import type { WalletAccount, WalletProvider } from "./types";

/**
 * Aptos wallet adapter stub.
 *
 * Production integration requires:
 * - `@aptos-labs/wallet-adapter-react`
 * - `@aptos-labs/wallet-adapter-ant-design` or custom modal UI
 * - Wallets: Petra, Pontem, Martian, Rise
 * - `@aptos-labs/ts-sdk` for Move transaction payload creation and BCS serialization.
 */
export function createAptosProvider(providerName = "Petra / Pontem"): WalletProvider {
  return {
    family: "aptos",
    name: providerName,
    icon: "/images/wallets/aptos.svg",

    async connect(): Promise<WalletAccount> {
      throw new Error(
        "Aptos wallet support requires installing @aptos-labs/wallet-adapter-react. Please install the adapter package to connect Aptos wallets."
      );
    },

    async disconnect(): Promise<void> {
      // No-op for stub
    },

    async signMessage(_msg: string): Promise<string> {
      throw new Error("Aptos wallet support requires @aptos-labs/wallet-adapter-react.");
    },

    async signTransaction(_tx: unknown): Promise<string> {
      throw new Error("Aptos wallet support requires @aptos-labs/wallet-adapter-react.");
    },

    async getBalance(_address: string): Promise<string> {
      throw new Error("Aptos wallet support requires @aptos-labs/wallet-adapter-react.");
    },
  };
}
