"use client";

import Link from "next/link";
import { useId, useState } from "react";
import { api } from "../../../lib/api-client";
import type { DataSnapshot } from "../../../lib/contracts";
import { serializeSnapshotJson, snapshotFilename } from "../../../lib/snapshot-export";
import { deriveViewState } from "../../../lib/view-state";
import { useResource, useUrlParams } from "../hooks";
import { InfoTip, PageHeader, Provenance, Tabs, ViewStateBlock } from "../ui";
import { useWorkspace } from "../workspace-context";
import { metricQuality, metricTime, metricValue } from "./markets";

/** Chains the API serves intelligence for. Others show the empty state. */
const SERVED = new Set(["near"]);
const TIMEFRAMES = ["7D", "30D", "90D", "1Y", "All"] as const;
type Timeframe = (typeof TIMEFRAMES)[number];

function downloadSnapshot(snapshot: DataSnapshot) {
  try {
    const json = serializeSnapshotJson(snapshot);
    const blob = new Blob([json], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = snapshotFilename(snapshot, "json");
    a.click();
    URL.revokeObjectURL(url);
  } catch {}
}

export function IntelligencePage() {
  const { chains, online, chainName, toast } = useWorkspace();
  const { get, update } = useUrlParams();
  const selectId = useId();
  const chain = get("chain") || "near";
  const [timeframe, setTimeframe] = useState<Timeframe>("30D");
  const served = SERVED.has(chain);
  const res = useResource(served ? `intelligence:${chain}` : null, api.intelligence);
  const metrics = res.data?.metrics ?? [];

  const state = served
    ? deriveViewState({
      loading: res.loading,
      online,
      error: res.error,
      hasData: res.data !== null,
      isEmpty: metrics.length === 0,
      stale: res.data?.status === "stale",
      partial: metrics.some((m) => m.value === null),
    })
    : "empty";

  const handleDownload = () => {
    if (!res.data) return;
    downloadSnapshot(res.data);
    toast("Snapshot downloaded");
  };

  return (
    <>
      <PageHeader
        title="Intelligence"
        sub="Network activity and ecosystem health."
        testData={Boolean(res.data?.testData)}
        actions={
          <div className="ws-inline">
            {res.data && (
              <button type="button" className="button button-secondary button-small" onClick={handleDownload}>
                Download snapshot
              </button>
            )}
            <Link className="button button-secondary button-small" href="/workspace/markets">
              See market intelligence
            </Link>
          </div>
        }
      />

      <div className="ws-filters">
        <div className="field ws-filter">
          <label htmlFor={selectId}>Chain</label>
          <select id={selectId} value={chain} onChange={(e) => update({ chain: e.target.value })}>
            <option value="" disabled>Select chain</option>
            {(chains.data ?? []).map((c) => <option key={c.key} value={c.key}>{c.name}</option>)}
          </select>
        </div>
      </div>

      <div className="section">
        <Tabs tabs={TIMEFRAMES} active={timeframe} label="Intelligence timeframe" onChange={(t) => setTimeframe(t)} />
      </div>

      <ViewStateBlock view="intelligence" state={state} staleTime={res.data?.retrievedAt ?? undefined} onAction={res.reload} />

      {served && metrics.length > 0 && (
        <section className="panel panel-pad" aria-label={`${chainName(chain)} metrics (${timeframe})`}>
          <div className="panel-title">
            <h2>{chainName(chain)} metrics</h2>
            <span className="subtle-meta">Showing {timeframe} trend</span>
          </div>
          <div className="ecosystem-list">
            {metrics.map((m) => {
              const isDerived = m.classification === "derived";
              return (
                <div key={m.key} className="ecosystem-row ws-metric-row">
                  <div className="row-title">
                    <div className="ws-inline">
                      <strong>
                        {m.label} <InfoTip label={`Definition of ${m.label}`} text={m.definition} />
                      </strong>
                      <span className={`state-pill${isDerived ? " warn" : ""}`}>
                        {isDerived ? "Derived" : "Raw"}
                      </span>
                    </div>
                    <small>
                      {m.scope}
                      {isDerived && (
                        <>
                          {" · "}
                          <Link href="/workspace/research" className="ws-formula-link" title="View calculation methodology and assumptions">
                            View formula and inputs
                          </Link>
                        </>
                      )}
                    </small>
                  </div>
                  <span>{metricValue(m)}</span>
                  <Provenance source={m.source} time={metricTime(m)} quality={metricQuality(m)} />
                </div>
              );
            })}
          </div>
        </section>
      )}
    </>
  );
}
