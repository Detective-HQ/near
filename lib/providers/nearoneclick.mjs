import { randomUUID } from "node:crypto";
import { DomainError, formatUnits } from "../domain.mjs";

const MAX_RESPONSE_BYTES = 2_000_000;
const DEFAULT_BASE_URL = "https://1click.chaindefuser.com";

function providerError(code, message, status = 502) {
  return new DomainError(code, message, status);
}

async function readBoundedText(response) {
  const reader = response.body?.getReader?.();
  let text;

  if (reader) {
    const chunks = [];
    let size = 0;
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > MAX_RESPONSE_BYTES) {
        await reader.cancel().catch(() => {});
        throw providerError("PROVIDER_RESPONSE_TOO_LARGE", "The swap provider returned an oversized response.");
      }
      chunks.push(Buffer.from(value));
    }
    text = Buffer.concat(chunks).toString("utf8");
  } else if (typeof response.text === "function") {
    text = await response.text();
    if (Buffer.byteLength(text, "utf8") > MAX_RESPONSE_BYTES) {
      throw providerError("PROVIDER_RESPONSE_TOO_LARGE", "The swap provider returned an oversized response.");
    }
  } else {
    throw providerError("PROVIDER_INVALID_RESPONSE", "The swap provider returned an unreadable response.");
  }

  return text;
}

function validateTokens(payload, observedAt) {
  if (!Array.isArray(payload)) {
    throw providerError("PROVIDER_INVALID_CATALOGUE", "The swap provider returned an invalid asset catalogue.");
  }

  const seenAssetIds = new Set();
  return payload.map((token) => {
    if (
      !token ||
      typeof token.assetId !== "string" ||
      token.assetId.length < 1 ||
      token.assetId.length > 512 ||
      typeof token.blockchain !== "string" ||
      token.blockchain.length < 1 ||
      typeof token.symbol !== "string" ||
      token.symbol.length < 1 ||
      !Number.isInteger(token.decimals) ||
      token.decimals < 0 ||
      token.decimals > 36
    ) {
      throw providerError("PROVIDER_INVALID_CATALOGUE", "The swap provider returned an invalid asset entry.");
    }
    if (seenAssetIds.has(token.assetId)) {
      throw providerError("PROVIDER_INVALID_CATALOGUE", "The swap provider returned duplicate asset identifiers.");
    }
    seenAssetIds.add(token.assetId);

    return {
      id: token.assetId,
      providerAssetId: token.assetId,
      symbol: token.symbol.slice(0, 40),
      name: typeof token.name === "string" ? token.name.slice(0, 100) : token.symbol.slice(0, 40),
      chain: token.blockchain,
      providerBlockchain: token.blockchain,
      decimals: token.decimals,
      contractAddress: typeof token.contractAddress === "string" ? token.contractAddress : null,
      priceUsd: typeof token.price === "number" && Number.isFinite(token.price) ? token.price : null,
      source: "NEAR Intents 1Click token catalogue",
      observedAt: typeof token.priceUpdatedAt === "string" ? token.priceUpdatedAt : observedAt,
      enabledForLiveSwap: false
    };
  });
}

