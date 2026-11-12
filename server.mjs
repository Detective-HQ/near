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
  base: "base",
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
const executableEvmChains = new Set(["ethereum", "base", "arbitrum", "polygon", "avalanche", "bnb"]);
const terminalSwapStates = new Set(["SUCCEEDED", "REFUNDED", "FAILED", "EXPIRED", "CANCELLED"]);
const providerStatusStates = {
  PENDING_DEPOSIT: "AWAITING_USER_DEPOSIT",
  KNOWN_DEPOSIT_TX: "DEPOSIT_SUBMITTED",
  INCOMPLETE_DEPOSIT: "INCOMPLETE_DEPOSIT",
  PROCESSING: "PROCESSING",
  SUCCESS: "SUCCEEDED",
  REFUNDED: "REFUNDED",
  FAILED: "FAILED"
};
const providerStateRanks = {
  AWAITING_USER_DEPOSIT: 0,
  DEPOSIT_SUBMITTED: 1,
  INCOMPLETE_DEPOSIT: 1,
  PROCESSING: 2,
  SUCCEEDED: 3,
  REFUNDED: 3,
  FAILED: 3
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

function isEvmAddress(value) {
  return typeof value === "string" && /^0x[a-f0-9]{40}$/i.test(value);
}

function canBuildEvmTransfer(asset) {
  if (!asset || !executableEvmChains.has(asset.chain)) return false;
  const chain = chains.find((candidate) => candidate.key === asset.chain);
  if (!chain?.nativeCurrency) return false;
  if (typeof asset.contractAddress === "string" && asset.contractAddress.length > 0) {
    return isEvmAddress(asset.contractAddress);
  }
  return typeof asset.symbol === "string"
    && asset.symbol.toUpperCase() === chain.nativeCurrency.symbol.toUpperCase();
}

function lastKnownProviderState(swap) {
  if (Object.hasOwn(providerStateRanks, swap.state)) return swap.state;
  for (let index = swap.events.length - 1; index >= 0; index -= 1) {
    const eventState = swap.events[index]?.state;
    if (Object.hasOwn(providerStateRanks, eventState)) return eventState;
  }
  return null;
}

function safeExplorerUrl(transaction) {
  const candidate = transaction?.explorerUrl ?? transaction?.url;
  if (typeof candidate !== "string" || candidate.length > 2048) return null;
  try {
    const parsed = new URL(candidate);
    return parsed.protocol === "https:" && !parsed.username && !parsed.password
      ? parsed.toString()
      : null;
  } catch {
    return null;
  }
}

function mapProviderStatus(swap, status, now = Date.now()) {
  if (!swap || terminalSwapStates.has(swap.state)) return swap;
  let mappedState = status.providerStatus === "PENDING_DEPOSIT" && swap.transactions.source
    ? "DEPOSIT_SUBMITTED"
    : providerStatusStates[status.providerStatus] ?? "UNKNOWN_PROVIDER_STATUS";
  if (mappedState !== "UNKNOWN_PROVIDER_STATUS") {
    const previousKnownState = lastKnownProviderState(swap);
    if (
      previousKnownState
      && providerStateRanks[mappedState] < providerStateRanks[previousKnownState]
    ) {
      mappedState = previousKnownState;
    }
  }
  const details = status.swapDetails ?? {};
  const originTransactions = Array.isArray(details.originChainTxHashes)
    ? details.originChainTxHashes
    : [];
  const destinationTransactions = Array.isArray(details.destinationChainTxHashes)
    ? details.destinationChainTxHashes
    : [];
  const destinationHash = destinationTransactions.find((transaction) =>
    typeof transaction?.hash === "string"
  )?.hash ?? swap.transactions.destination;
  const sourceHash = originTransactions.find((transaction) =>
    typeof transaction?.hash === "string"
  )?.hash ?? swap.transactions.source;
  const refundHash = status.providerStatus === "REFUNDED"
    ? originTransactions.find((transaction) =>
        typeof transaction?.hash === "string"
        && transaction.hash.toLowerCase() !== String(sourceHash ?? "").toLowerCase()
      )?.hash ?? swap.transactions.refund
    : swap.transactions.refund;
  const transactionLinks = {
    source: safeExplorerUrl(originTransactions.find((transaction) =>
      typeof transaction?.hash === "string"
      && transaction.hash.toLowerCase() === String(sourceHash ?? "").toLowerCase()
    )),
    destination: safeExplorerUrl(destinationTransactions.find((transaction) =>
      typeof transaction?.hash === "string"
      && transaction.hash.toLowerCase() === String(destinationHash ?? "").toLowerCase()
    )),
    refund: status.providerStatus === "REFUNDED"
      ? safeExplorerUrl(originTransactions.find((transaction) =>
          typeof transaction?.hash === "string"
          && transaction.hash.toLowerCase() === String(refundHash ?? "").toLowerCase()
        ))
      : swap.transactionLinks?.refund ?? null
  };
  const stateChanged = mappedState !== swap.state || status.providerStatus !== swap.providerStatus;
  return {
    ...swap,
    state: mappedState,
    providerStatus: status.providerStatus,
    providerUpdatedAt: status.updatedAt,
    providerDetails: details,
    realSettlement: mappedState === "SUCCEEDED" && status.providerStatus === "SUCCESS",
    transactions: {
      ...swap.transactions,
      source: sourceHash,
      destination: destinationHash,
      refund: refundHash
    },
    transactionLinks: {
      source: transactionLinks.source ?? swap.transactionLinks?.source ?? null,
      destination: transactionLinks.destination ?? swap.transactionLinks?.destination ?? null,
      refund: transactionLinks.refund ?? swap.transactionLinks?.refund ?? null
    },
    updatedAt: new Date(now).toISOString(),
    events: stateChanged
      ? [
          ...swap.events,
          {
            state: mappedState,
            detail: mappedState === "UNKNOWN_PROVIDER_STATUS"
              ? "The provider returned an unrecognized status. This swap is not marked complete."
              : `Provider status: ${status.providerStatus}.`,
            at: new Date(now).toISOString()
          }
        ]
      : swap.events
  };
}

function makeLiveSwap(quote, reviewedQuoteId, now = Date.now()) {
  const timestamp = new Date(now).toISOString();
  return {
    id: `swp_${randomUUID()}`,
    quoteId: quote.id,
    reviewedQuoteId,
    state: "AWAITING_USER_DEPOSIT",
    providerStatus: null,
    mode: "live",
    realSettlement: false,
    transactions: { source: null, destination: null, refund: null },
    createdAt: timestamp,
    updatedAt: timestamp,
    events: [{
      state: "AWAITING_USER_DEPOSIT",
      detail: "Executable quote created. No funds have been sent.",
      at: timestamp
    }],
    quote
  };
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
  demoMode = process.env.DEMO_MODE === "true",
  quoteMode = process.env.SWAP_QUOTE_MODE ?? "demo",
  marketDataMode = process.env.MARKET_DATA_MODE ?? "fixture",
  executionEnabled = process.env.ENABLE_LIVE_EXECUTION === "true",
  nearOneClick = createNearOneClickProvider({
    baseUrl: process.env.ONECLICK_BASE_URL ?? "https://1click.chaindefuser.com",
    apiKey: process.env.ONECLICK_JWT ?? "",
    timeoutMs: Number.parseInt(process.env.ONECLICK_TIMEOUT_MS ?? "10000", 10),
    catalogueTtlMs: Number.parseInt(process.env.ASSET_CATALOGUE_TTL_SECONDS ?? "300", 10) * 1000,
    logger: ({ event, ...fields }) => logEvent(event, { provider: "near_1click", ...fields })
  }),
  defiLlama = createDefiLlamaProvider({
    ttlMs: Number.parseInt(process.env.MARKET_DATA_TTL_SECONDS ?? "300", 10) * 1000,
    timeoutMs: Number.parseInt(process.env.MARKET_DATA_TIMEOUT_MS ?? "10000", 10)
  }),
  maxRequestsPerMinute = 120,
  statusPollIntervalMs = Number.parseInt(process.env.SWAP_STATUS_POLL_SECONDS ?? "5", 10) * 1000
} = {}) {
  if (quoteMode !== "demo" && quoteMode !== "live") {
    throw new TypeError("SWAP_QUOTE_MODE must be either 'demo' or 'live'.");
  }
  if (marketDataMode !== "fixture" && marketDataMode !== "live") {
    throw new TypeError("MARKET_DATA_MODE must be either 'fixture' or 'live'.");
  }
  const liveExecutionReady = quoteMode === "live" && executionEnabled === true && nearOneClick?.configured === true;
  const rateWindows = new Map();
  const pendingSwapCreations = new Map();
  const boundedPollIntervalMs = Number.isFinite(statusPollIntervalMs)
    ? Math.max(1000, Math.min(statusPollIntervalMs, 60_000))
    : 5000;
  const configuredDeadlineSeconds = Number.parseInt(process.env.ONECLICK_QUOTE_DEADLINE_SECONDS ?? "180", 10);
  const executableQuoteDeadlineSeconds = Number.isFinite(configuredDeadlineSeconds)
    ? Math.min(Math.max(configuredDeadlineSeconds, 60), 600)
    : 180;
  let pollRunning = false;

  const server = createHttpServer(async (request, response) => {
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
            mode: quoteMode === "live" ? "live-provider" : "local-demo",
            quoteMode,
            demoMode,
            liveQuotesConfigured: quoteMode === "live" && Boolean(nearOneClick?.configured),
            marketDataMode,
            executionEnabled: liveExecutionReady,
            capturedAt: fixtureDate
          }, requestId);
          return;
        }
        if (request.method === "GET" && pathname === "/api/v1/chains") {
          json(response, 200, {
            mode: "configuration",
            chains: chains.map((chain) => {
              const swapEnabled = liveExecutionReady && executableEvmChains.has(chain.key);
              return {
                ...chain,
                swapEnabled,
                status: swapEnabled
                  ? "EVM deposit enabled · route per pair"
                  : chain.key === "celo"
                    ? "Feature-flagged"
                    : chain.walletSupported
                      ? "Wallet-only"
                      : "Planned"
              };
            })
          }, requestId);
          return;
        }
        if (request.method === "GET" && pathname === "/api/v1/assets") {
          const chain = url.searchParams.get("chain");
          if (quoteMode === "live") {
            if (!nearOneClick) throw new DomainError("PROVIDER_NOT_CONFIGURED", "The live provider adapter is not configured.", 503);
            const providerAssets = await nearOneClick.getTokens();
            const liveAssets = providerAssets.map((asset) => ({
              ...asset,
              chain: providerChainKeys[asset.providerBlockchain] ?? asset.providerBlockchain,
              sourceDepositSupported: canBuildEvmTransfer({
                ...asset,
                chain: providerChainKeys[asset.providerBlockchain] ?? asset.providerBlockchain
              })
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
              throw new DomainError("PROVIDER_NOT_CONFIGURED", "Live quotes require a server-side NEAR Intents partner JWT.", 503);
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
            const normalizedOrigin = {
              ...originAsset,
              chain: providerChainKeys[originAsset.providerBlockchain] ?? originAsset.providerBlockchain
            };
            const normalizedDestination = {
              ...destinationAsset,
              chain: providerChainKeys[destinationAsset.providerBlockchain] ?? destinationAsset.providerBlockchain
            };
            if (
              !executableEvmChains.has(normalizedOrigin.chain)
              || !executableEvmChains.has(normalizedDestination.chain)
            ) {
              throw new DomainError(
                "CHAIN_EXECUTION_UNAVAILABLE",
                "This first executable slice supports EVM-origin and EVM-destination assets only."
              );
            }
            if (!isEvmAddress(body.recipient)) {
              throw new DomainError("INVALID_RECIPIENT", "Enter a valid EVM destination address.");
            }
            if (!isEvmAddress(body.refundTo)) {
              throw new DomainError("INVALID_REFUND_ADDRESS", "Enter a valid EVM refund address.");
            }
            const amountAtomic = parseUnits(body.amountIn, originAsset.decimals);
            const slippageBps = body.slippageBps ?? 100;
            const quote = await nearOneClick.getDryQuote({
              originAsset: normalizedOrigin,
              destinationAsset: normalizedDestination,
              amountAtomic: amountAtomic.toString(),
              recipient: body.recipient,
              refundTo: body.refundTo,
              slippageBps
            });
            repository.saveQuote(quote);
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
            if (!liveExecutionReady) {
              throw new DomainError(
                "LIVE_EXECUTION_DISABLED",
                "Live execution needs a configured 1Click partner JWT and ENABLE_LIVE_EXECUTION=true.",
                503
              );
            }
            const body = await readJsonBody(request);
            if (!body || typeof body !== "object" || Array.isArray(body)) {
              throw new DomainError("INVALID_REQUEST", "Swap request must be a JSON object.");
            }
            const idempotencyKey = request.headers["idempotency-key"];
            if (typeof idempotencyKey !== "string" || idempotencyKey.length < 8 || idempotencyKey.length > 128) {
              throw new DomainError("IDEMPOTENCY_KEY_REQUIRED", "Provide an Idempotency-Key header between 8 and 128 characters.");
            }
            const existing = repository.getSwapByIdempotencyKey(idempotencyKey);
            if (existing) {
              json(response, 200, { swap: existing, created: false }, requestId);
              return;
            }
            const inFlight = pendingSwapCreations.get(idempotencyKey);
            if (inFlight) {
              const swap = await inFlight;
              json(response, 200, { swap, created: false }, requestId);
              return;
            }
            const creation = (async () => {
              const dryQuote = repository.getQuote(body.quoteId);
              if (!dryQuote || dryQuote.mode !== "provider_dry_quote") {
                throw new DomainError("QUOTE_NOT_FOUND", "A fresh provider dry quote is required.", 404);
              }
              const recipient = typeof body.recipient === "string" ? body.recipient.trim() : "";
              const refundTo = typeof body.refundTo === "string" ? body.refundTo.trim() : "";
              if (!isEvmAddress(recipient)) {
                throw new DomainError("INVALID_RECIPIENT", "Enter a valid EVM destination address.");
              }
              if (!isEvmAddress(refundTo)) {
                throw new DomainError("INVALID_REFUND_ADDRESS", "Enter a valid EVM refund address.");
              }
              if (
                !executableEvmChains.has(dryQuote.originAsset.chain)
                || !executableEvmChains.has(dryQuote.destinationAsset.chain)
              ) {
                throw new DomainError(
                  "CHAIN_EXECUTION_UNAVAILABLE",
                  "This first executable slice supports EVM-origin and EVM-destination assets only."
                );
              }
              if (!canBuildEvmTransfer(dryQuote.originAsset)) {
                throw new DomainError(
                  "SOURCE_TRANSFER_UNAVAILABLE",
                  "This asset does not have verified EVM transfer metadata for the wallet deposit builder."
                );
              }

              const catalogue = await nearOneClick.getTokens();
              const providerAssetById = new Map(catalogue.map((asset) => [asset.id, asset]));
              const rawOrigin = providerAssetById.get(dryQuote.originAssetId);
              const rawDestination = providerAssetById.get(dryQuote.destinationAssetId);
              if (!rawOrigin || !rawDestination) {
                throw new DomainError("UNKNOWN_ASSET", "The reviewed asset is no longer present in the provider catalogue.");
              }
              const originAsset = {
                ...rawOrigin,
                chain: providerChainKeys[rawOrigin.providerBlockchain] ?? rawOrigin.providerBlockchain
              };
              const destinationAsset = {
                ...rawDestination,
                chain: providerChainKeys[rawDestination.providerBlockchain] ?? rawDestination.providerBlockchain
              };
              if (
                originAsset.chain !== dryQuote.originAsset.chain
                || destinationAsset.chain !== dryQuote.destinationAsset.chain
                || originAsset.decimals !== dryQuote.originAsset.decimals
                || destinationAsset.decimals !== dryQuote.destinationAsset.decimals
              ) {
                throw new DomainError("CATALOGUE_CHANGED", "The provider catalogue changed. Request and review a fresh quote.");
              }

              const quote = await nearOneClick.getExecutableQuote({
                originAsset,
                destinationAsset,
                amountAtomic: dryQuote.amountInAtomic,
                recipient,
                refundTo,
                slippageBps: dryQuote.slippageBps,
                deadline: new Date(Date.now() + executableQuoteDeadlineSeconds * 1000).toISOString(),
                depositMode: "SIMPLE"
              });
              if (!isEvmAddress(quote.depositAddress)) {
                throw new DomainError(
                  "UNSUPPORTED_DEPOSIT_ADDRESS",
                  "The provider returned a deposit address that this EVM wallet cannot safely use."
                );
              }
              if (quote.depositMemo) {
                throw new DomainError(
                  "UNSUPPORTED_DEPOSIT_MEMO",
                  "This EVM wallet path cannot safely submit a deposit memo, so no deposit instructions were issued."
                );
              }
              if (quote.expiresAt && Date.parse(quote.expiresAt) <= Date.now()) {
                throw new DomainError("QUOTE_EXPIRED", "The provider returned an expired executable quote. Request a fresh quote.");
              }
              repository.saveQuote(quote);
              const swap = makeLiveSwap(quote, dryQuote.id);
              return repository.createSwap(swap, idempotencyKey).swap;
            })();
            pendingSwapCreations.set(idempotencyKey, creation);
            try {
              const swap = await creation;
              logEvent("swap.executable_quote_created", {
                request_id: requestId,
                swap_id: swap.id,
                provider_quote_id: swap.quote.providerQuoteId
              });
              json(response, 201, { swap, created: true }, requestId);
            } finally {
              pendingSwapCreations.delete(idempotencyKey);
            }
            return;
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
        const depositMatch = pathname.match(/^\/api\/v1\/swaps\/([a-zA-Z0-9_-]+)\/deposit$/);
        const cancelMatch = pathname.match(/^\/api\/v1\/swaps\/([a-zA-Z0-9_-]+)\/cancel$/);
        if (request.method === "POST" && depositMatch) {
          if (!liveExecutionReady) {
            throw new DomainError("LIVE_EXECUTION_DISABLED", "Live execution is not enabled on this server.", 503);
          }
          const swap = repository.getSwap(depositMatch[1]);
          if (!swap || swap.mode !== "live") {
            throw new DomainError("SWAP_NOT_FOUND", "Live swap was not found.", 404);
          }
          const body = await readJsonBody(request);
          if (!body || typeof body !== "object" || Array.isArray(body)) {
            throw new DomainError("INVALID_REQUEST", "Deposit report must be a JSON object.");
          }
          if (typeof body.txHash !== "string" || !/^0x[a-f0-9]{64}$/i.test(body.txHash)) {
            throw new DomainError("INVALID_TRANSACTION_HASH", "Enter a valid EVM transaction hash.");
          }
          const originChain = swap.quote.originAsset.chain;
          const expectedChain = chains.find((chain) => chain.key === originChain);
          if (!expectedChain?.chainId || String(body.chainId) !== String(expectedChain.chainId)) {
            throw new DomainError("WRONG_NETWORK", "The reported transaction network does not match the quoted source chain.");
          }
          if (swap.transactions.source) {
            if (swap.transactions.source.toLowerCase() !== body.txHash.toLowerCase()) {
              throw new DomainError("DEPOSIT_ALREADY_RECORDED", "A different source transaction is already recorded for this swap.", 409);
            }
            json(response, 200, { swap, duplicate: true }, requestId);
            return;
          }
          if (swap.state !== "AWAITING_USER_DEPOSIT") {
            throw new DomainError("INVALID_SWAP_STATE", "This swap is not waiting for a wallet deposit.", 409);
          }
          if (swap.quote.expiresAt && Date.parse(swap.quote.expiresAt) <= Date.now()) {
            throw new DomainError("QUOTE_EXPIRED", "The deposit address has expired. Do not send funds to it.");
          }
          if (repository.listSwaps().some((item) =>
            item.id !== swap.id
            && typeof item.transactions?.source === "string"
            && item.transactions.source.toLowerCase() === body.txHash.toLowerCase()
          )) {
            throw new DomainError("TRANSACTION_ALREADY_RECORDED", "This transaction hash is already linked to another swap.", 409);
          }

          const timestamp = new Date().toISOString();
          let updated = {
            ...swap,
            state: "DEPOSIT_SUBMITTED",
            updatedAt: timestamp,
            transactions: { ...swap.transactions, source: body.txHash },
            events: [
              ...swap.events,
              {
                state: "DEPOSIT_SUBMITTED",
                detail: "Wallet reported a source transaction. Provider settlement is still unverified.",
                at: timestamp
              }
            ]
          };
          repository.updateSwap(updated);
          logEvent("wallet.deposit.broadcast", {
            request_id: requestId,
            swap_id: updated.id,
            origin_chain: originChain,
            tx_hash: body.txHash
          });

          try {
            const acknowledgement = await nearOneClick.submitDepositTx({
              txHash: body.txHash,
              depositAddress: swap.quote.depositAddress,
              depositMemo: swap.quote.depositMemo
            });
            const withStatus = mapProviderStatus(updated, acknowledgement);
            if (withStatus !== updated) {
              updated = withStatus;
              repository.updateSwap(updated);
            }
            logEvent("provider.deposit.submitted", {
              request_id: requestId,
              swap_id: updated.id,
              provider_status: acknowledgement.providerStatus
            });
          } catch (error) {
            logEvent("provider.deposit.submit_failed", {
              request_id: requestId,
              swap_id: updated.id,
              error_code: error instanceof DomainError ? error.code : "PROVIDER_ERROR"
            });
          }
          json(response, 202, { swap: updated, duplicate: false }, requestId);
          return;
        }

        if (request.method === "POST" && cancelMatch) {
          const swap = repository.getSwap(cancelMatch[1]);
          if (!swap || swap.mode !== "live") {
            throw new DomainError("SWAP_NOT_FOUND", "Live swap was not found.", 404);
          }
          if (swap.state === "CANCELLED" || swap.state === "EXPIRED") {
            json(response, 200, { swap }, requestId);
            return;
          }
          if (swap.state !== "AWAITING_USER_DEPOSIT" || swap.transactions?.source) {
            throw new DomainError(
              "INVALID_SWAP_STATE",
              "Only an unfunded executable quote can be discarded.",
              409
            );
          }
          const expired = Boolean(swap.quote.expiresAt && Date.parse(swap.quote.expiresAt) <= Date.now());
          const state = expired ? "EXPIRED" : "CANCELLED";
          const timestamp = new Date().toISOString();
          const updated = {
            ...swap,
            state,
            updatedAt: timestamp,
            events: [
              ...swap.events,
              {
                state,
                detail: expired
                  ? "The provider deposit deadline passed before a deposit was recorded."
                  : "The user discarded the unfunded deposit instructions.",
                at: timestamp
              }
            ]
          };
          repository.updateSwap(updated);
          logEvent("swap.instructions.discarded", {
            request_id: requestId,
            swap_id: updated.id,
            state
          });
          json(response, 200, { swap: updated }, requestId);
          return;
        }

        if (request.method === "GET" && swapMatch) {
          let swap = repository.getSwap(swapMatch[1]);
          if (!swap) throw new DomainError("SWAP_NOT_FOUND", "Swap was not found.", 404);
          if (
            swap.mode === "live"
            && swap.state === "AWAITING_USER_DEPOSIT"
            && !swap.transactions?.source
            && swap.quote.expiresAt
            && Date.parse(swap.quote.expiresAt) <= Date.now()
          ) {
            const timestamp = new Date().toISOString();
            swap = repository.updateSwap({
              ...swap,
              state: "EXPIRED",
              updatedAt: timestamp,
              events: [
                ...swap.events,
                {
                  state: "EXPIRED",
                  detail: "The provider deposit deadline passed before a deposit was recorded.",
                  at: timestamp
                }
              ]
            });
          }
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

  async function pollPendingSwaps() {
    if (pollRunning || !liveExecutionReady || typeof nearOneClick?.getStatus !== "function") return;
    pollRunning = true;
    try {
      const pending = repository.listSwaps().filter((swap) =>
        swap.mode === "live"
        && !terminalSwapStates.has(swap.state)
      );
      for (const swap of pending) {
        if (
          swap.state === "AWAITING_USER_DEPOSIT"
          && !swap.transactions?.source
          && swap.quote.expiresAt
          && Date.parse(swap.quote.expiresAt) <= Date.now()
        ) {
          const timestamp = new Date().toISOString();
          const expired = {
            ...swap,
            state: "EXPIRED",
            updatedAt: timestamp,
            events: [
              ...swap.events,
              {
                state: "EXPIRED",
                detail: "The provider deposit deadline passed before a deposit was recorded.",
                at: timestamp
              }
            ]
          };
          repository.updateSwap(expired);
          logEvent("swap.status.changed", {
            swap_id: swap.id,
            old_state: swap.state,
            new_state: expired.state
          });
          continue;
        }
        if (!swap.transactions?.source) continue;
        try {
          const status = await nearOneClick.getStatus({
            depositAddress: swap.quote.depositAddress,
            depositMemo: swap.quote.depositMemo
          });
          const updated = mapProviderStatus(swap, status);
          if (updated.providerStatus !== swap.providerStatus || updated.state !== swap.state) {
            repository.updateSwap(updated);
            logEvent("swap.status.changed", {
              swap_id: swap.id,
              old_state: swap.state,
              new_state: updated.state,
              provider_status: updated.providerStatus
            });
            if (updated.state === "UNKNOWN_PROVIDER_STATUS") {
              logEvent("swap.provider_status.unknown", {
                swap_id: swap.id,
                provider_status: updated.providerStatus
              });
            }
          }
        } catch (error) {
          logEvent("swap.status.poll_failed", {
            swap_id: swap.id,
            error_code: error instanceof DomainError ? error.code : "PROVIDER_ERROR"
          });
        }
      }
    } finally {
      pollRunning = false;
    }
  }

  if (liveExecutionReady && typeof nearOneClick?.getStatus === "function") {
    const pollTimer = setInterval(() => { void pollPendingSwaps(); }, boundedPollIntervalMs);
    pollTimer.unref?.();
    server.on("close", () => clearInterval(pollTimer));
  }
  return server;
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const port = Number.parseInt(process.env.PORT ?? "8080", 10);
  const host = process.env.HOST ?? "127.0.0.1";
  const dataPath = process.env.DATA_FILE ?? "./data/udi.json";
  const demoMode = process.env.DEMO_MODE === "true" && process.env.NODE_ENV !== "production";
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
