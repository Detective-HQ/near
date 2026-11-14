"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { api } from "../../../lib/api-client";
import { formatPercent } from "../../../lib/decimal";
import { asOf, formatTime } from "../../../lib/format";
import { qualityFromStatus, regimeLabel } from "../../../lib/quality";
import type { Signal } from "../../../lib/trading-contracts";
import { deriveViewState } from "../../../lib/view-state";
import { useResource, useUrlParams } from "../hooks";
import { FilterSelect, InfoTip, PageHeader, Provenance, QualityBadge, ScoreCell, SearchField, Skeleton, Tabs, ViewStateBlock } from "../ui";
import { useWorkspace } from "../workspace-context";

const TABS = ["Top 20 bullish", "Top 20 bearish"] as const;
const FILTER_KEYS = ["timeframe", "chain", "asset", "venue", "quote", "liquidityTier"] as const;

export const SCORE_TIP = "A ranking score from 0 to 100. Not a probability.";
export const COVERAGE_TIP = "Share of evidence available for this score.";

function Move({ value }: { value: number | null }) {
  if (value === null) return <span>Not available</span>;
  if (value === 0) return <span aria-label="No change in rank">—</span>;
  return <span aria-label={`${value > 0 ? "Up" : "Down"} ${Math.abs(value)} places`}>{value > 0 ? "▲" : "▼"} {Math.abs(value)}</span>;
}

