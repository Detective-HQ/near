"use client";

import Link from "next/link";
import { useState } from "react";
import { api, ApiRequestError, USE_MOCKS } from "../../../lib/api-client";
import { formatDecimal, formatPercent, formatUsd } from "../../../lib/decimal";
import { asOf, formatTime } from "../../../lib/format";
import { qualityFromStatus } from "../../../lib/quality";
import type { MetricValue, Outcome, ZoneStatus } from "../../../lib/trading-contracts";
import { deriveViewState } from "../../../lib/view-state";
import { useResource, useUrlParams } from "../hooks";
import { DefRow, InfoTip, PageHeader, Provenance, QualityBadge, ScoreCell, Tabs, ViewStateBlock } from "../ui";
import { useWorkspace } from "../workspace-context";
import { COVERAGE_TIP, SCORE_TIP } from "./trading";
import { OVERLAYS, PriceChart, ZONE_LABEL } from "../price-chart";
import type { Overlay } from "../price-chart";

const TABS = ["Overview", "Derivatives", "Order flow", "Scenario", "Risk", "History"] as const;
type Tab = (typeof TABS)[number];

export const OUTCOME_LABEL: Record<Outcome, string> = {
  target_hit: "Target hit",
  stop_hit: "Stop hit",
  expired: "Expired",
  open: "Open",
};

export function fmtMetric(m: MetricValue | null | undefined): string {
  if (!m || m.value === null) return "Not available";
  if (m.unit.toUpperCase().startsWith("USD")) return `${formatUsd(m.value)}${m.unit.length > 3 ? ` ${m.unit.slice(4)}` : ""}`;
  if (m.unit.startsWith("%")) return `${formatDecimal(m.value, 4)}%${m.unit.length > 1 ? ` ${m.unit.slice(2)}` : ""}`;
  return `${formatDecimal(m.value, 4)} ${m.unit}`;
}

function MetricRow({ label, metric, note }: { label: string; metric: MetricValue | null; note?: string }) {
  return (
    <div className="ecosystem-row ws-metric-row">
      <div className="row-title">
        <strong>{label}</strong>
        {note && <small>{note}</small>}
      </div>
      <span>{fmtMetric(metric)}</span>
      {metric ? <Provenance source={metric.source} time={metric.asOf} quality={qualityFromStatus(metric.quality, metric.value !== null)} /> : <QualityBadge quality="Unavailable" />}
    </div>
  );
}

