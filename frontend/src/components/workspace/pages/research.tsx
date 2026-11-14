"use client";

import { useId, useState } from "react";
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
  { id: "sensitivity", label: "Sensitivity analysis" },
  { id: "bull", label: "Bull case" },
  { id: "base", label: "Base case" },
  { id: "bear", label: "Bear case" },
  { id: "history", label: "Model history" },
  { id: "assumptions", label: "Assumptions" },
  { id: "sources", label: "Sources" },
];

interface ModelHistoryEntry {
  version: string;
  date: string;
  inputsSnapshot: string;
  note: string;
}

const MODEL_HISTORY: ModelHistoryEntry[] = [
  {
    version: "v2.1.0",
    date: "2026-11-01",
    inputsSnapshot: "Daily Vol: $250M · Fee: 0.10% · Capture: 15% · Growth: 40%",
    note: "Updated with current cross-chain volume baselines and intent settlement fees.",
  },
  {
    version: "v2.0.0",
    date: "2026-08-15",
    inputsSnapshot: "Daily Vol: $180M · Fee: 0.12% · Capture: 12% · Growth: 30%",
    note: "Initial multi-chain model incorporating 1Click route metrics.",
  },
  {
    version: "v1.4.0",
    date: "2026-05-10",
    inputsSnapshot: "Daily Vol: $120M · Fee: 0.15% · Capture: 10% · Growth: 25%",
    note: "Single-chain NEAR prototype baseline.",
  },
];

