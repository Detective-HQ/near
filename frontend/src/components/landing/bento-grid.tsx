import Link from "next/link";
import type { ReactNode } from "react";

// Visuals are abstract skeletons: shapes only, never prices, percentages or invented data.

function RankedLists() {
  const rows = [0, 1, 2, 3, 4, 5, 6, 7];
  const list = (side: "bull" | "bear", title: string) => (
    <div className={`v-rank v-rank-${side}`}>
      <span className="v-rank-title">{title}</span>
      <ol>
        {rows.map((i) => (
          <li key={i} className={i < 3 ? "top" : undefined}>
            <span className="v-rank-n">{String(i + 1).padStart(2, "0")}</span>
            <span className="v-skel" style={{ width: `${46 - (i % 3) * 8}%` }} />
            <span className="v-score" style={{ width: `${92 - i * 9}%` }} />
          </li>
        ))}
      </ol>
    </div>
  );
  return (
    <div className="v v-ranked" aria-hidden="true">
      {list("bull", "Bullish")}
      {list("bear", "Bearish")}
    </div>
  );
}

function SwapRoute() {
  return (
    <div className="v v-swap" aria-hidden="true">
      <span className="v-token"><i /><b /></span>
      <svg viewBox="0 0 200 60" preserveAspectRatio="none" className="v-route">
        <path d="M0 30 C 60 30, 70 8, 100 8 S 140 30, 200 30" />
        <circle r="3" className="v-route-dot">
          <animateMotion dur="5s" repeatCount="indefinite" path="M0 30 C 60 30, 70 8, 100 8 S 140 30, 200 30" />
        </circle>
      </svg>
      <span className="v-token"><i /><b /></span>
    </div>
  );
}

function Candles() {
  // x, open/close top, body height, wick top, wick bottom
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

function ProvenanceChips() {
  return (
    <div className="v v-chips" aria-hidden="true">
      {["source", "as-of", "model version", "freshness"].map((t) => (
        <span key={t} className="v-chip"><i />{t}</span>
      ))}
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
  return (
    <div className="v v-journal" aria-hidden="true">
      {[0, 1, 2, 3].map((i) => (
        <span key={i} className="v-jrow">
          <i />
          <b style={{ width: `${70 - i * 12}%` }} />
          <em />
        </span>
      ))}
    </div>
  );
}

function RiskRows() {
  return (
    <div className="v v-risk" aria-hidden="true">
      <span className="r-row r-fresh"><i />fresh<b /></span>
      <span className="r-row r-stale"><i />stale<b /></span>
      <span className="r-row r-missing"><i />unavailable<b /></span>
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
    href: "/workspace/swap", visual: <SwapRoute />,
  },
  {
    id: "structure", size: "c", label: "Market structure", title: "Zones with rules you can read.",
    text: "Order Blocks, BOS, CHoCH and Fair Value Gaps from published, versioned rules.",
    href: "/workspace/markets", visual: <Candles />,
  },
  {
    id: "provenance", size: "d", label: "Provenance", title: "Source and timestamp on everything.",
    text: "Every metric shows where it came from and how fresh it is.",
    href: "/workspace/intelligence", visual: <ProvenanceChips />,
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
        <h2 id="lp-bento-h">Everything you need to move, measure and decide.</h2>
        <p>Swap across chains, then check the evidence behind every number.</p>
      </header>
      <ul className="lp-bento">
        {CARDS.map((c) => (
          <li key={c.id} className={`lp-cell lp-cell-${c.size}`} id={c.id === "trading" ? "trading" : undefined}>
            <Link className="lp-card" href={c.href}>
              <div className="lp-card-visual">{c.visual}</div>
              <div className="lp-card-copy">
                <span className="lp-label">{c.label}</span>
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