function validateQuoteResponse(payload, { originAsset, destinationAsset, amountAtomic, slippageBps }) {
  const quote = payload?.quote;
  if (
    !payload ||
    typeof payload.correlationId !== "string" ||
    !quote ||
    typeof quote !== "object" ||
    typeof quote.amountOut !== "string" ||
    !/^\d+$/.test(quote.amountOut) ||
    (quote.minAmountOut !== undefined && (typeof quote.minAmountOut !== "string" || !/^\d+$/.test(quote.minAmountOut)))
  ) {
    throw providerError("PROVIDER_INVALID_QUOTE", "The swap provider returned an incomplete quote.");
  }

  // The request is dry-only. If the provider ever returns an executable address
  // for a dry request, reject it rather than allowing it to reach the UI.
  if (quote.depositAddress !== undefined || quote.depositMemo !== undefined) {
    throw providerError("PROVIDER_UNEXPECTED_DEPOSIT_DATA", "The provider returned unexpected deposit instructions for a dry quote.");
  }

  const amountOutAtomic = quote.amountOut;
  const minAmountOutAtomic = quote.minAmountOut ?? null;
  return {
    id: `qte_${randomUUID()}`,
    mode: "provider_dry_quote",
    provider: "near_1click",
    providerQuoteId: payload.correlationId,
    providerSignature: typeof payload.signature === "string" ? payload.signature : null,
    providerTimestamp: typeof payload.timestamp === "string" ? payload.timestamp : null,
    originAssetId: originAsset.id,
    destinationAssetId: destinationAsset.id,
    originAsset: {
      id: originAsset.id,
      symbol: originAsset.symbol,
      chain: originAsset.chain,
      decimals: originAsset.decimals
    },
    destinationAsset: {
      id: destinationAsset.id,
      symbol: destinationAsset.symbol,
      chain: destinationAsset.chain,
      decimals: destinationAsset.decimals
    },
    amountInAtomic: amountAtomic,
    amountIn: formatUnits(amountAtomic, originAsset.decimals),
    amountOutEstimateAtomic: amountOutAtomic,
    amountOutEstimate: typeof quote.amountOutFormatted === "string"
      ? quote.amountOutFormatted
      : formatUnits(amountOutAtomic, destinationAsset.decimals, 8),
    minimumAmountOutAtomic: minAmountOutAtomic,
    minimumAmountOut: minAmountOutAtomic === null
      ? null
      : (typeof quote.minAmountOutFormatted === "string"
        ? quote.minAmountOutFormatted
        : formatUnits(minAmountOutAtomic, destinationAsset.decimals, 8)),
    slippageBps,
    expiresAt: null,
    depositAddress: null,
    depositMemo: null,
    warning: "Live provider estimate only. This dry quote contains no deposit instructions and cannot be used to send funds."
  };
}

