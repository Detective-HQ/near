import test from "node:test";
import assert from "node:assert/strict";
import {
  serializeSnapshotCsv,
  serializeSnapshotJson,
  snapshotFilename
} from "../src/lib/snapshot-export.ts";

const sampleSnapshot = () => ({
  mode: "live",
  provider: "DeFiLlama",
  chain: "NEAR Protocol",
  status: "partial",
  retrievedAt: "2026-11-10T12:30:00.000Z",
  methodologyVersion: "near-defillama-v1",
  notice: "Some live sources were unavailable.",
  metrics: [
    {
      key: "tvl",
      label: 'TVL, "tracked"',
      value: 0,
      unit: "USD",
      scope: "NEAR DeFi",
      definition: "Provider-reported TVL.",
      source: "=HYPERLINK(\"https://invalid.example\")",
      sourceUrl: "https://example.test/tvl",
      observedAt: "2026-11-10T00:00:00.000Z",
      retrievedAt: "2026-11-10T12:30:00.000Z",
      status: "available",
      classification: "observed",
      methodologyVersion: "near-defillama-v1",
      error: null
    },
    {
      key: "dex_volume_24h",
      label: "DEX volume",
      value: null,
      unit: "USD / rolling 24 hours",
      scope: "NEAR DeFi",
      definition: "Volume is not revenue.",
      source: "DeFiLlama",
      sourceUrl: "https://example.test/dex",
      observedAt: null,
      retrievedAt: "2026-11-10T12:30:00.000Z",
      status: "unavailable",
      classification: "missing",
      methodologyVersion: "near-defillama-v1",
      error: "No observation."
    }
  ],
  series: [
    { date: "2026-11-09T00:00:00.000Z", tvl: 123.45 },
    { date: "2026-11-10T00:00:00.000Z", tvl: 0 }
  ]
});

test("JSON snapshot export preserves raw values, source details, status, and series", () => {
  const snapshot = sampleSnapshot();
  const payload = JSON.parse(serializeSnapshotJson(snapshot, "2026-11-10T12:31:00.000Z"));

  assert.equal(payload.schemaVersion, "udi.near-snapshot/v1");
  assert.equal(payload.exportedAt, "2026-11-10T12:31:00.000Z");
  assert.deepEqual(payload.snapshot, snapshot);
  assert.equal(payload.snapshot.metrics[0].value, 0);
  assert.equal(payload.snapshot.metrics[1].value, null);
});

test("CSV snapshot export contains metric and history rows and neutralizes formula cells", () => {
  const csv = serializeSnapshotCsv(sampleSnapshot());

  assert.match(csv, /^record_type,key,label,date,value,unit,/);
  assert.match(csv, /"TVL, ""tracked"""/);
  assert.match(csv, /'=HYPERLINK\(""https:\/\/invalid\.example""\)/);
  assert.match(csv, /,0,USD,NEAR DeFi,/);
  assert.match(csv, /,series\.tvl,.*2026-11-09T00:00:00\.000Z,123\.45,USD,/);
  assert.match(csv, /,series\.tvl,.*2026-11-10T00:00:00\.000Z,0,USD,/);
  assert.match(csv, /,live,DeFiLlama,NEAR Protocol,partial,/);
  assert.equal(csv.endsWith("\r\n"), true);
});

test("snapshot filenames include a safe chain slug and a deterministic UTC timestamp", () => {
  assert.equal(
    snapshotFilename(sampleSnapshot(), "json"),
    "udi-near-protocol-snapshot-2026-11-10T12-30-00-000Z.json"
  );
  assert.throws(() => snapshotFilename(sampleSnapshot(), "xlsx"), /must be csv or json/i);
});

test("snapshot serializers reject invalid payload shapes", () => {
  assert.throws(() => serializeSnapshotJson({ metrics: [] }), /snapshot with metrics and series is required/i);
  assert.throws(() => serializeSnapshotCsv(null), /snapshot with metrics and series is required/i);
});
