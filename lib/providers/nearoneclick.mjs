import { randomUUID } from "node:crypto";
import { verifyQuoteSignature } from "@defuse-protocol/one-click-sdk-typescript";
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

function validateQuoteResponse(
  payload,
  {
    originAsset,
    destinationAsset,
    amountAtomic,
    slippageBps,
    dry = true,
    request = null,
    verifySignature,
    now = Date.now()
  }
) {
  const quote = payload?.quote;
  if (
    !payload ||
    typeof payload.correlationId !== "string" ||
    !quote ||
    typeof quote !== "object" ||
    typeof quote.amountOut !== "string" ||
    !/^\d+$/.test(quote.amountOut) ||
    typeof quote.minAmountOut !== "string" ||
    !/^\d+$/.test(quote.minAmountOut) ||
    typeof payload.timestamp !== "string" ||
    typeof payload.signature !== "string"
  ) {
    throw providerError("PROVIDER_INVALID_QUOTE", "The swap provider returned an incomplete quote.");
  }

  if (BigInt(quote.minAmountOut) > BigInt(quote.amountOut)) {
    throw providerError("PROVIDER_INVALID_QUOTE", "The provider minimum output exceeds its quoted output.");
  }

  const signedRequest = payload.quoteRequest;
  const matchesRequest = signedRequest
    && signedRequest.dry === request?.dry
    && signedRequest.swapType === "EXACT_INPUT"
    && signedRequest.depositType === "ORIGIN_CHAIN"
    && signedRequest.refundType === "ORIGIN_CHAIN"
    && signedRequest.recipientType === "DESTINATION_CHAIN"
    && signedRequest.originAsset === request?.originAsset
    && signedRequest.destinationAsset === request?.destinationAsset
    && String(signedRequest.amount) === String(request?.amount)
    && signedRequest.recipient === request?.recipient
    && signedRequest.refundTo === request?.refundTo
    && signedRequest.slippageTolerance === request?.slippageTolerance
    && (request?.deadline === undefined || signedRequest.deadline === request.deadline)
    && (request?.depositMode === undefined || signedRequest.depositMode === request.depositMode);
  if (!matchesRequest) {
    throw providerError("PROVIDER_QUOTE_MISMATCH", "The provider quote does not match the requested swap.");
  }

  let signatureValid = false;
  try {
    signatureValid = typeof verifySignature === "function" && verifySignature(payload);
  } catch {
    signatureValid = false;
  }
  if (!signatureValid) {
    throw providerError("PROVIDER_SIGNATURE_INVALID", "The provider quote signature could not be verified.");
  }

  if (dry) {
    // A dry quote must never be mistaken for an instruction to transfer funds.
    if (
      quote.depositAddress != null
      || quote.depositMemo != null
      || quote.deadline != null
      || quote.timeWhenInactive != null
    ) {
      throw providerError("PROVIDER_UNEXPECTED_DEPOSIT_DATA", "The provider returned unexpected deposit instructions for a dry quote.");
    }
  } else {
    if (typeof quote.depositAddress !== "string" || quote.depositAddress.length < 1 || quote.depositAddress.length > 512) {
      throw providerError("PROVIDER_INVALID_DEPOSIT", "The provider did not return a valid deposit address.");
    }
    if (quote.depositMemo !== undefined && quote.depositMemo !== null && typeof quote.depositMemo !== "string") {
      throw providerError("PROVIDER_INVALID_DEPOSIT", "The provider returned an invalid deposit memo.");
    }
    if (
      typeof quote.deadline !== "string"
      || !Number.isFinite(Date.parse(quote.deadline))
      || Date.parse(quote.deadline) <= now
    ) {
      throw providerError("PROVIDER_INVALID_DEADLINE", "The provider did not return a valid future deposit deadline.");
    }
  }

  const amountOutAtomic = quote.amountOut;
  const minAmountOutAtomic = quote.minAmountOut;
  return {
    id: `qte_${randomUUID()}`,
    mode: dry ? "provider_dry_quote" : "provider_executable_quote",
    provider: "near_1click",
    providerQuoteId: payload.correlationId,
    providerSignature: payload.signature,
    providerTimestamp: payload.timestamp,
    originAssetId: originAsset.id,
    destinationAssetId: destinationAsset.id,
    originAsset: {
      id: originAsset.id,
      symbol: originAsset.symbol,
      chain: originAsset.chain,
      decimals: originAsset.decimals,
      contractAddress: originAsset.contractAddress ?? null
    },
    destinationAsset: {
      id: destinationAsset.id,
      symbol: destinationAsset.symbol,
      chain: destinationAsset.chain,
      decimals: destinationAsset.decimals,
      contractAddress: destinationAsset.contractAddress ?? null
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
    expiresAt: typeof quote.deadline === "string" ? quote.deadline : null,
    deadline: typeof quote.deadline === "string" ? quote.deadline : null,
    timeWhenInactive: typeof quote.timeWhenInactive === "string" ? quote.timeWhenInactive : null,
    timeEstimateSeconds: Number.isFinite(quote.timeEstimate) ? quote.timeEstimate : null,
    depositAddress: dry ? null : quote.depositAddress,
    depositMemo: dry ? null : (quote.depositMemo ?? null),
    refundFeeAtomic: typeof quote.refundFee === "string" && /^\d+$/.test(quote.refundFee) ? quote.refundFee : null,
    withdrawFeeAtomic: typeof quote.withdrawFee === "string" && /^\d+$/.test(quote.withdrawFee) ? quote.withdrawFee : null,
    warning: dry
      ? "Live provider estimate only. This dry quote contains no deposit instructions and cannot be used to send funds."
      : "Executable provider quote. Check the amount, destination, refund address, deposit address, expiry and wallet transaction before signing."
  };
}

export function createNearOneClickProvider({
  baseUrl = DEFAULT_BASE_URL,
  apiKey = "",
  verifySignature = verifyQuoteSignature,
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
    if (authenticated && credential) headers.authorization = `Bearer ${credential}`;

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
      const quoteRequest = {
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
      };
      return validateQuoteResponse(payload, {
        originAsset,
        destinationAsset,
        amountAtomic: String(amountAtomic),
        slippageBps,
        dry: true,
        request: quoteRequest,
        verifySignature,
        now: now()
      });
    },

    async getExecutableQuote({
      originAsset,
      destinationAsset,
      amountAtomic,
      recipient,
      refundTo,
      slippageBps = 100,
      deadline,
      depositMode = "SIMPLE"
    }) {
      if (!credential) {
        throw providerError("PROVIDER_NOT_CONFIGURED", "Live execution requires a server-side 1Click partner JWT.", 503);
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
      if (typeof deadline !== "string" || !Number.isFinite(Date.parse(deadline)) || Date.parse(deadline) <= now()) {
        throw new DomainError("INVALID_DEADLINE", "The requested swap deadline is invalid.");
      }
      if (depositMode !== "SIMPLE" && depositMode !== "MEMO") {
        throw new DomainError("INVALID_DEPOSIT_MODE", "The deposit mode is not supported.");
      }

      const requestBody = {
        dry: false,
        swapType: "EXACT_INPUT",
        slippageTolerance: slippageBps,
        originAsset: originAsset.id,
        depositType: "ORIGIN_CHAIN",
        destinationAsset: destinationAsset.id,
        amount: String(amountAtomic),
        refundTo: refundTo.trim(),
        refundType: "ORIGIN_CHAIN",
        recipient: recipient.trim(),
        recipientType: "DESTINATION_CHAIN",
        deadline,
        depositMode
      };
      const payload = await request("/v0/quote", {
        method: "POST",
        authenticated: true,
        body: requestBody
      });
      return validateQuoteResponse(payload, {
        originAsset,
        destinationAsset,
        amountAtomic: String(amountAtomic),
        slippageBps,
        dry: false,
        request: requestBody,
        verifySignature,
        now: now()
      });
    },

    async getStatus({ depositAddress, depositMemo = null }) {
      if (typeof depositAddress !== "string" || depositAddress.length < 1 || depositAddress.length > 512) {
        throw new DomainError("INVALID_DEPOSIT_ADDRESS", "A valid provider deposit address is required.");
      }
      const query = new URLSearchParams({ depositAddress });
      if (typeof depositMemo === "string" && depositMemo.length > 0) query.set("depositMemo", depositMemo);
      const payload = await request(`/v0/status?${query.toString()}`, { authenticated: true });
      if (
        !payload
        || typeof payload.status !== "string"
        || typeof payload.updatedAt !== "string"
        || !payload.swapDetails
        || typeof payload.swapDetails !== "object"
      ) {
        throw providerError("PROVIDER_INVALID_STATUS", "The swap provider returned an invalid status response.");
      }
      return {
        providerStatus: payload.status,
        updatedAt: payload.updatedAt,
        swapDetails: payload.swapDetails
      };
    },

    async submitDepositTx({ txHash, depositAddress, depositMemo = null }) {
      if (!credential) {
        throw providerError("PROVIDER_NOT_CONFIGURED", "Live execution requires a server-side 1Click partner JWT.", 503);
      }
      if (typeof txHash !== "string" || txHash.length < 1 || txHash.length > 256) {
        throw new DomainError("INVALID_TRANSACTION_HASH", "A valid source transaction hash is required.");
      }
      if (typeof depositAddress !== "string" || depositAddress.length < 1 || depositAddress.length > 512) {
        throw new DomainError("INVALID_DEPOSIT_ADDRESS", "A valid provider deposit address is required.");
      }
      const payload = await request("/v0/deposit/submit", {
        method: "POST",
        authenticated: true,
        body: {
          txHash,
          depositAddress,
          ...(typeof depositMemo === "string" && depositMemo.length > 0 ? { memo: depositMemo } : {})
        }
      });
      if (typeof payload?.status !== "string" || typeof payload?.updatedAt !== "string") {
        throw providerError("PROVIDER_INVALID_STATUS", "The swap provider returned an invalid deposit acknowledgement.");
      }
      return {
        providerStatus: payload.status,
        updatedAt: payload.updatedAt,
        swapDetails: payload.swapDetails && typeof payload.swapDetails === "object" ? payload.swapDetails : {}
      };
    }
  };
}
