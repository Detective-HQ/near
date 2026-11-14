"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { api, USE_MOCKS } from "../../../lib/api-client";
import type { SwapRecord } from "../../../lib/contracts";
import { formatDecimal } from "../../../lib/decimal";
import { explorerTxUrl } from "../../../lib/explorer";
import { formatTime, shortAddress } from "../../../lib/format";
import { isFinalSwapState, swapPill, swapPillTone } from "../../../lib/swap-flow";
import type { SwapPill } from "../../../lib/swap-flow";
import { deriveViewState } from "../../../lib/view-state";
import { useResource, useUrlParams } from "../hooks";
import { CopyButton, Drawer, FilterSelect, PageHeader, Pager, Provenance, ViewStateBlock } from "../ui";
import { useWorkspace } from "../workspace-context";

const PILLS: SwapPill[] = ["Pending", "Processing", "Completed", "Failed", "Refunded"];
const PAGE_SIZE = 20;

function Pill({ state }: { state: string }) {
  const p = swapPill(state);
  const tone = swapPillTone(p);
  return <span className={`state-pill${tone ? ` ${tone}` : ""}`}>{p}</span>;
}

export function ActivityPage() {
  const { wallet, online, chainName, chainByKey, catalogue } = useWorkspace();
  const demoBackend = Boolean(catalogue.data?.simulation);
  const { get, update } = useUrlParams();
  const res = useResource(wallet.connected ? `activity:${wallet.state.account}` : null, () => api.activity());
  const [live, setLive] = useState<Record<string, SwapRecord>>({});
  const [extraSwaps, setExtraSwaps] = useState<SwapRecord[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [loadingMore, setLoadingMore] = useState(false);

  useEffect(() => {
    if (res.data) {
      setCursor(res.data.nextCursor ?? null);
      setExtraSwaps([]);
    }
  }, [res.data]);

  const loadMore = async () => {
    if (!cursor || loadingMore) return;
    setLoadingMore(true);
    try {
      const more = await api.activity(cursor, 50);
      setExtraSwaps((prev) => {
        const seen = new Set(prev.map((s) => s.id));
        (res.data?.swaps ?? []).forEach((s) => seen.add(s.id));
        const additions = more.swaps.filter((s) => !seen.has(s.id));
        return [...prev, ...additions];
      });
      setCursor(more.nextCursor ?? null);
    } catch {
      // keep current
    } finally {
      setLoadingMore(false);
    }
  };

  const records = useMemo(() => {
    const all = [...(res.data?.swaps ?? []), ...extraSwaps];
    const list = all.map((r) => live[r.id] ?? r);
    // Simulated records appear in TEST DATA mode, or when the backend itself runs in demo mode.
    return USE_MOCKS || demoBackend ? list : list.filter((r) => r.mode !== "simulation");
  }, [res.data, extraSwaps, live, demoBackend]);
  const hasSimulated = records.some((r) => r.mode === "simulation");
  const testData = USE_MOCKS && hasSimulated;

  // Poll in-progress swaps; stop once each reaches a final state.
  const pending = records.filter((r) => !isFinalSwapState(r.state)).map((r) => r.id).join(",");
  useEffect(() => {
    if (!pending) return;
    const ids = pending.split(",");
    const t = setInterval(() => {
      ids.forEach((id) => {
        api.swap(id).then((s) => setLive((m) => ({ ...m, [id]: s })), () => undefined);
      });
    }, 5000);
    return () => clearInterval(t);
  }, [pending]);

  const status = get("status");
  const chain = get("chain");
  const fromDate = get("from");
  const toDate = get("to");
  const page = Math.max(1, Number(get("page")) || 1);

  const filtered = records
    .filter((r) =>
      (!status || swapPill(r.state) === status)
      && (!chain || r.quote.originAsset.chain === chain || r.quote.destinationAsset.chain === chain)
      && (!fromDate || r.createdAt >= `${fromDate}T00:00:00Z`)
      && (!toDate || r.createdAt <= `${toDate}T23:59:59Z`))
    .sort((a, b) => b.createdAt.localeCompare(a.createdAt));

  const chainOptions = Array.from(new Set(records.flatMap((r) => [r.quote.originAsset.chain, r.quote.destinationAsset.chain])))
    .map((k) => ({ value: k, label: chainName(k) }));

  const openId = get("swap");
  const open = records.find((r) => r.id === openId) ?? null;
  const anyFilter = Boolean(status || chain || fromDate || toDate);

  const state = deriveViewState({
    loading: res.loading,
    online,
    error: res.error,
    hasData: res.data !== null,
    isEmpty: filtered.length === 0,
  });

  if (!wallet.connected) {
    return (
      <>
        <PageHeader title="Activity" sub="Every swap, start to finish." />
        <div className="empty-state ws-state" role="status">
          <p>Connect a wallet to see your swaps.</p>
          <button type="button" className="button button-small" onClick={wallet.openPicker}>Connect wallet</button>
        </div>
      </>
    );
  }

  const txRow = (label: string, hash: string | null, link: string | null | undefined, chainKey: string) => {
    if (!hash) return null;
    const url = link ?? explorerTxUrl(chainByKey(chainKey), hash);
    return (
      <div className="transaction-reference">
        <span>{label}</span>
        <code>{shortAddress(hash)}</code>
        <CopyButton value={hash} label={`Copy ${label.toLowerCase()} hash`} />
        {url ? <a href={url} target="_blank" rel="noreferrer">View on explorer</a> : <span className="subtle-meta">View on explorer: Not available</span>}
      </div>
    );
  };

  return (
    <>
      <PageHeader title="Activity" sub="Every swap, start to finish." testData={testData} simulation={hasSimulated} />

      <div className="ws-filters">
        <FilterSelect label="Status" value={status} options={PILLS.map((p) => ({ value: p, label: p }))} onChange={(v) => update({ status: v })} />
        <FilterSelect label="Chain" value={chain} options={chainOptions} onChange={(v) => update({ chain: v })} />
        <div className="field ws-filter">
          <label htmlFor="act-from">From date</label>
          <input id="act-from" type="date" value={fromDate} onChange={(e) => update({ from: e.target.value })} />
        </div>
        <div className="field ws-filter">
          <label htmlFor="act-to">To date</label>
          <input id="act-to" type="date" value={toDate} onChange={(e) => update({ to: e.target.value })} />
        </div>
      </div>

      {state === "empty" ? (
        <div className="empty-state ws-state" role="status">
          <p>No swaps yet.</p>
          {anyFilter ? (
            <button type="button" className="button button-secondary button-small" onClick={() => update({ status: null, chain: null, from: null, to: null })}>Clear filters</button>
          ) : (
            <Link className="button button-small" href="/workspace/swap">Make your first swap</Link>
          )}
        </div>
      ) : (
        <ViewStateBlock view="activity" state={state} onAction={res.reload} />
      )}

      {filtered.length > 0 && (
        <section className="panel panel-pad" aria-label="Swaps">
          <div className="panel-title">
            <h2>Swaps</h2>
            <Provenance source={testData ? "TEST DATA" : hasSimulated ? "Zonyx API (simulation)" : "Zonyx API"} time={res.fetchedAt} quality="Live" />
          </div>
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th scope="col">Date</th>
                  <th scope="col">From → To</th>
                  <th scope="col" className="num">Amount</th>
                  <th scope="col">Status</th>
                  <th scope="col">Explorer</th>
                </tr>
              </thead>
              <tbody>
                {filtered.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE).map((r) => {
                  const src = r.transactions.source;
                  const url = r.transactionLinks?.source ?? (src ? explorerTxUrl(chainByKey(r.quote.originAsset.chain), src) : null);
                  return (
                    <tr key={r.id}>
                      <td>
                        <button type="button" className="ws-row-btn" onClick={() => update({ swap: r.id, page: String(page) })} aria-label={`Open swap details from ${formatTime(r.createdAt)}`}>
                          {formatTime(r.createdAt)}
                        </button>
                      </td>
                      <td>
                        {r.quote.originAsset.symbol} <span className="subtle-meta">{chainName(r.quote.originAsset.chain)}</span> → {r.quote.destinationAsset.symbol}{" "}
                        <span className="subtle-meta">{chainName(r.quote.destinationAsset.chain)}</span>
                      </td>
                      <td className="num">{formatDecimal(r.quote.amountIn, 8)} {r.quote.originAsset.symbol}</td>
                      <td><Pill state={r.state} /></td>
                      <td>{url ? <a href={url} target="_blank" rel="noreferrer">View on explorer</a> : "Not available"}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          {page * PAGE_SIZE < filtered.length && (
            <div className="deposit-actions section" style={{ justifyContent: "center" }}>
              <button
                type="button"
                className="button button-secondary"
                onClick={() => update({ page: String(page + 1) })}
              >
                Load more
              </button>
            </div>
          )}
          <Pager page={page} pageSize={PAGE_SIZE} total={filtered.length} onPage={(p) => update({ page: String(p) })} />
          {cursor && (
            <div style={{ textAlign: "center", marginTop: "1rem" }}>
              <button type="button" className="button button-secondary button-small" disabled={loadingMore} onClick={() => void loadMore()}>
                {loadingMore ? "Loading..." : "Load more"}
              </button>
            </div>
          )}
        </section>
      )}

      <Drawer open={open !== null} title="Swap details" onClose={() => update({ swap: null, page: String(page) })}>
        {open && (
          <div className="ws-drawer-body">
            <p>
              <Pill state={open.state} /> {formatDecimal(open.quote.amountIn, 8)} {open.quote.originAsset.symbol} → {open.quote.destinationAsset.symbol}
            </p>
            <ol className="swap-timeline">
              {open.events.map((e, i) => (
                <li key={`${e.state}-${e.at}`} className={i === open.events.length - 1 ? "current" : undefined}>
                  <strong>{swapPill(e.state)}</strong>
                  <small>{e.detail}</small>
                  <small>{formatTime(e.at)}</small>
                </li>
              ))}
            </ol>
            <div className="swap-transactions">
              {txRow("Deposit", open.transactions.source, open.transactionLinks?.source, open.quote.originAsset.chain)}
              {txRow("Destination", open.transactions.destination, open.transactionLinks?.destination, open.quote.destinationAsset.chain)}
              {txRow("Refund", open.transactions.refund, open.transactionLinks?.refund, open.quote.originAsset.chain)}
              {!open.transactions.source && !open.transactions.destination && !open.transactions.refund && (
                <p className="subtle-meta">Transaction hashes: Not available</p>
              )}
            </div>
          </div>
        )}
      </Drawer>
    </>
  );
}
