# API contract

All API responses are JSON. Error bodies have the form:

```json
{
  "error": {
    "code": "INVALID_AMOUNT",
    "message": "Enter a positive decimal amount."
  },
  "requestId": "..."
}
```

The `requestId` response header is set for successful and failed JSON responses. API responses use `Cache-Control: no-store`.

## Read endpoints

| Method and route | Response | Notes |
| --- | --- | --- |
| `GET /api/v1/health` | status, quote mode, market data mode, capability flags, fixture date | `executionEnabled` is true only when live quote mode, a configured partner JWT, and the execution gate are all enabled. Credentials are not exposed. |
| `GET /api/v1/chains` | configured chain entries | EVM entries report swap-enabled only when the server execution gate is active; route support remains pair-specific. |
| `GET /api/v1/assets?chain=near` | fixture assets, or the live provider catalogue | `demo` mode returns fixture IDs; `live` mode returns 1Click `assetId` values unchanged. Catalogue presence does not confirm a route. |
| `GET /api/v1/markets/overview` | fixture overview or live DeFiLlama NEAR snapshot | `?refresh=true` bypasses the live in-memory cache. Live values include source, retrieval time, status, and methodology version. |
| `GET /api/v1/intelligence/near` | metric dictionary and fixture or live TVL series | Live mode returns the same snapshot shape and cache as Markets. Missing values are `null`; no fixture substitution occurs. |
| `GET /api/v1/research/scenarios` | base assumptions and formula | Scenario inputs are illustrative. |
| `GET /api/v1/activity` | saved simulation and local live-prototype swap records | No account ownership filter is implemented. |
| `GET /api/v1/swaps/{id}` | one simulation or live-prototype swap record | Live records retain provider status and separate source, destination, and refund transaction references. |

## Write endpoints

### `POST /api/v1/quotes`

In default `demo` mode, this endpoint returns the local fixture estimate described below. In `live` mode, it makes a server-authenticated **dry quote only** request to the configured 1Click API. The live quote path requires `ONECLICK_JWT` and current provider asset IDs from `GET /api/v1/assets`. The current deposit builder is limited to supported EVM-origin and EVM-destination pairs; the provider must still return a route quote for the selected pair.

Live-mode request:

```json
{
  "originAssetId": "<provider assetId from GET /api/v1/assets>",
  "destinationAssetId": "<provider assetId from GET /api/v1/assets>",
  "amountIn": "1.25",
  "recipient": "0x<40 hex characters>",
  "refundTo": "0x<40 hex characters>"
}
```

The API validates EVM address shape, converts `amountIn` to atomic units using the provider catalogue decimals, and sends `dry: true`, `swapType: EXACT_INPUT`, and the recipient/refund addresses to the provider. The returned dry quote is stored for the local prototype and has `mode: provider_dry_quote`; it has no deposit address or executable transaction. Provider catalogue presence does not guarantee a route.

Demo-mode request:

```json
{
  "originAssetId": "demo:near:near",
  "destinationAssetId": "demo:base:usdc",
  "amountIn": "10"
}
```

The amount is a base-10 decimal string, never a floating-point JSON number. The demo service rejects non-positive values, exponent notation, unknown demo IDs, identical asset IDs, and precision beyond the source asset's fixture decimals.

Demo response status is `201`. The quote includes integer atomic amounts as strings, a formatted estimate, a minimum display estimate, fixture mode, fixed demo assumptions, a two-minute local expiry, and a warning. It contains no deposit address, memo, refund instruction, provider quote ID, or executable transaction. Live dry-quote response status is `200`; creating a swap requires a separate fresh executable quote.

Provider rate limits, unavailable responses, invalid JSON, unexpected quote shapes, and timeout conditions return typed, sanitized errors; upstream response bodies and credentials are not returned.

## Live market data

`MARKET_DATA_MODE=live` enables server-side, read-only requests to DeFiLlama's public chain TVL, DEX overview, fees/revenue overview, and stablecoin-chain endpoints. The snapshot includes NEAR TVL, up to 90 recent TVL observations, 24-hour DEX volume, stablecoin supply, protocol fees, and protocol revenue. Each metric carries `source`, `sourceUrl`, `retrievedAt`, `observedAt` when supplied, `status`, and `methodologyVersion`. A source error produces a `null` value or preserves a last-good value marked `stale`; fixture values are never substituted. These API responses are not immutable research snapshots and are not sufficient by themselves for a formal investment thesis.

Markets and Intelligence provide client-side JSON and CSV downloads of the current snapshot. JSON retains the complete response, including the series and provenance. CSV emits metric and series records with source and snapshot status; text cells are escaped to reduce spreadsheet formula-injection risk. Downloads are user-triggered and are not stored by the service. Fixture snapshots remain explicitly marked as illustrative.

### `POST /api/v1/swaps`

Requires an `Idempotency-Key` header from 8 to 128 characters. In demo mode, the JSON body is:

```json
{ "quoteId": "qte_..." }
```

