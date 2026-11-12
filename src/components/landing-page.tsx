"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { api } from "../lib/api-client";
import type { DataSnapshot, MetricObservation, SeriesPoint } from "../lib/contracts";
import { displaySource, formatMetric, formatTimestamp } from "../lib/format";
import { serializeSnapshotCsv, serializeSnapshotJson, snapshotFilename } from "../lib/snapshot-export";

type LandingMetric = {
  key: string;
  label: string;
  aliases: string[];
  seriesAliases: string[];
};

const LANDING_METRICS: LandingMetric[] = [
  {
    key: "active_accounts",
    label: "Active accounts",
    aliases: ["active_accounts", "near_active_accounts"],
    seriesAliases: ["activeAccounts", "active_accounts", "near_active_accounts"]
  },
  {
    key: "transactions",
    label: "Transactions",
    aliases: ["transactions", "near_transactions"],
    seriesAliases: ["transactions", "near_transactions"]
  },
  {
    key: "tvl",
    label: "TVL",
    aliases: ["tvl", "near_tvl"],
    seriesAliases: ["tvl", "near_tvl"]
  },
  {
    key: "dex_volume",
    label: "DEX volume",
    aliases: ["dex_volume", "dex_volume_24h"],
    seriesAliases: ["dex_volume", "dex_volume_24h"]
  },
  {
    key: "stablecoin_supply",
    label: "Stablecoins",
    aliases: ["stablecoin_supply", "near_stablecoin_supply"],
    seriesAliases: ["stablecoin_supply", "near_stablecoin_supply"]
  },
  {
    key: "fees",
    label: "Fees",
    aliases: ["fees", "protocol_fees"],
    seriesAliases: ["fees", "protocol_fees"]
  }
];

function findMetric(snapshot: DataSnapshot | null, aliases: string[]): MetricObservation | undefined {
  return snapshot?.metrics.find((metric) => aliases.includes(metric.key));
}

function findSeriesKey(points: SeriesPoint[], aliases: string[]): string | undefined {
  return aliases.find((key) => points.some((point) => typeof point[key] === "number"));
}

function downloadSnapshot(snapshot: DataSnapshot, format: "csv" | "json") {
  const content = format === "json" ? serializeSnapshotJson(snapshot) : serializeSnapshotCsv(snapshot);
  const blob = new Blob([content], { type: format === "json" ? "application/json" : "text/csv" });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = snapshotFilename(snapshot, format);
  anchor.click();
  URL.revokeObjectURL(url);
}

function MetricChart({ points, seriesKey }: { points: SeriesPoint[]; seriesKey: string | undefined }) {
  const chart = useMemo(() => {
    if (!seriesKey) return null;
    const usable = points.flatMap((point) => {
      const value = point[seriesKey];
      return typeof value === "number" && Number.isFinite(value) ? [{ date: point.date, value }] : [];
    });
    if (usable.length < 2) return null;
    const values = usable.map((point) => point.value);
    const low = Math.min(...values);
    const high = Math.max(...values);
    const span = high - low || Math.max(Math.abs(high) * 0.08, 1);
    const left = 10;
    const right = 630;
    const top = 12;
    const bottom = 116;
    const coordinates = usable.map((point, index) => ({
      ...point,
      x: left + (right - left) * index / (usable.length - 1),
      y: bottom - (point.value - low) / span * (bottom - top)
    }));
    return {
      line: coordinates.map((point, index) => `${index === 0 ? "M" : "L"} ${point.x.toFixed(1)} ${point.y.toFixed(1)}`).join(" "),
      firstDate: coordinates[0].date,
      middleDate: coordinates[Math.floor(coordinates.length / 2)].date,
      lastDate: coordinates[coordinates.length - 1].date
    };
  }, [points, seriesKey]);

  if (!chart) {
    return <p className="chart-caption">A comparable time series is not available for this metric.</p>;
  }

  return (
    <div className="metric-chart-wrap">
      <p className="chart-caption">Observed series · no forecast</p>
      <div id="metric-chart" aria-label="Metric history chart">
        <svg viewBox="0 0 640 132" role="img" aria-label={`Historical ${seriesKey} values`}>
          <path className="chart-axis" d="M 10 117.5 H 630" />
          <path className="chart-line" d={chart.line} />
        </svg>
      </div>
      <div className="chart-caption chart-range">
        <span>{chart.firstDate}</span>
        <span>{chart.middleDate}</span>
        <span>{chart.lastDate}</span>
      </div>
    </div>
  );
}

function BrandSymbol() {
  return (
    <span className="brand-symbol" aria-hidden="true">
      <i />
      <i />
      <i />
    </span>
  );
}

