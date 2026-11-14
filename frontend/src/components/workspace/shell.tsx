"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useId, useMemo, useRef, useState } from "react";
import type { ReactNode } from "react";
import { USE_MOCKS } from "../../lib/api-client";
import { shortenEvmAddress } from "../../lib/evm-wallet";
import { explorerAddressUrl } from "../../lib/explorer";
import { STATE_COPY } from "../../lib/view-state";
import { Modal } from "./ui";
import { useWorkspace, WorkspaceProvider } from "./workspace-context";

export const NAV = [
  { key: "explore", label: "Explore", glyph: "◎" },
  { key: "swap", label: "Swap", glyph: "⇄" },
  { key: "activity", label: "Activity", glyph: "↗" },
  { key: "portfolio", label: "Portfolio", glyph: "◉" },
  { key: "markets", label: "Markets", glyph: "⌁" },
  { key: "trading", label: "Trading", glyph: "≡" },
  { key: "intelligence", label: "Intelligence", glyph: "#" },
  { key: "research", label: "Research", glyph: "▤" },
  { key: "risk", label: "Risk", glyph: "◇" },
] as const;

function activeKey(pathname: string): string {
  const seg = pathname.split("/")[2] ?? "";
  return seg === "" || seg === "overview" ? "explore" : seg;
}

function BrandMark() {
  // eslint-disable-next-line @next/next/no-img-element
  return <img className="brand-mark" src="/images/sightline-mark.svg" alt="" width={28} height={28} />;
}

function GlobalSearch() {
  const { catalogue, chains } = useWorkspace();
  const router = useRouter();
  const [q, setQ] = useState("");
  const [open, setOpen] = useState(false);
  const [cursor, setCursor] = useState(0);
  const id = useId();
  const term = q.trim().toLowerCase();

  const results = useMemo(() => {
    if (!term) return [];
    const chainHits = (chains.data ?? [])
      .filter((c) => c.name.toLowerCase().includes(term) || c.key.includes(term))
      .map((c) => ({ id: `chain:${c.key}`, label: c.name, meta: "Chain", href: `/workspace/explore?chain=${encodeURIComponent(c.key)}` }));
    const assetHits = (catalogue.data?.assets ?? [])
      .filter((a) => a.symbol.toLowerCase().includes(term) || a.name.toLowerCase().includes(term) || (a.contractAddress ?? "").toLowerCase().includes(term))
      .slice(0, 8)
      .map((a) => ({ id: `asset:${a.id}`, label: `${a.symbol} · ${a.name}`, meta: a.chain, href: `/workspace/explore?q=${encodeURIComponent(a.contractAddress || a.symbol)}&chain=${encodeURIComponent(a.chain)}` }));
    return [...chainHits, ...assetHits];
  }, [term, chains.data, catalogue.data]);

  const go = (href: string) => {
    setOpen(false);
    setQ("");
    router.push(href);
  };

  return (
    <div className="ws-search" role="search">
      <label className="ws-sr" htmlFor={id}>Search</label>
      <input
        id={id}
        type="search"
        role="combobox"
        aria-expanded={open && term !== ""}
        aria-controls={`${id}-list`}
        aria-autocomplete="list"
        placeholder="Search assets, chains or contracts"
        value={q}
        onChange={(e) => {
          setQ(e.target.value);
          setOpen(true);
          setCursor(0);
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setTimeout(() => setOpen(false), 150)}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown") setCursor((c) => Math.min(c + 1, results.length - 1));
          else if (e.key === "ArrowUp") setCursor((c) => Math.max(c - 1, 0));
          else if (e.key === "Enter" && results[cursor]) go(results[cursor].href);
          else if (e.key === "Escape") setOpen(false);
          else return;
          e.preventDefault();
        }}
      />
      {open && term && (
        <ul className="ws-search-results panel" id={`${id}-list`} role="listbox">
          {results.length === 0 ? (
            <li className="ws-search-empty" role="option" aria-selected={false}>No matches for &quot;{q.trim()}&quot;</li>
          ) : (
            results.map((r, i) => (
              <li key={r.id} role="option" aria-selected={i === cursor}>
                <button type="button" tabIndex={-1} onMouseDown={(e) => e.preventDefault()} onClick={() => go(r.href)}>
                  <span>{r.label}</span>
                  <span className="subtle-meta">{r.meta}</span>
                </button>
              </li>
            ))
          )}
        </ul>
      )}
    </div>
  );
}

function WalletButton() {
  const { wallet, chains, toast } = useWorkspace();
  const [menu, setMenu] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!menu) return;
    const close = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setMenu(false);
    };
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setMenu(false);
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", esc);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", esc);
    };
  }, [menu]);

  if (!wallet.connected || !wallet.state.account) {
    return (
      <button type="button" className="button" onClick={wallet.openPicker}>
        Connect wallet
      </button>
    );
  }
  const account = wallet.state.account;
  const chain = chains.data?.find((c) => c.chainId === wallet.state.chainId) ?? null;
  const explorer = explorerAddressUrl(chain, account);
  return (
    <div className="ws-wallet" ref={ref}>
      <button type="button" className="button button-secondary" aria-haspopup="menu" aria-expanded={menu} onClick={() => setMenu((v) => !v)}>
        {shortenEvmAddress(account)}
        {wallet.session ? <span className="subtle-meta"> · signed in</span> : null}
      </button>
      {menu && (
        <div className="ws-menu panel" role="menu">
          <button
            type="button"
            role="menuitem"
            onClick={() => {
              void navigator.clipboard?.writeText(account).then(() => toast("Copied"), () => undefined);
              setMenu(false);
            }}
          >
            Copy address
          </button>
          {wallet.session ? (
            <button
              type="button"
              role="menuitem"
              onClick={() => {
                setMenu(false);
                void wallet.signOut();
              }}
            >
              Sign out
            </button>
          ) : (
            <button
              type="button"
              role="menuitem"
              disabled={wallet.signingIn}
              onClick={() => {
                setMenu(false);
                void wallet.signIn();
              }}
            >
              {wallet.signingIn ? "Confirm in your wallet..." : "Sign in with wallet"}
            </button>
          )}
          {explorer ? (
            <a role="menuitem" href={explorer} target="_blank" rel="noreferrer" onClick={() => setMenu(false)}>View on explorer</a>
          ) : (
            <button type="button" role="menuitem" disabled>View on explorer</button>
          )}
          <button
            type="button"
            role="menuitem"
            onClick={() => {
              setMenu(false);
              wallet.disconnect();
            }}
          >
            Disconnect
          </button>
        </div>
      )}
    </div>
  );
}

