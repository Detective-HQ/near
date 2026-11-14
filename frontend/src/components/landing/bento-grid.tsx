import Link from "next/link";
import type { ReactNode } from "react";
import { CardFernCorner, LeafSprout } from "./foliage";

// Live previews and real token selectors replacing abstract skeletons.

const BULL_TOKENS = [
  { symbol: "NEAR", change: "+14.2%", score: 94 },
  { symbol: "SOL", change: "+8.6%", score: 87 },
  { symbol: "ETH", change: "+5.1%", score: 81 },
  { symbol: "SUI", change: "+4.4%", score: 73 },
  { symbol: "APT", change: "+3.8%", score: 67 },
  { symbol: "ARB", change: "+3.2%", score: 59 },
  { symbol: "XLM", change: "+2.1%", score: 51 },
  { symbol: "CELO", change: "+1.6%", score: 44 },
];

const BEAR_TOKENS = [
  { symbol: "AVAX", change: "-6.2%", score: 89 },
  { symbol: "DOT", change: "-4.6%", score: 82 },
  { symbol: "MATIC", change: "-3.9%", score: 75 },
  { symbol: "ATOM", change: "-3.1%", score: 68 },
  { symbol: "LINK", change: "-2.4%", score: 61 },
  { symbol: "OP", change: "-2.0%", score: 54 },
  { symbol: "UNI", change: "-1.8%", score: 47 },
  { symbol: "AAVE", change: "-1.1%", score: 40 },
];

function RankedLists() {
  const list = (side: "bull" | "bear", title: string, items: typeof BULL_TOKENS) => (
    <div className={`v-rank v-rank-${side}`}>
      <span className="v-rank-title">{title}</span>
      <ol>
        {items.map((item, i) => (
          <li key={item.symbol} className={i < 3 ? "top" : undefined}>
            <span className="v-rank-n">{String(i + 1).padStart(2, "0")}</span>
            <span className="v-token-cell">
              <span className="v-symbol">{item.symbol}</span>
              <span className="v-change">{item.change}</span>
            </span>
            <span className="v-score-wrap">
              <span className="v-score-bar" style={{ width: `${item.score}%` }} />
              <span className="v-score-num">{item.score}</span>
            </span>
          </li>
        ))}
      </ol>
    </div>
  );
  return (
    <div className="v v-ranked" aria-hidden="true">
      {list("bull", "Bullish Setups", BULL_TOKENS)}
      {list("bear", "Bearish Setups", BEAR_TOKENS)}
    </div>
  );
}

function MiniSwapSelector() {
  return (
    <div className="v v-mini-swap" aria-hidden="true">
      <div className="swap-box pay-box">
        <div className="swap-box-header">
          <span className="swap-label">Pay from Ethereum</span>
          <span className="swap-bal">Balance: 4.82 ETH</span>
        </div>
        <div className="swap-box-body">
          <div className="swap-asset-pill">
            <span className="swap-asset-icon eth-icon">Ξ</span>
            <div className="swap-asset-info">
              <span className="swap-asset-symbol">ETH</span>
              <span className="swap-asset-chain">Ethereum</span>
            </div>
          </div>
          <div className="swap-amount-display">
            <span className="swap-val">1.50</span>
            <span className="swap-usd">≈ $4,680.00</span>
          </div>
        </div>
      </div>

      <div className="swap-divider">
        <div className="swap-divider-line" />
        <div className="swap-rate-badge">
          <svg className="swap-switch-icon" viewBox="0 0 16 16" width="12" height="12">
            <path d="M4 2v9m0 0l-2-2m2 2l2-2M12 14V5m0 0l2 2m-2-2l-2 2" stroke="currentColor" fill="none" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
          </svg>
          <span>1 ETH ≈ 582.4 NEAR</span>
          <span className="swap-tag">1Click Fast</span>
        </div>
        <div className="swap-divider-line" />
      </div>

      <div className="swap-box receive-box">
        <div className="swap-box-header">
          <span className="swap-label">Receive on NEAR Protocol</span>
          <span className="swap-est">Estimated (-0.12% slip)</span>
        </div>
        <div className="swap-box-body">
          <div className="swap-asset-pill">
            <span className="swap-asset-icon near-icon">N</span>
            <div className="swap-asset-info">
              <span className="swap-asset-symbol">NEAR</span>
              <span className="swap-asset-chain">NEAR Mainnet</span>
            </div>
          </div>
          <div className="swap-amount-display">
            <span className="swap-val highlight">873.60</span>
            <span className="swap-usd">≈ $4,674.38</span>
          </div>
        </div>
      </div>
    </div>
  );
}

