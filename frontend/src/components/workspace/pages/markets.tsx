"use client";

import Link from "next/link";
import { useState } from "react";
import { api } from "../../../lib/api-client";
import type { MetricObservation } from "../../../lib/contracts";
import { formatDecimal, formatPercent, formatUsd, numberToDecimalString } from "../../../lib/decimal";
import { qualityFromStatus, regimeLabel } from "../../../lib/quality";
import { deriveViewState } from "../../../lib/view-state";
import { useResource } from "../hooks";
import { PageHeader, Provenance, TestDataTag, ViewStateBlock } from "../ui";
import { useWorkspace } from "../workspace-context";

export function metricValue(m: MetricObservation | undefined): string {
  if (!m || m.value === null || m.value === undefined) return "Not available";
  const v = numberToDecimalString(m.value);
  if (!v) return "Not available";
  return m.unit.toUpperCase().startsWith("USD") ? formatUsd(v) : `${formatDecimal(v, 2)} ${m.unit}`;
}

export function metricTime(m: MetricObservation | undefined): string | null {
  return m?.observedAt ?? m?.retrievedAt ?? m?.capturedAt ?? null;
}

export function metricQuality(m: MetricObservation | undefined) {
  return qualityFromStatus(m?.status ?? "available", Boolean(m && m.value !== null && m.value !== undefined));
}

function find(metrics: MetricObservation[], key: string) {
  return metrics.find((m) => m.key === key || m.key.includes(key));
}

const CARDS: Array<{ label: string; keys: string[] }> = [
  { label: "Price", keys: ["price"] },
  { label: "TVL", keys: ["tvl"] },
  { label: "DEX volume", keys: ["dex_volume"] },
  { label: "Stablecoin supply", keys: ["stablecoin_supply"] },
  { label: "Fees & revenue", keys: ["fees", "revenue"] },
];

export function MarketsPage() {
  const { online } = useWorkspace();
  const [refresh, setRefresh] = useState(false);
  const snap = useResource(`markets:${refresh}`, () => api.markets(refresh));
  const overview = useResource("trading-overview", api.tradingOverview);
  const nearAsset = useResource("near-asset", () => api.tradingAsset("near:near"));

  const rawMetrics = snap.data?.metrics ?? [];
  const priceMetric: MetricObservation | null = nearAsset.data?.lastPrice ? {
    key: "near_price",
    label: "NEAR / USD",
    value: parseFloat(nearAsset.data.lastPrice),
    unit: "USD",
    scope: "Near ecosystem",
    definition: "Current market price for NEAR token.",
    source: nearAsset.data.source || "DeFiLlama",
    observedAt: nearAsset.data.asOf,
    status: "available",
  } : null;

  const metrics = priceMetric ? [priceMetric, ...rawMetrics] : rawMetrics;
  const cards = CARDS.map((c) => ({ ...c, items: c.keys.map((k) => find(metrics, k)).filter(Boolean) as MetricObservation[] }));
  const state = deriveViewState({
    loading: snap.loading,
    online,
    error: snap.error,
    hasData: snap.data !== null,
    stale: snap.data?.status === "stale",
    partial: cards.some((c) => c.items.length === 0 || c.items.some((m) => m.value === null)),
  });

  const o = overview.data;
  const regime = overview.error || !o ? "Unavailable" : regimeLabel(o.regime);
  const coverage = o?.coverage ?? null;

  return (
    <>
      <PageHeader
        title="Markets"
        sub="Prices, liquidity and market breadth."
        testData={Boolean(snap.data?.testData)}
        actions={<Link className="button" href="/workspace/trading">Open scanner</Link>}
      />
      <ViewStateBlock
        view="markets"
        state={state}
        staleTime={snap.data?.retrievedAt ?? snap.data?.capturedAt ?? undefined}
        onAction={() => {
          if (state === "stale") setRefresh(true);
          snap.reload();
        }}
      />

      {snap.data && (
        <div className="ws-cards">
          {cards.map((c) => (
            <section key={c.label} className="panel panel-pad ws-card" aria-label={c.label}>
              <span className="metric-label">{c.label}</span>
              {c.items.length === 0 ? (
                <>
                  <span className="metric-value">Not available</span>
                  <Provenance source={null} time={null} quality="Unavailable" />
                </>
              ) : (
                c.items.map((m) => (
                  <div key={m.key} className="ws-card-metric">
                    {c.items.length > 1 && <span className="subtle-meta">{m.label}</span>}
                    <span className="metric-value">{metricValue(m)}</span>
                    <Provenance source={m.source} time={metricTime(m)} quality={metricQuality(m)} />
                  </div>
                ))
              )}
            </section>
          ))}
        </div>
      )}

      <section className="panel panel-pad section" aria-label="Market regime">
        <div className="panel-title">
          <h2>Market regime</h2>
          {o?.testData && <TestDataTag />}
        </div>
        {overview.loading && !o ? (
          <p className="subtle-meta">Loading</p>
        ) : (
          <>
            <p className="metric-value">{regime}</p>
            {o && regime !== "Unavailable" && o.breadth && (
              <p>{o.breadth.bullish} bullish · {o.breadth.neutral} neutral · {o.breadth.bearish} bearish</p>
            )}
            {o && o.universe && (
              <p className="subtle-meta">
                Scanned {o.universe.scanned} of {o.universe.total} assets ({coverage ? formatPercent(coverage, 1) : "Not available"} coverage)
              </p>
            )}
            {o ? (
              <Provenance source={o.source} time={o.asOf} quality={qualityFromStatus(o.quality)} />
            ) : (
              <Provenance source={null} time={null} quality="Unavailable" />
            )}
            <div className="deposit-actions">
              {o?.regimeMethodologyUrl ? (
                <a className="button button-secondary button-small" href={o.regimeMethodologyUrl} target="_blank" rel="noreferrer">How this is calculated</a>
              ) : (
                <button type="button" className="button button-secondary button-small" disabled>How this is calculated</button>
              )}
            </div>
          </>
        )}
      </section>
    </>
  );
}