Demo mode returns `201` when a `QUOTE_READY` simulation record is created and `200` when the same idempotency key is replayed. Expired quotes are rejected.

In live mode, the body must contain `{ "quoteId": "qte_...", "recipient": "0x...", "refundTo": "0x..." }`. The server requires a live dry quote, validates that both assets are on the implemented EVM chains, rechecks their provider catalogue metadata, requests and verifies a fresh executable quote, then persists the provider deposit address/memo, deadline, and normalized quote. Arbitrary deposit-address overrides are not accepted. The response is `201` with `AWAITING_USER_DEPOSIT`, or `200` for an idempotent replay. Live creation returns `503` unless `SWAP_QUOTE_MODE=live`, `ONECLICK_JWT` is configured, and `ENABLE_LIVE_EXECUTION=true`.

The frontend currently builds native-asset or ERC-20 transfer transactions on the supported EVM source chain and asks the injected wallet to sign and broadcast. Review the complete transaction in the wallet. An executable quote may result in irreversible loss if sent on the wrong network or to the wrong address; this prototype has not been audited.

### `POST /api/v1/swaps/{id}/deposit`

Records a wallet-reported EVM transaction. The body requires `txHash` (a 32-byte `0x` hash) and `chainId` (the quoted source chain's decimal chain ID). The API rejects a wrong network, a conflicting hash, an expired unfunded quote, and a hash already attached to another swap. The first accepted report returns `202`; an identical repeat returns `200` with `duplicate: true`.

The source hash is not independently verified against an RPC. The server persists it, attempts `/v0/deposit/submit` as an accelerator, and polls provider status by the quote's deposit address/memo. A failed submit notification does not erase the hash; provider polling remains the authority for the tracked swap outcome. A `202` response is not settlement success.

### `POST /api/v1/swaps/{id}/cancel`

Discards an unfunded live quote instruction. It does not cancel an on-chain transaction. It is accepted only while the swap is awaiting a user deposit and no source hash is recorded. The result is `CANCELLED`, or `EXPIRED` if the provider deadline has passed.

### Provider lifecycle

The in-process poller maps known provider statuses to internal states:

```text
PENDING_DEPOSIT -> AWAITING_USER_DEPOSIT
KNOWN_DEPOSIT_TX -> DEPOSIT_SUBMITTED
INCOMPLETE_DEPOSIT -> INCOMPLETE_DEPOSIT
PROCESSING -> PROCESSING
SUCCESS -> SUCCEEDED
REFUNDED -> REFUNDED
FAILED -> FAILED
```

Unknown values are retained as `UNKNOWN_PROVIDER_STATUS`; they are never treated as success. Known states do not regress. Only a provider `SUCCESS` status marks `realSettlement: true`. Source, destination, and refund transaction hashes are stored separately. Polling runs in the API process and is not a durable worker.

### `POST /api/v1/swaps/{id}/simulate`

Advances exactly one local state:

```text
QUOTE_READY -> SIMULATED_DEPOSIT
SIMULATED_DEPOSIT -> SIMULATED_PROCESSING
SIMULATED_PROCESSING -> SIMULATION_COMPLETE
```

This is a demo control, not a deposit notification or provider status update. It is disabled when `DEMO_MODE=false`, `NODE_ENV=production`, or `SWAP_QUOTE_MODE=live`.

The local JSON repository has no user authentication or record ownership checks. Do not expose it as a multi-user service.

## Current error codes

Errors are typed and sanitized. Important swap codes include `INVALID_JSON`, `BODY_TOO_LARGE`, `INVALID_AMOUNT`, `UNKNOWN_ASSET`, `QUOTE_EXPIRED`, `QUOTE_NOT_FOUND`, `IDEMPOTENCY_KEY_REQUIRED`, `LIVE_EXECUTION_DISABLED`, `CHAIN_EXECUTION_UNAVAILABLE`, `SOURCE_TRANSFER_UNAVAILABLE`, `INVALID_RECIPIENT`, `INVALID_REFUND_ADDRESS`, `WRONG_NETWORK`, `INVALID_TRANSACTION_HASH`, `TRANSACTION_ALREADY_RECORDED`, `DEPOSIT_ALREADY_RECORDED`, `INVALID_SWAP_STATE`, `PROVIDER_NOT_CONFIGURED`, `PROVIDER_AUTH_FAILED`, `PROVIDER_RATE_LIMITED`, `PROVIDER_UNAVAILABLE`, `PROVIDER_TIMEOUT`, `PROVIDER_INVALID_RESPONSE`, `PROVIDER_INVALID_CATALOGUE`, `PROVIDER_INVALID_QUOTE`, `PROVIDER_QUOTE_MISMATCH`, `PROVIDER_SIGNATURE_INVALID`, `PROVIDER_INVALID_DEADLINE`, `PROVIDER_UNEXPECTED_DEPOSIT_DATA`, `RATE_LIMITED`, and `NOT_FOUND`.