function Candles() {
  const c = [
    [14, 70, 26, 60, 104], [34, 58, 30, 48, 96], [54, 64, 18, 52, 90], [74, 44, 30, 34, 82],
    [94, 36, 20, 26, 64], [114, 46, 28, 38, 82], [134, 30, 24, 18, 62], [154, 22, 30, 14, 60],
    [174, 38, 22, 30, 68], [194, 26, 34, 16, 66], [214, 18, 24, 8, 50], [234, 30, 20, 22, 58],
  ];
  return (
    <svg className="v v-candles" viewBox="0 0 250 120" aria-hidden="true">
      <rect className="v-zone" x="0" y="64" width="250" height="22" />
      <line className="v-zone-edge" x1="0" x2="250" y1="64" y2="64" />
      <line className="v-zone-edge" x1="0" x2="250" y1="86" y2="86" />
      <g className="v-candle-g">
        {c.map(([x, y, h, wt, wb], i) => (
          <g key={i} className={i % 3 === 1 ? "hollow" : undefined}>
            <line x1={x + 4} x2={x + 4} y1={wt} y2={wb} />
            <rect x={x} y={y} width="8" height={h} rx="1.5" />
          </g>
        ))}
      </g>
    </svg>
  );
}

function DeFiLlamaTvlSparkline() {
  const points = [
    [0, 68], [18, 64], [36, 70], [54, 58], [72, 62],
    [90, 52], [108, 48], [126, 42], [144, 46], [162, 38],
    [180, 32], [198, 35], [216, 26], [234, 22], [252, 14], [270, 8]
  ];
  const polylinePoints = points.map(([x, y]) => `${x},${y}`).join(" ");
  const areaPoints = `0,90 ${polylinePoints} 270,90`;

  return (
    <div className="v v-tvl-preview" aria-hidden="true">
      <div className="tvl-top-bar">
        <div className="tvl-source-tag">
          <span className="tvl-live-pulse" />
          <span>DeFiLlama Live</span>
        </div>
        <span className="tvl-change-tag">+14.6% 30d</span>
      </div>

      <div className="tvl-metric-row">
        <div className="tvl-metric-left">
          <span className="tvl-metric-title">NEAR Protocol TVL</span>
          <span className="tvl-metric-val">$264.8M</span>
        </div>
        <div className="tvl-metric-right">
          <span className="tvl-sub-label">24h Vol</span>
          <span className="tvl-sub-val">$48.2M</span>
        </div>
      </div>

      <div className="tvl-chart-wrap">
        <svg viewBox="0 0 270 90" className="tvl-sparkline-svg" preserveAspectRatio="none">
          <defs>
            <linearGradient id="tvlGradient" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="var(--bull)" stopOpacity="0.35" />
              <stop offset="100%" stopColor="var(--bull)" stopOpacity="0.0" />
            </linearGradient>
          </defs>
          <polygon points={areaPoints} fill="url(#tvlGradient)" />
          <polyline points={polylinePoints} fill="none" stroke="var(--bull)" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />
          <circle cx="270" cy="8" r="3.5" fill="var(--bull)" />
          <circle cx="270" cy="8" r="7" fill="var(--bull)" opacity="0.4" className="tvl-ping" />
        </svg>
      </div>

      <div className="tvl-provenance-footer">
        <span className="tvl-prov-item">source: api.llama.fi</span>
        <span className="tvl-prov-dot">•</span>
        <span className="tvl-prov-item">updated: 3m ago</span>
      </div>
    </div>
  );
}

function ScenarioLevels() {
  return (
    <svg className="v v-levels" viewBox="0 0 240 110" aria-hidden="true">
      <line className="lvl-target" x1="0" x2="190" y1="16" y2="16" />
      <line className="lvl-target" x1="0" x2="190" y1="36" y2="36" />
      <rect className="lvl-entry" x="0" y="56" width="190" height="16" />
      <line className="lvl-invalid" x1="0" x2="190" y1="94" y2="94" />
      <text x="198" y="19">target</text>
      <text x="198" y="39">target</text>
      <text x="198" y="67">entry</text>
      <text x="198" y="97">invalid</text>
    </svg>
  );
}

function DepthChart() {
  return (
    <svg className="v v-depth" viewBox="0 0 300 110" preserveAspectRatio="none" aria-hidden="true">
      <path className="d-bid" d="M0 20 H40 V34 H70 V46 H96 V60 H120 V74 H142 V110 H0Z" />
      <path className="d-ask" d="M158 110 V76 H180 V62 H204 V50 H232 V36 H262 V24 H300 V110Z" />
      <line className="d-mid" x1="150" x2="150" y1="0" y2="110" />
      <g className="d-bars">
        {[12, 20, 16, 28, 22, 34, 26, 18, 30, 24].map((h, i) => (
          <rect key={i} x={6 + i * 29} y={110 - h} width="10" height={h} />
        ))}
      </g>
    </svg>
  );
}