export function createNearOneClickProvider({
  baseUrl = DEFAULT_BASE_URL,
  apiKey = "",
  timeoutMs = 10_000,
  catalogueTtlMs = 300_000,
  fetchImpl = globalThis.fetch,
  now = () => Date.now(),
  logger = () => {}
} = {}) {
  if (typeof fetchImpl !== "function") throw new TypeError("A Fetch-compatible implementation is required.");
  const endpoint = new URL(baseUrl);
  if (endpoint.protocol !== "https:" && endpoint.hostname !== "localhost" && endpoint.hostname !== "127.0.0.1") {
    throw new TypeError("The provider base URL must use HTTPS.");
  }
  if (endpoint.search || endpoint.hash) throw new TypeError("The provider base URL must not include a query or fragment.");
  const root = endpoint.toString().replace(/\/+$/, "");
  const requestTimeoutMs = Number.isFinite(timeoutMs) ? Math.min(Math.max(timeoutMs, 100), 60_000) : 10_000;
  const catalogueCacheTtlMs = Number.isFinite(catalogueTtlMs)
    ? Math.min(Math.max(catalogueTtlMs, 1_000), 3_600_000)
    : 300_000;
  const credential = typeof apiKey === "string" ? apiKey.trim() : "";
  let cachedTokens = null;
  let cacheExpiresAt = 0;
  let catalogueInFlight = null;

  async function request(path, { method = "GET", body, authenticated = false } = {}) {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), requestTimeoutMs);
    const startedAt = now();
    const headers = { accept: "application/json" };
    if (body !== undefined) headers["content-type"] = "application/json";
    if (authenticated && credential) headers["X-API-Key"] = credential;

    try {
      const response = await fetchImpl(`${root}${path}`, {
        method,
        headers,
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
        signal: controller.signal,
        redirect: "error"
      });
      const responseText = await readBoundedText(response);
      logger({
        event: "oneclick.request.completed",
        path,
        status: response.status,
        duration_ms: Math.max(0, now() - startedAt)
      });
      if (!response.ok) {
        if (response.status === 401 || response.status === 403) {
          throw providerError("PROVIDER_AUTH_FAILED", "The swap provider rejected server credentials.", 503);
        }
        if (response.status === 429) {
          throw providerError("PROVIDER_RATE_LIMITED", "The swap provider is rate-limiting requests. Try again shortly.", 503);
        }
        if (response.status >= 500) {
          throw providerError("PROVIDER_UNAVAILABLE", "The swap provider is temporarily unavailable.", 503);
        }
        throw providerError("PROVIDER_REQUEST_REJECTED", "The swap provider could not complete this request.", 422);
      }
      try {
        return JSON.parse(responseText);
      } catch {
        throw providerError("PROVIDER_INVALID_RESPONSE", "The swap provider returned invalid JSON.");
      }
    } catch (error) {
      if (error instanceof DomainError) throw error;
      const timedOut = error?.name === "AbortError";
      logger({
        event: "oneclick.request.failed",
        path,
        error_code: timedOut ? "PROVIDER_TIMEOUT" : "PROVIDER_NETWORK_ERROR",
        duration_ms: Math.max(0, now() - startedAt)
      });
      throw providerError(
        timedOut ? "PROVIDER_TIMEOUT" : "PROVIDER_NETWORK_ERROR",
        timedOut ? "The swap provider did not respond in time." : "The swap provider could not be reached.",
        timedOut ? 504 : 502
      );
    } finally {
      clearTimeout(timeout);
    }
  }

  return {
    configured: Boolean(credential),

    async getTokens() {
      if (cachedTokens && now() < cacheExpiresAt) return cachedTokens;
      if (catalogueInFlight) return catalogueInFlight;
      catalogueInFlight = (async () => {
        const payload = await request("/v0/tokens");
        const observedAt = new Date(now()).toISOString();
        const tokens = validateTokens(payload, observedAt);
        cachedTokens = tokens;
        cacheExpiresAt = now() + catalogueCacheTtlMs;
        return tokens;
      })();
      try {
        return await catalogueInFlight;
      } finally {
        catalogueInFlight = null;
      }
    },

    async getDryQuote({
      originAsset,
      destinationAsset,
      amountAtomic,
      recipient,
      refundTo,
      slippageBps = 100
    }) {
      if (!credential) {
        throw providerError("PROVIDER_NOT_CONFIGURED", "Live quotes are not configured on this server.", 503);
      }
      if (!originAsset || !destinationAsset || originAsset.id === destinationAsset.id) {
        throw new DomainError("INVALID_ASSET_PAIR", "Choose two different assets from the live provider catalogue.");
      }
      if (typeof recipient !== "string" || recipient.trim().length < 1 || recipient.length > 256) {
        throw new DomainError("RECIPIENT_REQUIRED", "Enter a destination address for the quote.");
      }
      if (typeof refundTo !== "string" || refundTo.trim().length < 1 || refundTo.length > 256) {
        throw new DomainError("REFUND_ADDRESS_REQUIRED", "Enter a refund address for the quote.");
      }
      if (!/^\d+$/.test(String(amountAtomic)) || BigInt(amountAtomic) <= 0n) {
        throw new DomainError("INVALID_AMOUNT", "Amount must be a positive integer in the asset's smallest unit.");
      }
      if (!Number.isInteger(slippageBps) || slippageBps < 0 || slippageBps > 5000) {
        throw new DomainError("INVALID_SLIPPAGE", "Slippage must be between 0 and 5000 basis points.");
      }

      const payload = await request("/v0/quote", {
        method: "POST",
        authenticated: true,
        body: {
          dry: true,
          swapType: "EXACT_INPUT",
          slippageTolerance: slippageBps,
          originAsset: originAsset.id,
          depositType: "ORIGIN_CHAIN",
          destinationAsset: destinationAsset.id,
          amount: String(amountAtomic),
          refundTo: refundTo.trim(),
          refundType: "ORIGIN_CHAIN",
          recipient: recipient.trim(),
          recipientType: "DESTINATION_CHAIN"
        }
      });
      return validateQuoteResponse(payload, {
        originAsset,
        destinationAsset,
        amountAtomic: String(amountAtomic),
        slippageBps
      });
    }
  };
}
