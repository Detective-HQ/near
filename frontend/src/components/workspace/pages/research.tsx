"use client";

import { api } from "../../../lib/api-client";
import type { ResearchScenario } from "../../../lib/contracts";
import { add, div, formatPercent, formatUsd, mul, numberToDecimalString } from "../../../lib/decimal";
import { asOf } from "../../../lib/format";
import { deriveViewState } from "../../../lib/view-state";
import { useResource } from "../hooks";
import { DefRow, PageHeader, QualityBadge, ViewStateBlock } from "../ui";
import { useWorkspace } from "../workspace-context";

const SECTIONS = [
  { id: "summary", label: "Summary" },
  { id: "bull", label: "Bull case" },
  { id: "base", label: "Base case" },
  { id: "bear", label: "Bear case" },
  { id: "assumptions", label: "Assumptions" },
  { id: "sources", label: "Sources" },
];

/** Annual captured fees = daily volume × (1 + growth) × fee rate × capture share × 365, in exact decimals. */
function annualFees(s: ResearchScenario): string | null {
  const d = numberToDecimalString;
  const growth = add("1", div(d(s.activityGrowthPct), "100", 8) ?? "0");
  const fee = div(d(s.feeRatePct), "100", 8);
  const capture = div(d(s.capturePct), "100", 8);
  if (!fee || !capture) return null;
  return mul(mul(mul(mul(d(s.dailyVolumeUsd), growth), fee), capture), "365");
}

function Case({ s }: { s: ResearchScenario | undefined }) {
  if (!s) return <p className="subtle-meta">Not available</p>;
  const fees = annualFees(s);
  return (
    <div className="quote-result">
      <DefRow label="Daily volume">{formatUsd(numberToDecimalString(s.dailyVolumeUsd))}</DefRow>
      <DefRow label="Fee rate">{formatPercent(numberToDecimalString(s.feeRatePct))}</DefRow>
      <DefRow label="Capture share">{formatPercent(numberToDecimalString(s.capturePct), 0)}</DefRow>
      <DefRow label="Activity growth">{formatPercent(numberToDecimalString(s.activityGrowthPct), 0, true)}</DefRow>
      <DefRow label="Annual captured fees">{fees ? formatUsd(fees) : "Not available"}</DefRow>
    </div>
  );
}

export function ResearchPage() {
  const { online, toast } = useWorkspace();
  const res = useResource("research", api.research);
  const d = res.data;
  const byKey = (k: string) => d?.scenarios.find((s) => s.key === k);
  const state = deriveViewState({ loading: res.loading, online, error: res.error, hasData: d !== null, isEmpty: d?.scenarios.length === 0 });

  return (
    <>
      <PageHeader
        title="Research"
        sub="NEAR ecosystem thesis, with sources."
        testData={Boolean(d?.testData)}
        actions={
          <button
            type="button"
            className="button button-secondary"
            onClick={() => void navigator.clipboard?.writeText(window.location.href).then(() => toast("Copied"), () => undefined)}
          >
            Copy link
          </button>
        }
      />
      <p className="notice-block">Research scenarios are separate from trading scenarios.</p>
      <ViewStateBlock view="research" state={state} onAction={res.reload} />

      {d && (
        <div className="content-grid section ws-research">
          <article>
            <section id="summary" className="panel panel-pad">
              <h2 className="card-heading">Summary</h2>
              <p>{d.note}</p>
              <p className="subtle-meta ws-prov-line">Zonyx research API · {asOf(res.fetchedAt)} <QualityBadge quality="Live" /></p>
            </section>
            <section id="bull" className="panel panel-pad section"><h2 className="card-heading">Bull case</h2><Case s={byKey("bull")} /></section>
            <section id="base" className="panel panel-pad section"><h2 className="card-heading">Base case</h2><Case s={byKey("base")} /></section>
            <section id="bear" className="panel panel-pad section"><h2 className="card-heading">Bear case</h2><Case s={byKey("bear")} /></section>
            <section id="assumptions" className="panel panel-pad section">
              <h2 className="card-heading">Assumptions</h2>
              <p>{d.formula}</p>
              <p className="subtle-meta">Unit: {d.unit}</p>
            </section>
            <section id="sources" className="panel panel-pad section">
              <h2 className="card-heading">Sources</h2>
              <p className="subtle-meta">Not available</p>
            </section>
          </article>
          <nav className="panel panel-pad ws-toc" aria-label="On this page">
            <h2 className="card-heading">On this page</h2>
            <ul>
              {SECTIONS.map((s) => <li key={s.id}><a href={`#${s.id}`}>{s.label}</a></li>)}
            </ul>
          </nav>
        </div>
      )}
    </>
  );
}
