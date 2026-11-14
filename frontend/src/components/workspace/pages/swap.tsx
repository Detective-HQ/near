"use client";

import Link from "next/link";
import { useEffect, useId, useMemo, useRef, useState } from "react";
import { api, ApiRequestError, USE_MOCKS } from "../../../lib/api-client";
import type { Catalogue } from "../../../lib/api-client";
import type { SwapQuote, SwapRecord } from "../../../lib/contracts";
import { bpsToPercent, cmp, div, formatDecimal, formatPercent, fromAtomic, percentToBps } from "../../../lib/decimal";
import { sendEvmDeposit } from "../../../lib/evm-wallet";
import { asOf } from "../../../lib/format";
import {
  EXPIRY_WARNING_SECONDS,
  finalLabel,
  isValidAmount,
  PROGRESS_STEPS,
  secondsLeft,
  swapCta,
} from "../../../lib/swap-flow";
import type { ProgressPhase, SwapOutcome } from "../../../lib/swap-flow";
import { runSwap } from "../../../lib/swap-runner";
import { deriveViewState } from "../../../lib/view-state";
import { useDebounced, useNow, useUrlParams } from "../hooks";
import { DefRow, Modal, PageHeader, QualityBadge, TestDataTag, ViewStateBlock } from "../ui";
import { useWorkspace } from "../workspace-context";

type CatalogueAsset = Catalogue["assets"][number];

// Error codes meaning "this pair cannot be quoted" (Go API codes first, legacy names kept).
const ROUTE_ERRORS = new Set([
  "NO_ROUTE", "UNKNOWN_ASSET", "INVALID_ASSET", "INVALID_ASSET_PAIR", "SAME_ASSET",
  "UNSUPPORTED_ROUTE", "ROUTE_NOT_SUPPORTED", "UNSUPPORTED_ASSET", "ASSET_NOT_FOUND", "VALIDATION_FAILED", "INVALID_REQUEST",
]);

function AssetPicker({
  side,
  assets,
  value,
  onChange,
  chainName,
}: {
  side: "From" | "To";
  assets: CatalogueAsset[];
  value: CatalogueAsset | null;
  onChange: (a: CatalogueAsset) => void;
  chainName: (k: string) => string;
}) {
  const [open, setOpen] = useState(false);
  const [chain, setChain] = useState("");
  const [q, setQ] = useState("");
  const chainId = useId();
  const searchId = useId();
  const chains = useMemo(() => Array.from(new Set(assets.map((a) => a.chain))), [assets]);
  const term = q.trim().toLowerCase();
  const list = assets.filter((a) =>
    (!chain || a.chain === chain)
    && (!term || a.symbol.toLowerCase().includes(term) || a.name.toLowerCase().includes(term) || (a.contractAddress ?? "").toLowerCase() === term));

  return (
    <>
      <button type="button" className="button button-secondary ws-picker-btn" aria-label={`${side} asset: ${value ? `${value.symbol} on ${chainName(value.chain)}` : "not selected"}`} onClick={() => setOpen(true)}>
        {value ? (
          <span><strong>{value.symbol}</strong> <span className="subtle-meta">{chainName(value.chain)}</span></span>
        ) : (
          "Select asset"
        )}
      </button>
      <Modal open={open} title={side} onClose={() => setOpen(false)}>
        <div className="form-grid">
          <div className="field">
            <label htmlFor={chainId}>Chain</label>
            <select id={chainId} value={chain} onChange={(e) => setChain(e.target.value)}>
              <option value="">All</option>
              {chains.map((c) => <option key={c} value={c}>{chainName(c)}</option>)}
            </select>
          </div>
          <div className="field">
            <label htmlFor={searchId}>Asset</label>
            <input id={searchId} type="search" placeholder="Search asset or paste contract" value={q} onChange={(e) => setQ(e.target.value)} />
          </div>
        </div>
        <ul className="ws-pick-list">
          {list.length === 0 && <li className="subtle-meta">No matches for &quot;{q.trim()}&quot;</li>}
          {list.map((a) => (
            <li key={a.id}>
              <button
                type="button"
                onClick={() => {
                  onChange(a);
                  setOpen(false);
                }}
              >
                <span><strong>{a.symbol}</strong> {a.name}</span>
                <span className="subtle-meta">{chainName(a.chain)}</span>
              </button>
            </li>
          ))}
        </ul>
      </Modal>
    </>
  );
}

