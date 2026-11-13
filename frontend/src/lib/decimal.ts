// Exact decimal arithmetic on strings, backed by BigInt. Prices, amounts and ratios
// never pass through JavaScript floats. No imports so it can be unit-tested directly.

export interface Dec {
  int: bigint; // value * 10^scale
  scale: number;
}

const DECIMAL = /^([+-])?(\d+)(?:\.(\d+))?$/;
const EXPONENT = /^([+-])?(\d+)(?:\.(\d+))?[eE]([+-]?\d+)$/;

/** Parses "12", "-0.5", "1.2e-7". Returns null for anything else. */
export function parseDecimal(input: string | number | null | undefined): Dec | null {
  if (input === null || input === undefined) return null;
  let s = typeof input === "number" ? numberToDecimalString(input) : input.trim();
  if (s === null || s === "") return null;
  const exp = EXPONENT.exec(s);
  if (exp) s = expandExponent(exp);
  const m = DECIMAL.exec(s);
  if (!m) return null;
  const frac = m[3] ?? "";
  const int = BigInt(m[2] + frac) * (m[1] === "-" ? -1n : 1n);
  return { int, scale: frac.length };
}

/** Converts a JSON number at the API boundary into a decimal string without float maths. */
export function numberToDecimalString(value: number): string {
  if (!Number.isFinite(value)) return "";
  const s = String(value);
  const exp = EXPONENT.exec(s);
  return exp ? expandExponent(exp) : s;
}

function expandExponent(m: RegExpExecArray): string {
  const sign = m[1] === "-" ? "-" : "";
  const digits = m[2] + (m[3] ?? "");
  const point = m[2].length + Number(m[4]);
  if (point <= 0) return `${sign}0.${"0".repeat(-point)}${digits}`;
  if (point >= digits.length) return `${sign}${digits}${"0".repeat(point - digits.length)}`;
  return `${sign}${digits.slice(0, point)}.${digits.slice(point)}`;
}

function align(a: Dec, b: Dec): [bigint, bigint, number] {
  const scale = Math.max(a.scale, b.scale);
  return [a.int * 10n ** BigInt(scale - a.scale), b.int * 10n ** BigInt(scale - b.scale), scale];
}

export function toDecimalString(d: Dec): string {
  const neg = d.int < 0n;
  let digits = (neg ? -d.int : d.int).toString();
  if (d.scale === 0) return (neg ? "-" : "") + digits;
  digits = digits.padStart(d.scale + 1, "0");
  const whole = digits.slice(0, -d.scale);
  const frac = digits.slice(-d.scale).replace(/0+$/, "");
  const out = frac ? `${whole}.${frac}` : whole;
  return neg && out !== "0" ? `-${out}` : out;
}

function req(v: string | Dec): Dec {
  const d = typeof v === "string" ? parseDecimal(v) : v;
  if (!d) throw new TypeError(`Not a decimal: ${String(v)}`);
  return d;
}

export function add(a: string, b: string): string {
  const [x, y, scale] = align(req(a), req(b));
  return toDecimalString({ int: x + y, scale });
}
export function sub(a: string, b: string): string {
  const [x, y, scale] = align(req(a), req(b));
  return toDecimalString({ int: x - y, scale });
}
export function mul(a: string, b: string): string {
  const x = req(a);
  const y = req(b);
  return toDecimalString({ int: x.int * y.int, scale: x.scale + y.scale });
}
/** Division rounded half away from zero to `scale` fraction digits. Returns null when dividing by zero. */
export function div(a: string, b: string, scale = 18): string | null {
  const x = req(a);
  const y = req(b);
  if (y.int === 0n) return null;
  // a/b = (x.int / 10^xs) / (y.int / 10^ys)
  const num = x.int * 10n ** BigInt(scale + y.scale + 1);
  const den = y.int * 10n ** BigInt(x.scale);
  let q = num / den;
  q = q >= 0n ? (q + 5n) / 10n : (q - 5n) / 10n;
  return toDecimalString({ int: q, scale });
}
export function cmp(a: string, b: string): -1 | 0 | 1 {
  const [x, y] = align(req(a), req(b));
  return x === y ? 0 : x < y ? -1 : 1;
}
export function isZero(a: string): boolean {
  return req(a).int === 0n;
}

/** Rounds half away from zero to `digits` fraction digits. */
export function round(a: string, digits: number): string {
  const d = req(a);
  if (d.scale <= digits) return toDecimalString(d);
  const drop = 10n ** BigInt(d.scale - digits);
  const half = drop / 2n;
  const q = d.int >= 0n ? (d.int + half) / drop : (d.int - half) / drop;
  return toDecimalString({ int: q, scale: digits });
}

/** Atomic integer string -> human decimal string. */
export function fromAtomic(atomic: string, decimals: number): string | null {
  if (!/^-?\d+$/.test(atomic)) return null;
  return toDecimalString({ int: BigInt(atomic), scale: decimals });
}

/** Human decimal string -> atomic integer string. Null when it has more precision than the asset allows. */
export function toAtomic(value: string, decimals: number): string | null {
  const d = parseDecimal(value);
  if (!d || d.int < 0n || d.scale > decimals) return null;
  return (d.int * 10n ** BigInt(decimals - d.scale)).toString();
}

function group(whole: string): string {
  return whole.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
}

/** Display formatting with thousands separators. */
export function formatDecimal(value: string, maxFraction = 6, minFraction = 0): string {
  const r = round(value, maxFraction);
  const neg = r.startsWith("-");
  const [whole, frac = ""] = (neg ? r.slice(1) : r).split(".");
  const f = frac.padEnd(minFraction, "0");
  return `${neg ? "-" : ""}${group(whole)}${f ? `.${f}` : ""}`;
}

const UNITS: Array<[string, string]> = [["1000000000000", "T"], ["1000000000", "B"], ["1000000", "M"]];

/** "$125.00M", "$1,234.56". */
export function formatUsd(value: string, compact = true): string {
  const neg = value.trim().startsWith("-");
  const abs = neg ? value.trim().slice(1) : value.trim();
  if (compact) {
    for (const [size, unit] of UNITS) {
      if (cmp(abs, size) >= 0) return `${neg ? "-" : ""}$${formatDecimal(div(abs, size, 4) ?? "0", 2, 2)}${unit}`;
    }
  }
  const digits = cmp(abs, "1") >= 0 ? 2 : 6;
  return `${neg ? "-" : ""}$${formatDecimal(abs, digits, cmp(abs, "1") >= 0 ? 2 : 0)}`;
}

/** "12.5" -> "12.50%". Input is already a percentage. */
export function formatPercent(value: string, digits = 2, signed = false): string {
  const out = formatDecimal(value, digits, digits);
  return `${signed && !out.startsWith("-") && !isZero(value) ? "+" : ""}${out}%`;
}

/** Basis points (integer) -> percent decimal string. 50 -> "0.5". */
export function bpsToPercent(bps: number): string {
  return toDecimalString({ int: BigInt(Math.trunc(bps)), scale: 2 });
}

/** Percent string with at most two fraction digits -> integer basis points. "0.5" -> 50. */
export function percentToBps(value: string): number | null {
  const d = parseDecimal(value);
  if (!d || d.int < 0n || d.scale > 2) return null;
  return Number(d.int * 10n ** BigInt(2 - d.scale));
}

/** Share of part in whole as a percent string, or null if whole is zero. */
export function percentOf(part: string, whole: string, digits = 2): string | null {
  const ratio = div(mul(part, "100"), whole, digits + 2);
  return ratio === null ? null : round(ratio, digits);
}
