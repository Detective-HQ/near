"use client";

import { useEffect, useId, useRef, useState } from "react";
import type { KeyboardEvent, ReactNode } from "react";
import { asOf } from "../../lib/format";
import { qualityTone, scoreBand } from "../../lib/quality";
import type { Quality } from "../../lib/quality";
import { viewMessage } from "../../lib/view-state";
import type { ViewKey, ViewState } from "../../lib/view-state";
import { useWorkspace } from "./workspace-context";

export function PageHeader({ title, sub, actions, testData }: { title: string; sub: string; actions?: ReactNode; testData?: boolean }) {
  return (
    <header className="page-header">
      <div>
        <h1>{title}</h1>
        <p>{sub}</p>
      </div>
      {(actions || testData) && (
        <div className="header-actions">
          {testData && <TestDataTag />}
          {actions}
        </div>
      )}
    </header>
  );
}

export function TestDataTag() {
  return <span className="state-pill warn" title="Mock values for interface testing. Not real data.">TEST DATA</span>;
}

export function QualityBadge({ quality }: { quality: Quality }) {
  const tone = qualityTone(quality);
  return <span className={`state-pill${tone ? ` ${tone}` : ""}`}>{quality}</span>;
}

/** Source, "As of {time}" and the quality badge: shown with every metric and signal. */
export function Provenance({ source, time, quality }: { source: string | null | undefined; time: string | null | undefined; quality: Quality }) {
  return (
    <span className="ws-prov">
      <span>{source || "Not available"}</span>
      <span aria-hidden="true">·</span>
      <span>{asOf(time)}</span>
      <QualityBadge quality={quality} />
    </span>
  );
}

export function ScoreCell({ score }: { score: string | null }) {
  const band = scoreBand(score);
  if (!score || !band) return <span>Not available</span>;
  return (
    <span className="ws-score" aria-label={`Ranking score ${score}, ${band}`}>
      <strong>{score}</strong> <span className="subtle-meta">{band}</span>
    </span>
  );
}

export function Skeleton({ rows = 4 }: { rows?: number }) {
  return (
    <div className="ws-skeleton" aria-busy="true" aria-label="Loading">
      {Array.from({ length: rows }, (_, i) => (
        <span key={i} style={{ width: `${92 - (i % 3) * 14}%` }} />
      ))}
    </div>
  );
}

/** Renders the shared state for a view. Returns null when the view is ready. */
export function ViewStateBlock({
  view,
  state,
  staleTime,
  onAction,
  actionHref,
}: {
  view: ViewKey;
  state: ViewState;
  staleTime?: string;
  onAction?: () => void;
  actionHref?: string;
}) {
  if (state === "loading") return <Skeleton rows={5} />;
  const msg = viewMessage(view, state, staleTime);
  if (!msg) return null;
  const banner = state === "stale" || state === "partial";
  return (
    <div className={banner ? "fixture-banner ws-banner" : "empty-state ws-state"} role={state === "error" ? "alert" : "status"}>
      <p>{msg.message}</p>
      {msg.action && (actionHref ? (
        <a className="button button-secondary button-small" href={actionHref}>{msg.action}</a>
      ) : onAction ? (
        <button className="button button-secondary button-small" type="button" onClick={onAction}>{msg.action}</button>
      ) : null)}
    </div>
  );
}

export function CopyButton({ value, label }: { value: string; label: string }) {
  const { toast } = useWorkspace();
  return (
    <button
      type="button"
      className="button button-quiet button-small"
      aria-label={label}
      onClick={() => {
        void navigator.clipboard?.writeText(value).then(() => toast("Copied"), () => undefined);
      }}
    >
      Copy
    </button>
  );
}

