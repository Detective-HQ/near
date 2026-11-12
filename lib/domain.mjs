import { randomUUID } from "node:crypto";

export class DomainError extends Error {
  constructor(code, message, status = 400) {
    super(message);
    this.name = "DomainError";
    this.code = code;
    this.status = status;
  }
}

export function parseUnits(value, decimals) {
  if (typeof value !== "string" || value.length > 80 || !/^\d+(?:\.\d+)?$/.test(value)) {
    throw new DomainError("INVALID_AMOUNT", "Enter a positive decimal amount.");
  }
  if (!Number.isInteger(decimals) || decimals < 0 || decimals > 36) {
    throw new DomainError("INVALID_DECIMALS", "Asset precision is invalid.", 500);
  }

  const [whole, fraction = ""] = value.split(".");
  if (fraction.length > decimals) {
    throw new DomainError("TOO_MANY_DECIMALS", `This asset supports at most ${decimals} decimal places.`);
  }

  const atomic = BigInt(whole) * 10n ** BigInt(decimals)
    + BigInt((fraction + "0".repeat(decimals)).slice(0, decimals) || "0");
  if (atomic <= 0n) {
    throw new DomainError("INVALID_AMOUNT", "Amount must be greater than zero.");
  }
  return atomic;
}

export function formatUnits(atomic, decimals, maxFraction = decimals) {
  const value = BigInt(atomic);
  const scale = 10n ** BigInt(decimals);
  const whole = value / scale;
  const fraction = (value % scale).toString().padStart(decimals, "0");
  const trimmed = fraction.slice(0, maxFraction).replace(/0+$/, "");
  return trimmed ? `${whole}.${trimmed}` : whole.toString();
}

export function createQuote({ originAsset, destinationAsset, amountIn, now = Date.now() }) {
  if (!originAsset || !destinationAsset) {
    throw new DomainError("UNKNOWN_ASSET", "Select assets from the configured demo catalogue.");
  }
  if (originAsset.id === destinationAsset.id) {
    throw new DomainError("SAME_ASSET", "Choose two different assets.");
  }

  const inputAtomic = parseUnits(amountIn, originAsset.decimals);
  const inputUsdMicros = inputAtomic * BigInt(originAsset.fixturePriceMicros)
    / 10n ** BigInt(originAsset.decimals);
  const grossOutputAtomic = inputUsdMicros * 10n ** BigInt(destinationAsset.decimals)
    / BigInt(destinationAsset.fixturePriceMicros);
  const outputAtomic = grossOutputAtomic * 997n / 1000n;
  const minimumOutputAtomic = outputAtomic * 995n / 1000n;

  if (outputAtomic <= 0n) {
    throw new DomainError("AMOUNT_TOO_SMALL", "Amount is too small for this demo route.");
  }

  return {
    id: `qte_${randomUUID()}`,
    mode: "simulation",
    provider: "local_fixture",
    originAssetId: originAsset.id,
    destinationAssetId: destinationAsset.id,
    originAsset: { id: originAsset.id, symbol: originAsset.symbol, chain: originAsset.chain, decimals: originAsset.decimals },
    destinationAsset: { id: destinationAsset.id, symbol: destinationAsset.symbol, chain: destinationAsset.chain, decimals: destinationAsset.decimals },
    amountInAtomic: inputAtomic.toString(),
    amountIn: formatUnits(inputAtomic, originAsset.decimals),
    amountOutEstimateAtomic: outputAtomic.toString(),
    amountOutEstimate: formatUnits(outputAtomic, destinationAsset.decimals, 8),
    minimumAmountOutAtomic: minimumOutputAtomic.toString(),
    minimumAmountOut: formatUnits(minimumOutputAtomic, destinationAsset.decimals, 8),
    feeBps: 30,
    slippageBps: 50,
    expiresAt: new Date(now + 120_000).toISOString(),
    route: [originAsset.chain, destinationAsset.chain],
    warning: "Estimate uses fixture prices and a fixed demo fee. It is not a provider quote or an executable transaction."
  };
}

export function createSwap(quote, { now = Date.now(), id = randomUUID() } = {}) {
  if (!quote) throw new DomainError("QUOTE_NOT_FOUND", "Quote was not found.", 404);
  if (Date.parse(quote.expiresAt) <= now) {
    throw new DomainError("QUOTE_EXPIRED", "Quote expired. Request a fresh simulation.");
  }
  return {
    id: `swp_${id}`,
    quoteId: quote.id,
    state: "QUOTE_READY",
    providerStatus: null,
    mode: "simulation",
    realSettlement: false,
    transactions: { source: null, destination: null, refund: null },
    createdAt: new Date(now).toISOString(),
    updatedAt: new Date(now).toISOString(),
    events: [
      { state: "QUOTE_READY", detail: "Local simulation record created; no deposit was requested or broadcast.", at: new Date(now).toISOString() }
    ],
    quote
  };
}

const simulationTransitions = {
  QUOTE_READY: ["SIMULATED_DEPOSIT", "A simulated deposit event was added. No chain transaction exists."],
  SIMULATED_DEPOSIT: ["SIMULATED_PROCESSING", "The local mock advanced to processing. No provider was contacted."],
  SIMULATED_PROCESSING: ["SIMULATION_COMPLETE", "The demo lifecycle ended. This is not a settled swap."]
};

export function advanceSimulation(swap, { now = Date.now() } = {}) {
  if (!swap) throw new DomainError("SWAP_NOT_FOUND", "Simulation record was not found.", 404);
  const transition = simulationTransitions[swap.state];
  if (!transition) throw new DomainError("TERMINAL_STATE", "This simulation has no further demo transitions.");

  const [state, detail] = transition;
  return {
    ...swap,
    state,
    realSettlement: false,
    providerStatus: null,
    updatedAt: new Date(now).toISOString(),
    events: [...swap.events, { state, detail, at: new Date(now).toISOString() }]
  };
}