function formatSeconds(s: number | null | undefined): string {
  if (s === null || s === undefined) return "Not available";
  return s < 90 ? `~${s}s` : `~${Math.round(s / 60)} min`;
}

export function SwapPage() {
  const { catalogue, risk, online, wallet, chainName, chainByKey, toast } = useWorkspace();
  const { get, update } = useUrlParams();
  const assets = catalogue.data?.assets ?? [];
  const byId = (id: string) => assets.find((a) => a.id === id) ?? null;
  const from = byId(get("from"));
  const to = byId(get("to"));
  const [amount, setAmount] = useState("");
  const [slippage, setSlippage] = useState("");
  const amountId = useId();
  const slipId = useId();

  const slippageBps = slippage.trim() === "" ? 50 : percentToBps(slippage.trim());
  const maxBps = risk.data?.maxSlippageBps ?? 500;
  const slippageValid = slippageBps !== null && slippageBps > 0 && slippageBps <= maxBps;
  const amountValid = isValidAmount(amount) && (!from || (amount.split(".")[1]?.length ?? 0) <= from.decimals);

  // Balance: only the native asset of the wallet's current chain can be read without an indexer.
  const fromChain = from ? chainByKey(from.chain) : null;
  const isNativeOnWallet = Boolean(
    from && wallet.connected && !from.contractAddress && fromChain?.chainId && fromChain.chainId === wallet.state.chainId,
  );
  const balance = isNativeOnWallet && wallet.balanceAtomic !== null && from ? fromAtomic(wallet.balanceAtomic, from.decimals) : null;
  const exceedsBalance = balance !== null && amountValid && cmp(amount, balance) > 0;

  const testData = Boolean(catalogue.data?.testData);
  const liveRouteCandidate = Boolean(from && to && from.id !== to.id && (testData || (from.enabledForLiveSwap && to.enabledForLiveSwap)));

  // Live quotes need the recipient and refund address up front (the provider prices a concrete
  // route). Live routes are EVM to EVM, so the connected EVM account is both.
  const toChain = to ? chainByKey(to.chain) : null;
  const evmRoute = fromChain?.family === "EVM" && toChain?.family === "EVM";
  const quoteAccount = evmRoute && wallet.connected && wallet.state.account ? wallet.state.account : "";

  // Quotes: debounced 400ms, latest request wins.
  const request = useDebounced(
    from && to && amountValid && slippageValid && liveRouteCandidate ? `${from.id}|${to.id}|${amount}|${slippageBps}|${quoteAccount}` : "",
    400,
  );
  const [quote, setQuote] = useState<SwapQuote | null>(null);
  const [quoteStatus, setQuoteStatus] = useState<"idle" | "loading" | "ready" | "error">("idle");
  const [quoteError, setQuoteError] = useState<unknown>(null);
  const [quoteFetchedAt, setQuoteFetchedAt] = useState<string | null>(null);
  const [refreshNonce, setRefreshNonce] = useState(0);
  const seq = useRef(0);

  useEffect(() => {
    setQuote(null);
    setQuoteError(null);
    if (!request) {
      setQuoteStatus("idle");
      return;
    }
    const [o, d, amt, bps, account] = request.split("|");
    const n = ++seq.current;
    setQuoteStatus("loading");
    api
      .quote({
        originAssetId: o,
        destinationAssetId: d,
        amountIn: amt,
        slippageBps: Number(bps),
        ...(account ? { recipient: account, refundTo: account } : {}),
      })
      .then((q) => {
        if (n !== seq.current) return;
        if (q.mode === "simulation" && !USE_MOCKS) {
          setQuoteError(new ApiRequestError("Simulated quotes are test data.", 503, "API_UNAVAILABLE", null));
          setQuoteStatus("error");
          return;
        }
        setQuote(q);
        setQuoteFetchedAt(new Date().toISOString());
        setQuoteStatus("ready");
      })
      .catch((e: unknown) => {
        if (n !== seq.current) return;
        setQuoteError(e);
        setQuoteStatus("error");
      });
  }, [request, refreshNonce]);

  const now = useNow(1000);
  const left = secondsLeft(quote?.expiresAt, now);
  const expired = quoteStatus === "ready" && left === 0;
  const stale = risk.data?.catalogueStale === true;

  // Review and progress
  const [reviewOpen, setReviewOpen] = useState(false);
  const [phase, setPhase] = useState<ProgressPhase | null>(null);
  const [runError, setRunError] = useState(false);
  const [record, setRecord] = useState<SwapRecord | null>(null);
  const running = phase !== null && phase !== "completed" && phase !== "failed" && phase !== "refunded";

  // Auto-refresh an expired quote once, unless the user is reviewing or submitting.
  const autoRefreshed = useRef<string | null>(null);
  useEffect(() => {
    if (expired && quote && !reviewOpen && autoRefreshed.current !== quote.id) {
      autoRefreshed.current = quote.id;
      setRefreshNonce((v) => v + 1);
    }
  }, [expired, quote, reviewOpen]);

  const routeError = quoteStatus === "error" && quoteError instanceof ApiRequestError && quoteError.code !== null && ROUTE_ERRORS.has(quoteError.code);
  const routeSupported = liveRouteCandidate && !routeError;

  // Live swaps sign on the source EVM chain; the wallet must be on it.
  const needsNetwork = Boolean(
    quote && quote.mode !== "simulation" && wallet.connected && fromChain?.chainId && fromChain.chainId !== wallet.state.chainId,
  );

  const cta = swapCta({
    walletConnected: wallet.connected,
    fromSelected: Boolean(from),
    toSelected: Boolean(to),
    amountValid: amountValid && slippageValid,
    exceedsBalance,
    routeSupported,
    quoteStatus: quoteStatus === "error" && !routeError ? "error" : quoteStatus,
    quoteExpired: expired,
    stale: stale || needsNetwork,
  });

  const state = deriveViewState({ loading: catalogue.loading, online, error: catalogue.error, hasData: catalogue.data !== null });

  const highSlippage = slippageBps !== null && slippageBps > (risk.data?.warningSlippageBps ?? 200);
  const lowLiquidity = Boolean(quote?.warning && /liquidity/i.test(quote.warning));
  const aboutToExpire = left !== null && left > 0 && left <= EXPIRY_WARNING_SECONDS;

  const flip = () => update({ from: to?.id ?? null, to: from?.id ?? null });

  const setMax = () => {
    if (balance) setAmount(balance);
  };

  async function confirm() {
    if (!quote) return;
    setRunError(false);
    setRecord(null);
    const provider = wallet.provider;
    const account = wallet.state.account;
    const chainId = wallet.state.chainId;
    try {
      const result = await runSwap({
        quote,
        idempotencyKey: `swap-${quote.id}-${crypto.randomUUID()}`,
        api: {
          createLiveSwap: api.createLiveSwap,
          createSimulation: api.createSimulation,
          advanceSimulation: api.advanceSimulation,
          recordDeposit: api.recordDeposit,
          cancelSwap: api.cancelSwap,
          getSwap: api.swap,
        },
        wallet: provider && account && chainId ? {
          account,
          chainId,
          sendDeposit: (swap) => {
            if (!swap.quote.depositAddress) throw new Error("The provider did not return a deposit address.");
            return sendEvmDeposit({
              provider,
              account,
              chainId,
              depositAddress: swap.quote.depositAddress,
              amountAtomic: swap.quote.amountInAtomic,
              tokenAddress: swap.quote.originAsset.contractAddress ?? null,
              depositMemo: swap.quote.depositMemo,
            });
          },
        } : null,
        onProgress: (p, swap) => {
          setPhase(p);
          if (swap) setRecord(swap);
        },
      });
      if (result.kind === "rejected") {
        setPhase(null);
        toast("Request rejected in wallet");
      }
    } catch {
      setRunError(true);
    }
  }

  const resetAll = () => {
    setReviewOpen(false);
    setPhase(null);
    setRecord(null);
    setRunError(false);
    setAmount("");
    setRefreshNonce((v) => v + 1);
  };

  const tryAgain = () => {
    setReviewOpen(false);
    setPhase(null);
    setRecord(null);
    setRunError(false);
    setRefreshNonce((v) => v + 1);
  };

  const outcome: SwapOutcome | null = phase === "completed" || phase === "failed" || phase === "refunded" ? phase : null;
  const stepIndex = phase === "confirm" ? 0 : phase === "submitted" ? 1 : phase === "processing" ? 2 : 3;

  const onCta = () => {
    if (cta.action === "connect") wallet.openPicker();
    else if (cta.action === "refresh") setRefreshNonce((v) => v + 1);
    else if (cta.action === "review") setReviewOpen(true);
  };

  const rate = quote ? div(quote.amountOutEstimate, quote.amountIn, 8) : null;
  const quoteRows = quote && (
    <>
      <DefRow label="Rate">{rate ? `1 ${quote.originAsset.symbol} = ${formatDecimal(rate, 8)} ${quote.destinationAsset.symbol}` : "Not available"}</DefRow>
      <DefRow label="Fees">{typeof quote.feeBps === "number" ? formatPercent(bpsToPercent(quote.feeBps)) : "Not available"}</DefRow>
      <DefRow label="Estimated time">{formatSeconds(quote.timeEstimateSeconds)}</DefRow>
      <DefRow label="Max slippage">{formatPercent(bpsToPercent(quote.slippageBps))}</DefRow>
      <DefRow label="Minimum received">{quote.minimumAmountOut ? `${formatDecimal(quote.minimumAmountOut, 8)} ${quote.destinationAsset.symbol}` : "Not available"}</DefRow>
      <DefRow label="Provider">{quote.provider}</DefRow>
    </>
  );

  return (
    <>
      <PageHeader title="Swap" sub="Move assets across chains from your own wallet." testData={testData} />
      <ViewStateBlock view="swap" state={state} onAction={catalogue.reload} />

      {catalogue.data && (
        <div className="content-grid">
          <section className="panel panel-pad form-panel" aria-label="Swap form">
            <div className="ws-swap-side">
              <span className="field-label">From</span>
              <AssetPicker side="From" assets={assets} value={from} chainName={chainName} onChange={(a) => update({ from: a.id })} />
              <div className="ws-amount">
                <label className="ws-sr" htmlFor={amountId}>Amount</label>
                <input
                  id={amountId}
                  inputMode="decimal"
                  autoComplete="off"
                  placeholder="0.00"
                  value={amount}
                  onChange={(e) => setAmount(e.target.value.replace(",", "."))}
                />
                <button type="button" className="button button-quiet button-small" onClick={setMax} disabled={!balance}>Max</button>
              </div>
              <span className="field-hint">Balance: {balance !== null && from ? `${formatDecimal(balance, 6)} ${from.symbol}` : "Not available"}</span>
            </div>

            <div className="swap-direction">
              <button type="button" className="button button-secondary button-small" aria-label="Switch direction" onClick={flip}>⇅</button>
            </div>

            <div className="ws-swap-side">
              <span className="field-label">To</span>
              <AssetPicker side="To" assets={assets} value={to} chainName={chainName} onChange={(a) => update({ to: a.id })} />
            </div>

            <div className="field ws-slippage">
              <label htmlFor={slipId}>Max slippage (%)</label>
              <input id={slipId} inputMode="decimal" placeholder="0.5" value={slippage} onChange={(e) => setSlippage(e.target.value.replace(",", "."))} aria-invalid={!slippageValid} />
              {!slippageValid && <span className="field-hint">Enter up to {formatPercent(bpsToPercent(maxBps))} with at most two decimals.</span>}
            </div>

            {needsNetwork && fromChain?.chainId && (
              <div className="fixture-banner ws-banner" role="alert">
                <p>Switch to {fromChain.name} to continue.</p>
                <button type="button" className="button button-secondary button-small" onClick={() => void wallet.switchNetwork(fromChain.chainId!)}>Switch network</button>
              </div>
            )}

            <ul className="ws-warnings">
              {highSlippage && <li className="warning-text">High slippage: you may receive less than quoted.</li>}
              {lowLiquidity && <li className="warning-text">Low liquidity on this route.</li>}
              {aboutToExpire && <li className="warning-text">Quote is about to expire.</li>}
              {stale && <li className="warning-text">Price data is stale. Refresh before swapping.</li>}
            </ul>

            <div className="swap-actions">
              <button type="button" className="button ws-cta" disabled={cta.disabled} onClick={onCta}>{cta.label}</button>
            </div>
          </section>

          <section className="panel panel-pad" aria-label="Quote" aria-live="polite">
            <div className="panel-title">
              <h2>Quote</h2>
              {quote && quote.mode === "simulation" && <TestDataTag />}
            </div>
            {quoteStatus === "idle" && <p className="subtle-meta">Select assets and enter an amount to see a quote.</p>}
            {quoteStatus === "loading" && <p className="subtle-meta">Getting quote...</p>}
            {quoteStatus === "error" && !routeError && (
              <div className="empty-state ws-state" role="alert">
                <p>{quoteError instanceof ApiRequestError && (quoteError.status >= 502 || quoteError.code === "API_UNAVAILABLE") ? "Data unavailable right now." : "Something went wrong. Please try again."}</p>
                <button type="button" className="button button-secondary button-small" onClick={() => setRefreshNonce((v) => v + 1)}>Retry</button>
              </div>
            )}
            {quote && quoteStatus === "ready" && (
              <div className="quote-result">
                <h3>You receive (estimate)</h3>
                <p className="quote-number">
                  {formatDecimal(quote.amountOutEstimate, 8)} <small>{quote.destinationAsset.symbol}</small>
                </p>
                {quoteRows}
                <DefRow label="Expiry">{left === null ? "Not available" : expired ? "Expired" : `Quote expires in ${left}s`}</DefRow>
                <p className="subtle-meta ws-prov-line">
                  {quote.provider} · {asOf(quote.providerTimestamp ?? quoteFetchedAt)} <QualityBadge quality={stale ? "Stale" : "Live"} />
                </p>
              </div>
            )}
          </section>
        </div>
      )}

      <Modal open={reviewOpen} title="Review swap" onClose={() => !running && setReviewOpen(false)}>
        {quote && phase === null && (
          <div className="quote-result">
            <DefRow label="You pay">{`${formatDecimal(quote.amountIn, 8)} ${quote.originAsset.symbol} on ${chainName(quote.originAsset.chain)}`}</DefRow>
            <DefRow label="You receive (estimate)">{`${formatDecimal(quote.amountOutEstimate, 8)} ${quote.destinationAsset.symbol} on ${chainName(quote.destinationAsset.chain)}`}</DefRow>
            {quoteRows}
            <DefRow label="Expiry">{left === null ? "Not available" : expired ? "Expired" : `Quote expires in ${left}s`}</DefRow>
            {expired && <p className="warning-text">Quote expired - Refresh</p>}
            {runError && <p className="warning-text" role="alert">Something went wrong. Please try again.</p>}
            <div className="deposit-actions">
              <button type="button" className="button" disabled={expired || stale || needsNetwork} onClick={() => void confirm()}>Confirm in wallet</button>
              <button type="button" className="button button-secondary" onClick={() => setReviewOpen(false)}>Back</button>
            </div>
          </div>
        )}

        {phase !== null && (
          <div>
            <ol className="swap-timeline" aria-label="Swap progress">
              {PROGRESS_STEPS.map((label, i) => (
                <li key={label} className={i === stepIndex ? "current" : undefined} aria-current={i === stepIndex ? "step" : undefined}>
                  <strong>{label}</strong>
                  {i < stepIndex && <small>Done</small>}
                </li>
              ))}
              <li className={outcome ? "current" : undefined} aria-current={outcome ? "step" : undefined}>
                <strong>{outcome ? finalLabel(outcome) : "Completed"}</strong>
              </li>
            </ol>
            {runError && <p className="warning-text" role="alert">Something went wrong. Please try again.</p>}
            {outcome === "completed" && (
              <div className="deposit-actions">
                <p>Swap completed.</p>
                <Link className="button" href={record ? `/workspace/activity?swap=${encodeURIComponent(record.id)}` : "/workspace/activity"}>View in Activity</Link>
                <button type="button" className="button button-secondary" onClick={resetAll}>New swap</button>
              </div>
            )}
            {(outcome === "failed" || outcome === "refunded") && (
              <div className="deposit-actions">
                <p>{outcome === "failed" ? "Swap failed. Your funds were not moved." : "Swap refunded to your wallet."}</p>
                <button type="button" className="button" onClick={tryAgain}>Try again</button>
              </div>
            )}
          </div>
        )}
      </Modal>
    </>
  );
}
