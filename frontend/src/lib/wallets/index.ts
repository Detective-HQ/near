import type { ChainFamily, WalletProvider } from "./types";
import { createEvmProvider } from "./evm";
import { createSolanaProvider } from "./solana";
import { createNearProvider } from "./near";
import { createAptosProvider } from "./aptos";
import { createStellarProvider } from "./stellar";

export * from "./types";
export * from "./evm";
export * from "./solana";
export * from "./near";
export * from "./aptos";
export * from "./stellar";

export interface FamilyMetadata {
  family: ChainFamily;
  label: string;
  icon: string;
  description: string;
  supportedChains: string[];
  status: "active" | "planned";
}

export const SUPPORTED_FAMILIES: FamilyMetadata[] = [
  {
    family: "evm",
    label: "Ethereum & EVM",
    icon: "⟠",
    description: "Ethereum, Base, Arbitrum, Polygon, Avalanche, BNB Chain, Celo",
    supportedChains: ["1", "8453", "42161", "137", "43114", "56", "42220"],
    status: "active",
  },
  {
    family: "solana",
    label: "Solana",
    icon: "◎",
    description: "SPL tokens via Phantom, Solflare, Backpack",
    supportedChains: ["solana"],
    status: "planned",
  },
  {
    family: "near",
    label: "NEAR Protocol",
    icon: "Ⓝ",
    description: "NEAR accounts via MyNearWallet, Meteor",
    supportedChains: ["near"],
    status: "planned",
  },
  {
    family: "aptos",
    label: "Aptos",
    icon: "▲",
    description: "Move tokens via Petra, Pontem, Martian",
    supportedChains: ["aptos"],
    status: "planned",
  },
  {
    family: "stellar",
    label: "Stellar",
    icon: "✦",
    description: "Stellar assets with deposit memo via Freighter",
    supportedChains: ["stellar"],
    status: "planned",
  },
];

export function createProvider(family: ChainFamily, name?: string): WalletProvider {
  switch (family) {
    case "evm":
      return createEvmProvider(undefined, name);
    case "solana":
      return createSolanaProvider(name);
    case "near":
      return createNearProvider(name);
    case "aptos":
      return createAptosProvider(name);
    case "stellar":
      return createStellarProvider(name);
    default: {
      const exhaustiveCheck: never = family;
      throw new Error(`Unsupported chain family: ${String(exhaustiveCheck)}`);
    }
  }
}
