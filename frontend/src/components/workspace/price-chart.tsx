"use client";

import { useMemo, useRef, useState } from "react";
import type { MouseEvent } from "react";
import { formatDecimal, numberToDecimalString } from "../../lib/decimal";
import type { StructureResponse, ZoneStatus } from "../../lib/trading-contracts";

export const OVERLAYS = ["Order Blocks", "Fair Value Gaps", "BOS / CHoCH", "Swings", "Liquidity levels"] as const;
export type Overlay = (typeof OVERLAYS)[number];

export const ZONE_LABEL: Record<ZoneStatus, string> = {
  fresh: "Fresh",
  tested: "Tested",
  mitigated: "Mitigated",
  invalidated: "Invalidated",
  expired: "Expired",
};

const MAX_CANDLES = 120;
// Layout (SVG units). Price axis on the right, time axis at the bottom, like a trading terminal.
const W = 1000;
const H = 440;
const AXIS_R = 78;
const AXIS_B = 26;
const TOP = 12;
const PLOT_W = W - AXIS_R;
const PLOT_H = H - AXIS_B - TOP;

/** Fraction digits that suit the price magnitude. */
function digitsFor(p: number): number {
  const a = Math.abs(p);
  if (a >= 100) return 2;
  if (a >= 1) return 4;
  if (a >= 0.01) return 6;
  return 8;
}

/** Display a price string exactly; axis ticks (chart geometry) go through the same formatter. */
function fmt(value: string | number, digits: number): string {
  const s = typeof value === "number" ? numberToDecimalString(value) : value;
  return s ? formatDecimal(s, digits, digits) : "—";
}

function niceTicks(min: number, max: number, count = 6): number[] {
  const span = max - min || Math.abs(max) || 1;
  const raw = span / count;
  const pow = 10 ** Math.floor(Math.log10(raw));
  const step = [1, 2, 2.5, 5, 10].map((m) => m * pow).find((s) => s >= raw) ?? raw;
  const out: number[] = [];
  for (let v = Math.ceil(min / step) * step; v <= max; v += step) out.push(Number(v.toPrecision(12)));
  return out;
}

