const DEFAULT_API_BASE_URL = "https://api.llama.fi";
const DEFAULT_STABLECOIN_BASE_URL = "https://stablecoins.llama.fi";
const MAX_RESPONSE_BYTES = 4_000_000;
const METHODOLOGY_VERSION = "defillama-near-v1";

function finiteNumber(value) {
  if (value === null || value === undefined || value === "" || typeof value === "boolean") return null;
  const number = typeof value === "number" ? value : Number(value);
  return Number.isFinite(number) && number >= 0 ? number : null;
}

function timestampFromSeconds(value) {
  const seconds = finiteNumber(value);
  if (seconds === null || seconds === 0) return null;
  const timestamp = new Date(seconds * 1000);
  return Number.isNaN(timestamp.getTime()) ? null : timestamp.toISOString();
}

function makeMetric({
  key,
  label,
  value,
  unit,
  scope,
  definition,
  source,
  sourceUrl,
  observedAt = null,
  retrievedAt,
  error = null,
  stale = false
}) {
  return {
    key,
    label,
    value: finiteNumber(value),
    unit,
    scope,
    definition,
    source,
    sourceUrl,
    observedAt,
    retrievedAt,
    methodologyVersion: METHODOLOGY_VERSION,
    classification: finiteNumber(value) === null ? "unavailable" : stale ? "stale" : "observed",
    status: finiteNumber(value) === null ? "unavailable" : stale ? "stale" : "available",
    ...(error || finiteNumber(value) === null ? { error: error ?? "No usable observation was returned by this source." } : {})
  };
}

