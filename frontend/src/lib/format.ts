export function formatMetric(value: number | null | undefined, unit = ""): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return "—";
  if (unit.toLowerCase().startsWith("usd")) {
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency: "USD",
      maximumFractionDigits: value >= 1000 ? 0 : 2
    }).format(value);
  }
  return new Intl.NumberFormat("en-US", {
    maximumFractionDigits: value >= 1000 ? 0 : 2
  }).format(value);
}

export function formatTimestamp(value: string | null | undefined): string {
  if (!value) return "Not provided";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "UTC"
  }).format(date) + " UTC";
}

export function displaySource(mode: string, provider?: string): string {
  if (mode === "live") return provider || "Live source";
  return "Demo fixture";
}

/** API timestamps are UTC; they are localised only here, at display time. */
export function formatTime(value: string | null | undefined): string {
  if (!value) return "Not available";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Not available";
  // dateStyle/timeStyle cannot be combined with timeZoneName, so list the fields.
  return new Intl.DateTimeFormat(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    timeZoneName: "short"
  }).format(date);
}

export function asOf(value: string | null | undefined): string {
  return `As of ${formatTime(value)}`;
}

export function shortAddress(value: string | null | undefined): string {
  if (!value) return "Not available";
  return value.length > 14 ? `${value.slice(0, 6)}…${value.slice(-4)}` : value;
}