function TopMoversCard() {
  const { chainName } = useWorkspace();
  const topMovers = useResource("trading-top-movers", () => api.tradingTopMovers());
  const d = topMovers.data;
  if (topMovers.loading) return <Skeleton rows={4} />;
  if (topMovers.error || !d) {
    return (
      <section className="panel panel-pad section" aria-label="Top movers">
        <div className="panel-title">
          <h2>Top movers</h2>
        </div>
        <p className="subtle-meta">Data unavailable right now.</p>
      </section>
    );
  }
  return (
    <section className="panel panel-pad section" aria-label="Top movers">
      <div className="panel-title">
        <h2>Top movers</h2>
        <Provenance source={d.source} time={d.asOf} quality={qualityFromStatus(d.quality)} />
      </div>
      <div className="table-wrap">
        <table className="data-table">
          <thead>
            <tr>
              <th scope="col">Asset</th>
              <th scope="col">Chain</th>
              <th scope="col" className="num">24h Change</th>
              <th scope="col">Direction</th>
            </tr>
          </thead>
          <tbody>
            {d.items.slice(0, 10).map((s) => (
              <tr key={s.assetId}>
                <td>
                  <Link href={`/workspace/trading/${encodeURIComponent(s.assetId)}`}>
                    <strong>{s.symbol}</strong>
                  </Link>
                </td>
                <td>{chainName(s.chain)}</td>
                <td className="num">{s.change ? formatPercent(s.change, 2, true) : "Not available"}</td>
                <td>{s.direction ?? "neutral"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

export function TradingPage() {
  const { online, chainName } = useWorkspace();
  const router = useRouter();
  const { get, update, params } = useUrlParams();
  const tab = get("dir") === "bearish" ? TABS[1] : TABS[0];

  const apiParams = new URLSearchParams();
  FILTER_KEYS.forEach((k) => get(k) && apiParams.set(k, get(k)));
  const key = apiParams.toString();
  const overview = useResource("trading-overview", api.tradingOverview);
  const signals = useResource(`signals:${key}`, () => api.tradingSignals(apiParams));

  const data = signals.data;
  const list: Signal[] = data ? (tab === TABS[0] ? data.bullish : data.bearish) : [];
  const state = deriveViewState({
    loading: signals.loading,
    online,
    error: signals.error,
    hasData: data !== null,
    isEmpty: list.length === 0,
    stale: data?.quality === "stale",
    partial: data?.quality === "partial" || list.some((s) => s.quality === "partial"),
  });
  const o = overview.data;
  const anyFilter = FILTER_KEYS.some((k) => params.get(k));
  const f = data?.filters;

  return (
    <>
      <PageHeader
        title="Trading Intelligence"
        sub="Ranked setups, both directions. Read-only."
        testData={Boolean(data?.testData || o?.testData)}
        actions={
          <>
            <Link className="button button-secondary button-small" href="/workspace/trading/history">Signal journal</Link>
            <Link className="button button-secondary button-small" href="/workspace/trading/backtest">Backtests</Link>
          </>
        }
      />

      <dl className="ws-stats panel panel-pad" aria-label="Scan summary">
        <div><dt>Regime</dt><dd>{o ? regimeLabel(o.regime) : "Unavailable"}</dd></div>
        <div><dt>Breadth</dt><dd>{o?.breadth ? `${o.breadth.bullish} bullish · ${o.breadth.neutral} neutral · ${o.breadth.bearish} bearish` : "Not available"}</dd></div>
        <div><dt>Universe</dt><dd>{o?.universe ? `${o.universe.scanned} of ${o.universe.total} assets` : "Not available"}</dd></div>
        <div><dt>Coverage</dt><dd>{o?.coverage ? formatPercent(o.coverage, 1) : "Not available"}</dd></div>
        <div><dt>Model</dt><dd>{o?.modelVersion ? `Model ${o.modelVersion}` : "Not available"}</dd></div>
        <div><dt>As of</dt><dd>{o ? formatTime(o.asOf) : "Not available"}</dd></div>
      </dl>

      <TopMoversCard />

      {f && (
        <div className="ws-filters">
          <SearchField label="Asset" placeholder="Filter by asset" value={get("asset")} onChange={(v) => update({ asset: v })} />
          {f.timeframe.length > 0 && <FilterSelect label="Timeframe" value={get("timeframe")} options={f.timeframe.map((v) => ({ value: v, label: v }))} onChange={(v) => update({ timeframe: v })} />}
          {f.chain.length > 0 && <FilterSelect label="Chain" value={get("chain")} options={f.chain.map((v) => ({ value: v, label: chainName(v) }))} onChange={(v) => update({ chain: v })} />}
          {f.venue.length > 0 && <FilterSelect label="Venue" value={get("venue")} options={f.venue.map((v) => ({ value: v, label: v }))} onChange={(v) => update({ venue: v })} />}
          {f.quote.length > 0 && <FilterSelect label="Quote" value={get("quote")} options={f.quote.map((v) => ({ value: v, label: v }))} onChange={(v) => update({ quote: v })} />}
          {f.liquidityTier.length > 0 && <FilterSelect label="Liquidity tier" value={get("liquidityTier")} options={f.liquidityTier.map((v) => ({ value: v, label: v }))} onChange={(v) => update({ liquidityTier: v })} />}
          <button type="button" className="button button-quiet button-small ws-reset" disabled={!anyFilter} onClick={() => update(Object.fromEntries(FILTER_KEYS.map((k) => [k, null])))}>
            Reset filters
          </button>
        </div>
      )}

      <Tabs tabs={TABS} active={tab} label="Direction" onChange={(t) => update({ dir: t === TABS[1] ? "bearish" : null })} />

      <ViewStateBlock view="trading" state={state} staleTime={data?.asOf ?? undefined} onAction={signals.reload} />

      {data && list.length > 0 && (
        <section className="panel panel-pad" aria-label={tab} role="tabpanel">
          <div className="panel-title">
            <h2>{tab}</h2>
            <Provenance source={data.source} time={data.asOf} quality={qualityFromStatus(data.quality)} />
          </div>
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th scope="col">Rank</th>
                  <th scope="col">Move</th>
                  <th scope="col">Asset</th>
                  <th scope="col">Score <InfoTip label="About the score" text={SCORE_TIP} /></th>
                  <th scope="col" className="num">Change</th>
                  <th scope="col">Coverage <InfoTip label="About coverage" text={COVERAGE_TIP} /></th>
                  <th scope="col">Timeframe</th>
                  <th scope="col">As of</th>
                  <th scope="col">Quality</th>
                </tr>
              </thead>
              <tbody>
                {list.map((s) => {
                  const href = `/workspace/trading/${encodeURIComponent(s.assetId)}`;
                  if (s.suppressed) {
                    return (
                      <tr key={s.assetId}>
                        <td>{s.rank}</td>
                        <td colSpan={7}><strong>{s.symbol}</strong> <span className="subtle-meta">{chainName(s.chain)}</span> · Hidden: data is stale.</td>
                        <td><QualityBadge quality="Stale" /></td>
                      </tr>
                    );
                  }
                  return (
                    <tr key={s.assetId} className="ws-row-link" onClick={() => router.push(href)}>
                      <td>
                        {s.rank}
                        {s.rank <= 3 && <span className="state-pill good ws-top3">Top 3</span>}
                      </td>
                      <td><Move value={s.rankChange} /></td>
                      <td>
                        <Link href={href} onClick={(e) => e.stopPropagation()}><strong>{s.symbol}</strong></Link>{" "}
                        <span className="subtle-meta">{chainName(s.chain)} · {s.venue} · {s.quote}</span>
                      </td>
                      <td><ScoreCell score={s.score} /></td>
                      <td className="num">{s.change ? formatPercent(s.change, 2, true) : "Not available"}</td>
                      <td>{s.coverage ? formatPercent(s.coverage, 0) : "Not available"}</td>
                      <td>{s.timeframe}</td>
                      <td className="subtle-meta">{asOf(s.asOf).replace("As of ", "")}</td>
                      <td><QualityBadge quality={qualityFromStatus(s.quality)} /></td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {data && data.notRanked.length > 0 && (
        <details className="panel panel-pad section ws-details">
          <summary>Not ranked ({data.notRanked.length})</summary>
          <ul className="risk-list">
            {data.notRanked.map((n) => (
              <li key={n.assetId} className="risk-row">
                <strong>{n.symbol}</strong>
                <p>{n.reason}</p>
                <span className="subtle-meta">{chainName(n.chain)}</span>
              </li>
            ))}
          </ul>
        </details>
      )}
    </>
  );
}
