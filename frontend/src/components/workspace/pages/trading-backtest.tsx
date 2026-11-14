"use client";

import Link from "next/link";
import { api } from "../../../lib/api-client";
import { formatPercent } from "../../../lib/decimal";
import { qualityFromStatus } from "../../../lib/quality";
import { deriveViewState } from "../../../lib/view-state";
import { useResource, useUrlParams } from "../hooks";
import { Drawer, PageHeader, Provenance, ViewStateBlock } from "../ui";
import { useWorkspace } from "../workspace-context";

/** Plain decimals are percentages; anything with a unit (e.g. "5.18R", risk multiples) is shown as sent. */
function fmtDrawdown(v: string | null): string {
  if (!v) return "Not available";
  return /^-?\d+(\.\d+)?$/.test(v.trim()) ? formatPercent(v, 1) : v;
}

export function TradingBacktestPage() {
  const { online } = useWorkspace();
  const { get, update } = useUrlParams();
  const res = useResource("backtests", api.tradingBacktests);
  const d = res.data;
  const open = d?.items.find((b) => b.id === get("run")) ?? null;
  const state = deriveViewState({ loading: res.loading, online, error: res.error, hasData: d !== null, isEmpty: d?.items.length === 0, stale: d?.quality === "stale" });

  return (
    <>
      <Link className="button button-quiet button-small" href="/workspace/trading">← Scanner</Link>
      <PageHeader title="Backtests" sub="How the model performed on past data." testData={Boolean(d?.testData)} />
      <div className="fixture-banner ws-banner" role="note"><p>Historical results do not predict future results.</p></div>
      <ViewStateBlock view="trading-backtest" state={state} staleTime={d?.asOf ?? undefined} onAction={res.reload} />

      {d && d.items.length > 0 && (
        <section className="panel panel-pad" aria-label="Backtests">
          <div className="panel-title">
            <h2>Runs</h2>
            <Provenance source={d.source} time={d.asOf} quality={qualityFromStatus(d.quality)} />
          </div>
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th scope="col">Model</th>
                  <th scope="col">Period</th>
                  <th scope="col">Universe</th>
                  <th scope="col">Timeframe</th>
                  <th scope="col" className="num">Sample size</th>
                  <th scope="col">Costs</th>
                  <th scope="col">Benchmark</th>
                  <th scope="col" className="num">Max drawdown</th>
                </tr>
              </thead>
              <tbody>
                {d.items.map((b) => (
                  <tr key={b.id}>
                    <td><button type="button" className="ws-row-btn" onClick={() => update({ run: b.id })}>{b.model}</button></td>
                    <td>{b.period.from.slice(0, 10)} – {b.period.to.slice(0, 10)}</td>
                    <td>{b.universe}</td>
                    <td>{b.timeframe}</td>
                    <td className="num">{b.sampleSize}</td>
                    <td>{b.costs}</td>
                    <td>{b.benchmark}</td>
                    <td className="num">{fmtDrawdown(b.maxDrawdown)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      <Drawer open={open !== null} title={open ? `Backtest ${open.model}` : "Backtest"} onClose={() => update({ run: null })}>
        {open && (
          <div className="ws-drawer-body">
            <h3 className="card-heading">Metrics</h3>
            {open.metrics.map((m) => (
              <div key={m.label} className="quote-detail"><span>{m.label}</span><strong>{m.value}</strong></div>
            ))}
            <h3 className="card-heading section">Methodology</h3>
            <p>{open.methodology}</p>
            <h3 className="card-heading section">Costs assumed</h3>
            <p>{open.costsAssumed}</p>
            <div className="deposit-actions">
              {open.methodologyUrl ? (
                <a className="button button-secondary" href={open.methodologyUrl} target="_blank" rel="noreferrer">View methodology</a>
              ) : (
                <button type="button" className="button button-secondary" disabled>View methodology</button>
              )}
            </div>
          </div>
        )}
      </Drawer>
    </>
  );
}
