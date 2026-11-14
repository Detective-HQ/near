"use client";

import { useMemo } from "react";
import { api } from "../../../lib/api-client";
import { add, cmp, formatDecimal, formatPercent, formatUsd, fromAtomic, percentOf } from "../../../lib/decimal";
import type { PortfolioBalance } from "../../../lib/trading-contracts";
import { deriveViewState } from "../../../lib/view-state";
import { useResource, useUrlParams } from "../hooks";
import { PageHeader, Provenance, ViewStateBlock } from "../ui";
import { useWorkspace } from "../workspace-context";

const SMALL_USD = "1";

function Breakdown({ title, rows, total }: { title: string; rows: Array<{ key: string; label: string; value: string }>; total: string | null }) {
  return (
    <section className="panel panel-pad">
      <div className="panel-title"><h2>{title}</h2></div>
      {rows.length === 0 ? (
        <p className="subtle-meta">Not available</p>
      ) : (
        <div className="ecosystem-list">
          {rows.map((r) => {
            const share = total ? percentOf(r.value, total) : null;
            return (
              <div key={r.key} className="ecosystem-row">
                <div className="row-title"><strong>{r.label}</strong></div>
                <span>{formatUsd(r.value, false)}</span>
                <span className="subtle-meta">{share ? formatPercent(share, 1) : "Not available"}</span>
              </div>
            );
          })}
        </div>
      )}
    </section>
  );
}

