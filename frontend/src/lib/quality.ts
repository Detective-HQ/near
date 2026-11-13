// Data quality badges and ranking-score bands. No imports so it can be unit-tested directly.

export const NOT_AVAILABLE = "Not available";

export type Quality = "Live" | "Delayed" | "Stale" | "Partial" | "Unavailable";

/** Maps an API status string to a badge. Unknown or missing values are Unavailable, which is neutral. */
export function qualityFromStatus(status: string | null | undefined, hasValue = true): Quality {
  if (!hasValue) return "Unavailable";
  switch ((status ?? "").toLowerCase()) {
    case "live":
    case "available":
    case "ok":
      return "Live";
    case "delayed":
      return "Delayed";
    case "stale":
      return "Stale";
    case "partial":
      return "Partial";
    default:
      return "Unavailable";
  }
}

/** Pill tone for a badge. Unavailable is deliberately neutral: it is never a bearish signal. */
export function qualityTone(q: Quality): "" | "good" | "warn" {
  if (q === "Live") return "good";
  if (q === "Unavailable") return "";
  return "warn";
}

export type ScoreBand = "Strong" | "Setup" | "Moderate" | "Neutral" | "Weak";

/** Bands: 85+ Strong | 70-84 Setup | 55-69 Moderate | 40-54 Neutral | <40 Weak. */
export function scoreBand(score: string | null | undefined): ScoreBand | null {
  if (score === null || score === undefined || !/^\d+(\.\d+)?$/.test(score.trim())) return null;
  const whole = Number(score.trim().split(".")[0]);
  if (whole >= 85) return "Strong";
  if (whole >= 70) return "Setup";
  if (whole >= 55) return "Moderate";
  if (whole >= 40) return "Neutral";
  return "Weak";
}

export type Regime = "Bullish" | "Bearish" | "Mixed" | "Unavailable";

/** Only the API's own regime is shown; anything unrecognised is Unavailable. */
export function regimeLabel(value: string | null | undefined): Regime {
  switch ((value ?? "").toLowerCase()) {
    case "bullish":
      return "Bullish";
    case "bearish":
      return "Bearish";
    case "mixed":
      return "Mixed";
    default:
      return "Unavailable";
  }
}
