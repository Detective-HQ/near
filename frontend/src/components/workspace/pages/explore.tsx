"use client";

import Link from "next/link";
import { useMemo } from "react";
import { shortAddress } from "../../../lib/format";
import { deriveViewState } from "../../../lib/view-state";
import { useUrlParams } from "../hooks";
import { CopyButton, FilterSelect, PageHeader, Pager, Provenance, SearchField, ViewStateBlock } from "../ui";
import { useWorkspace } from "../workspace-context";

const PAGE_SIZE = 25;

export function ExplorePage() {
  const { catalogue, chains, online, chainName, risk } = useWorkspace();
  const { get, update } = useUrlParams();
  const q = get("q");
  const chain = get("chain");
  const route = get("route");
  const page = Math.max(1, Number(get("page")) || 1);

  const assets = catalogue.data?.assets ?? [];
  const chainOptions = useMemo(
    () => Array.from(new Set(assets.map((a) => a.chain))).map((k) => ({ value: k, label: chainName(k) })),
    [assets, chainName],
  );

  const filtered = useMemo(() => {
    const term = q.trim().toLowerCase();
    return assets.filter((a) =>
      (!chain || a.chain === chain)
      && (!route || (route === "yes" ? a.enabledForLiveSwap : !a.enabledForLiveSwap))
      && (!term || a.symbol.toLowerCase().includes(term) || a.name.toLowerCase().includes(term) || (a.contractAddress ?? "").toLowerCase().includes(term)));
  }, [assets, q, chain, route]);

  const stale = risk.data?.catalogueStale === true;
  const state = deriveViewState({
    loading: catalogue.loading,
    online,
    error: catalogue.error,
    hasData: catalogue.data !== null,
    isEmpty: filtered.length === 0,
    stale,
  });
  const rows = filtered.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);
  const anyFilter = Boolean(q || chain || route);

  return (
    <>
      <PageHeader title="Explore" sub="Chains, assets and routes you can use." testData={catalogue.data?.testData} />

      <div className="ws-filters">
        <SearchField label="Search" placeholder="Search by name, symbol or contract" value={q} onChange={(v) => update({ q: v })} />
        <FilterSelect label="Chain" value={chain} options={chainOptions} onChange={(v) => update({ chain: v })} />
        <FilterSelect
          label="Route available"
          value={route}
          options={[{ value: "yes", label: "Yes" }, { value: "no", label: "No" }]}
          onChange={(v) => update({ route: v })}
        />
      </div>

      {state === "empty" && anyFilter ? (
        <div className="empty-state ws-state" role="status">
          <p>No assets match these filters.</p>
          <button type="button" className="button button-secondary button-small" onClick={() => update({ q: null, chain: null, route: null })}>
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
                {rows.map((a) => (
                  <tr key={`${a.chain}:${a.contractAddress ?? a.id}`}>
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
                      <span className={`state-pill${a.enabledForLiveSwap ? " good" : ""}`}>{a.enabledForLiveSwap ? "Enabled" : "Not enabled"}</span>
                    </td>
                    <td>
                      <span className="ws-inline">
                        <Link className="button button-secondary button-small" href={`/workspace/swap?from=${encodeURIComponent(a.id)}`} aria-label={`Swap ${a.symbol} on ${chainName(a.chain)}`}>
                          Swap
                        </Link>
                        <Link className="button button-quiet button-small" href="/workspace/markets" aria-label={`View market for ${a.symbol}`}>
                          View market
                        </Link>
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <Pager page={page} pageSize={PAGE_SIZE} total={filtered.length} onPage={(p) => update({ page: String(p) })} />
        </section>
      )}
    </>
  );
}