export function PortfolioPage() {
  const { wallet, online, chains, chainName } = useWorkspace();
  const { get, update } = useUrlParams();
  const hideSmall = get("small") === "hide";

  const walletChain = chains.data?.find((c) => c.chainId === wallet.state.chainId) ?? null;
  const portfolioRes = useResource(
    wallet.connected ? `portfolio:${wallet.state.account ?? ""}:${walletChain?.key ?? ""}` : null,
    () => api.portfolio(wallet.state.account ?? undefined, walletChain?.key ?? undefined)
  );

  const nativeRow: PortfolioBalance | null = useMemo(() => {
    if (!walletChain?.nativeCurrency || wallet.balanceAtomic === null || !walletChain.portfolioBalanceSupported) return null;
    const balance = fromAtomic(wallet.balanceAtomic, walletChain.nativeCurrency.decimals);
    if (!balance) return null;
    return {
      assetId: `${walletChain.key}:native`,
      symbol: walletChain.nativeCurrency.symbol,
      chain: walletChain.key,
      balance,
      valueUsd: null, // live price populated from portfolio engine if available
      source: "Wallet RPC",
      asOf: new Date().toISOString(),
    };
  }, [walletChain, wallet.balanceAtomic]);

  const testData = Boolean(
    portfolioRes.data?.testData ||
    portfolioRes.data?.source?.includes("TEST DATA") ||
    portfolioRes.data?.source?.includes("Demo Fixture")
  );

  const all = useMemo(() => {
    const backendBalances = portfolioRes.data?.balances ?? [];
    const list: PortfolioBalance[] = [];
    const seen = new Set<string>();

    if (nativeRow && cmp(nativeRow.balance, "0") > 0) {
      const match = backendBalances.find(
        (b) => b.chain.toLowerCase() === nativeRow.chain.toLowerCase() && b.symbol.toLowerCase() === nativeRow.symbol.toLowerCase()
      );
      if (match?.valueUsd && match?.balance && cmp(match.balance, "0") > 0) {
        nativeRow.valueUsd = match.valueUsd;
      }
      list.push(nativeRow);
      seen.add(`${nativeRow.chain.toLowerCase()}:${nativeRow.symbol.toLowerCase()}`);
    }

    for (const b of backendBalances) {
      const key = `${b.chain.toLowerCase()}:${b.symbol.toLowerCase()}`;
      if (!seen.has(key) && cmp(b.balance, "0") > 0) {
        seen.add(key);
        list.push(b);
      }
    }

    return list;
  }, [nativeRow, portfolioRes.data?.balances]);

  const visible = hideSmall ? all.filter((r) => r.valueUsd === null || cmp(r.valueUsd, SMALL_USD) >= 0) : all;

  // Assets without a price are excluded from totals.
  const priced = visible.filter((r) => r.valueUsd !== null);
  const total = priced.length ? priced.reduce((s, r) => add(s, r.valueUsd as string), "0") : null;
  const group = (key: (r: PortfolioBalance) => string, label: (k: string) => string) => {
    const m = new Map<string, string>();
    priced.forEach((r) => m.set(key(r), add(m.get(key(r)) ?? "0", r.valueUsd as string)));
    return Array.from(m.entries())
      .sort((a, b) => cmp(b[1], a[1]))
      .map(([k, v]) => ({ key: k, label: label(k), value: v }));
  };

  if (!wallet.connected) {
    return (
      <>
        <PageHeader title="Portfolio" sub="Your balances, read-only." />
        <div className="empty-state ws-state" role="status">
          <p>Connect a wallet to see your portfolio.</p>
          <button type="button" className="button button-small" onClick={wallet.openPicker}>Connect wallet</button>
        </div>
      </>
    );
  }

  const balanceLoading = wallet.state.status === "connected" && wallet.balanceAtomic === null && Boolean(walletChain?.portfolioBalanceSupported);
  const state = deriveViewState({
    loading: portfolioRes.loading || balanceLoading,
    online,
    error: portfolioRes.error,
    hasData: !portfolioRes.loading && !balanceLoading,
    isEmpty: all.length === 0,
    partial: all.some((r) => r.valueUsd === null),
  });

  return (
    <>
      <PageHeader title="Portfolio" sub="Your balances, read-only." testData={testData} />
      <ViewStateBlock view="portfolio" state={state} onAction={portfolioRes.reload} />

      {portfolioRes.data?.notice && (
        <p className="subtle-meta" style={{ marginTop: "0.5rem", marginBottom: "1rem" }}>
          {portfolioRes.data.notice}
        </p>
      )}

      {all.length > 0 && (
        <>
          <div className="metric-strip">
            <div className="metric">
              <span className="metric-label">Total value</span>
              <span className="metric-value">{total ? formatUsd(total, false) : "Not available"}</span>
              <span className="metric-note">Assets without a price are excluded from totals.</span>
            </div>
          </div>

          <div className="two-column section">
            <Breakdown title="By chain" rows={group((r) => r.chain, chainName)} total={total} />
            <Breakdown title="By asset" rows={group((r) => r.symbol, (k) => k)} total={total} />
          </div>

          <section className="panel panel-pad section" aria-label="Balances">
            <div className="panel-title">
              <h2>Balances</h2>
              <label className="ws-toggle">
                <input type="checkbox" checked={hideSmall} onChange={(e) => update({ small: e.target.checked ? "hide" : null })} />
                Hide small balances
              </label>
            </div>
            <div className="table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th scope="col">Asset</th>
                    <th scope="col">Chain</th>
                    <th scope="col" className="num">Balance</th>
                    <th scope="col" className="num">Value</th>
                    <th scope="col" className="num">Share</th>
                    <th scope="col">Source</th>
                  </tr>
                </thead>
                <tbody>
                  {visible.map((r) => {
                    const share = r.valueUsd && total ? percentOf(r.valueUsd, total) : null;
                    return (
                      <tr key={r.assetId}>
                        <td><strong>{r.symbol}</strong></td>
                        <td>{chainName(r.chain)}</td>
                        <td className="num">{formatDecimal(r.balance, 6)}</td>
                        <td className="num">{r.valueUsd ? formatUsd(r.valueUsd, false) : "Not available"}</td>
                        <td className="num">{share ? formatPercent(share, 1) : "Not available"}</td>
                        <td><Provenance source={r.source} time={r.asOf} quality={r.valueUsd === null ? "Partial" : "Live"} /></td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </section>
        </>
      )}
    </>
  );
}
