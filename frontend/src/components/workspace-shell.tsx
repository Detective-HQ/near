"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  useCallback,
  useEffect,
  useRef,
  useState
} from "react";
import type { FormEvent, ReactNode } from "react";
import { api, ApiRequestError } from "../lib/api-client";
import type {
  Asset,
  ChainConfig,
  DataSnapshot,
  HealthStatus,
  MetricObservation,
  ResearchInputs,
  SeriesPoint,
  SwapQuote,
  SwapRecord
} from "../lib/contracts";
import {
  createReadOnlyEvmWallet,
  normalizeEvmAddress,
  sendEvmDeposit,
  shortenEvmAddress
} from "../lib/evm-wallet";
import type { Eip1193Provider, EvmWalletState, ReadOnlyEvmWallet } from "../lib/evm-wallet";
import { displaySource, formatMetric, formatTimestamp } from "../lib/format";
import {
  serializeSnapshotCsv,
  serializeSnapshotJson,
  snapshotFilename
} from "../lib/snapshot-export";

declare global {
  interface Window {
    ethereum?: Eip1193Provider;
  }
}

export type SectionKey =
  | "overview"
  | "swap"
  | "activity"
  | "portfolio"
  | "markets"
  | "intelligence"
  | "research"
  | "risk";

interface PageInfo {
  title: string;
  description: string;
}

interface AppData {
  health: HealthStatus | null;
  chains: ChainConfig[];
  assets: Asset[];
  markets: DataSnapshot | null;
  intelligence: DataSnapshot | null;
  swaps: SwapRecord[];
  research: ResearchInputs | null;
  error: string | null;
  loading: boolean;
}

const PAGE_INFO: Record<SectionKey, PageInfo> = {
  overview: {
    title: "Overview",
    description: "Cross-chain swap activity and evidence-led ecosystem research."
  },
  swap: {
    title: "Universal swap",
    description: "Review a provider quote, sign the source transfer in your EVM wallet, and follow provider settlement."
  },
  activity: {
    title: "Activity",
    description: "Source deposits and provider-reported swap outcomes, kept as separate lifecycle events."
  },
  portfolio: {
    title: "Portfolio",
    description: "Read-only native EVM balance lookup. Token holdings and other wallet families are not indexed."
  },
  markets: {
    title: "Markets",
    description: "Reported ecosystem metrics with source, unit, status and observation time."
  },
  intelligence: {
    title: "NEAR intelligence",
    description: "A provenance-first view of NEAR ecosystem indicators and their limitations."
  },
  research: {
    title: "Research model",
    description: "Change explicit assumptions and inspect a reproducible annual-fee scenario."
  },
  risk: {
    title: "Risk center",
    description: "Current integration boundaries, data quality limits and execution warnings."
  }
};

const NAV_GROUPS: Array<{ title: string; items: Array<{ key: SectionKey; label: string; glyph: string }> }> = [
  {
    title: "Workspace",
    items: [
      { key: "overview", label: "Overview", glyph: "◫" },
      { key: "swap", label: "Universal swap", glyph: "⇄" },
      { key: "activity", label: "Activity", glyph: "↗" },
      { key: "portfolio", label: "Portfolio", glyph: "◉" }
    ]
  },
  {
    title: "Research",
    items: [
      { key: "markets", label: "Markets", glyph: "⌁" },
      { key: "intelligence", label: "Intelligence", glyph: "⌗" },
      { key: "research", label: "Research model", glyph: "▤" },
      { key: "risk", label: "Risk center", glyph: "◇" }
    ]
  }
];

const KNOWN_SECTIONS = new Set<SectionKey>([
  "overview", "swap", "activity", "portfolio", "markets", "intelligence", "research", "risk"
]);

const METRIC_GROUPS: Array<{ label: string; aliases: string[] }> = [
  { label: "Active accounts", aliases: ["near_active_accounts", "active_accounts"] },
  { label: "Transactions", aliases: ["near_transactions", "transactions"] },
  { label: "TVL", aliases: ["near_tvl", "tvl"] },
  { label: "Stablecoin supply", aliases: ["near_stablecoin_supply", "stablecoin_supply"] }
];

const RISK_ROWS = [
  {
    title: "Wallet execution",
    detail: "The initial transaction builder supports injected EVM wallets on the configured EVM networks. Other wallet families remain disabled.",
    status: "EVM only",
    tone: "warn"
  },
  {
    title: "Fixture data",
    detail: "Default metrics and asset prices are illustrative. They are not live prices, liquidity or return estimates.",
    status: "Illustrative",
    tone: "warn"
  },
  {
    title: "Route coverage",
    detail: "A configured chain or listed asset does not guarantee a routable pair. Route support must be quoted.",
    status: "Unconfirmed",
    tone: ""
  },
  {
    title: "Portfolio coverage",
    detail: "Only native balances on a supported injected EVM network can be read. SPL, Stellar, Aptos and NEAR token balances are not indexed.",
    status: "Partial",
    tone: "warn"
  }
];

const EXECUTABLE_EVM_CHAINS = new Set([
  "ethereum",
  "base",
  "arbitrum",
  "polygon",
  "avalanche",
  "bnb"
]);

const TERMINAL_SWAP_STATES = new Set([
  "SUCCEEDED",
  "REFUNDED",
  "FAILED",
  "EXPIRED",
  "CANCELLED",
  "SIMULATION_COMPLETE"
]);

function transactionExplorerUrl(value: string | null | undefined): string | null {
  if (typeof value !== "string" || value.length > 2048) return null;
  try {
    const parsed = new URL(value);
    return parsed.protocol === "https:" && !parsed.username && !parsed.password
      ? parsed.toString()
      : null;
  } catch {
    return null;
  }
}

function SwapTransaction({
  label,
  hash,
  explorerUrl
}: {
  label: string;
  hash: string | null;
  explorerUrl?: string | null;
}) {
  if (!hash) return null;
  const safeUrl = transactionExplorerUrl(explorerUrl);
  return (
    <div className="transaction-reference">
      <span>{label}</span>
      {safeUrl ? (
        <a href={safeUrl} target="_blank" rel="noreferrer noopener">{`${hash.slice(0, 10)}…${hash.slice(-8)}`}</a>
      ) : (
        <code title={hash}>{`${hash.slice(0, 10)}…${hash.slice(-8)}`}</code>
      )}
    </div>
  );
}

function messageFrom(error: unknown): string {
  if (error instanceof ApiRequestError) {
    return error.code ? `${error.message} (${error.code})` : error.message;
  }
  return error instanceof Error ? error.message : "The request could not be completed.";
}

function getMetric(snapshot: DataSnapshot | null, aliases: string[]): MetricObservation | undefined {
  return snapshot?.metrics.find((metric) => aliases.includes(metric.key));
}

function displayObservation(metric: MetricObservation | undefined): string {
  if (!metric) return "—";
  return formatMetric(
    metric.value,
    metric.unit.toLowerCase().startsWith("usd") ? "USD" : metric.unit
  );
}

function latestSeriesValue(series: SeriesPoint[], aliases: string[]): number | null {
  for (let index = series.length - 1; index >= 0; index -= 1) {
    for (const key of aliases) {
      const value = series[index][key];
      if (typeof value === "number" && Number.isFinite(value)) return value;
    }
  }
  return null;
}

function StatusPill({ children, tone = "" }: { children: string; tone?: string }) {
  return <span className={`state-pill${tone ? ` ${tone}` : ""}`}>{children}</span>;
}

function BrandMark() {
  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img className="brand-mark" src="/images/sightline-mark.svg" alt="" width={28} height={28} />
  );
}

function PanelTitle({
  title,
  description,
  action
}: {
  title: string;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <div className="panel-title">
      <div>
        <h2>{title}</h2>
        {description ? <p>{description}</p> : null}
      </div>
      {action}
    </div>
  );
}

