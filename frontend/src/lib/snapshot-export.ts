import type { DataSnapshot, MetricObservation, SeriesPoint } from "./contracts.ts";

const CSV_COLUMNS = [
  "record_type",
  "key",
  "label",
  "date",
  "value",
  "unit",
  "scope",
  "definition",
  "source",
  "source_url",
  "observed_at",
  "retrieved_at",
  "status",
  "classification",
  "error",
  "methodology_version",
  "snapshot_mode",
  "provider",
  "chain",
  "snapshot_status",
  "snapshot_retrieved_at",
  "snapshot_notice"
];

function assertSnapshot(snapshot: unknown): asserts snapshot is DataSnapshot {
  if (!snapshot || typeof snapshot !== "object"
    || !("metrics" in snapshot)
    || !Array.isArray(snapshot.metrics)
    || !("series" in snapshot)
    || !Array.isArray(snapshot.series)) {
    throw new TypeError("A market snapshot with metrics and series is required.");
  }
}

function textValue(value: unknown): string {
  if (value === null || value === undefined) return "";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function csvCell(value: unknown, numeric = false): string {
  let text = textValue(value);
  const numericValue = numeric && /^-?(?:\d+\.?\d*|\.\d+)(?:e[+-]?\d+)?$/i.test(text);
  if (!numericValue && /^[\s]*[=+\-@]/.test(text)) text = `'${text}`;
  if (/[",\r\n]/.test(text)) return `"${text.replaceAll('"', '""')}"`;
  return text;
}

function snapshotColumns(snapshot: DataSnapshot): unknown[] {
  return [
    snapshot.mode,
    snapshot.provider,
    snapshot.chain,
    snapshot.status,
    snapshot.retrievedAt ?? snapshot.capturedAt,
    snapshot.notice
  ];
}

function rowForMetric(metric: MetricObservation, snapshot: DataSnapshot): unknown[] {
  return [
    "metric",
    metric.key,
    metric.label,
    "",
    metric.value,
    metric.unit,
    metric.scope,
    metric.definition,
    metric.source,
    metric.sourceUrl,
    metric.observedAt,
    metric.retrievedAt,
    metric.status,
    metric.classification,
    metric.error,
    metric.methodologyVersion,
    ...snapshotColumns(snapshot)
  ];
}

function rowForSeries(
  point: SeriesPoint,
  key: string,
  metric: MetricObservation | undefined,
  snapshot: DataSnapshot
): unknown[] {
  return [
    "series",
    `series.${key}`,
    metric?.label ?? `${key} history`,
    point.date,
    point[key],
    metric?.unit,
    metric?.scope ?? snapshot.chain,
    metric?.definition ?? "Historical series observation.",
    metric?.source ?? snapshot.provider,
    metric?.sourceUrl,
    point.date,
    snapshot.retrievedAt ?? snapshot.capturedAt,
    metric?.status ?? snapshot.status,
    metric?.classification,
    metric?.error,
    metric?.methodologyVersion ?? snapshot.methodologyVersion,
    ...snapshotColumns(snapshot)
  ];
}

function csvRow(values: unknown[]): string {
  return values.map((value, index) => csvCell(value, index === 4)).join(",");
}

export function serializeSnapshotJson(
  snapshot: DataSnapshot,
  exportedAt = new Date().toISOString()
): string {
  assertSnapshot(snapshot);
  return `${JSON.stringify({
    schemaVersion: "udi.near-snapshot/v1",
    exportedAt,
    snapshot
  }, null, 2)}\n`;
}

export function serializeSnapshotCsv(snapshot: DataSnapshot): string {
  assertSnapshot(snapshot);
  const metrics = snapshot.metrics.map((metric) => rowForMetric(metric, snapshot));
  const numericSeriesKeys = new Set(
    snapshot.series.flatMap((point) => Object.keys(point).filter(
      (key) => key !== "date" && Number.isFinite(Number(point[key]))
    ))
  );
  const seriesMetrics = new Map(snapshot.metrics.map((metric) => [metric.key, metric]));
  const seriesRows = snapshot.series.flatMap((point) => [...numericSeriesKeys]
    .filter((key) => Number.isFinite(Number(point[key])))
    .map((key) => {
      const metric = seriesMetrics.get(key);
      return rowForSeries(point, key, metric, snapshot);
    }));
  return [csvRow(CSV_COLUMNS), ...metrics.map(csvRow), ...seriesRows.map(csvRow)].join("\r\n") + "\r\n";
}

export function snapshotFilename(
  snapshot: DataSnapshot,
  format: "csv" | "json",
  downloadedAt = new Date()
): string {
  if (!["csv", "json"].includes(format)) throw new TypeError("Snapshot format must be csv or json.");
  const chain = String(snapshot?.chain ?? "near")
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, "-")
    .replace(/^-+|-+$/g, "") || "near";
  const sourceDate = snapshot?.retrievedAt ?? snapshot?.capturedAt ?? downloadedAt;
  const date = new Date(sourceDate);
  const timestamp = (Number.isNaN(date.getTime()) ? downloadedAt : date)
    .toISOString()
    .replace(/[:.]/g, "-");
  return `udi-${chain}-snapshot-${timestamp}.${format}`;
}