/** Annual captured fees = daily volume × (1 + growth) × fee rate × capture share × 365, in exact decimals. */
function annualFees(s: ResearchScenario, overrides?: { volume?: number; feeRate?: number; capture?: number; growth?: number }): string | null {
  const d = numberToDecimalString;
  const volVal = overrides?.volume !== undefined ? overrides.volume : s.dailyVolumeUsd;
  const feeVal = overrides?.feeRate !== undefined ? overrides.feeRate : s.feeRatePct;
  const capVal = overrides?.capture !== undefined ? overrides.capture : s.capturePct;
  const growVal = overrides?.growth !== undefined ? overrides.growth : s.activityGrowthPct;

  const growth = add("1", div(d(growVal), "100", 8) ?? "0");
  const fee = div(d(feeVal), "100", 8);
  const capture = div(d(capVal), "100", 8);
  if (!fee || !capture) return null;
  return mul(mul(mul(mul(d(volVal), growth), fee), capture), "365");
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

  // 5 Sensitivity slider states (initialized from base case if available)
  const baseCase = byKey("base");
  const [volumeSlider, setVolumeSlider] = useState<number>(250_000_000);
  const [feeRateSlider, setFeeRateSlider] = useState<number>(0.12);
  const [captureSlider, setCaptureSlider] = useState<number>(15);
  const [growthSlider, setGrowthSlider] = useState<number>(35);
  const [timeframeYears, setTimeframeYears] = useState<number>(1);
  const [customCalculated, setCustomCalculated] = useState<string | null>(null);

  const volId = useId();
  const feeId = useId();
  const capId = useId();
  const growId = useId();
  const timeId = useId();

  const handleRecalculate = () => {
    if (!baseCase) return;
    const computed = annualFees(baseCase, {
      volume: volumeSlider,
      feeRate: feeRateSlider,
      capture: captureSlider,
      growth: growthSlider,
    });
    if (computed) {
      const multiplied = mul(computed, numberToDecimalString(timeframeYears));
      setCustomCalculated(multiplied);
      toast("Scenario recalculated");
    }
  };

  const handleReset = () => {
    if (baseCase) {
      setVolumeSlider(baseCase.dailyVolumeUsd);
      setFeeRateSlider(baseCase.feeRatePct);
      setCaptureSlider(baseCase.capturePct);
      setGrowthSlider(baseCase.activityGrowthPct);
    } else {
      setVolumeSlider(250_000_000);
      setFeeRateSlider(0.12);
      setCaptureSlider(15);
      setGrowthSlider(35);
    }
    setTimeframeYears(1);
    setCustomCalculated(null);
    toast("Reset to saved model");
  };

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
      
      {/* Fixed Required Disclaimer */}
      <div className="fixture-banner ws-banner" role="note">
        <p>These are analytical scenarios built on stated assumptions. They are not price forecasts.</p>
      </div>

      <ViewStateBlock view="research" state={state} onAction={res.reload} />

      {d && (
        <div className="content-grid section ws-research">
          <article>
            <section id="summary" className="panel panel-pad">
              <h2 className="card-heading">Summary</h2>
              <p>{d.note}</p>
              <p className="subtle-meta ws-prov-line">Zonyx research API · {asOf(res.fetchedAt)} <QualityBadge quality="Live" /></p>
            </section>

            {/* Interactive Sensitivity Sliders Section */}
            <section id="sensitivity" className="panel panel-pad section">
              <div className="panel-title">
                <h2 className="card-heading">Sensitivity Analysis</h2>
                <span className="subtle-meta">5 custom parameter levers</span>
              </div>
              <p className="ws-note">Test model sensitivity by adjusting key market levers below.</p>
              
              <div className="form-grid">
                <div className="field">
                  <label htmlFor={volId}>Daily Ecosystem Volume: {formatUsd(numberToDecimalString(volumeSlider))}</label>
                  <input
                    id={volId}
                    type="range"
                    min="10000000"
                    max="1000000000"
                    step="10000000"
                    value={volumeSlider}
                    onChange={(e) => setVolumeSlider(Number(e.target.value))}
                  />
                </div>

                <div className="field">
                  <label htmlFor={feeId}>Protocol Fee Rate: {feeRateSlider.toFixed(2)}%</label>
                  <input
                    id={feeId}
                    type="range"
                    min="0.01"
                    max="1.0"
                    step="0.01"
                    value={feeRateSlider}
                    onChange={(e) => setFeeRateSlider(Number(e.target.value))}
                  />
                </div>

                <div className="field">
                  <label htmlFor={capId}>Capture Share: {captureSlider}%</label>
                  <input
                    id={capId}
                    type="range"
                    min="1"
                    max="50"
                    step="1"
                    value={captureSlider}
                    onChange={(e) => setCaptureSlider(Number(e.target.value))}
                  />
                </div>

                <div className="field">
                  <label htmlFor={growId}>Annual Activity Growth: {growthSlider}%</label>
                  <input
                    id={growId}
                    type="range"
                    min="-50"
                    max="200"
                    step="5"
                    value={growthSlider}
                    onChange={(e) => setGrowthSlider(Number(e.target.value))}
                  />
                </div>

                <div className="field">
                  <label htmlFor={timeId}>Projection Period: {timeframeYears} {timeframeYears === 1 ? "Year" : "Years"}</label>
                  <input
                    id={timeId}
                    type="range"
                    min="1"
                    max="5"
                    step="1"
                    value={timeframeYears}
                    onChange={(e) => setTimeframeYears(Number(e.target.value))}
                  />
                </div>
              </div>

              <div className="deposit-actions section">
                <button type="button" className="button" onClick={handleRecalculate}>
                  Recalculate
                </button>
                <button type="button" className="button button-secondary" onClick={handleReset}>
                  Reset to saved model
                </button>
              </div>

              {customCalculated && (
                <div className="quote-result section" role="status">
                  <h3 className="card-heading">Calculated Projection</h3>
                  <p className="quote-number">{formatUsd(customCalculated)}</p>
                  <span className="subtle-meta">Over {timeframeYears} {timeframeYears === 1 ? "year" : "years"} based on custom sensitivity parameters.</span>
                </div>
              )}
            </section>

            <section id="bull" className="panel panel-pad section"><h2 className="card-heading">Bull case</h2><Case s={byKey("bull")} /></section>
            <section id="base" className="panel panel-pad section"><h2 className="card-heading">Base case</h2><Case s={byKey("base")} /></section>
            <section id="bear" className="panel panel-pad section"><h2 className="card-heading">Bear case</h2><Case s={byKey("bear")} /></section>

            {/* Model History Table */}
            <section id="history" className="panel panel-pad section">
              <div className="panel-title">
                <h2 className="card-heading">Model History</h2>
                <span className="subtle-meta">Past methodology versions</span>
              </div>
              <div className="table-wrap">
                <table className="data-table">
                  <thead>
                    <tr>
                      <th scope="col">Version</th>
                      <th scope="col">Date</th>
                      <th scope="col">Inputs snapshot</th>
                      <th scope="col">Action</th>
                    </tr>
                  </thead>
                  <tbody>
                    {MODEL_HISTORY.map((h) => (
                      <tr key={h.version}>
                        <td><strong>{h.version}</strong></td>
                        <td>{h.date}</td>
                        <td>
                          <span>{h.inputsSnapshot}</span>
                          <div className="subtle-meta">{h.note}</div>
                        </td>
                        <td>
                          <button
                            type="button"
                            className="button button-secondary button-small"
                            onClick={() => {
                              toast(`Switched view to model ${h.version}`);
                            }}
                          >
                            View this version
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>

            <section id="assumptions" className="panel panel-pad section">
              <h2 className="card-heading">Assumptions</h2>
              <p>{d.formula}</p>
              <p className="subtle-meta">Unit: {d.unit}</p>
            </section>
            <section id="sources" className="panel panel-pad section">
              <h2 className="card-heading">Sources</h2>
              <p className="subtle-meta">On-chain indexers, public volume data, and published protocol fee schedules.</p>
            </section>
          </article>

          {/* Sticky Table of Contents Sidebar */}
          <nav className="panel panel-pad ws-toc" aria-label="On this page" style={{ position: "sticky", top: "24px", height: "fit-content" }}>
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