export function createDefiLlamaProvider({
  apiBaseUrl = DEFAULT_API_BASE_URL,
  stablecoinBaseUrl = DEFAULT_STABLECOIN_BASE_URL,
  ttlMs = 300_000,
  timeoutMs = 10_000,
  fetchImpl = fetch,
  clock = () => new Date()
} = {}) {
  let cached = null;
  let inFlight = null;

  async function getJson(url) {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), timeoutMs);
    try {
      const response = await fetchImpl(url, {
        method: "GET",
        headers: { accept: "application/json" },
        signal: controller.signal
      });
      let body;
      const reader = response.body?.getReader?.();
      if (reader) {
        const chunks = [];
        let size = 0;
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          size += value.byteLength;
          if (size > MAX_RESPONSE_BYTES) {
            await reader.cancel().catch(() => {});
            throw new Error("Provider response exceeded the configured size limit.");
          }
          chunks.push(Buffer.from(value));
        }
        body = Buffer.concat(chunks).toString("utf8");
      } else {
        body = await response.text();
        if (Buffer.byteLength(body, "utf8") > MAX_RESPONSE_BYTES) {
          throw new Error("Provider response exceeded the configured size limit.");
        }
      }
      if (!response.ok) throw new Error(`Provider returned HTTP ${response.status}.`);
      try {
        return JSON.parse(body);
      } catch {
        throw new Error("Provider returned invalid JSON.");
      }
    } finally {
      clearTimeout(timeout);
    }
  }

  function url(base, path, query = {}) {
    const result = new URL(path, base);
    for (const [key, value] of Object.entries(query)) {
      result.searchParams.set(key, String(value));
    }
    return result.toString();
  }

  async function collect(force) {
    if (!force && cached && cached.expiresAt > Date.now()) return structuredClone(cached.snapshot);
    if (inFlight) return structuredClone(await inFlight);

    inFlight = (async () => {
      const retrievedAt = clock().toISOString();
      const apiRoot = apiBaseUrl.replace(/\/+$/, "");
      const stablecoinRoot = stablecoinBaseUrl.replace(/\/+$/, "");

      const chainListUrl = url(apiRoot, "/v2/chains");
      let chainName = "Near";
      let currentChainTvl = null;
      let chainListError = null;
      try {
        const chainList = await getJson(chainListUrl);
        const nearChain = Array.isArray(chainList)
          ? chainList.find((entry) => typeof entry?.name === "string" && entry.name.toLowerCase() === "near")
          : null;
        if (nearChain) {
          chainName = nearChain.name;
          currentChainTvl = finiteNumber(nearChain.tvl);
        } else {
          chainListError = "NEAR is not present in the provider chain catalogue.";
        }
      } catch {
        chainListError = "The provider chain catalogue could not be loaded.";
      }

      const encodedChain = encodeURIComponent(chainName);
      const urls = {
        tvl: url(apiRoot, `/v2/historicalChainTvl/${encodedChain}`),
        dex: url(apiRoot, `/overview/dexs/${encodedChain}`, {
          excludeTotalDataChart: true,
          excludeTotalDataChartBreakdown: true
        }),
        fees: url(apiRoot, `/overview/fees/${encodedChain}`, {
          excludeTotalDataChart: true,
          excludeTotalDataChartBreakdown: true,
          dataType: "dailyFees"
        }),
        revenue: url(apiRoot, `/overview/fees/${encodedChain}`, {
          excludeTotalDataChart: true,
          excludeTotalDataChartBreakdown: true,
          dataType: "dailyRevenue"
        }),
        stablecoins: url(stablecoinRoot, "/stablecoinchains")
      };
      const [tvlResult, dexResult, feesResult, revenueResult, stablecoinResult] = await Promise.allSettled([
        getJson(urls.tvl),
        getJson(urls.dex),
        getJson(urls.fees),
        getJson(urls.revenue),
        getJson(urls.stablecoins)
      ]);

      const tvlRows = tvlResult.status === "fulfilled" && Array.isArray(tvlResult.value)
        ? tvlResult.value
          .filter((point) => finiteNumber(point?.date) !== null && finiteNumber(point?.tvl) !== null)
          .map((point) => ({
            date: timestampFromSeconds(point.date),
            tvl: finiteNumber(point.tvl)
          }))
          .filter((point) => point.date)
          .slice(-90)
        : [];
      const latestTvlDate = tvlRows.at(-1)?.date ?? null;
      const liveTvl = currentChainTvl ?? tvlRows.at(-1)?.tvl ?? null;

      const dexData = dexResult.status === "fulfilled" ? dexResult.value : null;
      const feesData = feesResult.status === "fulfilled" ? feesResult.value : null;
      const revenueData = revenueResult.status === "fulfilled" ? revenueResult.value : null;
      const stablecoinRows = stablecoinResult.status === "fulfilled" && Array.isArray(stablecoinResult.value)
        ? stablecoinResult.value
        : [];
      const stablecoinChain = stablecoinRows.find(
        (entry) => typeof entry?.name === "string" && entry.name.toLowerCase() === chainName.toLowerCase()
      );
      const stablecoinSupply = finiteNumber(
        stablecoinChain?.totalCirculatingUSD?.peggedUSD
        ?? stablecoinChain?.totalCirculating?.peggedUSD
      );

      const metrics = [
        makeMetric({
          key: "tvl",
          label: "Total value locked",
          value: liveTvl,
          unit: "USD",
          scope: "NEAR DeFi",
          definition: "Provider-reported chain TVL. Coverage and accounting follow the source methodology; this is not a measure of available trading liquidity.",
          source: "DeFiLlama",
          sourceUrl: chainListError && currentChainTvl === null ? urls.tvl : chainListUrl,
          observedAt: latestTvlDate,
          retrievedAt,
          error: liveTvl === null ? chainListError ?? "No usable NEAR TVL observation was returned." : null
        }),
        makeMetric({
          key: "dex_volume_24h",
          label: "DEX volume",
          value: dexData?.total24h,
          unit: "USD / rolling 24 hours",
          scope: "NEAR DeFi",
          definition: "Provider-reported aggregate DEX volume over the rolling 24-hour window. Coverage may vary; volume is not revenue.",
          source: "DeFiLlama",
          sourceUrl: urls.dex,
          retrievedAt,
          error: dexResult.status === "rejected" ? "The DEX overview source could not be loaded." : null
        }),
        makeMetric({
          key: "stablecoin_supply",
          label: "Stablecoin supply",
          value: stablecoinSupply,
          unit: "USD",
          scope: "NEAR ecosystem",
          definition: "Provider-reported circulating stablecoin value on NEAR. Supply does not equal liquidity available for trading.",
          source: "DeFiLlama Stablecoins",
          sourceUrl: urls.stablecoins,
          retrievedAt,
          error: stablecoinResult.status === "rejected"
            ? "The stablecoin source could not be loaded."
            : !stablecoinChain ? "NEAR was not present in the stablecoin chain response." : null
        }),
        makeMetric({
          key: "protocol_fees_24h",
          label: "Protocol fees",
          value: feesData?.total24h,
          unit: "USD / rolling 24 hours",
          scope: "NEAR DeFi",
          definition: "Fees attributed to tracked protocols over the rolling 24-hour window. Fees paid by users are not the same as revenue retained by protocols.",
          source: "DeFiLlama Fees",
          sourceUrl: urls.fees,
          retrievedAt,
          error: feesResult.status === "rejected" ? "The protocol-fees source could not be loaded." : null
        }),
        makeMetric({
          key: "protocol_revenue_24h",
          label: "Protocol revenue",
          value: revenueData?.total24h,
          unit: "USD / rolling 24 hours",
          scope: "NEAR DeFi",
          definition: "Revenue attributed to tracked protocols over the rolling 24-hour window. Source coverage and attribution methodology vary by protocol.",
          source: "DeFiLlama Fees",
          sourceUrl: urls.revenue,
          retrievedAt,
          error: revenueResult.status === "rejected" ? "The protocol-revenue source could not be loaded." : null
        })
      ];

      if (cached?.snapshot?.metrics) {
        const previousByKey = new Map(cached.snapshot.metrics.map((metric) => [metric.key, metric]));
        for (let index = 0; index < metrics.length; index += 1) {
          const current = metrics[index];
          const previous = previousByKey.get(current.key);
          if (current.status === "unavailable" && previous?.value !== null && previous?.value !== undefined) {
            metrics[index] = {
              ...previous,
              status: "stale",
              classification: "stale",
              retrievedAt,
              error: current.error ?? "Refresh failed; showing the last successful observation."
            };
          }
        }
      }

      const availableCount = metrics.filter((metric) => metric.value !== null).length;
      const staleCount = metrics.filter((metric) => metric.status === "stale").length;
      const status = availableCount === 0 ? "unavailable"
        : staleCount > 0 ? "stale"
          : availableCount === metrics.length ? "available" : "partial";
      const snapshot = {
        mode: "live",
        provider: "DeFiLlama",
        status,
        chain: chainName,
        retrievedAt,
        capturedAt: retrievedAt,
        notice: status === "available"
          ? "Live public data from DeFiLlama. Each metric retains its source, retrieval time, and methodology label."
          : status === "partial"
            ? "Some live sources did not return usable data. Unavailable values are shown as missing; no fixture values are substituted."
            : status === "stale"
              ? "A refresh failed for one or more sources. Values marked stale are the last successful observations."
              : "Live sources are currently unavailable. No fixture values are substituted.",
        metrics,
        series: tvlRows.map((point) => ({ date: point.date, tvl: point.tvl })),
        chainComparisons: [],
        methodologyVersion: METHODOLOGY_VERSION
      };
      cached = { snapshot: structuredClone(snapshot), expiresAt: Date.now() + ttlMs };
      return snapshot;
    })();

    try {
      return structuredClone(await inFlight);
    } finally {
      inFlight = null;
    }
  }

  return {
    async getNearSnapshot({ force = false } = {}) {
      return collect(force);
    }
  };
}
