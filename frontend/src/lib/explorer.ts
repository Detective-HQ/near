// Block explorer base URLs (configuration, not data).
const EVM: Record<string, string> = {
  "1": "https://etherscan.io",
  "8453": "https://basescan.org",
  "42161": "https://arbiscan.io",
  "137": "https://polygonscan.com",
  "43114": "https://snowtrace.io",
  "56": "https://bscscan.com",
  "42220": "https://celoscan.io",
};

const BY_CHAIN: Record<string, { address: string; tx: string }> = {
  near: { address: "https://nearblocks.io/address/", tx: "https://nearblocks.io/txns/" },
  solana: { address: "https://explorer.solana.com/address/", tx: "https://explorer.solana.com/tx/" },
  stellar: { address: "https://stellar.expert/explorer/public/account/", tx: "https://stellar.expert/explorer/public/tx/" },
  aptos: { address: "https://explorer.aptoslabs.com/account/", tx: "https://explorer.aptoslabs.com/txn/" },
};

export function explorerAddressUrl(chain: { key?: string; chainId?: string | null } | null, address: string): string | null {
  if (!chain) return null;
  if (chain.chainId && EVM[chain.chainId]) return `${EVM[chain.chainId]}/address/${address}`;
  const c = chain.key ? BY_CHAIN[chain.key] : undefined;
  return c ? c.address + address : null;
}

export function explorerTxUrl(chain: { key?: string; chainId?: string | null } | null, hash: string): string | null {
  if (!chain) return null;
  if (chain.chainId && EVM[chain.chainId]) return `${EVM[chain.chainId]}/tx/${hash}`;
  const c = chain.key ? BY_CHAIN[chain.key] : undefined;
  return c ? c.tx + hash : null;
}
