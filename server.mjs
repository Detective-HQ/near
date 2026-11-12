import { createServer as createHttpServer } from "node:http";
import { randomUUID } from "node:crypto";
import { extname, resolve, sep } from "node:path";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { assets, chains, fixtureDate, fixtureNotice, marketOverview, nearMetrics, researchInputs } from "./lib/data.mjs";
import { advanceSimulation, createQuote, createSwap, DomainError, parseUnits } from "./lib/domain.mjs";
import { createRepository } from "./lib/repository.mjs";
import { createNearOneClickProvider } from "./lib/providers/nearoneclick.mjs";
import { createDefiLlamaProvider } from "./lib/providers/defillama.mjs";

const projectDir = fileURLToPath(new URL(".", import.meta.url));
const publicDir = resolve(projectDir, "public");
const assetById = new Map(assets.map((asset) => [asset.id, asset]));
const providerChainKeys = {
  eth: "ethereum",
  arb: "arbitrum",
  pol: "polygon",
  avax: "avalanche",
  bsc: "bnb",
  sol: "solana",
  xlm: "stellar",
  near: "near",
  aptos: "aptos",
  celo: "celo",
  op: "optimism",
  gnosis: "gnosis"
};
const mimeTypes = {
  ".css": "text/css; charset=utf-8",
  ".html": "text/html; charset=utf-8",
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".js": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".svg": "image/svg+xml"
};

function json(response, status, value, requestId) {
  response.writeHead(status, {
    "content-type": "application/json; charset=utf-8",
    "cache-control": "no-store",
    "x-request-id": requestId
  });
  response.end(JSON.stringify(value));
}

async function readJsonBody(request, maxBytes = 32_768) {
  const chunks = [];
  let size = 0;
  for await (const chunk of request) {
    size += chunk.length;
    if (size > maxBytes) throw new DomainError("BODY_TOO_LARGE", "Request body exceeds 32 KB.", 413);
    chunks.push(chunk);
  }
  try {
    return JSON.parse(Buffer.concat(chunks).toString("utf8"));
  } catch {
    throw new DomainError("INVALID_JSON", "Request body must be valid JSON.");
  }
}

function logEvent(event, fields = {}) {
  process.stdout.write(`${JSON.stringify({
    timestamp: new Date().toISOString(),
    level: "INFO",
    service: "udi-local-api",
    event,
    ...fields
  })}\n`);
}

async function serveStatic(pathname, response) {
  let decoded;
  try {
    decoded = decodeURIComponent(pathname);
  } catch {
    response.writeHead(400).end("Bad path");
    return;
  }
  const requested = decoded === "/" ? "/index.html" : decoded;
  const filePath = resolve(publicDir, `.${requested}`);
  if (!filePath.startsWith(`${publicDir}${sep}`)) {
    response.writeHead(403).end("Forbidden");
    return;
  }
  try {
    const contents = await readFile(filePath);
    response.writeHead(200, {
      "content-type": mimeTypes[extname(filePath)] ?? "application/octet-stream",
      "cache-control": "no-cache",
      "x-content-type-options": "nosniff"
    });
    response.end(contents);
  } catch {
    response.writeHead(404, { "content-type": "text/plain; charset=utf-8" }).end("Not found");
  }
}