function useDialog(open: boolean, onClose: () => void) {
  const ref = useRef<HTMLDivElement>(null);
  const restore = useRef<HTMLElement | null>(null);
  useEffect(() => {
    if (!open) return;
    restore.current = document.activeElement as HTMLElement | null;
    const node = ref.current;
    const first = node?.querySelector<HTMLElement>("input, button, [href], select, [tabindex]:not([tabindex='-1'])");
    (first ?? node)?.focus();
    const onKey = (e: globalThis.KeyboardEvent) => {
      if (e.key === "Escape") onClose();
      if (e.key !== "Tab" || !node) return;
      const items = Array.from(node.querySelectorAll<HTMLElement>("input, button:not([disabled]), [href], select, [tabindex]:not([tabindex='-1'])"));
      if (items.length === 0) return;
      const firstEl = items[0];
      const lastEl = items[items.length - 1];
      if (e.shiftKey && document.activeElement === firstEl) {
        e.preventDefault();
        lastEl.focus();
      } else if (!e.shiftKey && document.activeElement === lastEl) {
        e.preventDefault();
        firstEl.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      restore.current?.focus?.();
    };
  }, [open, onClose]);
  return ref;
}

export function Modal({ open, title, onClose, children }: { open: boolean; title: string; onClose: () => void; children: ReactNode }) {
  const ref = useDialog(open, onClose);
  const id = useId();
  if (!open) return null;
  return (
    <div className="ws-overlay" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className="ws-modal panel" role="dialog" aria-modal="true" aria-labelledby={id} ref={ref} tabIndex={-1}>
        <div className="ws-dialog-head">
          <h2 id={id}>{title}</h2>
          <button type="button" className="button button-quiet button-small" aria-label="Close" onClick={onClose}>✕</button>
        </div>
        {children}
      </div>
    </div>
  );
}

export function Drawer({ open, title, onClose, children }: { open: boolean; title: string; onClose: () => void; children: ReactNode }) {
  const ref = useDialog(open, onClose);
  const id = useId();
  if (!open) return null;
  return (
    <div className="ws-overlay ws-overlay-right" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <aside className="ws-drawer" role="dialog" aria-modal="true" aria-labelledby={id} ref={ref} tabIndex={-1}>
        <div className="ws-dialog-head">
          <h2 id={id}>{title}</h2>
          <button type="button" className="button button-quiet button-small" aria-label="Close" onClick={onClose}>✕</button>
        </div>
        {children}
      </aside>
    </div>
  );
}

export function Tabs<T extends string>({ tabs, active, onChange, label }: { tabs: readonly T[]; active: T; onChange: (t: T) => void; label: string }) {
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const i = tabs.indexOf(active);
    if (e.key === "ArrowRight") onChange(tabs[(i + 1) % tabs.length]);
    else if (e.key === "ArrowLeft") onChange(tabs[(i - 1 + tabs.length) % tabs.length]);
    else return;
    e.preventDefault();
    requestAnimationFrame(() => (document.querySelector(".ws-tab[aria-selected='true']") as HTMLElement | null)?.focus());
  };
  return (
    <div className="ws-tabs" role="tablist" aria-label={label} onKeyDown={onKey}>
      {tabs.map((t) => (
        <button
          key={t}
          type="button"
          role="tab"
          className="ws-tab"
          aria-selected={t === active}
          tabIndex={t === active ? 0 : -1}
          onClick={() => onChange(t)}
        >
          {t}
        </button>
      ))}
    </div>
  );
}

/** Small "i" button that reveals a definition on hover or focus. */
export function InfoTip({ label, text }: { label: string; text: string }) {
  const [open, setOpen] = useState(false);
  const id = useId();
  return (
    <span className="ws-tip" onMouseEnter={() => setOpen(true)} onMouseLeave={() => setOpen(false)}>
      <button
        type="button"
        className="ws-tip-btn"
        aria-label={label}
        aria-describedby={id}
        aria-expanded={open}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        onClick={() => setOpen((v) => !v)}
      >
        i
      </button>
      <span role="tooltip" id={id} className="ws-tip-body" hidden={!open}>{text}</span>
    </span>
  );
}

export function FilterSelect({ label, value, options, onChange }: { label: string; value: string; options: Array<{ value: string; label: string }>; onChange: (v: string) => void }) {
  const id = useId();
  return (
    <div className="field ws-filter">
      <label htmlFor={id}>{label}</label>
      <select id={id} value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">All</option>
        {options.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
      </select>
    </div>
  );
}

export function SearchField({ label, placeholder, value, onChange }: { label: string; placeholder: string; value: string; onChange: (v: string) => void }) {
  const id = useId();
  return (
    <div className="field ws-filter ws-filter-search">
      <label htmlFor={id}>{label}</label>
      <input id={id} type="search" placeholder={placeholder} value={value} onChange={(e) => onChange(e.target.value)} />
    </div>
  );
}

export function Pager({ page, pageSize, total, onPage }: { page: number; pageSize: number; total: number; onPage: (p: number) => void }) {
  const pages = Math.max(1, Math.ceil(total / pageSize));
  if (pages <= 1) return null;
  return (
    <nav className="ws-pager" aria-label="Pagination">
      <button type="button" className="button button-secondary button-small" disabled={page <= 1} onClick={() => onPage(page - 1)}>Previous</button>
      <span className="subtle-meta">Page {page} of {pages}</span>
      <button type="button" className="button button-secondary button-small" disabled={page >= pages} onClick={() => onPage(page + 1)}>Next</button>
    </nav>
  );
}

export function DefRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="quote-detail">
      <span>{label}</span>
      <strong>{children}</strong>
    </div>
  );
}
