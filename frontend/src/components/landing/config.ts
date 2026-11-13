// Landing page content that changes with what the product actually supports.
// Flip `enabled` to false to hide an entry; nothing else needs to change.

export interface LogoEntry {
  name: string;
  /** Key into BRAND_ICONS. Omit when no official mark is bundled; a monogram is drawn instead. */
  icon?: string;
  enabled: boolean;
}

/** Chains in the marquee. All six are in the backend chain registry; none is swap-enabled yet. */
export const CHAINS: LogoEntry[] = [
  { name: "NEAR", icon: "near", enabled: true },
  { name: "Ethereum", icon: "ethereum", enabled: true },
  { name: "Solana", icon: "solana", enabled: true },
  { name: "Stellar", icon: "stellar", enabled: true },
  { name: "Celo", enabled: true },
  { name: "Aptos", enabled: true },
];

export const STACK: LogoEntry[] = [
  { name: "Next.js", icon: "nextdotjs", enabled: true },
  { name: "TypeScript", icon: "typescript", enabled: true },
  { name: "Go", icon: "go", enabled: true },
  { name: "PostgreSQL", icon: "postgresql", enabled: true },
  { name: "Redis", icon: "redis", enabled: true },
  { name: "Docker", icon: "docker", enabled: true },
  { name: "GitHub Actions", icon: "githubactions", enabled: true },
];

/**
 * Data and routing providers. Only providers with a working adapter in the backend are enabled:
 * NEAR Intents (backend/internal/swap/providers/nearoneclick) and DefiLlama
 * (backend/internal/analytics/connectors/defillama). The rest are candidates.
 */
export const PROVIDERS: LogoEntry[] = [
  { name: "NEAR Intents", icon: "near", enabled: true },
  { name: "Binance", icon: "binance", enabled: false },
  { name: "Hyperliquid", enabled: false },
  { name: "CoinGlass", enabled: false },
  { name: "DefiLlama", enabled: true },
  { name: "Dune", enabled: false },
];

export const LINKS = {
  app: "/workspace",
  trading: "/workspace/markets",
  research: "/workspace/research",
  docs: "https://github.com/Detective-HQ/near/tree/main/docs",
  methodology: "https://github.com/Detective-HQ/near/blob/main/docs/research-methodology.md",
};

export const MODULES: Array<{ name: string; text: string; href: string }> = [
  { name: "Explore", text: "Find supported chains, assets and routes.", href: "/workspace" },
  { name: "Swap", text: "Move assets across chains from your wallet.", href: "/workspace/swap" },
  { name: "Activity", text: "Track every swap from start to finish.", href: "/workspace/activity" },
  { name: "Portfolio", text: "See your balances and allocation, read-only.", href: "/workspace/portfolio" },
  { name: "Markets", text: "Prices, TVL, volume and market breadth.", href: "/workspace/markets" },
  { name: "Trading", text: "Ranked setups with zones and scenarios.", href: "/workspace/markets" },
  { name: "Intelligence", text: "Network activity and ecosystem health.", href: "/workspace/intelligence" },
  { name: "Research", text: "A reproducible NEAR thesis with bull, base and bear cases.", href: "/workspace/research" },
  { name: "Risk", text: "Slippage, stale data and model-risk warnings in one place.", href: "/workspace/risk" },
];
