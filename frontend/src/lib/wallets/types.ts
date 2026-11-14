export type ChainFamily = "evm" | "solana" | "near" | "aptos" | "stellar";

export interface WalletAccount {
  address: string;
  chainFamily: ChainFamily;
  chainId?: string;
  label?: string;
}

export interface WalletProvider {
  family: ChainFamily;
  name: string;
  icon: string;
  connect: () => Promise<WalletAccount>;
  disconnect: () => Promise<void>;
  signMessage?: (msg: string) => Promise<string>;
  signTransaction?: (tx: unknown) => Promise<string>;
  switchChain?: (chainId: string) => Promise<void>;
  getBalance?: (address: string) => Promise<string>;
}

export type ConnectionState =
  | "disconnected"
  | "connecting"
  | "connected"
  | "switching"
  | "confirming"
  | "error";

export interface WalletState {
  account: WalletAccount | null;
  state: ConnectionState;
  error: string | null;
  provider: WalletProvider | null;
}