function MetricStrip({ snapshot }: { snapshot: DataSnapshot | null }) {
  const active = getMetric(snapshot, ["near_active_accounts", "active_accounts"]);
  const transactions = getMetric(snapshot, ["near_transactions", "transactions"]);
  const tvl = getMetric(snapshot, ["near_tvl", "tvl"]);
  const stablecoins = getMetric(snapshot, ["near_stablecoin_supply", "stablecoin_supply"]);
  const seriesActive = latestSeriesValue(snapshot?.series ?? [], ["activeAccounts", "active_accounts"]);
  const seriesTx = latestSeriesValue(snapshot?.series ?? [], ["transactions"]);
  const items = [
    { label: active?.label ?? "Active accounts", value: active ? displayObservation(active) : formatMetric(seriesActive), note: active?.scope ?? "Not covered by live source" },
    { label: transactions?.label ?? "Transactions", value: transactions ? displayObservation(transactions) : formatMetric(seriesTx), note: transactions?.scope ?? "Not covered by live source" },
    { label: tvl?.label ?? "TVL", value: displayObservation(tvl), note: tvl?.scope ?? "No observation" },
    { label: stablecoins?.label ?? "Stablecoin supply", value: displayObservation(stablecoins), note: stablecoins?.scope ?? "No observation" }
  ];

  return (
    <div className="metric-strip" aria-label="NEAR ecosystem indicators">
      {items.map((item) => (
        <div className="metric" key={item.label}>
          <span className="metric-label">{item.label}</span>
          <span className="metric-value">{item.value}</span>
          <span className="metric-note">{item.note}{snapshot ? ` · ${displaySource(snapshot.mode, snapshot.provider)}` : ""}</span>
        </div>
      ))}
    </div>
  );
}

function ActivityChart({ snapshot }: { snapshot: DataSnapshot | null }) {
  const points = snapshot?.series ?? [];
  const key = ["activeAccounts", "near_active_accounts", "tvl"].find((candidate) =>
    points.some((point) => typeof point[candidate] === "number")
  );
  if (!key || points.length < 2) {
    return <div className="empty-state">No comparable time series is available from the current source.</div>;
  }
  const values = points.map((point) => Number(point[key]));
  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min || 1;
  const width = 680;
  const height = 220;
  const left = 48;
  const top = 12;
  const right = width - 8;
  const bottom = height - 28;
  const coords = values.map((value, index) => ({
    x: left + (right - left) * index / (values.length - 1),
    y: bottom - (value - min) / range * (bottom - top)
  }));
  const path = coords.map((point, index) =>
    `${index === 0 ? "M" : "L"} ${point.x.toFixed(1)} ${point.y.toFixed(1)}`
  ).join(" ");
  const labels = [points[0]?.date, points[Math.floor(points.length / 2)]?.date, points.at(-1)?.date];

  return (
    <>
      <div className="chart-wrap">
        <svg className="chart-svg" viewBox={`0 0 ${width} ${height}`} role="img" aria-label={`${key} history`}>
          {[0, 1, 2, 3].map((line) => {
            const y = top + (bottom - top) * line / 3;
            return <line className="chart-grid" key={line} x1={left} x2={right} y1={y} y2={y} />;
          })}
          <text className="chart-axis" x="0" y={top + 4}>{formatMetric(max)}</text>
          <text className="chart-axis" x="0" y={bottom}>{formatMetric(min)}</text>
          <path className="chart-line" d={path} />
          <circle className="chart-point" cx={coords.at(-1)?.x} cy={coords.at(-1)?.y} r="3.5" />
        </svg>
      </div>
      <div className="chart-x">{labels.map((label, index) => <span key={`${label}-${index}`}>{label ?? "—"}</span>)}</div>
      <div className="chart-caption">
        <span>Source: {displaySource(snapshot?.mode ?? "fixture", snapshot?.provider)}</span>
        <span>{points.length} observations</span>
      </div>
    </>
  );
}

