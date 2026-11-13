"use client";

import Link from "next/link";
import { api } from "../../../lib/api-client";
import { formatPercent } from "../../../lib/decimal";
import { formatTime } from "../../../lib/format";
import { qualityFromStatus } from "../../../lib/quality";
import { deriveViewState } from "../../../lib/view-state";
import { useResource, useUrlParams } from "../hooks";
import { Drawer, FilterSelect, PageHeader, Pager, Provenance, ScoreCell, SearchField, ViewStateBlock } from "../ui";
import { useWorkspace } from "../workspace-context";
import { OUTCOME_LABEL } from "./trading-asset";

const KEYS = ["asset", "direction", "timeframe", "outcome", "from", "to", "page"] as const;

export function TradingHistoryPage() {
  const { online, chainName } = useWorkspace();
  const { get, update } = useUrlParams();
  const apiParams = new URLSearchParams();
  KEYS.forEach((k) => get(k) && apiParams.set(k, get(k)));
  const res = useResource(`history:${apiParams.toString()}`, () => api.tradingHistory(apiParams));
  const d = res.data;
  const open = d?.items.find((i) => i.id === get("signal")) ?? null;

  const state = deriveViewState({
    loading: res.loading,
    online,
    error: res.error,
    hasData: d !== null,
    isEmpty: d?.items.length === 0,
    stale: d?.quality === "stale",
    partial: d?.quality === "partial",
  });

  return (
    <>
      <Link className="button button-quiet button-small" href="/workspace/trading">← Scanner</Link>
      <PageHeader title="Signal journal" sub="Every signal and what happened next." testData={Boolean(d?.testData)} />

      <div className="ws-filters">
        <SearchField label="Asset" placeholder="Search asset" value={get("asset")} onChange={(v) => update({ asset: v })} />
        <FilterSelect label="Direction" value={get("direction")} options={[{ value: "bullish", label: "Bullish" }, { value: "bearish", label: "Bearish" }]} onChange={(v) => update({ direction: v })} />
        {d && d.filters.timeframe.length > 0 && (
          <FilterSelect label="Timeframe" value={get("timeframe")} options={d.filters.timeframe.map((v) => ({ value: v, label: v }))} onChange={(v) => update({ timeframe: v })} />
        )}
        <FilterSelect label="Outcome" value={get("outcome")} options={Object.entries(OUTCOME_LABEL).map(([value, label]) => ({ value, label }))} onChange={(v) => update({ outcome: v })} />
        <div className="field ws-filter">
          <label htmlFor="sj-from">From date</label>
          <input id="sj-from" type="date" value={get("from")} onChange={(e) => update({ from: e.target.value })} />
        </div>
        <div className="field ws-filter">
          <label htmlFor="sj-to">To date</label>
          <input id="sj-to" type="date" value={get("to")} onChange={(e) => update({ to: e.target.value })} />
        </div>
      </div>

      <ViewStateBlock view="trading-history" state={state} staleTime={d?.asOf ?? undefined} onAction={res.reload} />

      {d && d.aggregate && d.aggregate.methodology && d.aggregate.sampleSize > 0 && (
        <p className="notice-block">
          {d.aggregate.label}: {d.aggregate.value} · Sample size {d.aggregate.sampleSize} · {d.aggregate.methodology}
        </p>
      )}

      {d && d.items.length > 0 && (
        <section className="panel panel-pad" aria-label="Signals">
          <div className="panel-title">
            <h2>Signals</h2>
            <Provenance source={d.source} time={d.asOf} quality={qualityFromStatus(d.quality)} />
          </div>
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th scope="col">Generated</th>
                  <th scope="col">Asset</th>
                  <th scope="col">Direction</th>
                  <th scope="col">Score</th>
                  <th scope="col">Timeframe</th>
                  <th scope="col">Model</th>
                  <th scope="col">Outcome</th>
                  <th scope="col" className="num">MFE</th>
                  <th scope="col" className="num">MAE</th>
                </tr>
              </thead>
              <tbody>
                {d.items.map((h) => (
                  <tr key={h.id}>
                    <td>
                      <button type="button" className="ws-row-btn" onClick={() => update({ signal: h.id, page: get("page") || null })} aria-label={`Open signal timeline for ${h.symbol}`}>
                        {formatTime(h.generatedAt)}
                      </button>
                    </td>
                    <td><strong>{h.symbol}</strong> <span className="subtle-meta">{chainName(h.chain)}</span></td>
                    <td>{h.direction === "bullish" ? "Bullish" : "Bearish"}</td>
                    <td><ScoreCell score={h.score} /></td>
                    <td>{h.timeframe}</td>
                    <td>{h.modelVersion}</td>
                    <td>{OUTCOME_LABEL[h.outcome]}</td>
                    <td className="num">{h.mfe ? formatPercent(h.mfe, 2, true) : "Not available"}</td>
                    <td className="num">{h.mae ? formatPercent(h.mae, 2, true) : "Not available"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pager page={d.page} pageSize={d.pageSize} total={d.total} onPage={(p) => update({ page: String(p) })} />
        </section>
      )}

      <Drawer open={open !== null} title="Signal timeline" onClose={() => update({ signal: null, page: get("page") || null })}>
        {open && (
          <div className="ws-drawer-body">
            <p><strong>{open.symbol}</strong> · {open.direction === "bullish" ? "Bullish" : "Bearish"} · {open.timeframe} · Model {open.modelVersion}</p>
            <ol className="swap-timeline">
              {open.timeline.map((e, i) => (
                <li key={`${e.at}-${e.label}`} className={i === open.timeline.length - 1 ? "current" : undefined}>
                  <strong>{e.label}</strong>
                  <small>{formatTime(e.at)}</small>
                </li>
              ))}
            </ol>
          </div>
        )}
      </Drawer>
    </>
  );
}