export function TradingAssetPage({ assetId }: { assetId: string }) {
  const { online, chainName } = useWorkspace();
  const { get, update } = useUrlParams();
  const tab = (TABS.find((t) => t === get("tab")) ?? "Overview") as Tab;

  const asset = useResource(`asset:${assetId}`, () => api.tradingAsset(assetId));
  const a = asset.data;
  const tf = get("tf") || a?.timeframes[0] || "4h";
  const structure = useResource(a ? `structure:${assetId}:${tf}` : null, () => api.tradingStructure(assetId, tf));
  const liquidity = useResource(a ? `liquidity:${assetId}` : null, () => api.tradingLiquidity(assetId));
  const scenario = useResource(a && tab === "Scenario" ? `scenario:${assetId}` : null, () => api.tradingScenario(assetId));
  const history = useResource(a && tab === "History" ? `history:${assetId}` : null, () => api.tradingHistory(new URLSearchParams({ asset: assetId })));
  const [on, setOn] = useState<Set<Overlay>>(new Set(OVERLAYS));

  const notFound = asset.error instanceof ApiRequestError && (asset.error.code === "ASSET_NOT_FOUND" || (USE_MOCKS && asset.error.status === 404));
  const state = deriveViewState({ loading: asset.loading, online, error: asset.error, hasData: a !== null, stale: a?.quality === "stale", partial: a?.quality === "partial" });

  const back = <Link className="button button-quiet button-small" href="/workspace/trading">← Scanner</Link>;

  if (notFound) {
    return (
      <>
        {back}
        <div className="empty-state ws-state" role="status">
          <p>Asset not found.</p>
          <Link className="button button-secondary button-small" href="/workspace/trading">Back to scanner</Link>
        </div>
      </>
    );
  }

  const derivs = a ? ([
    ["Open interest", a.derivatives.openInterest],
    ["Funding rate", a.derivatives.fundingRate],
    ["Liquidations", a.derivatives.liquidations],
    ["Futures volume", a.derivatives.futuresVolume],
  ] as const) : [];
  const missingDerivs = derivs.filter(([, m]) => !m || m.value === null).map(([l]) => l);

  return (
    <>
      {back}
      <PageHeader title={a ? a.symbol : "Trading Intelligence"} sub={a ? `${chainName(a.chain)} · read-only analysis` : "Ranked setups, both directions. Read-only."} testData={Boolean(a?.testData)} />
      <ViewStateBlock view="trading-asset" state={state} staleTime={a?.asOf ?? undefined} onAction={asset.reload} />

      {a && (
        <>
          <dl className="ws-stats panel panel-pad" aria-label="Asset summary">
            <div><dt>Symbol</dt><dd>{a.symbol}</dd></div>
            <div><dt>Chain</dt><dd>{chainName(a.chain)}</dd></div>
            <div><dt>Venues</dt><dd>{a.venues.length ? a.venues.join(", ") : "Not available"}</dd></div>
            <div><dt>Last price</dt><dd>{a.lastPrice ? formatDecimal(a.lastPrice, 6) : "Not available"}</dd></div>
            <div><dt>24h</dt><dd>{a.change24h ? formatPercent(a.change24h, 2, true) : "Not available"}</dd></div>
            <div><dt>As of</dt><dd>{formatTime(a.asOf)}</dd></div>
          </dl>

          <Tabs tabs={TABS} active={tab} label="Asset sections" onChange={(t) => update({ tab: t === "Overview" ? null : t })} />

          <div role="tabpanel" aria-label={tab} className="section">
            {tab === "Overview" && (
              <>
                <div className="kpi-grid panel panel-pad">
                  <div className="kpi-item"><span className="metric-label">Bullish score <InfoTip label="About the score" text={SCORE_TIP} /></span><span className="metric-value"><ScoreCell score={a.scores.bullish} /></span></div>
                  <div className="kpi-item"><span className="metric-label">Bearish score <InfoTip label="About the score" text={SCORE_TIP} /></span><span className="metric-value"><ScoreCell score={a.scores.bearish} /></span></div>
                  <div className="kpi-item"><span className="metric-label">Coverage <InfoTip label="About coverage" text={COVERAGE_TIP} /></span><span className="metric-value">{a.scores.coverage ? formatPercent(a.scores.coverage, 0) : "Not available"}</span></div>
                  <div className="kpi-item"><span className="metric-label">Model</span><span className="metric-value">{a.scores.modelVersion ?? "Not available"}</span></div>
                </div>

                <section className="panel panel-pad section" aria-label="Chart">
                  <div className="panel-title">
                    <h2>Chart</h2>
                    <div className="ws-inline" role="group" aria-label="Timeframe">
                      {a.timeframes.map((t) => (
                        <button key={t} type="button" className={`button button-small ${t === tf ? "" : "button-secondary"}`} aria-pressed={t === tf} onClick={() => update({ tf: t })}>{t}</button>
                      ))}
                    </div>
                  </div>
                  <div className="ws-inline ws-overlays" role="group" aria-label="Overlays">
                    {OVERLAYS.map((o) => (
                      <label key={o} className="ws-toggle">
                        <input
                          type="checkbox"
                          checked={on.has(o)}
                          onChange={(e) => setOn((s) => {
                            const next = new Set(s);
                            if (e.target.checked) next.add(o);
                            else next.delete(o);
                            return next;
                          })}
                        />
                        {o}
                      </label>
                    ))}
                  </div>
                  {structure.data ? (
                    <>
                      <PriceChart data={structure.data} levels={liquidity.data?.levels ?? []} on={on} />
                      <ul className="ws-legend" aria-label="Zone legend">
                        {(["fresh", "tested", "mitigated", "invalidated", "expired"] as ZoneStatus[]).map((s) => (
                          <li key={s}><span className={`ws-swatch ws-zone-${s}`} aria-hidden="true" />{ZONE_LABEL[s]}</li>
                        ))}
                      </ul>
                      <Provenance source={structure.data.source} time={structure.data.asOf} quality={qualityFromStatus(structure.data.quality)} />
                    </>
                  ) : (
                    <ViewStateBlock view="trading-asset" state={deriveViewState({ loading: structure.loading, online, error: structure.error, hasData: false })} onAction={structure.reload} />
                  )}
                </section>

                <section className="panel panel-pad section" aria-label="Evidence">
                  <div className="panel-title"><h2>Evidence</h2></div>
                  {a.evidence.length === 0 ? <p className="subtle-meta">Not available</p> : (
                    <div className="table-wrap">
                      <table className="data-table">
                        <thead><tr><th scope="col">Type</th><th scope="col">Value</th><th scope="col">Source</th><th scope="col">Observed</th></tr></thead>
                        <tbody>
                          {a.evidence.map((e) => (
                            <tr key={`${e.type}-${e.value}`}><td>{e.type}</td><td>{e.value}</td><td>{e.source}</td><td>{formatTime(e.observedAt)}</td></tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                </section>
              </>
            )}

            {tab === "Derivatives" && (
              <section className="panel panel-pad">
                {derivs.filter(([, m]) => m && m.value !== null).map(([label, m]) => <MetricRow key={label} label={label} metric={m} />)}
                {missingDerivs.length > 0 && (
                  <p className="subtle-meta ws-note">{missingDerivs.join(", ")}: Not supplied by provider.</p>
                )}
              </section>
            )}

            {tab === "Order flow" && (
              <section className="panel panel-pad">
                {liquidity.data ? (
                  <>
                    <MetricRow label="Spot volume" metric={liquidity.data.spotVolume} />
                    <MetricRow label="CVD" metric={liquidity.data.cvd} note={liquidity.data.cvd ? `Method: ${liquidity.data.cvd.method}` : undefined} />
                    <div className="ecosystem-row ws-metric-row">
                      <div className="row-title">
                        <strong>Depth</strong>
                        <small>Snapshot of displayed liquidity.</small>
                      </div>
                      <span>
                        {liquidity.data.depth
                          ? `Bid ${liquidity.data.depth.bid ? formatUsd(liquidity.data.depth.bid) : "Not available"} · Ask ${liquidity.data.depth.ask ? formatUsd(liquidity.data.depth.ask) : "Not available"} (${liquidity.data.depth.range})`
                          : "Not available"}
                      </span>
                      {liquidity.data.depth ? <Provenance source={liquidity.data.depth.source} time={liquidity.data.depth.asOf} quality={qualityFromStatus(liquidity.data.depth.quality)} /> : <QualityBadge quality="Unavailable" />}
                    </div>
                  </>
                ) : (
                  <ViewStateBlock view="trading-asset" state={deriveViewState({ loading: liquidity.loading, online, error: liquidity.error, hasData: false })} onAction={liquidity.reload} />
                )}
              </section>
            )}

            {tab === "Scenario" && (
              <section className="panel panel-pad">
                <p className="ws-note">Conditional levels. Valid only if the stated conditions hold.</p>
                {scenario.data ? (
                  scenario.data.status === "stale" ? (
                    <p className="empty-state">Scenario hidden: data is out of date.</p>
                  ) : scenario.data.status === "none" ? (
                    <p className="empty-state">No scenario available for this setup.</p>
                  ) : (
                    <div className="quote-result">
                      <DefRow label="Reference price">{scenario.data.referencePrice ? formatDecimal(scenario.data.referencePrice, 6) : "Not available"}</DefRow>
                      <DefRow label="Entry area">{scenario.data.entryArea ? `${formatDecimal(scenario.data.entryArea.low, 6)} – ${formatDecimal(scenario.data.entryArea.high, 6)}` : "Not available"}</DefRow>
                      <DefRow label="Target 1">{scenario.data.target1 ? formatDecimal(scenario.data.target1, 6) : "Not available"}</DefRow>
                      <DefRow label="Target 2">{scenario.data.target2 ? formatDecimal(scenario.data.target2, 6) : "Not available"}</DefRow>
                      <DefRow label="Invalidation">{scenario.data.invalidation ? formatDecimal(scenario.data.invalidation, 6) : "Not available"}</DefRow>
                      <DefRow label="Risk/reward">{scenario.data.riskReward ? `${formatDecimal(scenario.data.riskReward, 2)} : 1` : "Not available"}</DefRow>
                      <DefRow label="Scenario distance">{scenario.data.distance ? formatPercent(scenario.data.distance, 2, true) : "Not available"}</DefRow>
                      <DefRow label="Expires">{formatTime(scenario.data.expiresAt)}</DefRow>
                      <p className="subtle-meta ws-prov-line">
                        <Provenance source={scenario.data.source} time={scenario.data.asOf} quality={qualityFromStatus(scenario.data.quality)} />
                      </p>
                    </div>
                  )
                ) : (
                  <ViewStateBlock view="trading-asset" state={deriveViewState({ loading: scenario.loading, online, error: scenario.error, hasData: false })} onAction={scenario.reload} />
                )}
              </section>
            )}

            {tab === "Risk" && (
              <section className="panel panel-pad">
                <MetricRow label="Volatility" metric={a.risk.volatility} />
                <MetricRow label="Spread & depth" metric={a.risk.spreadDepth} />
                <MetricRow label="Data freshness" metric={a.risk.dataFreshness} />
                <MetricRow label="Venue concentration" metric={a.risk.venueConcentration} />
              </section>
            )}

            {tab === "History" && (
              <section className="panel panel-pad">
                {history.data ? (
                  history.data.items.length === 0 ? (
                    <p className="empty-state">No earlier signals for this asset.</p>
                  ) : (
                    <div className="table-wrap">
                      <table className="data-table">
                        <thead><tr><th scope="col">Generated</th><th scope="col">Direction</th><th scope="col">Score</th><th scope="col">Timeframe</th><th scope="col">Outcome</th></tr></thead>
                        <tbody>
                          {history.data.items.map((h) => (
                            <tr key={h.id}>
                              <td>{formatTime(h.generatedAt)}</td>
                              <td>{h.direction === "bullish" ? "Bullish" : "Bearish"}</td>
                              <td><ScoreCell score={h.score} /></td>
                              <td>{h.timeframe}</td>
                              <td>{OUTCOME_LABEL[h.outcome]}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )
                ) : (
                  <ViewStateBlock view="trading-asset" state={deriveViewState({ loading: history.loading, online, error: history.error, hasData: false })} onAction={history.reload} />
                )}
              </section>
            )}
          </div>
          <p className="subtle-meta ws-prov-line">{a.source} · {asOf(a.asOf)} <QualityBadge quality={qualityFromStatus(a.quality)} /></p>
        </>
      )}
    </>
  );
}