function WalletPicker() {
  const { wallet } = useWorkspace();
  return (
    <Modal open={wallet.pickerOpen} title="Choose a wallet" onClose={wallet.closePicker}>
      {wallet.connecting ? (
        <p className="ws-modal-note" role="status">Confirm in your wallet...</p>
      ) : wallet.options.length === 0 ? (
        <p className="ws-modal-note">No browser wallet detected. Install an EVM wallet extension, then reload this page.</p>
      ) : (
        <ul className="ws-wallet-list">
          {wallet.options.map((o) => (
            <li key={o.id}>
              <button type="button" className="button button-secondary" onClick={() => void wallet.connect(o)}>
                {o.name}
              </button>
            </li>
          ))}
        </ul>
      )}
    </Modal>
  );
}

function NetworkBanner() {
  const { wallet, chains } = useWorkspace();
  if (wallet.state.status !== "unsupported-network" || !wallet.state.account) return null;
  const target = chains.data?.find((c) => c.walletSupported && c.chainId) ?? null;
  if (!target?.chainId) return null;
  return (
    <div className="fixture-banner ws-banner" role="alert">
      <p>Switch to {target.name} to continue.</p>
      <button type="button" className="button button-secondary button-small" onClick={() => void wallet.switchNetwork(target.chainId!)}>
        Switch network
      </button>
    </div>
  );
}

function CapabilityBanner() {
  const { capabilities } = useWorkspace();
  if (!capabilities) return null;
  if (capabilities.demoMode) {
    return (
      <div className="fixture-banner ws-banner" role="status">
        <p>Demo mode: swaps are simulations.</p>
      </div>
    );
  }
  if (capabilities.quoteMode === "live" && !capabilities.executionEnabled) {
    return (
      <div className="fixture-banner ws-banner" role="status">
        <p>Live execution is off.</p>
      </div>
    );
  }
  return null;
}

function Frame({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const { online, toasts } = useWorkspace();
  const [menuOpen, setMenuOpen] = useState(false);
  const active = activeKey(pathname);
  const current = NAV.find((n) => n.key === active);

  useEffect(() => setMenuOpen(false), [pathname]);

  return (
    <div className="workspace-page">
      <a className="ws-skip" href="#ws-main">Skip to content</a>
      <button className="mobile-scrim" type="button" hidden={!menuOpen} aria-label="Close navigation" onClick={() => setMenuOpen(false)} />
      <aside className={`sidebar${menuOpen ? " is-open" : ""}`} aria-label="Workspace navigation">
        <Link className="brand" href="/">
          <BrandMark />
          <span className="brand-name">Zonyx</span>
        </Link>
        <nav className="nav-groups" aria-label="Sections">
          <div>
            {NAV.map((item) => (
              <Link
                key={item.key}
                className="nav-link"
                href={`/workspace/${item.key}`}
                aria-current={active === item.key ? "page" : undefined}
              >
                <span className="nav-icon" aria-hidden="true">{item.glyph}</span>
                <span>{item.label}</span>
              </Link>
            ))}
          </div>
        </nav>
        {USE_MOCKS && (
          <div className="sidebar-bottom">
            <p className="subtle-meta">TEST DATA mode is on. Mock values are labelled on every page.</p>
          </div>
        )}
      </aside>

      <div className="mobile-bar">
        <button className="menu-button" type="button" aria-label="Open navigation" aria-expanded={menuOpen} onClick={() => setMenuOpen(true)}>
          <span /><span /><span />
        </button>
        <span className="mobile-title">{current?.label ?? "Zonyx"}</span>
        <Link className="mobile-mark" href="/" aria-label="Zonyx home"><BrandMark /></Link>
      </div>

      <div className="main-shell">
        <div className="ws-topbar">
          <GlobalSearch />
          <WalletButton />
        </div>
        {!online && (
          <div className="fixture-banner ws-banner" role="status">
            <p>{STATE_COPY.offline}</p>
          </div>
        )}
        <NetworkBanner />
        <CapabilityBanner />
        <main className="app-content" id="ws-main" tabIndex={-1}>
          {children}
        </main>
        <footer className="app-footer">
          <span>Analysis, not financial advice. Scores are ranking scores, not probabilities.</span>
        </footer>
      </div>

      <WalletPicker />
      <div className="toast-region" aria-live="polite" role="status">
        {toasts.map((t) => (
          <div key={t.id} className="toast">{t.message}</div>
        ))}
      </div>
    </div>
  );
}

export function WorkspaceShell({ children }: { children: ReactNode }) {
  return (
    <WorkspaceProvider>
      <Frame>{children}</Frame>
    </WorkspaceProvider>
  );
}