export function createAppServer({
  repository = createRepository(),
  demoMode = process.env.DEMO_MODE !== "false",
  quoteMode = process.env.SWAP_QUOTE_MODE ?? "demo",
  marketDataMode = process.env.MARKET_DATA_MODE ?? "fixture",
  nearOneClick = createNearOneClickProvider({
    baseUrl: process.env.ONECLICK_BASE_URL ?? "https://1click.chaindefuser.com",
    apiKey: process.env.ONECLICK_API_KEY ?? "",
    timeoutMs: Number.parseInt(process.env.ONECLICK_TIMEOUT_MS ?? "10000", 10),
    catalogueTtlMs: Number.parseInt(process.env.ASSET_CATALOGUE_TTL_SECONDS ?? "300", 10) * 1000,
    logger: ({ event, ...fields }) => logEvent(event, { provider: "near_1click", ...fields })
  }),
  defiLlama = createDefiLlamaProvider({
    ttlMs: Number.parseInt(process.env.MARKET_DATA_TTL_SECONDS ?? "300", 10) * 1000,
    timeoutMs: Number.parseInt(process.env.MARKET_DATA_TIMEOUT_MS ?? "10000", 10)
  }),
  maxRequestsPerMinute = 120
} = {}) {
  if (quoteMode !== "demo" && quoteMode !== "live") {
    throw new TypeError("SWAP_QUOTE_MODE must be either 'demo' or 'live'.");
  }
  if (marketDataMode !== "fixture" && marketDataMode !== "live") {
    throw new TypeError("MARKET_DATA_MODE must be either 'fixture' or 'live'.");
  }
  const rateWindows = new Map();

  return createHttpServer(async (request, response) => {
    const requestId = randomUUID();
    response.setHeader("x-content-type-options", "nosniff");
    response.setHeader("x-frame-options", "DENY");
    response.setHeader("referrer-policy", "strict-origin-when-cross-origin");
    response.setHeader("permissions-policy", "camera=(), microphone=(), geolocation=()");
    response.setHeader(
      "content-security-policy",
      "default-src 'self'; script-src 'self'; style-src 'self' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'"
    );

    const clientKey = request.socket.remoteAddress ?? "unknown";
    const minute = Math.floor(Date.now() / 60_000);
    const window = rateWindows.get(clientKey);
    if (!window || window.minute !== minute) rateWindows.set(clientKey, { minute, count: 0 });
    const currentWindow = rateWindows.get(clientKey);
    currentWindow.count += 1;
    if (currentWindow.count > maxRequestsPerMinute) {
      json(response, 429, { error: { code: "RATE_LIMITED", message: "Too many requests. Try again shortly." } }, requestId);
      return;
    }

    let url;
    try {
      url = new URL(request.url, "http://localhost");
      if (url.pathname.startsWith("/api/")) {
        response.setHeader("cache-control", "no-store");
        const { pathname } = url;

        if (request.method === "GET" && pathname === "/api/v1/health") {
          json(response, 200, {
            status: "ok",
            mode: quoteMode === "live" ? "local-live-quotes" : "local-demo",
            quoteMode,
            demoMode,
            liveQuotesConfigured: quoteMode === "live" && Boolean(nearOneClick?.configured),
            marketDataMode,
            executionEnabled: false,
            capturedAt: fixtureDate
          }, requestId);
          return;
        }
        if (request.method === "GET" && pathname === "/api/v1/chains") {
          json(response, 200, { mode: "configuration", chains }, requestId);
          return;
        }
        if (request.method === "GET" && pathname === "/api/v1/assets") {
          const chain = url.searchParams.get("chain");
          if (quoteMode === "live") {
            if (!nearOneClick) throw new DomainError("PROVIDER_NOT_CONFIGURED", "The live provider adapter is not configured.", 503);
            const providerAssets = await nearOneClick.getTokens();
            const liveAssets = providerAssets.map((asset) => ({
              ...asset,
              chain: providerChainKeys[asset.providerBlockchain] ?? asset.providerBlockchain
            }));
            const filtered = chain ? liveAssets.filter((asset) => asset.chain === chain) : liveAssets;
            json(response, 200, {
              mode: "provider_catalogue",
              source: "NEAR Intents 1Click API",
              servedAt: new Date().toISOString(),
              notice: "Catalogue entries are not a guarantee that any particular route is available. Request a live dry quote to check a pair.",
              assets: filtered
            }, requestId);
            return;
          }
          const filtered = chain ? assets.filter((asset) => asset.chain === chain) : assets;
          json(response, 200, { mode: "fixture", notice: fixtureNotice, assets: filtered }, requestId);
          return;
        }
        if (request.method === "GET" && pathname === "/api/v1/markets/overview") {
          if (marketDataMode === "live") {
            if (!defiLlama) {
              throw new DomainError("MARKET_PROVIDER_UNAVAILABLE", "The live market-data adapter is not configured.", 503);
            }
            const snapshot = await defiLlama.getNearSnapshot({
              force: url.searchParams.get("refresh") === "true"
            });
            json(response, 200, snapshot, requestId);
            return;
          }
          json(response, 200, marketOverview, requestId);
          return;
        }
        if (request.method === "GET" && pathname === "/api/v1/intelligence/near") {
          if (marketDataMode === "live") {
            if (!defiLlama) {
              throw new DomainError("MARKET_PROVIDER_UNAVAILABLE", "The live market-data adapter is not configured.", 503);
            }
            const snapshot = await defiLlama.getNearSnapshot();
            json(response, 200, snapshot, requestId);
            return;
          }
          json(response, 200, {
            mode: "fixture",
            notice: fixtureNotice,
            capturedAt: fixtureDate,
            metrics: nearMetrics,
            series: marketOverview.series
          }, requestId);
          return;
        }
        if (request.method === "GET" && pathname === "/api/v1/research/scenarios") {
          json(response, 200, researchInputs, requestId);
          return;
        }
        if (request.method === "GET" && pathname === "/api/v1/activity") {
          json(response, 200, { swaps: repository.listSwaps() }, requestId);
          return;
        }

        if (request.method === "POST" && pathname === "/api/v1/quotes") {
          const body = await readJsonBody(request);
          if (quoteMode === "live") {
            if (!nearOneClick?.configured) {
              throw new DomainError("PROVIDER_NOT_CONFIGURED", "Live quotes require a server-side 1Click API key.", 503);
            }
            if (!body || typeof body !== "object" || Array.isArray(body)) {
              throw new DomainError("INVALID_REQUEST", "Quote request must be a JSON object.");
            }
            if (typeof body.originAssetId !== "string" || typeof body.destinationAssetId !== "string") {
              throw new DomainError("INVALID_ASSET", "Select input and output assets from the live provider catalogue.");
            }
            const catalogue = await nearOneClick.getTokens();
            const providerAssetById = new Map(catalogue.map((asset) => [asset.id, asset]));
            const originAsset = providerAssetById.get(body.originAssetId);
            const destinationAsset = providerAssetById.get(body.destinationAssetId);
            if (!originAsset || !destinationAsset) {
              throw new DomainError("UNKNOWN_ASSET", "Asset is not present in the current provider catalogue.");
            }
            if (originAsset.id === destinationAsset.id) {
              throw new DomainError("SAME_ASSET", "Choose two different assets.");
            }
            const amountAtomic = parseUnits(body.amountIn, originAsset.decimals);
            const quote = await nearOneClick.getDryQuote({
              originAsset,
              destinationAsset,
              amountAtomic: amountAtomic.toString(),
              recipient: body.recipient,
              refundTo: body.refundTo,
              slippageBps: body.slippageBps ?? 100
            });
            logEvent("quote.provider_dry_returned", {
              request_id: requestId,
              provider: "near_1click",
              provider_quote_id: quote.providerQuoteId
            });
            json(response, 200, { quote }, requestId);
            return;
          }
          if (!demoMode) throw new DomainError("DEMO_DISABLED", "Simulation endpoints are disabled.", 503);
          const quote = createQuote({
            originAsset: assetById.get(body.originAssetId),
            destinationAsset: assetById.get(body.destinationAssetId),
            amountIn: body.amountIn
          });
          repository.saveQuote(quote);
          logEvent("quote.simulated", { request_id: requestId, quote_id: quote.id });
          json(response, 201, { quote }, requestId);
          return;
        }

        if (request.method === "POST" && pathname === "/api/v1/swaps") {
          if (quoteMode === "live") {
            throw new DomainError("LIVE_EXECUTION_UNAVAILABLE", "Live quotes are dry-only. Wallet signing and deposits are not implemented.", 503);
          }
          if (!demoMode) throw new DomainError("DEMO_DISABLED", "Simulation endpoints are disabled.", 503);
          const body = await readJsonBody(request);
          const idempotencyKey = request.headers["idempotency-key"];
          if (typeof idempotencyKey !== "string" || idempotencyKey.length < 8 || idempotencyKey.length > 128) {
            throw new DomainError("IDEMPOTENCY_KEY_REQUIRED", "Provide an Idempotency-Key header between 8 and 128 characters.");
          }
          const quote = repository.getQuote(body.quoteId);
          const result = repository.createSwap(createSwap(quote), idempotencyKey);
          if (result.created) logEvent("swap.simulation_created", { request_id: requestId, swap_id: result.swap.id });
          json(response, result.created ? 201 : 200, { swap: result.swap, created: result.created }, requestId);
          return;
        }

        const swapMatch = pathname.match(/^\/api\/v1\/swaps\/([a-zA-Z0-9_-]+)$/);
        if (request.method === "GET" && swapMatch) {
          const swap = repository.getSwap(swapMatch[1]);
          if (!swap) throw new DomainError("SWAP_NOT_FOUND", "Simulation record was not found.", 404);
          json(response, 200, { swap }, requestId);
          return;
        }

        const simulateMatch = pathname.match(/^\/api\/v1\/swaps\/([a-zA-Z0-9_-]+)\/simulate$/);
        if (request.method === "POST" && simulateMatch) {
          if (quoteMode === "live") {
            throw new DomainError("LIVE_EXECUTION_UNAVAILABLE", "Provider status tracking is not implemented.", 503);
          }
          if (!demoMode) throw new DomainError("DEMO_DISABLED", "Simulation endpoints are disabled.", 503);
          const current = repository.getSwap(simulateMatch[1]);
          const next = advanceSimulation(current);
          repository.updateSwap(next);
          logEvent("swap.simulation_advanced", { request_id: requestId, swap_id: next.id, state: next.state });
          json(response, 200, { swap: next }, requestId);
          return;
        }

        throw new DomainError("NOT_FOUND", "API route was not found.", 404);
      }

      if (request.method !== "GET" && request.method !== "HEAD") {
        response.writeHead(405, { allow: "GET, HEAD" }).end("Method not allowed");
        return;
      }
      await serveStatic(url.pathname, response);
    } catch (error) {
      const domainError = error instanceof DomainError
        ? error
        : new DomainError("INTERNAL_ERROR", "The request could not be completed.", 500);
      if (domainError.status >= 500) {
        process.stderr.write(`${JSON.stringify({
          timestamp: new Date().toISOString(),
          level: "ERROR",
          service: "udi-local-api",
          event: "request.failed",
          request_id: requestId,
          error_code: domainError.code
        })}\n`);
      }
      json(response, domainError.status, {
        error: { code: domainError.code, message: domainError.message },
        requestId
      }, requestId);
    }
  });
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const port = Number.parseInt(process.env.PORT ?? "8080", 10);
  const host = process.env.HOST ?? "127.0.0.1";
  const dataPath = process.env.DATA_FILE ?? "./data/udi.json";
  const demoMode = process.env.DEMO_MODE !== "false" && process.env.NODE_ENV !== "production";
  const repository = createRepository(resolve(projectDir, dataPath));
  const server = createAppServer({ repository, demoMode });
  server.listen(port, host, () => {
    process.stdout.write(`${JSON.stringify({
      timestamp: new Date().toISOString(),
      level: "INFO",
      service: "udi-local-api",
      event: "server.started",
      host,
      port,
      demo_mode: demoMode
    })}\n`);
  });
}