function OverviewView({
  data,
  onRefresh
}: {
  data: AppData;
  onRefresh: () => void;
}) {
  const snapshot = data.markets;
  const chainPreview = data.chains.slice(0, 7);
  const assetsPreview = data.assets.slice(0, 5);

  return (
    <>
      <PageHeader section="overview" />
      {data.health?.marketDataMode !== "live" ? (
        <div className="fixture-banner" role="note">
          <span className="status-dot status-dot-neutral" />
          <p><strong>Fixture data</strong> No live provider catalogue or market source is connected.</p>
        </div>
      ) : null}
      {data.error ? (
        <div className="notice-block data-source-notice" role="status">
          <span className="notice-mark" aria-hidden="true">!</span>
          <p>{data.error}</p>
          <button className="button button-secondary button-small" type="button" onClick={onRefresh}>Retry</button>
        </div>
      ) : null}

      <div className="section-heading">
        <div>
          <h2>Integration readiness</h2>
          <p>Capabilities are intentionally limited until their adapters and sources are configured.</p>
        </div>
      </div>
      <div className="setup-strip">
        {[
          ["Route catalogue", data.health?.quoteMode === "live" ? "Provider catalogue mode" : "Local fixture assets", "/workspace/swap"],
          ["Wallet signing", "Not implemented", "/workspace/portfolio"],
          ["Market sources", data.health?.marketDataMode === "live" ? "Live source configured" : "Fixture data only", "/workspace/markets"],
          ["NEAR research", "Definitions and scenario model", "/workspace/intelligence"]
        ].map(([title, detail, href]) => (
          <Link className="setup-cell" href={href} key={title}>
            <strong>{title}</strong>
            <p>{detail}</p>
            <span className="cell-arrow" aria-hidden="true">↗</span>
          </Link>
        ))}
      </div>

      <section className="section">
        <div className="section-heading">
          <div>
            <h2>Network signals</h2>
            <p>{snapshot?.notice ?? "Values include source labels and their current data status."}</p>
          </div>
          <button className="button button-quiet button-small" type="button" onClick={onRefresh}>Refresh</button>
        </div>
        <MetricStrip snapshot={snapshot} />
      </section>

      <div className="overview-grid">
        <section className="panel panel-pad">
          <PanelTitle
            title={snapshot?.metrics[0]?.label ?? "NEAR activity"}
            description="Observed series from the current market-data response"
            action={<StatusPill>{snapshot?.mode === "live" ? "Live" : "Fixture"}</StatusPill>}
          />
          <ActivityChart snapshot={snapshot} />
        </section>
        <section className="panel panel-pad">
          <PanelTitle
            title="Chain registry"
            description={`${data.chains.length} configured entries · none swap-enabled`}
            action={<Link className="text-link" href="/workspace/risk">Details</Link>}
          />
          <div className="ecosystem-list">
            {chainPreview.map((chain) => (
              <div className="ecosystem-row" key={chain.key}>
                <div className="row-title">
                  <strong>{chain.name}</strong>
                  <small>{chain.family} · {chain.walletSupported ? "wallet metadata only" : "wallet unavailable"}</small>
                </div>
                <StatusPill tone={chain.status.toLowerCase().includes("flag") ? "warn" : ""}>{chain.status}</StatusPill>
                <span className="subtle-meta">{chain.swapEnabled ? "On" : "Off"}</span>
              </div>
            ))}
          </div>
        </section>
      </div>

      <section className="section panel panel-pad">
        <div className="section-heading">
          <div>
            <h2>Asset catalogue</h2>
            <p>{data.assets.length} assets returned by the configured catalogue mode. Availability is not route confirmation.</p>
          </div>
          <Link className="text-link" href="/workspace/swap">Review quote form</Link>
        </div>
        {assetsPreview.length ? (
          <div className="table-wrap">
            <table className="data-table mobile-stack">
              <thead><tr><th>Asset</th><th>Chain</th><th className="num">Decimals</th><th>Catalogue</th></tr></thead>
              <tbody>
                {assetsPreview.map((asset) => (
                  <tr key={asset.id}>
                    <td data-label="Asset"><strong>{asset.symbol}</strong><span className="row-title"><small>{asset.name}</small></span></td>
                    <td data-label="Chain">{asset.chain}</td>
                    <td className="num" data-label="Decimals">{asset.decimals}</td>
                    <td data-label="Status"><StatusPill>{asset.enabledForLiveSwap ? "Route unconfirmed" : "Not enabled"}</StatusPill></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="empty-state">Asset catalogue is unavailable.</div>
        )}
      </section>
    </>
  );
}

function PageHeader({ section, children }: { section: SectionKey; children?: React.ReactNode }) {
  const info = PAGE_INFO[section];
  return (
    <header className="page-header">
      <div>
        <h1>{info.title}</h1>
        <p>{info.description}</p>
      </div>
      {children ? <div className="header-actions">{children}</div> : null}
    </header>
  );
}

function SwapView({
  data,
  notify,
  onActivity
}: {
  data: AppData;
  notify: (message: string) => void;
  onActivity: () => void;
}) {
  const [originAssetId, setOriginAssetId] = useState("");
  const [destinationAssetId, setDestinationAssetId] = useState("");
  const [amountIn, setAmountIn] = useState("1");
  const [recipient, setRecipient] = useState("");
  const [refundTo, setRefundTo] = useState("");
  const [slippageBps, setSlippageBps] = useState(100);
  const [quote, setQuote] = useState<SwapQuote | null>(null);
  const [preparedSwap, setPreparedSwap] = useState<SwapRecord | null>(null);
  const [broadcastHash, setBroadcastHash] = useState("");
  const [busy, setBusy] = useState(false);
  const [walletBusy, setWalletBusy] = useState(false);
  const [now, setNow] = useState(Date.now());
  const [error, setError] = useState<string | null>(null);
  const [walletState, setWalletState] = useState<EvmWalletState>({
    status: "disconnected",
    providerId: null,
    providerName: null,
    account: null,
    chainId: null,
    balance: null,
    balanceStatus: "idle",
    error: null
  });
  const walletRef = useRef<ReadOnlyEvmWallet | null>(null);
  const walletProviderRef = useRef<Eip1193Provider | null>(null);
  const liveMode = data.health?.quoteMode === "live";
  const executionEnabled = data.health?.executionEnabled === true;
  const locked = Boolean(preparedSwap && !TERMINAL_SWAP_STATES.has(preparedSwap.state));
  const originAsset = data.assets.find((asset) => asset.id === originAssetId) ?? null;
  const destinationAsset = data.assets.find((asset) => asset.id === destinationAssetId) ?? null;
  const originChain = data.chains.find((chain) => chain.key === originAsset?.chain) ?? null;
  const walletOnOriginChain = Boolean(
    walletState.status === "connected"
    && originChain?.chainId
    && walletState.chainId === originChain.chainId
  );
  const executableOrigin = Boolean(
    originAsset
    && EXECUTABLE_EVM_CHAINS.has(originAsset.chain)
    && originAsset.sourceDepositSupported === true
  );
  const executableDestination = Boolean(
    destinationAsset && EXECUTABLE_EVM_CHAINS.has(destinationAsset.chain)
  );
  const remainingSeconds = preparedSwap?.quote.expiresAt
    ? Math.max(0, Math.ceil((Date.parse(preparedSwap.quote.expiresAt) - now) / 1000))
    : null;
  const preparedQuoteExpired = remainingSeconds === 0;

  useEffect(() => {
    walletRef.current = createReadOnlyEvmWallet({
      resolveChain: (chainId) => {
        const chain = data.chains.find((candidate) => candidate.chainId === chainId);
        return chain?.chainId ? { ...chain, chainId: chain.chainId } : null;
      },
      onChange: setWalletState
    });
    return () => {
      walletRef.current?.forget();
      walletRef.current = null;
      walletProviderRef.current = null;
    };
  }, [data.chains]);

  useEffect(() => {
    if (!locked || !preparedSwap) return;
    const interval = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(interval);
  }, [locked, preparedSwap]);

  useEffect(() => {
    if (!preparedSwap) return;
    const latest = data.swaps.find((swap) => swap.id === preparedSwap.id);
    if (latest && latest.updatedAt !== preparedSwap.updatedAt) setPreparedSwap(latest);
  }, [data.swaps, preparedSwap]);

  useEffect(() => {
    if (!data.assets.length) return;
    const selectable = liveMode
      ? data.assets.filter((asset) => EXECUTABLE_EVM_CHAINS.has(asset.chain))
      : data.assets;
    const originOptions = liveMode
      ? selectable.filter((asset) => asset.sourceDepositSupported)
      : selectable;
    const nextOriginId = originOptions.some((asset) => asset.id === originAssetId)
      ? originAssetId
      : originOptions[0]?.id ?? "";
    const nextDestinationId = selectable.some((asset) =>
      asset.id === destinationAssetId && asset.id !== nextOriginId
    )
      ? destinationAssetId
      : selectable.find((asset) => asset.id !== nextOriginId)?.id ?? "";
    if (nextOriginId !== originAssetId) setOriginAssetId(nextOriginId);
    if (nextDestinationId !== destinationAssetId) setDestinationAssetId(nextDestinationId);
  }, [data.assets, liveMode, originAssetId, destinationAssetId]);

  function invalidateReviewedQuote() {
    setQuote(null);
    setError(null);
  }

  async function connectWallet() {
    if (!window.ethereum) {
      setError("No injected EVM wallet was detected in this browser.");
      return;
    }
    const wallet = walletRef.current;
    if (!wallet) return;
    setWalletBusy(true);
    setError(null);
    const provider = window.ethereum;
    walletProviderRef.current = provider;
    const name = provider.isMetaMask
      ? "MetaMask"
      : provider.isCoinbaseWallet
        ? "Coinbase Wallet"
        : "Injected EVM wallet";
    try {
      const state = await wallet.connect({ id: "injected-evm", name, provider });
      if (state.account) {
        setRecipient((current) => current || state.account!);
        setRefundTo((current) => current || state.account!);
      }
    } finally {
      setWalletBusy(false);
    }
  }

  async function requestQuote(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    setQuote(null);
    try {
      const result = await api.quote({
        originAssetId,
        destinationAssetId,
        amountIn,
        recipient: recipient.trim() || undefined,
        refundTo: refundTo.trim() || undefined,
        slippageBps
      });
      setQuote(result.quote);
      setPreparedSwap(null);
      setBroadcastHash("");
    } catch (requestError) {
      setError(messageFrom(requestError));
    } finally {
      setBusy(false);
    }
  }

  async function createSimulation() {
    if (!quote || quote.mode !== "simulation") return;
    setBusy(true);
    setError(null);
    try {
      const idempotencyKey = globalThis.crypto?.randomUUID?.() ?? `udi-${Date.now()}-${Math.random()}`;
      const result = await api.createSimulation(quote.id, idempotencyKey);
      notify(result.created ? "Local simulation record created. No deposit was sent." : "Existing simulation record returned.");
      onActivity();
    } catch (requestError) {
      setError(messageFrom(requestError));
    } finally {
      setBusy(false);
    }
  }

  async function prepareExecutableSwap() {
    if (
      !quote
      || quote.mode !== "provider_dry_quote"
      || !executionEnabled
      || !walletOnOriginChain
    ) return;
    setBusy(true);
    setError(null);
    try {
      const idempotencyKey = globalThis.crypto?.randomUUID?.()
        ?? `udi-${Date.now()}-${Math.random()}`;
      const result = await api.createLiveSwap(
        { quoteId: quote.id, recipient: recipient.trim(), refundTo: refundTo.trim() },
        idempotencyKey
      );
      setPreparedSwap(result.swap);
      setNow(Date.now());
      setBroadcastHash("");
      onActivity();
      notify("Executable quote prepared. Review its amount and deadline before sending.");
    } catch (requestError) {
      setError(messageFrom(requestError));
    } finally {
      setBusy(false);
    }
  }

  async function recordBroadcast(txHash: string) {
    if (!preparedSwap || !originChain?.chainId) return;
    setBusy(true);
    setError(null);
    try {
      const result = await api.recordDeposit(preparedSwap.id, txHash, originChain.chainId);
      setPreparedSwap(result.swap);
      setBroadcastHash("");
      onActivity();
      notify("Source transaction recorded. Provider status will determine the outcome.");
    } catch (requestError) {
      setError(
        `${messageFrom(requestError)} The wallet already broadcast ${txHash.slice(0, 10)}…; retry recording this hash, not sending again.`
      );
    } finally {
      setBusy(false);
    }
  }

  async function sendDeposit() {
    if (!preparedSwap || !originAsset || !originChain?.chainId) return;
    const existingHash = broadcastHash || preparedSwap.transactions.source;
    if (existingHash) {
      await recordBroadcast(existingHash);
      return;
    }
    const provider = walletProviderRef.current;
    if (!provider || !walletState.account || !walletOnOriginChain) {
      setError("Connect the wallet and switch it to the quoted source network before sending.");
      return;
    }
    if (preparedQuoteExpired) {
      setError("This provider deposit address has expired. Discard it and request a fresh quote.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const txHash = await sendEvmDeposit({
        provider,
        account: walletState.account,
        chainId: originChain.chainId,
        depositAddress: preparedSwap.quote.depositAddress ?? "",
        amountAtomic: preparedSwap.quote.amountInAtomic,
        tokenAddress: originAsset.contractAddress ?? null,
        depositMemo: preparedSwap.quote.depositMemo
      });
      setBroadcastHash(txHash);
      await recordBroadcast(txHash);
    } catch (requestError) {
      setError(messageFrom(requestError));
      setBusy(false);
    }
  }

  async function discardInstructions() {
    if (!preparedSwap) return;
    setBusy(true);
    setError(null);
    try {
      const result = await api.cancelSwap(preparedSwap.id);
      setPreparedSwap(result.swap);
      onActivity();
      notify(result.swap.state === "EXPIRED"
        ? "The deposit deadline passed. Instructions are marked expired."
        : "Unfunded deposit instructions discarded.");
    } catch (requestError) {
      setError(messageFrom(requestError));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <PageHeader section="swap">
        <StatusPill tone={executionEnabled ? "good" : "warn"}>
          {executionEnabled ? "EVM execution enabled" : liveMode ? "Quote-only mode" : "Fixture mode"}
        </StatusPill>
      </PageHeader>
      <div className="content-grid">
        <section className="panel panel-pad form-panel">
          <h2 className="card-heading">Quote inputs</h2>
          <p className="card-description">
            {liveMode
              ? "Request a provider quote for an EVM-to-EVM pair. Wallet transfer is a separate, explicit confirmation."
              : "Fixture estimates exercise the interface only. They can never be turned into deposit instructions."}
          </p>
          <div className="wallet-connect-row">
            <div>
              <strong>{walletState.account ? shortenEvmAddress(walletState.account) : "EVM wallet disconnected"}</strong>
              <span>
                {walletState.account
                  ? `${data.chains.find((chain) => chain.chainId === walletState.chainId)?.name ?? "Unsupported network"}${walletState.providerName ? ` · ${walletState.providerName}` : ""}`
                  : "Connect to prepare a live deposit quote."}
              </span>
            </div>
            <button className="button button-secondary" type="button" onClick={() => void connectWallet()} disabled={walletBusy || locked}>
              {walletBusy ? "Connecting…" : walletState.account ? "Reconnect wallet" : "Connect wallet"}
            </button>
          </div>
          {walletState.error ? <p className="warning-text" role="alert">{walletState.error}</p> : null}
          <form onSubmit={requestQuote}>
            <div className="form-grid">
              <div className="field">
                <label htmlFor="origin-asset">From asset</label>
                <select id="origin-asset" value={originAssetId} onChange={(event) => { invalidateReviewedQuote(); setOriginAssetId(event.target.value); }} required disabled={locked}>
                  {(liveMode ? data.assets.filter((asset) =>
                    EXECUTABLE_EVM_CHAINS.has(asset.chain) && asset.sourceDepositSupported
                  ) : data.assets).map((asset) => (
                    <option value={asset.id} key={asset.id}>{asset.symbol} · {asset.chain} · {asset.name}</option>
                  ))}
                </select>
              </div>
              <div className="field">
                <label htmlFor="destination-asset">To asset</label>
                <select id="destination-asset" value={destinationAssetId} onChange={(event) => { invalidateReviewedQuote(); setDestinationAssetId(event.target.value); }} required disabled={locked}>
                  {data.assets.filter((asset) =>
                    asset.id !== originAssetId
                    && (!liveMode || EXECUTABLE_EVM_CHAINS.has(asset.chain))
                  ).map((asset) => (
                    <option value={asset.id} key={asset.id}>{asset.symbol} · {asset.chain} · {asset.name}</option>
                  ))}
                </select>
              </div>
              <div className="field">
                <label htmlFor="amount-in">Amount</label>
                <input id="amount-in" inputMode="decimal" autoComplete="off" value={amountIn} onChange={(event) => { invalidateReviewedQuote(); setAmountIn(event.target.value); }} required disabled={locked} />
                <span className="field-hint">Enter a decimal string; precision is checked against the asset catalogue.</span>
              </div>
              <div className="field">
                <label htmlFor="slippage">Maximum slippage</label>
                <select id="slippage" value={slippageBps} onChange={(event) => { invalidateReviewedQuote(); setSlippageBps(Number(event.target.value)); }} disabled={locked}>
                  <option value={50}>0.50%</option>
                  <option value={100}>1.00%</option>
                  <option value={300}>3.00%</option>
                </select>
              </div>
              <div className="field full-width">
                <label htmlFor="recipient">Destination address{liveMode ? " (required for live quote)" : ""}</label>
                <input id="recipient" autoComplete="off" value={recipient} onChange={(event) => { invalidateReviewedQuote(); setRecipient(event.target.value); }} required={liveMode} disabled={locked} />
              </div>
              <div className="field full-width">
                <label htmlFor="refund-address">Refund address{liveMode ? " (required for live quote)" : ""}</label>
                <input id="refund-address" autoComplete="off" value={refundTo} onChange={(event) => { invalidateReviewedQuote(); setRefundTo(event.target.value); }} required={liveMode} disabled={locked} />
              </div>
            </div>
            <div className="swap-actions">
              <button className="button" type="submit" disabled={busy || locked || !originAssetId || !destinationAssetId || originAssetId === destinationAssetId}>
                {busy ? "Requesting quote…" : "Review quote"}
              </button>
            </div>
          </form>
          {error ? <div className="notice-block" role="alert"><p>{error}</p></div> : null}
          {quote ? (
            <div className="quote-result" aria-live="polite">
              <h3>{quote.mode === "provider_dry_quote" ? "Provider dry quote" : "Illustrative fixture estimate"}</h3>
              <p className="quote-number">{quote.amountOutEstimate} <small>{quote.destinationAsset.symbol}</small></p>
              <div className="quote-detail"><span>Minimum output</span><strong>{quote.minimumAmountOut ?? "Not returned"} {quote.destinationAsset.symbol}</strong></div>
              <div className="quote-detail"><span>Slippage setting</span><strong>{(quote.slippageBps / 100).toFixed(2)}%</strong></div>
              <div className="quote-detail"><span>Provider</span><strong>{quote.provider}</strong></div>
              {quote.expiresAt ? <div className="quote-detail"><span>Quote expiry</span><strong>{formatTimestamp(quote.expiresAt)}</strong></div> : null}
              <p className="warning-text">{quote.warning}</p>
              {quote.mode === "provider_dry_quote" ? (
                <>
                  <p className="metric-note">
                    Dry quote only: no deposit address is active yet. The executable quote will refresh price and deadline.
                  </p>
                  <button
                    className="button button-secondary button-small"
                    type="button"
                    onClick={() => void prepareExecutableSwap()}
                    disabled={
                      busy
                      || !executionEnabled
                      || !walletOnOriginChain
                      || !executableOrigin
                      || !executableDestination
                      || !normalizeEvmAddress(recipient)
                      || !normalizeEvmAddress(refundTo)
                    }
                  >
                    {busy ? "Preparing…" : "Prepare executable quote"}
                  </button>
                  {!data.health?.liveQuotesConfigured ? (
                    <p className="warning-text">A server-side NEAR Intents partner JWT is required for live quotes.</p>
                  ) : !executionEnabled ? (
                    <p className="warning-text">Live deposits are server-gated. Enable them only after the provider credential and route review are complete.</p>
                  ) : !walletOnOriginChain ? (
                    <p className="warning-text">Connect an EVM wallet on {originChain?.name ?? "the quoted source network"} to continue.</p>
                  ) : !executableOrigin || !executableDestination ? (
                    <p className="warning-text">This pair is outside the verified EVM transfer slice.</p>
                  ) : null}
                </>
              ) : null}
              {quote.mode === "simulation" ? (
                <button className="button button-secondary button-small" type="button" onClick={createSimulation} disabled={busy}>
                  Create local simulation record
                </button>
              ) : null}
            </div>
          ) : null}
          {preparedSwap ? (
            <div className="quote-result executable-result" aria-live="polite">
              <h3>Provider deposit instructions</h3>
              <div className="quote-detail"><span>State</span><strong>{preparedSwap.state.replaceAll("_", " ")}</strong></div>
              <p className="quote-number">{preparedSwap.quote.amountOutEstimate} <small>{preparedSwap.quote.destinationAsset.symbol} estimated</small></p>
              <div className="quote-detail"><span>Minimum output</span><strong>{preparedSwap.quote.minimumAmountOut ?? "Not returned"} {preparedSwap.quote.destinationAsset.symbol}</strong></div>
              <div className="quote-detail"><span>Send exactly</span><strong>{preparedSwap.quote.amountIn} {preparedSwap.quote.originAsset.symbol}</strong></div>
              <div className="quote-detail"><span>Provider deadline</span><strong>{remainingSeconds === null ? "Not returned" : preparedQuoteExpired ? "Expired" : `${remainingSeconds}s remaining`}</strong></div>
              <div className="deposit-address">
                <span>Deposit address · {preparedSwap.quote.originAsset.chain}</span>
                <code>{preparedSwap.quote.depositAddress ?? "Not available"}</code>
              </div>
              {preparedSwap.quote.depositMemo ? (
                <div className="deposit-address"><span>Provider memo</span><code>{preparedSwap.quote.depositMemo}</code></div>
              ) : null}
              {preparedSwap.state === "AWAITING_USER_DEPOSIT" && !preparedSwap.transactions.source ? (
                <div className="deposit-actions">
                  <button
                    className="button"
                    type="button"
                    onClick={() => void sendDeposit()}
                    disabled={busy || preparedQuoteExpired || !walletOnOriginChain}
                  >
                    {busy
                      ? "Waiting for wallet…"
                      : broadcastHash
                        ? "Retry recording transaction hash"
                        : "Review and send in wallet"}
                  </button>
                  <button className="button button-quiet button-small" type="button" onClick={() => void discardInstructions()} disabled={busy}>
                    Discard instructions
                  </button>
                </div>
              ) : null}
              {preparedSwap.transactions.source ? (
                <SwapTransaction
                  label="Source transaction"
                  hash={preparedSwap.transactions.source}
                  explorerUrl={preparedSwap.transactionLinks?.source}
                />
              ) : null}
              {preparedSwap.transactions.destination ? (
                <SwapTransaction
                  label="Destination transaction"
                  hash={preparedSwap.transactions.destination}
                  explorerUrl={preparedSwap.transactionLinks?.destination}
                />
              ) : null}
              {preparedSwap.transactions.refund ? (
                <SwapTransaction
                  label="Refund transaction"
                  hash={preparedSwap.transactions.refund}
                  explorerUrl={preparedSwap.transactionLinks?.refund}
                />
              ) : null}
              {preparedSwap.providerStatus ? (
                <p className="metric-note">Provider status: {preparedSwap.providerStatus} · updated {formatTimestamp(preparedSwap.providerUpdatedAt)}</p>
              ) : null}
              <p className="warning-text">
                Verify the source network, exact amount and destination before approving. A wallet broadcast is not proof of swap settlement.
              </p>
            </div>
          ) : null}
        </section>

        <aside>
          <section className="panel panel-pad">
            <PanelTitle title="Execution checklist" description="Provider status is authoritative after wallet broadcast." />
            <ul className="swap-timeline">
              <li className="current"><strong>Quote review</strong><small>{liveMode ? "Provider dry quote, then a refreshed executable quote" : "Local estimate only; no route is requested"}</small></li>
              <li><strong>Wallet transfer</strong><small>Injected EVM wallet only; the wallet signs and broadcasts</small></li>
              <li><strong>Provider settlement</strong><small>Source, destination and refund hashes remain separate</small></li>
            </ul>
            <div className="simulation-callout">
              <strong>Capabilities are gated.</strong> Only EVM pairs with verified transfer metadata can reach wallet signing; live routes still require a successful quote.
            </div>
          </section>
          <section className="panel panel-pad section">
            <PanelTitle title="Catalogue note" />
            <p className="card-description">{data.assets.length} assets are visible. Catalogue presence is not route confirmation; only an executable quote supplies deposit instructions.</p>
            <Link className="text-link" href="/workspace/risk">Open risk center</Link>
          </section>
        </aside>
      </div>
    </>
  );
}

function ActivityView({
  swaps,
  loading,
  onAdvance,
  onCancel,
  onRefresh
}: {
  swaps: SwapRecord[];
  loading: boolean;
  onAdvance: (swap: SwapRecord) => void;
  onCancel: (swap: SwapRecord) => void;
  onRefresh: () => void;
}) {
  const liveCount = swaps.filter((swap) => swap.mode === "live").length;
  const simulationCount = swaps.length - liveCount;
  return (
    <>
      <PageHeader section="activity">
        <button className="button button-secondary" type="button" onClick={onRefresh}>Refresh activity</button>
      </PageHeader>
      <section className="panel panel-pad">
        <div className="section-heading">
          <div><h2>Swap history</h2><p>{liveCount} provider-tracked · {simulationCount} local simulations</p></div>
          <span className="subtle-meta">{loading ? "Refreshing…" : "Auto-refreshes pending provider states"}</span>
        </div>
        {swaps.length === 0 ? (
          <div className="empty-state">No swaps or simulations yet. Review a quote to start a record.</div>
        ) : (
          <div className="swap-record-list">
            {swaps.map((swap) => {
              const isSimulation = swap.mode !== "live";
              const canCancel = !isSimulation
                && swap.state === "AWAITING_USER_DEPOSIT"
                && !swap.transactions.source;
              const statusTone = swap.state === "SUCCEEDED" || swap.state === "SIMULATION_COMPLETE"
                ? "good"
                : TERMINAL_SWAP_STATES.has(swap.state)
                  ? "bad"
                  : "warn";
              return (
                <article className="swap-record" key={swap.id}>
                  <div className="swap-record-heading">
                    <div>
                      <strong>{swap.quote.originAsset.symbol} → {swap.quote.destinationAsset.symbol}</strong>
                      <span>{swap.quote.originAsset.chain} to {swap.quote.destinationAsset.chain}</span>
                    </div>
                    <StatusPill tone={statusTone}>{swap.state.replaceAll("_", " ")}</StatusPill>
                  </div>
                  <div className="swap-record-meta">
                    <span>{isSimulation ? "Local simulation" : `Provider: ${swap.providerStatus ?? "awaiting deposit"}`}</span>
                    <time dateTime={swap.createdAt}>{formatTimestamp(swap.createdAt)}</time>
                    <code title={swap.id}>{swap.id.slice(0, 14)}…</code>
                  </div>
                  <div className="swap-transactions">
                    <SwapTransaction label="Source" hash={swap.transactions.source} explorerUrl={swap.transactionLinks?.source} />
                    <SwapTransaction label="Destination" hash={swap.transactions.destination} explorerUrl={swap.transactionLinks?.destination} />
                    <SwapTransaction label="Refund" hash={swap.transactions.refund} explorerUrl={swap.transactionLinks?.refund} />
                    {!swap.transactions.source && !swap.transactions.destination && !swap.transactions.refund ? (
                      <span className="subtle-meta">{isSimulation ? "No chain transaction was created." : "No source deposit recorded yet."}</span>
                    ) : null}
                  </div>
                  <div className="swap-record-actions">
                    {!isSimulation && swap.providerUpdatedAt ? (
                      <span className="subtle-meta">Provider updated {formatTimestamp(swap.providerUpdatedAt)}</span>
                    ) : null}
                    {canCancel ? (
                      <button className="button button-quiet button-small" type="button" onClick={() => onCancel(swap)}>
                        Discard quote
                      </button>
                    ) : null}
                    {isSimulation ? (
                      <button
                        className="button button-quiet button-small"
                        type="button"
                        onClick={() => onAdvance(swap)}
                        disabled={TERMINAL_SWAP_STATES.has(swap.state)}
                      >
                        {TERMINAL_SWAP_STATES.has(swap.state) ? "Complete" : "Advance simulation"}
                      </button>
                    ) : null}
                  </div>
                </article>
              );
            })}
          </div>
        )}
        <p className="metric-note">A source transaction is only a deposit attempt; completion is shown only when the provider reports success.</p>
      </section>
    </>
  );
}

function PortfolioView({
  chains,
  notify
}: {
  chains: ChainConfig[];
  notify: (message: string) => void;
}) {
  const [walletState, setWalletState] = useState<EvmWalletState>({
    status: "disconnected",
    providerId: null,
    providerName: null,
    account: null,
    chainId: null,
    balance: null,
    balanceStatus: "idle",
    error: null
  });
  const walletRef = useRef<ReadOnlyEvmWallet | null>(null);

  useEffect(() => {
    walletRef.current = createReadOnlyEvmWallet({
      resolveChain: (chainId) => {
        const chain = chains.find((candidate) => candidate.chainId === chainId);
        return chain?.chainId ? { ...chain, chainId: chain.chainId } : null;
      },
      onChange: setWalletState
    });
    return () => {
      walletRef.current?.forget();
      walletRef.current = null;
    };
  }, [chains]);

  async function connect() {
    if (!window.ethereum) {
      notify("No injected EVM wallet was detected in this browser.");
      return;
    }
    const wallet = walletRef.current;
    if (!wallet) return;
    const provider = window.ethereum;
    const name = provider.isMetaMask ? "MetaMask" : provider.isCoinbaseWallet ? "Coinbase Wallet" : "Injected EVM wallet";
    await wallet.connect({ id: "injected-evm", name, provider });
  }

  const activeChain = chains.find((chain) => chain.chainId === walletState.chainId);

  return (
    <>
      <PageHeader section="portfolio">
        {walletState.status === "connected" || walletState.status === "unsupported-network"
          ? <button className="button button-secondary" type="button" onClick={() => walletRef.current?.forget()}>Disconnect</button>
          : <button className="button" type="button" onClick={() => void connect()}>Connect EVM wallet</button>}
      </PageHeader>
      <div className="content-grid">
        <section className="panel panel-pad">
          <PanelTitle
            title="Native balance"
            description="Read-only wallet RPC request. No signature is requested."
            action={walletState.status === "connected" && activeChain?.nativeCurrency
              ? (
                <button
                  className="button button-quiet balance-refresh"
                  type="button"
                  disabled={walletState.balanceStatus === "loading"}
                  onClick={() => void walletRef.current?.refreshBalance()}
                >
                  {walletState.balanceStatus === "loading" ? "Refreshing…" : "Refresh balance"}
                </button>
              )
              : undefined}
          />
          {walletState.status === "connected" && activeChain?.nativeCurrency ? (
            <div className="kpi-grid">
              <div className="kpi-item">
                <span className="metric-label">{activeChain.nativeCurrency.symbol} balance</span>
                <span className="metric-value">{walletState.balance ?? "—"}</span>
                <span className="metric-note">
                  {walletState.balanceStatus === "loading" && walletState.balance !== null
                    ? `${activeChain.name} · refreshing; showing last known balance`
                    : walletState.balanceStatus === "error" && walletState.balance !== null
                      ? `${activeChain.name} · refresh failed; showing last known balance`
                      : `${activeChain.name} · ${walletState.balanceStatus}`}
                </span>
              </div>
              <div className="kpi-item">
                <span className="metric-label">Connected account</span>
                <span className="metric-value mono" style={{ fontSize: "15px" }}>{shortenEvmAddress(walletState.account)}</span>
                <span className="metric-note">{walletState.providerName}</span>
              </div>
            </div>
          ) : (
            <div className="empty-state">
              {walletState.status === "unsupported-network"
                ? "Wallet connected on an unsupported network. No balance was requested."
                : walletState.status === "connecting"
                  ? "Check the wallet prompt to continue."
                  : "Connect an injected EVM wallet to read its native balance."}
            </div>
          )}
          {walletState.error ? <p className="warning-text" role="alert">{walletState.error}</p> : null}
          <div className="notice-block section">
            <h3>Coverage is intentionally narrow</h3>
            <p>This screen does not enumerate ERC-20 tokens, staking positions, LP positions or other chain families.</p>
          </div>
        </section>
        <section className="panel panel-pad">
          <PanelTitle title="Configured EVM networks" description="Wallet metadata does not imply swap support." />
          <div className="ecosystem-list">
            {chains.filter((chain) => chain.family === "EVM").map((chain) => (
              <div className="ecosystem-row" key={chain.key}>
                <div className="row-title"><strong>{chain.name}</strong><small>Chain ID {chain.chainId ?? "not configured"}</small></div>
                <StatusPill>{chain.walletSupported ? "Wallet metadata" : "Unavailable"}</StatusPill>
                <span className="subtle-meta">Swap off</span>
              </div>
            ))}
          </div>
        </section>
      </div>
    </>
  );
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

function MetricsView({
  section,
  snapshot,
  health,
  onRefresh
}: {
  section: "markets" | "intelligence";
  snapshot: DataSnapshot | null;
  health: HealthStatus | null;
  onRefresh: () => void;
}) {
  const title = section === "markets" ? "Market observations" : "NEAR indicator dictionary";
  const description = section === "markets"
    ? "Metrics are reported as received. TVL, volume, fees and revenue are different measures."
    : "Raw observations stay separate from derived interpretation and scenario assumptions.";

  return (
    <>
      <PageHeader section={section}>
        <button className="button button-secondary" type="button" onClick={onRefresh}>Refresh source</button>
        {snapshot ? (
          <>
            <button className="button button-secondary" type="button" onClick={() => downloadSnapshot(snapshot, "csv")}>Export CSV</button>
            <button className="button button-secondary" type="button" onClick={() => downloadSnapshot(snapshot, "json")}>Export JSON</button>
          </>
        ) : null}
      </PageHeader>
      <section className="panel panel-pad">
        <PanelTitle
          title={title}
          description={description}
          action={<StatusPill tone={snapshot?.mode === "live" ? "good" : "warn"}>{snapshot?.mode === "live" ? "Live" : "Fixture"}</StatusPill>}
        />
        {snapshot?.notice ? (
          <div className="notice-block data-source-notice section">
            <span className="notice-mark" aria-hidden="true">i</span>
            <p>{snapshot.notice}</p>
            <small>Mode: {health?.marketDataMode ?? snapshot.mode} · Retrieved: {formatTimestamp(snapshot.retrievedAt ?? snapshot.capturedAt)}</small>
          </div>
        ) : null}
        {snapshot?.metrics.length ? (
          <div className="table-wrap">
            <table className="data-table mobile-stack">
              <thead><tr><th>Metric</th><th className="num">Value</th><th>Unit and scope</th><th>Observed</th><th>Source</th></tr></thead>
              <tbody>
                {snapshot.metrics.map((metric) => (
                  <tr key={metric.key}>
                    <td data-label="Metric">
                      <strong>{metric.label}</strong>
                      <span className="row-title"><small>{metric.definition}</small></span>
                    </td>
                    <td className="num" data-label="Value">{displayObservation(metric)}</td>
                    <td data-label="Unit and scope">{metric.unit}<span className="row-title"><small>{metric.scope}</small></span></td>
                    <td data-label="Observed">{formatTimestamp(metric.observedAt ?? metric.retrievedAt)}</td>
                    <td data-label="Source">{metric.sourceUrl
                      ? <a href={metric.sourceUrl} target="_blank" rel="noreferrer">{metric.source}</a>
                      : metric.source}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="empty-state">{snapshot ? "No observations are available for this source." : "Loading metric observations…"}</div>
        )}
      </section>
      {snapshot?.chainComparisons?.length ? (
        <section className="section panel panel-pad">
          <PanelTitle title="Selected chain comparisons" description="Fixture comparison values remain illustrative." />
          <div className="table-wrap">
            <table className="data-table mobile-stack">
              <thead><tr><th>Chain</th><th className="num">TVL (USD)</th><th className="num">Daily volume (USD)</th><th>Source</th></tr></thead>
              <tbody>{snapshot.chainComparisons.map((item) => (
                <tr key={item.chain}>
                  <td data-label="Chain">{item.chain}</td>
                  <td className="num" data-label="TVL">{formatMetric(item.tvlUsd, "USD")}</td>
                  <td className="num" data-label="Daily volume">{formatMetric(item.dailyVolumeUsd, "USD")}</td>
                  <td data-label="Source">{item.source}</td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        </section>
      ) : null}
    </>
  );
}

function ResearchView({
  inputs,
  onRetry
}: {
  inputs: ResearchInputs | null;
  onRetry: () => void;
}) {
  const base = inputs?.scenarios.find((scenario) => scenario.key === "base") ?? inputs?.scenarios[0];
  const [dailyVolume, setDailyVolume] = useState(17_600_000);
  const [feeRatePct, setFeeRatePct] = useState(0.18);
  const [capturePct, setCapturePct] = useState(28);
  const [activityGrowthPct, setActivityGrowthPct] = useState(12);

  useEffect(() => {
    if (!base) return;
    setDailyVolume(base.dailyVolumeUsd);
    setFeeRatePct(base.feeRatePct);
    setCapturePct(base.capturePct);
    setActivityGrowthPct(base.activityGrowthPct);
  }, [base]);

  const annualFees = dailyVolume * (1 + activityGrowthPct / 100) * (feeRatePct / 100) * (capturePct / 100) * 365;

  return (
    <>
      <PageHeader section="research" />
      {!inputs ? (
        <div className="empty-state">Research inputs could not be loaded. <button className="text-link" type="button" onClick={onRetry}>Retry</button></div>
      ) : (
        <div className="content-grid">
          <section className="panel panel-pad">
            <h2 className="card-heading">Base-case assumptions</h2>
            <p className="card-description">{inputs.note}</p>
            <div className="scenario-controls">
              <RangeControl label="Daily DEX volume (USD)" value={dailyVolume} min={1_000_000} max={100_000_000} step={100_000} format={(value) => formatMetric(value, "USD")} onChange={setDailyVolume} />
              <RangeControl label="Fee rate" value={feeRatePct} min={0.01} max={1} step={0.01} format={(value) => `${value.toFixed(2)}%`} onChange={setFeeRatePct} />
              <RangeControl label="Fee capture" value={capturePct} min={1} max={100} step={1} format={(value) => `${value.toFixed(0)}%`} onChange={setCapturePct} />
              <RangeControl label="Activity growth" value={activityGrowthPct} min={-50} max={100} step={1} format={(value) => `${value > 0 ? "+" : ""}${value.toFixed(0)}%`} onChange={setActivityGrowthPct} />
            </div>
            <div className="quote-result scenario-table">
              <h3>Calculated annual captured fees</h3>
              <p className="quote-number">{formatMetric(annualFees, "USD")} <small>/ year</small></p>
              <p className="field-hint">Arithmetic scenario only. This is not revenue guidance or a token valuation.</p>
            </div>
          </section>
          <aside className="panel panel-pad">
            <PanelTitle title="Model inputs" description="Source data and fictional scenario assumptions are kept distinct." />
            <div className="table-wrap scenario-table">
              <table className="data-table mobile-stack">
                <thead><tr><th>Case</th><th className="num">Volume / day</th><th className="num">Fee</th><th className="num">Capture</th><th className="num">Growth</th></tr></thead>
                <tbody>{inputs.scenarios.map((scenario) => (
                  <tr key={scenario.key}>
                    <td data-label="Case">{scenario.label}</td>
                    <td className="num" data-label="Volume">{formatMetric(scenario.dailyVolumeUsd, "USD")}</td>
                    <td className="num" data-label="Fee">{scenario.feeRatePct.toFixed(2)}%</td>
                    <td className="num" data-label="Capture">{scenario.capturePct}%</td>
                    <td className="num" data-label="Growth">{scenario.activityGrowthPct > 0 ? "+" : ""}{scenario.activityGrowthPct}%</td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
            <div className="notice-block formula-block">
              <h3>Formula</h3>
              <p>{inputs.formula}</p>
            </div>
          </aside>
        </div>
      )}
    </>
  );
}

function RangeControl({
  label,
  value,
  min,
  max,
  step,
  format,
  onChange
}: {
  label: string;
  value: number;
  min: number;
  max: number;
  step: number;
  format: (value: number) => string;
  onChange: (value: number) => void;
}) {
  const id = `range-${label.toLowerCase().replace(/[^a-z0-9]+/g, "-")}`;
  return (
    <label className="range-field" htmlFor={id}>
      <span className="range-label"><span>{label}</span><output>{format(value)}</output></span>
      <input id={id} type="range" min={min} max={max} step={step} value={value} onChange={(event) => onChange(Number(event.target.value))} />
    </label>
  );
}

function RiskView({ health, snapshot, chains }: { health: HealthStatus | null; snapshot: DataSnapshot | null; chains: ChainConfig[] }) {
  const quoteStatus = health?.quoteMode === "live"
    ? health.liveQuotesConfigured ? "Dry quotes configured" : "Credential missing"
    : "Local fixture estimates";
  const executionStatus = health?.executionEnabled
    ? "EVM deposits enabled by server configuration"
    : "EVM deposits disabled";
  const dataStatus = health?.marketDataMode === "live"
    ? snapshot?.status ?? "Live source status unavailable"
    : "Illustrative fixture";
  const entries = [
    ...RISK_ROWS.slice(0, 2),
    {
      title: "Quote configuration",
      detail: `Current mode: ${quoteStatus}. Credentials remain server-side and never reach the browser.`,
      status: quoteStatus,
      tone: health?.liveQuotesConfigured ? "good" : "warn"
    },
    {
      title: "Swap execution",
      detail: "Only the EVM source-transfer path is implemented. Pair support still requires a fresh provider quote; this local prototype is not a production execution service.",
      status: executionStatus,
      tone: health?.executionEnabled ? "warn" : ""
    },
    {
      title: "Market data freshness",
      detail: `Current data status: ${dataStatus}. Retrieved ${formatTimestamp(snapshot?.retrievedAt ?? snapshot?.capturedAt)}.`,
      status: dataStatus,
      tone: snapshot?.mode === "live" ? "good" : "warn"
    },
    ...RISK_ROWS.slice(2)
  ];
  return (
    <>
      <PageHeader section="risk" />
      <div className="content-grid">
        <section className="panel panel-pad">
          <PanelTitle title="Execution and data controls" description="A deposit broadcast or frontend callback cannot mark a swap complete." />
          <div className="risk-list">
            {entries.map((entry) => (
              <div className="risk-row" key={entry.title}>
                <div className="row-title"><strong>{entry.title}</strong><small>{entry.status}</small></div>
                <p>{entry.detail}</p>
                <StatusPill tone={entry.tone}>{entry.status}</StatusPill>
              </div>
            ))}
          </div>
        </section>
        <aside className="panel panel-pad">
          <PanelTitle title="Network capability" description={`${chains.filter((chain) => chain.swapEnabled).length} swap-enabled chains`} />
          <p className="card-description">A chain may have wallet configuration without a provider route. Pair support is confirmed by quote, not by the chain list.</p>
          <div className="notice-block">
            <h3>Execution remains opt-in</h3>
            <p>
              {health?.executionEnabled
                ? "The server execution gate is enabled. This prototype is not audited or production-ready; verify every route and transaction before signing."
                : "Wallet signing, deposit submission and provider status tracking are implemented for the initial EVM slice but disabled by default."}
            </p>
          </div>
          <Link className="button button-secondary section" href="/workspace/swap">Open swap workspace</Link>
        </aside>
      </div>
    </>
  );
}

export function WorkspaceShell({ initialSection = "overview" }: { initialSection?: SectionKey }) {
  const pathname = usePathname();
  const [menuOpen, setMenuOpen] = useState(false);
  const [toast, setToast] = useState("");
  const [reloadVersion, setReloadVersion] = useState(0);
  const [data, setData] = useState<AppData>({
    health: null,
    chains: [],
    assets: [],
    markets: null,
    intelligence: null,
    swaps: [],
    research: null,
    error: null,
    loading: true
  });
  const sectionFromPath = pathname.split("/").filter(Boolean)[1] as SectionKey | undefined;
  const section = sectionFromPath && KNOWN_SECTIONS.has(sectionFromPath) ? sectionFromPath : initialSection;
  const page = PAGE_INFO[section];
  const hasPendingLiveSwaps = data.swaps.some((swap) =>
    swap.mode === "live" && !TERMINAL_SWAP_STATES.has(swap.state)
  );

  const reload = useCallback(() => setReloadVersion((version) => version + 1), []);

  useEffect(() => {
    let active = true;
    setData((current) => ({ ...current, loading: true, error: null }));
    void Promise.allSettled([
      api.health(),
      api.chains(),
      api.assets(),
      api.markets(),
      api.intelligence(),
      api.activity(),
      api.research()
    ]).then((results) => {
      if (!active) return;
      const [health, chains, assets, markets, intelligence, activity, research] = results;
      const firstFailure = results.find((result) => result.status === "rejected");
      setData({
        health: health.status === "fulfilled" ? health.value : null,
        chains: chains.status === "fulfilled" ? chains.value.chains : [],
        assets: assets.status === "fulfilled" ? assets.value.assets : [],
        markets: markets.status === "fulfilled" ? markets.value : null,
        intelligence: intelligence.status === "fulfilled" ? intelligence.value : null,
        swaps: activity.status === "fulfilled" ? activity.value.swaps : [],
        research: research.status === "fulfilled" ? research.value : null,
        error: firstFailure?.status === "rejected" ? messageFrom(firstFailure.reason) : null,
        loading: false
      });
    });
    return () => { active = false; };
  }, [reloadVersion]);

  useEffect(() => {
    if (!hasPendingLiveSwaps) return;
    let active = true;
    const refreshPendingActivity = async () => {
      try {
        const activity = await api.activity();
        if (active) setData((current) => ({ ...current, swaps: activity.swaps }));
      } catch {
        // Keep the last known lifecycle visible; manual refresh can surface broader API errors.
      }
    };
    const timer = window.setInterval(() => { void refreshPendingActivity(); }, 5000);
    return () => {
      active = false;
      window.clearInterval(timer);
    };
  }, [hasPendingLiveSwaps]);

  useEffect(() => {
    if (!toast) return;
    const timer = window.setTimeout(() => setToast(""), 4200);
    return () => window.clearTimeout(timer);
  }, [toast]);

  const notify = useCallback((message: string) => setToast(message), []);

  async function advanceSimulation(swap: SwapRecord) {
    try {
      await api.advanceSimulation(swap.id);
      notify("Simulation advanced. No chain transaction occurred.");
      reload();
    } catch (error) {
      notify(messageFrom(error));
    }
  }

  async function discardPendingSwap(swap: SwapRecord) {
    try {
      const result = await api.cancelSwap(swap.id);
      setData((current) => ({
        ...current,
        swaps: current.swaps.map((item) => item.id === result.swap.id ? result.swap : item)
      }));
      notify(result.swap.state === "EXPIRED"
        ? "The provider deadline passed; the quote is marked expired."
        : "Unfunded deposit instructions discarded.");
    } catch (error) {
      notify(messageFrom(error));
    }
  }

  const isLive = data.health?.marketDataMode === "live";
  const executionEnabled = data.health?.executionEnabled === true;
  const executionStatus = executionEnabled
    ? "EVM deposit flow enabled"
    : data.health?.liveQuotesConfigured
      ? "Provider quotes only"
      : "Swap execution disabled";

  return (
    <div className="workspace-page">
      <button className="mobile-scrim" type="button" hidden={!menuOpen} aria-label="Close navigation" onClick={() => setMenuOpen(false)} />
      <aside className={`sidebar${menuOpen ? " is-open" : ""}`} aria-label="Workspace navigation">
        <Link className="brand" href="/">
          <BrandMark />
          <span className="brand-name">Zonyx</span>
        </Link>
        <div className="workspace-switch">
          <span className="workspace-glyph">U</span>
          <span><strong>Research workspace</strong><small>{data.health?.mode ?? "Local demo"}</small></span>
          <span className="switch-chevron" aria-hidden="true">⌄</span>
        </div>
        <nav className="nav-groups">
          {NAV_GROUPS.map((group) => (
            <div key={group.title}>
              <p className="nav-label">{group.title}</p>
              {group.items.map((item) => (
                <Link
                  className="nav-link"
                  href={item.key === "overview" ? "/workspace" : `/workspace/${item.key}`}
                  aria-current={section === item.key ? "page" : undefined}
                  key={item.key}
                  onClick={() => setMenuOpen(false)}
                >
                  <span className="nav-icon" aria-hidden="true">{item.glyph}</span>
                  <span>{item.label}</span>
                </Link>
              ))}
            </div>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="connection-status">
            <span className={`status-dot ${isLive ? "status-dot-positive" : "status-dot-neutral"}`} />
            <span><strong>{isLive ? "Live research source" : "Sandbox mode"}</strong><small>{executionStatus}</small></span>
          </div>
          <Link className="sidebar-help" href="/workspace/risk">
            <span>Execution limits</span><span aria-hidden="true">↗</span>
          </Link>
        </div>
      </aside>

      <div className="mobile-bar">
        <button className="menu-button" type="button" aria-label="Open navigation" aria-expanded={menuOpen} onClick={() => setMenuOpen(true)}>
          <span /><span /><span />
        </button>
        <span className="mobile-title">{page.title}</span>
        <Link className="mobile-mark" href="/" aria-label="Zonyx home"><BrandMark /></Link>
      </div>

      <div className="main-shell">
        <div className="global-notice">
          <span className="notice-mark" aria-hidden="true">i</span>
          <p>
            <strong>Local prototype</strong>
            <span>
              {executionEnabled
                ? "EVM source deposits are enabled by server configuration. Review the route, exact amount and expiry before signing."
                : "Provider quotes and lifecycle tracking are available when configured; EVM deposits remain disabled by default."}
            </span>
          </p>
          <Link href="/workspace/risk">Review limits</Link>
        </div>
        <main className="app-content">
          {section === "overview" ? <OverviewView data={data} onRefresh={reload} /> : null}
          {section === "swap" ? <SwapView data={data} notify={notify} onActivity={reload} /> : null}
          {section === "activity" ? <ActivityView swaps={data.swaps} loading={data.loading} onAdvance={(swap) => void advanceSimulation(swap)} onCancel={(swap) => void discardPendingSwap(swap)} onRefresh={reload} /> : null}
          {section === "portfolio" ? <PortfolioView chains={data.chains} notify={notify} /> : null}
          {section === "markets" ? <MetricsView section="markets" snapshot={data.markets} health={data.health} onRefresh={reload} /> : null}
          {section === "intelligence" ? <MetricsView section="intelligence" snapshot={data.intelligence} health={data.health} onRefresh={reload} /> : null}
          {section === "research" ? <ResearchView inputs={data.research} onRetry={reload} /> : null}
          {section === "risk" ? <RiskView health={data.health} snapshot={data.markets} chains={data.chains} /> : null}
          {data.loading && !data.health ? <p className="loading-state" role="status">Connecting to the local API…</p> : null}
          <footer className="app-footer">
            <span>Zonyx · local research workspace</span>
            <span>Research interpretation is separate from transaction execution.</span>
          </footer>
        </main>
      </div>

      <div className="toast-region" aria-live="polite" aria-atomic="true">
        {toast ? <div className="toast">{toast}</div> : null}
      </div>
    </div>
  );
}
