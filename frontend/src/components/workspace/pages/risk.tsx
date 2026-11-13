"use client";

import { api } from "../../../lib/api-client";
import { bpsToPercent, formatPercent } from "../../../lib/decimal";
import { formatTime } from "../../../lib/format";
import { deriveViewState } from "../../../lib/view-state";
import { useResource } from "../hooks";
import { PageHeader, Provenance, ViewStateBlock } from "../ui";
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

  const providers: Array<{ name: string; capability: string; status: Health; last: string | null }> = [
    { name: "NEAR Intents 1Click", capability: "Swap quotes and status", status: r ? fromRiskStatus(r.status) : "Unavailable", last: r?.lastSuccessfulPoll ?? null },
    { name: "DefiLlama", capability: "Market data", status: marketHealth, last: markets.data && !markets.data.testData ? markets.data.retrievedAt ?? null : null },
    { name: "Zonyx trading API", capability: "Signals and scenarios", status: tradingHealth, last: trading.data && !trading.data.testData ? trading.data.asOf : null },
  ];

  return (
    <>
      <PageHeader title="Risk Center" sub="What could go wrong, and what we show you." />
      <ViewStateBlock view="risk" state={state} staleTime={r?.catalogueLastRefreshed ? formatTime(r.catalogueLastRefreshed) : undefined} onAction={risk.reload} />

      <div className="two-column">
        <section className="panel panel-pad" aria-labelledby="swap-risk">
          <div className="panel-title"><h2 id="swap-risk">Swap risk</h2></div>
          <div className="risk-list">
            <Item title="Slippage">
              {r ? `Quotes are capped at ${formatPercent(bpsToPercent(r.maxSlippageBps))} max slippage. Above ${formatPercent(bpsToPercent(r.warningSlippageBps))} the swap page shows a high-slippage warning.` : "Not available"}
            </Item>
            <Item title="Quote expiry">Quotes expire. The swap page counts down, refreshes an expired quote and disables review until a fresh quote arrives.</Item>
            <Item title="Liquidity">When the provider flags low liquidity on a route, the quote shows a warning before review.</Item>
            <Item title="Stale data">
              {r ? `Catalogue last refreshed ${formatTime(r.catalogueLastRefreshed)}. ${r.catalogueStale ? "It is currently stale, so swaps are disabled until it refreshes." : "It is currently fresh."}` : "Not available"}
            </Item>
          </div>
        </section>

        <section className="panel panel-pad" aria-labelledby="trading-risk">
          <div className="panel-title"><h2 id="trading-risk">Trading risk</h2></div>
          <div className="risk-list">
            <Item title="Invalidation">Every scenario states the level that proves it wrong. A setup past its invalidation is no longer valid.</Item>
            <Item title="Volatility">Fast markets can move through entry areas and targets between data updates.</Item>
            <Item title="Liquidity">Thin order books widen spreads, so displayed levels may not be reachable at size.</Item>
            <Item title="Model risk">Scores are ranking scores from versioned rules, not probabilities. Rules can miss conditions they were not built for.</Item>
          </div>
        </section>
      </div>

      <section className="panel panel-pad section" aria-labelledby="provider-health">
        <div className="panel-title">
          <h2 id="provider-health">Provider health</h2>
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
                  <td>{p.name}</td>
                  <td>{p.capability}</td>
                  <td data-label="Status"><HealthPill status={p.status} /></td>
                  <td data-label="Latency">Not available</td>
                  <td data-label="Last success">{formatTime(p.last)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <p className="notice-block section">An outage means data is unavailable. It is not a bearish signal.</p>
      </section>
    </>
  );
}