function JournalTimeline() {
  const transactions = [
    { pair: "ETH → NEAR", amount: "1.50 ETH", status: "Settled 1Click", time: "4.2s ago" },
    { pair: "USDC → SOL", amount: "250 USDC", status: "Settled", time: "18m ago" },
    { pair: "SOL → NEAR", amount: "12.0 SOL", status: "Settled", time: "1h ago" },
    { pair: "NEAR → ARB", amount: "300 NEAR", status: "Settled", time: "3h ago" },
  ];
  return (
    <div className="v v-journal" aria-hidden="true">
      {transactions.map((tx, i) => (
        <div key={i} className="v-jrow-item">
          <span className="v-jrow-pair">{tx.pair}</span>
          <span className="v-jrow-amount">{tx.amount}</span>
          <span className="v-jrow-status">{tx.status}</span>
          <span className="v-jrow-time">{tx.time}</span>
        </div>
      ))}
    </div>
  );
}

function RiskRows() {
  const checks = [
    { name: "Slippage Guard", val: "0.15% max", status: "strict" },
    { name: "1Click Route API", val: "22ms latency", status: "optimal" },
    { name: "RPC Node Health", val: "6/6 chains synced", status: "verified" },
  ];
  return (
    <div className="v v-risk" aria-hidden="true">
      {checks.map((c, i) => (
        <div key={i} className="r-row r-row-card">
          <i className="r-dot-active" />
          <div className="r-row-detail">
            <span className="r-row-name">{c.name}</span>
            <span className="r-row-badge">{c.val}</span>
            <span className="r-row-status">{c.status}</span>
          </div>
        </div>
      ))}
    </div>
  );
}

interface Card {
  id: string;
  size: "a" | "b" | "c" | "d" | "e" | "f" | "g" | "h";
  label: string;
  title: string;
  text: string;
  href: string;
  visual: ReactNode;
}

const CARDS: Card[] = [
  {
    id: "trading", size: "a", label: "Trading intelligence", title: "Scan the market, both directions.",
    text: "Top 20 bullish and bearish setups, ranked by evidence. The top three stand out.",
    href: "/workspace/markets", visual: <RankedLists />,
  },
  {
    id: "swap", size: "b", label: "Universal swap", title: "Any chain to any chain.",
    text: "Cross-chain swaps signed in your own wallet. We never hold your keys.",
    href: "/workspace/swap", visual: <MiniSwapSelector />,
  },
  {
    id: "structure", size: "c", label: "Market structure", title: "Zones with rules you can read.",
    text: "Order Blocks, BOS, CHoCH and Fair Value Gaps from published, versioned rules.",
    href: "/workspace/markets", visual: <Candles />,
  },
  {
    id: "provenance", size: "d", label: "DeFiLlama Telemetry", title: "Live NEAR TVL & provenance.",
    text: "Streaming on-chain TVL, 24h DEX volume and source-verified provenance timestamps directly from DeFiLlama.",
    href: "/workspace/intelligence", visual: <DeFiLlamaTvlSparkline />,
  },
  {
    id: "scenarios", size: "e", label: "Scenarios", title: "Conditional levels, clear invalidation.",
    text: "Entry area, targets and the level that proves the setup wrong.",
    href: "/workspace/research", visual: <ScenarioLevels />,
  },
  {
    id: "derivatives", size: "f", label: "Derivatives & liquidity", title: "Context, not noise.",
    text: "Open interest, funding, liquidations and order-book depth where the data exists.",
    href: "/workspace/markets", visual: <DepthChart />,
  },
  {
    id: "journal", size: "g", label: "Journal & backtests", title: "Every signal is recorded.",
    text: "See what happened next, with costs included.",
    href: "/workspace/activity", visual: <JournalTimeline />,
  },
  {
    id: "risk", size: "h", label: "Risk center", title: "Stale data is hidden, not dressed up.",
    text: "Unavailable never means bearish.",
    href: "/workspace/risk", visual: <RiskRows />,
  },
];

export function BentoGrid() {
  return (
    <section className="lp-section" id="product" aria-labelledby="lp-bento-h">
      <header className="lp-head">
        <span className="lp-label"><LeafSprout className="lp-label-sprout" />Grown intelligence</span>
        <h2 id="lp-bento-h">Everything you need to move, measure and decide.</h2>
        <p>Swap across chains, then check the evidence behind every number.</p>
      </header>
      <ul className="lp-bento">
        {CARDS.map((c) => (
          <li key={c.id} className={`lp-cell lp-cell-${c.size}`} id={c.id === "trading" ? "trading" : undefined}>
            <Link className="lp-card" href={c.href}>
              <CardFernCorner />
              <div className="lp-card-visual">{c.visual}</div>
              <div className="lp-card-copy">
                <span className="lp-label"><LeafSprout className="lp-label-sprout" />{c.label}</span>
                <h3>{c.title}</h3>
                <p>{c.text}</p>
              </div>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
