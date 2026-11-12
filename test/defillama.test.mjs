import test from "node:test";
import assert from "node:assert/strict";
import { createDefiLlamaProvider } from "../lib/providers/defillama.mjs";

function createFixtureFetcher({ fail = () => false, values = {} } = {}) {
  const requestedUrls = [];
  const tvlDate = Date.UTC(2026, 10, 9) / 1000;
  const defaults = {
    chains: [{ name: "Near", tvl: 170_000_000 }],
    tvl: [
      { date: tvlDate - 86_400, tvl: 165_000_000 },
      { date: tvlDate, tvl: 170_000_000 }
    ],
    dex: { chain: "Near", total24h: 2_500_000 },
    fees: { chain: "Near", total24h: 12_000 },
    revenue: { chain: "Near", total24h: 4_000 },
    stablecoins: [{
      name: "Near",
      totalCirculatingUSD: { peggedUSD: 85_000_000 }
    }]
  };

  const fetchImpl = async (input) => {
    const requestUrl = new URL(input);
    requestedUrls.push(requestUrl);
    if (fail(requestUrl)) return new Response("upstream unavailable", { status: 503 });

    let body;
    if (requestUrl.pathname === "/v2/chains") body = values.chains ?? defaults.chains;
    else if (requestUrl.pathname.startsWith("/v2/historicalChainTvl/")) body = values.tvl ?? defaults.tvl;
    else if (requestUrl.pathname.startsWith("/overview/dexs/")) body = values.dex ?? defaults.dex;
    else if (requestUrl.pathname.startsWith("/overview/fees/")) {
      body = requestUrl.searchParams.get("dataType") === "dailyRevenue"
        ? values.revenue ?? defaults.revenue
        : values.fees ?? defaults.fees;
    } else if (requestUrl.pathname === "/stablecoinchains") body = values.stablecoins ?? defaults.stablecoins;
    else return new Response("not found", { status: 404 });
    return new Response(JSON.stringify(body), { status: 200 });
  };

  return { fetchImpl, requestedUrls };
}

test("NEAR analytics maps public sources into timestamped, attributed observations", async () => {
  const fixture = createFixtureFetcher();
  const provider = createDefiLlamaProvider({
    apiBaseUrl: "https://api.example.test",
    stablecoinBaseUrl: "https://stablecoins.example.test",
    fetchImpl: fixture.fetchImpl,
    clock: () => new Date("2026-11-10T12:00:00.000Z")
  });

  const snapshot = await provider.getNearSnapshot();
  const metrics = Object.fromEntries(snapshot.metrics.map((metric) => [metric.key, metric]));

  assert.equal(snapshot.mode, "live");
  assert.equal(snapshot.status, "available");
  assert.equal(snapshot.chain, "Near");
  assert.equal(snapshot.retrievedAt, "2026-11-10T12:00:00.000Z");
  assert.equal(metrics.tvl.value, 170_000_000);
  assert.equal(metrics.dex_volume_24h.value, 2_500_000);
  assert.equal(metrics.stablecoin_supply.value, 85_000_000);
  assert.equal(metrics.protocol_fees_24h.value, 12_000);
  assert.equal(metrics.protocol_revenue_24h.value, 4_000);
  assert.equal(metrics.protocol_fees_24h.definition.includes("not the same as revenue"), true);
  assert.equal(metrics.protocol_revenue_24h.definition.includes("attribution methodology"), true);
  assert.equal(metrics.dex_volume_24h.sourceUrl.includes("overview/dexs/Near"), true);
  assert.equal(metrics.tvl.methodologyVersion, "defillama-near-v1");
  assert.equal(snapshot.series.length, 2);
  assert.equal(snapshot.series.at(-1).tvl, 170_000_000);
  assert.equal(fixture.requestedUrls.length, 6);

  snapshot.metrics[0].value = 1;
  const cached = await provider.getNearSnapshot();
  assert.equal(cached.metrics[0].value, 170_000_000);
  assert.equal(fixture.requestedUrls.length, 6);
});

test("missing upstream values remain unavailable instead of becoming zero", async () => {
  const fixture = createFixtureFetcher({ values: { revenue: { chain: "Near", total24h: null } } });
  const provider = createDefiLlamaProvider({
    apiBaseUrl: "https://api.example.test",
    stablecoinBaseUrl: "https://stablecoins.example.test",
    fetchImpl: fixture.fetchImpl
  });

  const snapshot = await provider.getNearSnapshot();
  const revenue = snapshot.metrics.find((metric) => metric.key === "protocol_revenue_24h");
  assert.equal(snapshot.status, "partial");
  assert.equal(revenue.value, null);
  assert.equal(revenue.status, "unavailable");
  assert.match(revenue.error, /no usable observation/i);
});

test("failed refresh preserves last good values but marks them stale", async () => {
  let failRequests = false;
  const fixture = createFixtureFetcher({ fail: () => failRequests });
  const provider = createDefiLlamaProvider({
    apiBaseUrl: "https://api.example.test",
    stablecoinBaseUrl: "https://stablecoins.example.test",
    fetchImpl: fixture.fetchImpl
  });

  const first = await provider.getNearSnapshot();
  assert.equal(first.status, "available");
  failRequests = true;
  const refreshed = await provider.getNearSnapshot({ force: true });

  assert.equal(refreshed.status, "stale");
  assert.equal(refreshed.metrics.every((metric) => metric.status === "stale"), true);
  assert.equal(refreshed.metrics[0].value, 170_000_000);
  assert.equal(refreshed.notice.includes("last successful observations"), true);
});

test("live source errors return unavailable values without substituting fixture observations", async () => {
  const fixture = createFixtureFetcher({ fail: () => true });
  const provider = createDefiLlamaProvider({
    apiBaseUrl: "https://api.example.test",
    stablecoinBaseUrl: "https://stablecoins.example.test",
    fetchImpl: fixture.fetchImpl
  });

  const snapshot = await provider.getNearSnapshot();
  assert.equal(snapshot.status, "unavailable");
  assert.equal(snapshot.metrics.every((metric) => metric.value === null), true);
  assert.equal(snapshot.series.length, 0);
  assert.match(snapshot.notice, /no fixture values are substituted/i);
});
