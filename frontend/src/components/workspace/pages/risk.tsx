"use client";

import { api } from "../../../lib/api-client";
import { bpsToPercent, formatPercent } from "../../../lib/decimal";
import { formatTime } from "../../../lib/format";
import { qualityFromStatus } from "../../../lib/quality";
import type { Quality } from "../../../lib/quality";
import { deriveViewState } from "../../../lib/view-state";
import { useResource } from "../hooks";
import { PageHeader, Provenance, QualityBadge, ViewStateBlock } from "../ui";
import { useWorkspace } from "../workspace-context";

type Health = "Operational" | "Degraded" | "Unavailable";

function HealthPill({ status }: { status: Health }) {
  const tone = status === "Operational" ? " good" : status === "Degraded" ? " warn" : "";
  return <span className={`state-pill${tone}`}>{status}</span>;
}

function fromRiskStatus(s: string | undefined): Health {
  if (s === "HEALTHY") return "Operational";
  if (s === "DEGRADED") return "Degraded";
  return "Unavailable";
}

function Item({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="risk-row">
      <strong>{title}</strong>
      <p>{children}</p>
      <span />
    </div>
  );
}

export function RiskPage() {
  const { risk, online } = useWorkspace();
  const markets = useResource("risk-markets", () => api.markets());
  const trading = useResource("risk-trading", api.tradingOverview);
  const r = risk.data;

  const state = deriveViewState({ loading: risk.loading, online, error: risk.error, hasData: r !== null, stale: r?.catalogueStale });

  const marketHealth: Health = markets.data
    ? markets.data.testData || markets.data.status === "unavailable"
      ? "Unavailable"
      : markets.data.status === "stale" || markets.data.status === "partial"
        ? "Degraded"
        : "Operational"
    : "Unavailable";
  const tradingHealth: Health = trading.data && !trading.data.testData ? "Operational" : "Unavailable";

  // Only values the API reports. Latency is not exposed by any public endpoint yet.
  const providers: Array<{ name: string; capability: string; status: Health; latency: string; last: string | null }> = [
    { name: "NEAR Intents 1Click", capability: "Swap quotes and settlement", status: r ? fromRiskStatus(r.status) : "Unavailable", latency: "Not available", last: r?.lastSuccessfulPoll ?? null },
    { name: "DeFiLlama", capability: "Market TVL and DEX volumes", status: marketHealth, latency: "Not available", last: markets.data && !markets.data.testData ? markets.data.retrievedAt ?? null : null },
    { name: "Zonyx trading engine", capability: "Signals, order blocks, scenarios", status: tradingHealth, latency: "Not available", last: trading.data && !trading.data.testData ? trading.data.asOf : null },
  ];

  const freshnessRows: Array<{ feed: string; observed: string; source: string; status: Quality }> = [
    {
      feed: "Swap token catalogue",
      observed: r?.catalogueLastRefreshed ? formatTime(r.catalogueLastRefreshed) : "Not available",
      source: "Zonyx API",
      status: r ? (r.catalogueStale ? "Stale" : "Live") : "Unavailable",
    },
    {
      feed: "Market TVL and volume",
      observed: markets.data?.retrievedAt ? formatTime(markets.data.retrievedAt) : "Not available",
      source: markets.data?.provider ?? "DeFiLlama",
      status: markets.data && !markets.data.testData ? qualityFromStatus(markets.data.status ?? "available") : "Unavailable",
    },
    {
      feed: "Trading signals",
      observed: trading.data?.asOf ? formatTime(trading.data.asOf) : "Not available",
      source: trading.data?.source ?? "Trading engine",
      status: trading.data && !trading.data.testData ? qualityFromStatus(trading.data.quality) : "Unavailable",
    },
  ];

  return (
    <>
      <PageHeader title="Risk Center" sub="What could go wrong, and what we show you." />

      {/* Prominent Fixed Notice (Neutral Tone: Outages are never bearish signals) */}
      <div className="fixture-banner ws-banner" role="note">
        <p>An outage means data is unavailable. It is not a bearish signal.</p>
      </div>

      <ViewStateBlock view="risk" state={state} staleTime={r?.catalogueLastRefreshed ? formatTime(r.catalogueLastRefreshed) : undefined} onAction={risk.reload} />

      {/* V1 Swap + V2 Trading Risk Disclosures */}
      <div className="two-column">
        <section className="panel panel-pad" aria-labelledby="swap-risk">
          <div className="panel-title"><h2 id="swap-risk">V1 Swap Risk Disclosures</h2></div>
          <div className="risk-list">
            <Item title="Slippage controls">
              {r ? `Quotes are capped at ${formatPercent(bpsToPercent(r.maxSlippageBps))} max slippage. Above ${formatPercent(bpsToPercent(r.warningSlippageBps))} the swap page displays a high-slippage warning banner.` : "Not available"}
            </Item>
            <Item title="Quote expiry">
              Quotes carry explicit deadlines. The swap page shows a live countdown, automatically refreshes expired quotes, and blocks submission once expired.
            </Item>
            <Item title="Liquidity depth">
              When market maker routes encounter thin liquidity, warning flags are attached to quotes before user review.
            </Item>
            <Item title="Non-custodial deposit safety">
              All transactions are signed client-side from your own wallet. The platform never holds custody or stores private keys.
            </Item>
            <Item title="Catalogue freshness">
              {r ? `Token catalogue last refreshed ${formatTime(r.catalogueLastRefreshed)}. ${r.catalogueStale ? "Currently stale: swap submissions are disabled until a fresh catalogue arrives." : "Currently fresh."}` : "Not available"}
            </Item>
          </div>
        </section>

        <section className="panel panel-pad" aria-labelledby="trading-risk">
          <div className="panel-title"><h2 id="trading-risk">V2 Trading Risk Disclosures</h2></div>
          <div className="risk-list">
            <Item title="Invalidation levels">
              Every scenario defines the precise price level that disproves the setup. A setup that breaches invalidation is terminated immediately.
            </Item>
            <Item title="Fast market volatility">
              Volatile market conditions can sweep entry areas and targets between periodic scan intervals.
            </Item>
            <Item title="Order book depth">
              Thin displayed liquidity can widen execution spreads. Displayed zones reflect detected resting liquidity snapshots.
            </Item>
            <Item title="Model risk & ranking scores">
              Scores are ranking scores (0–100) produced by versioned deterministic rules, not probabilities.
            </Item>
            <Item title="Read-only intelligence">
              Trading intelligence is strictly informational. No trade execution or automatic swap triggers operate within trading views.
            </Item>
          </div>
        </section>
      </div>

      {/* Provider Health Table */}
      <section className="panel panel-pad section" aria-labelledby="provider-health">
        <div className="panel-title">
          <h2 id="provider-health">Provider Health</h2>
          {r && <Provenance source="Zonyx API" time={risk.fetchedAt} quality={r.catalogueStale ? "Stale" : "Live"} />}
        </div>
        <div className="table-wrap">
          <table className="data-table mobile-stack">
            <thead>
              <tr>
                <th scope="col">Provider</th>
                <th scope="col">Capability</th>
                <th scope="col">Status</th>
                <th scope="col">Latency</th>
                <th scope="col">Last success</th>
              </tr>
            </thead>
            <tbody>
              {providers.map((p) => (
                <tr key={p.name}>
                  <td><strong>{p.name}</strong></td>
                  <td>{p.capability}</td>
                  <td data-label="Status"><HealthPill status={p.status} /></td>
                  <td data-label="Latency">{p.latency}</td>
                  <td data-label="Last success">{formatTime(p.last)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      {/* Data Freshness Table */}
      <section className="panel panel-pad section" aria-labelledby="data-freshness">
        <div className="panel-title">
          <h2 id="data-freshness">Data Freshness</h2>
          <span className="subtle-meta">Last update reported by each source</span>
        </div>
        <div className="table-wrap">
          <table className="data-table mobile-stack">
            <thead>
              <tr>
                <th scope="col">Metric / Feed</th>
                <th scope="col">Last updated</th>
                <th scope="col">Source</th>
                <th scope="col">Status</th>
              </tr>
            </thead>
            <tbody>
              {freshnessRows.map((row) => (
                <tr key={row.feed}>
                  <td><strong>{row.feed}</strong></td>
                  <td>{row.observed}</td>
                  <td>{row.source}</td>
                  <td><QualityBadge quality={row.status} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </>
  );
}
