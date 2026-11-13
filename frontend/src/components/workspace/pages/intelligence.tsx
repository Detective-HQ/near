"use client";

import Link from "next/link";
import { useId } from "react";
import { api } from "../../../lib/api-client";
import { deriveViewState } from "../../../lib/view-state";
import { useResource, useUrlParams } from "../hooks";
import { InfoTip, PageHeader, Provenance, ViewStateBlock } from "../ui";
import { useWorkspace } from "../workspace-context";
import { metricQuality, metricTime, metricValue } from "./markets";

/** Chains the API serves intelligence for. Others show the empty state. */
const SERVED = new Set(["near"]);

export function IntelligencePage() {
  const { chains, online, chainName } = useWorkspace();
  const { get, update } = useUrlParams();
  const selectId = useId();
  const chain = get("chain") || "near";
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

  return (
    <>
      <PageHeader
        title="Intelligence"
        sub="Network activity and ecosystem health."
        testData={Boolean(res.data?.testData)}
        actions={<Link className="button button-secondary" href="/workspace/markets">See market intelligence</Link>}
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

      <ViewStateBlock view="intelligence" state={state} staleTime={res.data?.retrievedAt ?? undefined} onAction={res.reload} />

      {served && metrics.length > 0 && (
        <section className="panel panel-pad" aria-label={`${chainName(chain)} metrics`}>
          <div className="ecosystem-list">
            {metrics.map((m) => (
              <div key={m.key} className="ecosystem-row ws-metric-row">
                <div className="row-title">
                  <strong>
                    {m.label} <InfoTip label={`Definition of ${m.label}`} text={m.definition} />
                  </strong>
                  <small>{m.scope}</small>
                </div>
                <span>{metricValue(m)}</span>
                <Provenance source={m.source} time={metricTime(m)} quality={metricQuality(m)} />
              </div>
            ))}
          </div>
        </section>
      )}
    </>
  );
}
