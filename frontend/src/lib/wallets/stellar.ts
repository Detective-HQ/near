import type { WalletAccount, WalletProvider } from "./types";

/**
 * Attaches a memo to a Stellar transaction object.
 *
 * Required by swap routes that specify depositMemo (e.g. centralized deposit
 * addresses or bridge aggregators routing via Stellar).
 *
 * In production using `@stellar/stellar-sdk`:
 * ```ts
 * const memoObj = /^\d+$/.test(memo) ? Memo.id(memo) : Memo.text(memo);
 * transactionBuilder.addMemo(memoObj);
 * ```
 */
export function attachMemo<T extends Record<string, unknown>>(tx: T, memo: string): T & { memo: string } {
  if (!memo || typeof memo !== "string") {
    throw new TypeError("A valid deposit memo string is required by this Stellar swap route.");
  }
  return {
    ...tx,
    memo,
  };
}

/**
 * Stellar wallet adapter stub.
 *
 * Production integration requires:
 * - `@stellar/freighter-api` (Freighter browser extension)
 * - `@stellar/stellar-sdk` for Horizon/Soroban RPC queries and TransactionBuilder.
 */
export function createStellarProvider(providerName = "Freighter"): WalletProvider {
  return {
    family: "stellar",
    name: providerName,
    icon: "/images/wallets/stellar.svg",

    async connect(): Promise<WalletAccount> {
      throw new Error(
        "Stellar wallet support requires installing @stellar/freighter-api. Please install Freighter to connect Stellar accounts."
      );
    },

    async disconnect(): Promise<void> {
      // No-op for stub
    },

    async signMessage(_msg: string): Promise<string> {
      throw new Error("Stellar wallet support requires @stellar/freighter-api.");
    },

    async signTransaction(_tx: unknown): Promise<string> {
      throw new Error("Stellar wallet support requires @stellar/freighter-api.");
    },

    async getBalance(_address: string): Promise<string> {
      throw new Error("Stellar wallet support requires @stellar/freighter-api.");
    },
  };
}