function timeLabel(iso: string, timeframe: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const daily = /d|w/i.test(timeframe);
  return new Intl.DateTimeFormat(undefined, daily ? { month: "short", day: "numeric" } : { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(d);
}

export function PriceChart({
  data,
  levels,
  on,
}: {
  data: StructureResponse;
  levels: Array<{ price: string; side: string; label: string }>;
  on: Set<Overlay>;
}) {
  const candles = useMemo(() => data.candles.slice(-MAX_CANDLES), [data.candles]);
  const firstT = candles[0]?.t ?? "";
  const svgRef = useRef<SVGSVGElement>(null);
  const [hover, setHover] = useState<number | null>(null);

  // Chart geometry only: Number() is used for pixel positions, never for displayed values.
  const n = Number;
  const visibleZones = data.zones.filter((z) => (z.to ?? "9999") >= firstT);
  const lows = candles.map((c) => n(c.l));
  const highs = candles.map((c) => n(c.h));
  let min = Math.min(...lows);
  let max = Math.max(...highs);
  const padP = (max - min) * 0.08 || max * 0.01 || 1;
  min -= padP;
  max += padP;
  const y = (p: number) => TOP + ((max - p) / (max - min)) * PLOT_H;
  const inRange = (p: number) => p >= min && p <= max;
  const step = PLOT_W / Math.max(candles.length, 1);
  const body = Math.max(1, step * 0.66);
  const xOf = (i: number) => i * step + step / 2;
  const indexAt = (t: string) => {
    const i = candles.findIndex((c) => c.t >= t);
    return i < 0 ? candles.length - 1 : i;
  };
  const digits = digitsFor(max);
  const ticks = niceTicks(min, max);
  const timeIdx = candles.length > 1 ? Array.from({ length: 5 }, (_, k) => Math.round(((k + 0.5) * (candles.length - 1)) / 5)) : [0];
  // Keep overlays readable: only recent structure and swings, one label per liquidity side.
  const structure = data.structure.filter((s) => s.at >= firstT && inRange(n(s.price))).slice(-4);
  const swings = data.swings.filter((s) => s.at >= firstT && inRange(n(s.price))).slice(-10);
  const seenLevel = new Set<string>();
  const labelYs: number[] = [];
  const freeLabel = (py: number) => {
    if (labelYs.some((v) => Math.abs(v - py) < 14)) return false;
    labelYs.push(py);
    return true;
  };
  const last = candles[candles.length - 1];
  const lastUp = last ? n(last.c) >= n(last.o) : true;
  const shown = hover !== null ? candles[hover] : last;
  const shownUp = shown ? n(shown.c) >= n(shown.o) : true;
  const tip = `Detected zone under our published rules (${data.rulesVersion}).`;

  const onMove = (e: MouseEvent<SVGSVGElement>) => {
    const svg = svgRef.current;
    if (!svg) return;
    const rect = svg.getBoundingClientRect();
    const x = ((e.clientX - rect.left) / rect.width) * W;
    if (x > PLOT_W) return setHover(null);
    setHover(Math.min(candles.length - 1, Math.max(0, Math.floor(x / step))));
  };

  if (!last) return <p className="subtle-meta">Not available</p>;

  return (
    <div className="ws-pc">
      {/* OHLC readout for the hovered (or latest) candle */}
      <div className="ws-pc-legend" aria-live="off">
        <span className="ws-pc-tf">{data.timeframe}</span>
        {(["o", "h", "l", "c"] as const).map((k) => (
          <span key={k}>
            {k.toUpperCase()} <b className={shownUp ? "up" : "down"}>{fmt(shown[k], digits)}</b>
          </span>
        ))}
        <span className="subtle-meta">{timeLabel(shown.t, "1h")}</span>
      </div>

      <svg
        ref={svgRef}
        className="chart-svg ws-chart"
        viewBox={`0 0 ${W} ${H}`}
        role="img"
        aria-label={`Price chart, ${data.timeframe}, last close ${fmt(last.c, digits)}`}
        onMouseMove={onMove}
        onMouseLeave={() => setHover(null)}
      >
        {/* grid + price axis */}
        {ticks.map((t) => (
          <g key={t} className="ws-pc-grid">
            <line x1={0} x2={PLOT_W} y1={y(t)} y2={y(t)} />
            <text className="ws-pc-axis" x={PLOT_W + 8} y={y(t) + 4}>{fmt(t, digits)}</text>
          </g>
        ))}
        {/* time axis */}
        {timeIdx.map((i) => (
          <g key={i} className="ws-pc-grid">
            <line x1={xOf(i)} x2={xOf(i)} y1={TOP} y2={TOP + PLOT_H} />
            <text className="ws-pc-axis" x={xOf(i)} y={H - 8} textAnchor="middle">{timeLabel(candles[i].t, data.timeframe)}</text>
          </g>
        ))}
        <line className="ws-pc-frame" x1={PLOT_W} x2={PLOT_W} y1={TOP} y2={TOP + PLOT_H} />
        <line className="ws-pc-frame" x1={0} x2={PLOT_W} y1={TOP + PLOT_H} y2={TOP + PLOT_H} />

        {/* zones (order blocks, fair value gaps) */}
        {visibleZones
          .filter((z) => (z.kind === "order_block" ? on.has("Order Blocks") : on.has("Fair Value Gaps")))
          .map((z) => {
            const top = Math.max(TOP, y(Math.min(n(z.top), max)));
            const bottom = Math.min(TOP + PLOT_H, y(Math.max(n(z.bottom), min)));
            if (bottom <= top) return null;
            const x1 = z.from < firstT ? 0 : xOf(indexAt(z.from)) - step / 2;
            const x2 = z.to ? xOf(indexAt(z.to)) + step / 2 : PLOT_W;
            const label = `${z.direction === "bullish" ? "Bull" : "Bear"} ${z.kind === "order_block" ? "OB" : "FVG"} · ${ZONE_LABEL[z.status]}`;
            const showLabel = (z.status === "fresh" || z.status === "tested") && labelYs.length < 3 && freeLabel(top);
            const nearEdge = x1 > PLOT_W - 170;
            return (
              <g key={z.id} className={`ws-zone ws-zone-${z.direction} ws-zone-${z.status}`} tabIndex={0} aria-label={`${label}. ${tip}`}>
                <title>{`${label}. ${tip}`}</title>
                <rect x={x1} y={top} width={Math.max(4, x2 - x1)} height={Math.max(2, bottom - top)} />
                {showLabel && (
                  <text x={nearEdge ? PLOT_W - 6 : x1 + 6} y={top + 12} textAnchor={nearEdge ? "end" : "start"}>{label}</text>
                )}
              </g>
            );
          })}

        {/* liquidity levels */}
        {on.has("Liquidity levels") && levels.filter((l) => inRange(n(l.price))).map((l) => {
          const first = !seenLevel.has(l.label);
          seenLevel.add(l.label);
          return (
            <g key={`${l.side}-${l.price}`} className="ws-level">
              <title>{`${l.label} at ${l.price}`}</title>
              <line x1={0} x2={PLOT_W} y1={y(n(l.price))} y2={y(n(l.price))} />
              {first && freeLabel(y(n(l.price)) - 4) && <text x={6} y={y(n(l.price)) - 4}>{l.label}</text>}
            </g>
          );
        })}

        {/* candles: green when close ≥ open, red when close < open */}
        {candles.map((c, i) => {
          const up = n(c.c) >= n(c.o);
          const x = xOf(i);
          const yo = y(n(c.o));
          const yc = y(n(c.c));
          return (
            <g key={c.t} className={up ? "ws-candle-up" : "ws-candle-down"}>
              <line x1={x} x2={x} y1={y(n(c.h))} y2={y(n(c.l))} />
              <rect x={x - body / 2} y={Math.min(yo, yc)} width={body} height={Math.max(1, Math.abs(yo - yc))} />
            </g>
          );
        })}

        {/* break of structure / change of character */}
        {on.has("BOS / CHoCH") && structure.map((s) => {
          const i = indexAt(s.at);
          const x1 = Math.max(0, xOf(i) - step * 10);
          return (
            <g key={`${s.kind}-${s.at}`} className={`ws-structure ws-structure-${s.direction}`}>
              <title>{`${s.kind} (${s.direction}) at ${s.price}`}</title>
              <line x1={x1} x2={xOf(i)} y1={y(n(s.price))} y2={y(n(s.price))} />
              <text x={(x1 + xOf(i)) / 2} y={y(n(s.price)) - 4} textAnchor="middle">{s.kind}</text>
            </g>
          );
        })}

        {/* swing highs / lows */}
        {on.has("Swings") && swings.map((s) => {
          const x = xOf(indexAt(s.at));
          const py = y(n(s.price));
          const high = s.kind === "high";
          const d = high ? `M${x - 4} ${py - 10} L${x + 4} ${py - 10} L${x} ${py - 4} Z` : `M${x - 4} ${py + 10} L${x + 4} ${py + 10} L${x} ${py + 4} Z`;
          return (
            <path key={`${s.kind}-${s.at}`} className="ws-swing" d={d}>
              <title>{`Swing ${s.kind} at ${s.price}`}</title>
            </path>
          );
        })}

        {/* last price line + tag */}
        <g className={`ws-pc-last ${lastUp ? "up" : "down"}`}>
          <line x1={0} x2={PLOT_W} y1={y(n(last.c))} y2={y(n(last.c))} />
          <rect x={PLOT_W + 2} y={y(n(last.c)) - 10} width={AXIS_R - 4} height={20} rx={3} />
          <text x={PLOT_W + 8} y={y(n(last.c)) + 4}>{fmt(last.c, digits)}</text>
        </g>

        {/* crosshair */}
        {hover !== null && (
          <g className="ws-pc-cross" pointerEvents="none">
            <line x1={xOf(hover)} x2={xOf(hover)} y1={TOP} y2={TOP + PLOT_H} />
            <line x1={0} x2={PLOT_W} y1={y(n(candles[hover].c))} y2={y(n(candles[hover].c))} />
          </g>
        )}
      </svg>
    </div>
  );
}