export function LandingPage() {
  const [snapshot, setSnapshot] = useState<DataSnapshot | null>(null);
  const [selectedKey, setSelectedKey] = useState("active_accounts");
  const [loadError, setLoadError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    void api.intelligence()
      .then((data) => {
        if (!active) return;
        setSnapshot(data);
        const matchingDefault = LANDING_METRICS.find((entry) => findMetric(data, entry.aliases));
        if (matchingDefault) setSelectedKey(matchingDefault.key);
      })
      .catch((error: unknown) => {
        if (active) setLoadError(error instanceof Error ? error.message : "Metric source is unavailable.");
      });

    const root = document.documentElement;
    root.classList.add("js");
    const revealTargets = Array.from(document.querySelectorAll<HTMLElement>("[data-reveal]"));
    let observer: IntersectionObserver | null = null;
    if ("IntersectionObserver" in window) {
      observer = new IntersectionObserver((entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            entry.target.classList.add("is-in");
            observer?.unobserve(entry.target);
          }
        }
      }, { rootMargin: "0px 0px -6% 0px" });
    }
    for (const target of revealTargets) observer?.observe(target);
    for (const target of revealTargets) {
      if (!observer) target.classList.add("is-in");
    }

    return () => {
      active = false;
      observer?.disconnect();
      root.classList.remove("js");
    };
  }, []);

  const selected = LANDING_METRICS.find((entry) => entry.key === selectedKey) ?? LANDING_METRICS[0];
  const metric = findMetric(snapshot, selected.aliases);
  const seriesKey = snapshot ? findSeriesKey(snapshot.series, selected.seriesAliases) : undefined;
  const mode = snapshot?.mode ?? "fixture";
  const unit = metric?.unit ?? "";
  const formattedValue = metric
    ? formatMetric(metric.value, unit.toLowerCase().startsWith("usd") ? "USD" : unit)
    : "—";

  return (
    <div className="marketing-page">
      <a className="skip-link" href="#main-content">Skip to content</a>
      <header className="site-header">
        <Link className="wordmark" href="/" aria-label="Universal DeFi Intelligence home">
          <BrandSymbol />
          <span>Universal DeFi<br />Intelligence</span>
        </Link>
        <nav className="site-nav" aria-label="Main navigation">
          <a href="#workflow">Workflow</a>
          <a href="#research">Research</a>
          <a href="#limits">Limits</a>
        </nav>
        <Link className="nav-action" href="/workspace">
          Open workspace <span aria-hidden="true">↗</span>
        </Link>
      </header>

      <main id="main-content">
        <section className="hero page-frame">
          <div className="hero-copy">
            <h1>
              <span>Across chains.</span>
              <span>With context.</span>
            </h1>
            <p className="hero-lede">
              Preview exact-input routes, then explore NEAR ecosystem metrics with their source,
              timestamp and method attached.
            </p>
            <div className="hero-actions">
              <Link className="button button-primary" href="/workspace">
                Explore the workspace <span aria-hidden="true">↗</span>
              </Link>
              <a className="text-link" href="#workflow">See what works today</a>
            </div>
            <p className="hero-note">
              Dry quotes only. Wallet execution and settlement checks are disabled.
            </p>
          </div>

          <section className="metric-panel" aria-labelledby="metric-panel-title" data-reveal data-ui>
            <div className="metric-panel-head">
              <div>
                <h2 id="metric-panel-title">NEAR, with its source</h2>
                <p>Read-only ecosystem snapshot</p>
              </div>
              <span className="source-state" data-mode={mode}>
                {snapshot ? (mode === "live" ? "Live source" : "Fixture data") : "Checking source…"}
              </span>
            </div>

            <div className="metric-tabs" role="tablist" aria-label="Choose a NEAR metric">
              {LANDING_METRICS.filter((entry) => findMetric(snapshot, entry.aliases)).slice(0, 4).map((entry) => (
                <button
                  className={`metric-tab${selectedKey === entry.key ? " is-selected" : ""}`}
                  type="button"
                  role="tab"
                  aria-selected={selectedKey === entry.key}
                  key={entry.key}
                  onClick={() => setSelectedKey(entry.key)}
                >
                  {entry.label}
                </button>
              ))}
            </div>

            <div className="metric-reading" role="tabpanel">
              <p className="metric-value">
                {snapshot ? formattedValue : "—"}
                {unit && unit !== "USD" && !unit.toLowerCase().startsWith("usd")
                  ? <span className="metric-unit">{unit}</span>
                  : null}
              </p>
              {snapshot
                ? <MetricChart points={snapshot.series} seriesKey={seriesKey} />
                : <p className="chart-caption">{loadError ?? "Waiting for the configured source."}</p>}
            </div>

            <div className="metric-receipt">
              <p>{metric?.definition ?? "Metric definition will appear with the selected observation."}</p>
              <div className="receipt-meta">
                <span>{metric ? displaySource(mode, metric.source) : "Source details"}</span>
                {metric?.sourceUrl
                  ? <a href={metric.sourceUrl} target="_blank" rel="noreferrer">Source details</a>
                  : null}
                <span>{formatTimestamp(metric?.observedAt ?? snapshot?.retrievedAt ?? snapshot?.capturedAt)}</span>
              </div>
              <p className="metric-caveat">
                {snapshot?.notice ?? loadError ?? "TVL is not available trading liquidity or a token valuation."}
              </p>
            </div>
          </section>
        </section>

        <div className="scope-ribbon" aria-label="Product capability status">
          <p><span className="scope-mark" />Research data includes source and timestamp</p>
          <p><span className="scope-mark" />Quotes are dry-run estimates</p>
          <p><span className="scope-mark scope-mark-muted" />Wallet execution is off</p>
        </div>

        <section className="section-space page-frame" id="workflow">
          <div className="section-intro" data-reveal>
            <h2>See the route before you act.</h2>
            <p>
              The workspace separates quote estimates from chain execution and separates reported
              metrics from research conclusions.
            </p>
          </div>
          <div className="workflow-body">
            <ol className="workflow-list">
              <li className="workflow-row" data-reveal>
                <div>
                  <h3>Discover available assets</h3>
                  <p>View the configured catalogue. In live mode, provider asset IDs are kept exactly as returned.</p>
                </div>
                <span className="workflow-status status-available">Catalogue</span>
              </li>
              <li className="workflow-row" data-reveal>
                <div>
                  <h3>Review an exact-input quote</h3>
                  <p>Compare estimated output, minimum output and the source used to calculate the preview.</p>
                </div>
                <span className="workflow-status status-available">Dry only</span>
              </li>
              <li className="workflow-row" data-reveal>
                <div>
                  <h3>Inspect settlement context</h3>
                  <p>Read ecosystem measures alongside definitions, provenance and data freshness.</p>
                </div>
                <span className="workflow-status status-available">Research</span>
              </li>
            </ol>

            <aside className="workflow-receipt" data-reveal data-ui>
              <div className="receipt-topline">
                <span>Execution boundary</span>
                <span className="boundary-indicator"><i /> Execution off</span>
              </div>
              <h3 className="receipt-title">Your wallet stays yours.</h3>
              <p className="receipt-copy">
                The current build does not request signatures, build deposit transactions, or verify settlement.
              </p>
              <Link href="/workspace/risk">Review execution limits <span aria-hidden="true">↗</span></Link>
            </aside>
          </div>
        </section>

        <section className="research section-space" id="research">
          <div className="page-frame">
            <div className="research-layout">
              <div className="research-sticky" data-reveal>
                <h2>Research with its assumptions attached.</h2>
                <p>
                  Follow NEAR metrics back to their source and test scenario inputs without presenting
                  an output as a token price or expected return.
                </p>
                <div className="research-selector" aria-label="Research sections">
                  <Link className="research-choice is-selected" href="/workspace/intelligence">Metric provenance</Link>
                  <Link className="research-choice" href="/workspace/markets">Market coverage</Link>
                  <Link className="research-choice" href="/workspace/research">Scenario assumptions</Link>
                </div>
              </div>
              <article className="research-output" data-reveal data-ui>
                <div className="output-heading">
                  <span>NEAR ecosystem snapshot</span>
                  <span id="research-status" data-status={snapshot?.status ?? "partial"}>
                    {snapshot?.status === "available" ? "Available" : snapshot ? snapshot.status : "Loading"}
                  </span>
                </div>
                <p
                  className="research-value"
                  aria-label={metric ? `${metric.label}: ${formattedValue}` : "Source coverage pending"}
                >
                  <span>{metric ? formattedValue : "Source pending"}</span>
                  {metric ? <span className="research-value-label">{metric.label}</span> : null}
                </p>
                <p className="research-definition">
                  {metric?.definition ?? "Definitions and source metadata load with the selected observation."}
                </p>
                <dl className="receipt-fields">
                  <div><dt>Scope</dt><dd>{metric?.scope ?? "NEAR ecosystem"}</dd></div>
                  <div><dt>Source</dt><dd>{metric?.source ?? snapshot?.provider ?? "Awaiting source"}</dd></div>
                  <div><dt>Observed</dt><dd>{formatTimestamp(metric?.observedAt ?? snapshot?.retrievedAt)}</dd></div>
                  <div><dt>Method</dt><dd>{metric?.methodologyVersion ?? snapshot?.methodologyVersion ?? "Not specified"}</dd></div>
                </dl>
                <p className="research-warning">
                  Measurements describe activity or reported value. They do not establish token value or investment return.
                </p>
              </article>
            </div>
            <figure className="research-capture" data-reveal>
              <img
                src="/images/near-intelligence-2026-11-10.jpg"
                alt="Illustrative NEAR ecosystem research visualization used by the intelligence workspace"
                loading="lazy"
              />
              <figcaption>Research starts with a defined metric, a source and an observation time.</figcaption>
            </figure>
          </div>
        </section>

        <section className="section-space page-frame" aria-labelledby="capabilities-title">
          <div className="capabilities">
            <div className="capability-heading" data-reveal>
              <h2 id="capabilities-title">A practical research workspace.</h2>
            </div>
            <div className="capability-list">
              <article className="capability-row" data-reveal>
                <h3>Cross-chain quote review</h3>
                <p>Fixture previews or provider dry quotes, with no deposit action in this release.</p>
                <span className="capability-tag">Quote</span>
              </article>
              <article className="capability-row" data-reveal>
                <h3>Market and network context</h3>
                <p>Metrics include units, chain scope, data status, definitions, source and timestamps.</p>
                <span className="capability-tag">Analytics</span>
              </article>
              <article className="capability-row" data-reveal>
                <h3>Reproducible scenarios</h3>
                <p>Change visible assumptions and inspect the formula behind every scenario output.</p>
                <span className="capability-tag">Research</span>
              </article>
              <article className="capability-row" data-reveal>
                <h3>Risk in the product flow</h3>
                <p>Disabled execution, unsupported routes and stale sources are labelled where they matter.</p>
                <span className="capability-tag">Risk</span>
              </article>
              <div className="export-fragment" data-reveal data-ui>
                <p className="export-title">Exportable evidence</p>
                <p>Download a metric snapshot with its provenance and series.</p>
                <div className="export-actions">
                  <button type="button" disabled={!snapshot} onClick={() => snapshot && downloadSnapshot(snapshot, "json")}>
                    Download JSON
                  </button>
                  <button type="button" disabled={!snapshot} onClick={() => snapshot && downloadSnapshot(snapshot, "csv")}>
                    Download CSV
                  </button>
                </div>
                <Link href="/workspace/intelligence">Open intelligence <span aria-hidden="true">↗</span></Link>
              </div>
            </div>
          </div>
        </section>

        <section className="limits section-space" id="limits">
          <div className="limits-inner page-frame">
            <div data-reveal>
              <h2>Research is not execution.</h2>
            </div>
            <div className="limits-copy" data-reveal>
              <p>
                Live 1Click quotes are dry estimates. No wallet transaction is built, signed or broadcast.
                A simulation record is not a settled swap.
              </p>
              <p>
                Fixture metrics are illustrative. Live market sources can be incomplete or stale, and
                reported TVL, volume and fees are not interchangeable.
              </p>
              <Link className="button button-light" href="/workspace/risk">
                Read the risk center <span aria-hidden="true">↗</span>
              </Link>
            </div>
          </div>
        </section>

        <section className="questions section-space page-frame" aria-labelledby="questions-title">
          <div className="question-heading" data-reveal>
            <h2 id="questions-title">Know what happens next.</h2>
          </div>
          <div className="question-list">
            <details data-reveal>
              <summary>Can I send a swap from this interface?</summary>
              <p>No. The current build can show local fixture estimates and optional provider dry quotes, but it cannot construct, sign, or broadcast a deposit transaction.</p>
            </details>
            <details data-reveal>
              <summary>Does a listed chain guarantee a route?</summary>
              <p>No. Asset catalogues and chain configuration are separate from route availability. A pair must return a quote before it can be described as routable.</p>
            </details>
            <details data-reveal>
              <summary>Are the NEAR scenario outputs predictions?</summary>
              <p>No. They are arithmetic over editable assumptions. They are not price targets, investment advice or expected returns.</p>
            </details>
          </div>
        </section>

        <section className="closing page-frame" data-reveal>
          <div>
            <h2>Start with the evidence.</h2>
          </div>
          <Link className="button button-primary" href="/workspace">
            Open the workspace <span aria-hidden="true">↗</span>
          </Link>
        </section>
      </main>

      <footer className="site-footer page-frame">
        <Link className="wordmark" href="/">
          <BrandSymbol />
          <span>Universal DeFi<br />Intelligence</span>
        </Link>
        <nav aria-label="Footer navigation">
          <Link href="/workspace/markets">Markets</Link>
          <Link href="/workspace/intelligence">Intelligence</Link>
          <Link href="/workspace/risk">Risk center</Link>
        </nav>
        <span>Research is separate from execution.</span>
      </footer>
    </div>
  );
}
