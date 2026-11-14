"use client";

import Link from "next/link";
import { useMemo } from "react";
import type { ChainConfig } from "../../../lib/contracts";
import { shortAddress } from "../../../lib/format";
import { deriveViewState } from "../../../lib/view-state";
import { useUrlParams } from "../hooks";
import { CopyButton, FilterSelect, PageHeader, Pager, Provenance, SearchField, ViewStateBlock } from "../ui";
import { useWorkspace } from "../workspace-context";

const PAGE_SIZE = 25;

function categorizeChainStatus(c: ChainConfig): "Swap-enabled" | "Wallet-only" | "Planned" {
  const s = (c.status || "").toLowerCase();
  if (c.swapEnabled || s.includes("swap") || s.includes("live")) return "Swap-enabled";
  if (c.walletSupported || s.includes("wallet")) return "Wallet-only";
  return "Planned";
}

export function ExplorePage() {
  const { catalogue, chains, online, chainName, risk } = useWorkspace();
  const { get, update } = useUrlParams();
  const q = get("q");
  const chain = get("chain");
  const swapOnly = get("swapOnly") === "true";
  const route = get("route");
  const page = Math.max(1, Number(get("page")) || 1);

  const chainList = chains.data ?? [];
  const chainStatusMap = useMemo(() => {
    const map = new Map<string, "Swap-enabled" | "Wallet-only" | "Planned">();
    chainList.forEach((c) => map.set(c.key, categorizeChainStatus(c)));
    return map;
  }, [chainList]);

  const ecosystemsGrouped = useMemo(() => {
    const groups: Record<"Swap-enabled" | "Wallet-only" | "Planned", ChainConfig[]> = {
      "Swap-enabled": [],
      "Wallet-only": [],
      "Planned": [],
    };
    chainList.forEach((c) => {
      const cat = categorizeChainStatus(c);
      groups[cat].push(c);
    });
    return groups;
  }, [chainList]);

  const assets = catalogue.data?.assets ?? [];
  const chainOptions = useMemo(
    () => Array.from(new Set(assets.map((a) => a.chain))).map((k) => ({ value: k, label: chainName(k) })),
    [assets, chainName],
  );

  const filtered = useMemo(() => {
    const term = q.trim().toLowerCase();
    return assets.filter((a) => {
      const chainCat = chainStatusMap.get(a.chain) ?? "Planned";
      if (swapOnly && !a.enabledForLiveSwap && chainCat !== "Swap-enabled") return false;
      if (chain && a.chain !== chain) return false;
      if (route && (route === "yes" ? !a.enabledForLiveSwap : a.enabledForLiveSwap)) return false;
      if (term && !(a.symbol.toLowerCase().includes(term) || a.name.toLowerCase().includes(term) || (a.contractAddress ?? "").toLowerCase().includes(term))) {
        return false;
      }
      return true;
    });
  }, [assets, q, chain, route, swapOnly, chainStatusMap]);

  const stale = risk.data?.catalogueStale === true;
  const state = deriveViewState({
    loading: catalogue.loading || chains.loading,
    online,
    error: catalogue.error || chains.error,
    hasData: catalogue.data !== null && chains.data !== null,
    isEmpty: filtered.length === 0,
    stale,
  });
  const rows = filtered.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);
  const anyFilter = Boolean(q || chain || route || swapOnly);

  return (
    <>
      <PageHeader title="Explore" sub="Chains, assets and routes you can use." testData={catalogue.data?.testData} />

      {/* Ecosystems grouped by status */}
      <section className="panel panel-pad section" aria-label="Ecosystems by status">
        <div className="panel-title">
          <h2>Ecosystems</h2>
          <span className="subtle-meta">Grouped by integration capability</span>
        </div>
        <div className="two-column">
          {(["Swap-enabled", "Wallet-only", "Planned"] as const).map((statusGroup) => {
            const list = ecosystemsGrouped[statusGroup];
            return (
              <div key={statusGroup} className="panel panel-pad ws-eco-group">
                <div className="panel-title">
                  <h3 className="card-heading">{statusGroup}</h3>
                  <span className={`state-pill${statusGroup === "Swap-enabled" ? " good" : statusGroup === "Wallet-only" ? " warn" : ""}`}>
                    {list.length} {list.length === 1 ? "chain" : "chains"}
                  </span>
                </div>
                {list.length === 0 ? (
                  <p className="subtle-meta">No chains in this status.</p>
                ) : (
                  <ul className="ecosystem-list">
                    {list.map((c) => (
                      <li key={c.key} className="ecosystem-row">
                        <div className="row-title">
                          <strong>{c.name}</strong>
                          <small>{c.family}</small>
                        </div>
                        <div className="ws-inline">
                          {statusGroup !== "Planned" && (
                            <Link
                              className="button button-secondary button-small"
                              href={`/workspace/swap?chain=${encodeURIComponent(c.key)}`}
                            >
                              Swap
                            </Link>
                          )}
                          <Link
                            className="button button-quiet button-small"
                            href={`/workspace/markets?chain=${encodeURIComponent(c.key)}`}
                          >
                            Market
                          </Link>
                        </div>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            );
          })}
        </div>
      </section>

      {/* Filters bar */}
      <div className="ws-filters">
        <SearchField label="Search" placeholder="Search by name, symbol or contract" value={q} onChange={(v) => update({ q: v })} />
        <FilterSelect label="Chain" value={chain} options={chainOptions} onChange={(v) => update({ chain: v })} />
        <FilterSelect
          label="Route available"
          value={route}
          options={[{ value: "yes", label: "Yes" }, { value: "no", label: "No" }]}
          onChange={(v) => update({ route: v })}
        />
        <label className="ws-toggle ws-filter">
          <input
            type="checkbox"
            checked={swapOnly}
            onChange={(e) => update({ swapOnly: e.target.checked ? "true" : null })}
          />
          Swap-enabled only
        </label>
      </div>

      {state === "empty" && anyFilter ? (
        <div className="empty-state ws-state" role="status">
          <p>No assets match these filters.</p>
          <button
            type="button"
            className="button button-secondary button-small"
            onClick={() => update({ q: null, chain: null, route: null, swapOnly: null })}
          >
            Clear filters
          </button>
        </div>
      ) : state === "empty" ? (
        <div className="empty-state ws-state" role="status"><p>No assets match these filters.</p></div>
      ) : (
        <ViewStateBlock
          view="explore"
          state={state}
          staleTime={risk.data?.catalogueLastRefreshed ?? undefined}
          onAction={() => {
            catalogue.reload();
            chains.reload();
          }}
        />
      )}

      {catalogue.data && rows.length > 0 && (
        <section className="panel panel-pad" aria-label="Assets">
          <div className="panel-title">
            <h2>Assets</h2>
            <Provenance
              source={catalogue.data.testData ? "TEST DATA" : "Provider catalogue"}
              time={catalogue.data.servedAt ?? catalogue.fetchedAt}
              quality={stale ? "Stale" : "Live"}
            />
          </div>
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th scope="col">Asset</th>
                  <th scope="col">Chain</th>
                  <th scope="col">Contract</th>
                  <th scope="col">Routes</th>
                  <th scope="col">Status</th>
                  <th scope="col"><span className="ws-sr">Actions</span></th>
                </tr>
              </thead>
              <tbody>
                {rows.map((a) => {
                  const assetKey = a.providerAssetId || a.id;
                  const chainCat = chainStatusMap.get(a.chain) ?? "Planned";
                  const canSwap = a.enabledForLiveSwap && chainCat !== "Planned";
                  return (
                    <tr key={assetKey}>
                      <td>
                        <strong>{a.symbol}</strong>
                        <div className="subtle-meta">{a.name}</div>
                      </td>
                      <td>{chainName(a.chain)}</td>
                      <td className="mono">
                        {a.contractAddress ? (
                          <span className="ws-inline">
                            {shortAddress(a.contractAddress)}
                            <CopyButton value={a.contractAddress} label={`Copy ${a.symbol} contract address`} />
                          </span>
                        ) : (
                          "Not available"
                        )}
                      </td>
                      <td>{a.enabledForLiveSwap ? "Available" : "Not available"}</td>
                      <td>
                        <span className={`state-pill${a.enabledForLiveSwap ? " good" : ""}`}>
                          {a.enabledForLiveSwap ? "Enabled" : "Not enabled"}
                        </span>
                      </td>
                      <td>
                        <span className="ws-inline">
                          {canSwap ? (
                            <Link
                              className="button button-secondary button-small"
                              href={`/workspace/swap?from=${encodeURIComponent(a.id)}`}
                              aria-label={`Swap ${a.symbol} on ${chainName(a.chain)}`}
                            >
                              Swap
                            </Link>
                          ) : (
                            <button type="button" className="button button-secondary button-small" disabled title={chainCat === "Planned" ? "Swaps planned in roadmap" : "Route not available"}>
                              Swap
                            </button>
                          )}
                          <Link
                            className="button button-quiet button-small"
                            href={`/workspace/markets?asset=${encodeURIComponent(a.symbol)}`}
                            aria-label={`View market for ${a.symbol}`}
                          >
                            View market
                          </Link>
                        </span>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          <Pager page={page} pageSize={PAGE_SIZE} total={filtered.length} onPage={(p) => update({ page: String(p) })} />
        </section>
      )}
    </>
  );
}
