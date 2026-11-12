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
| `GET /api/v1/health` | status, quote mode, market data mode, capability flags, fixture date | `executionEnabled` is always false. Credentials are not exposed. |
| `GET /api/v1/chains` | configured chain entries | Every entry reports `walletSupported: false` and `swapEnabled: false`. |
| `GET /api/v1/assets?chain=near` | fixture assets, or the live provider catalogue | `demo` mode returns fixture IDs; `live` mode returns 1Click `assetId` values unchanged. Catalogue presence does not confirm a route. |
| `GET /api/v1/markets/overview` | fixture overview or live DeFiLlama NEAR snapshot | `?refresh=true` bypasses the live in-memory cache. Live values include source, retrieval time, status, and methodology version. |
| `GET /api/v1/intelligence/near` | metric dictionary and fixture or live TVL series | Live mode returns the same snapshot shape and cache as Markets. Missing values are `null`; no fixture substitution occurs. |
| `GET /api/v1/research/scenarios` | base assumptions and formula | Scenario inputs are illustrative. |
| `GET /api/v1/activity` | saved local simulation records | No real chain activity is included. |
| `GET /api/v1/swaps/{id}` | one local simulation record | Transaction references remain null. |

## Write endpoints

### `POST /api/v1/quotes`

In default `demo` mode, this endpoint returns the local fixture estimate described below. In `live` mode, it makes a server-authenticated **dry quote only** request to the configured 1Click API. The live path requires `ONECLICK_API_KEY` and current provider asset IDs from `GET /api/v1/assets`.

Live-mode request:

```json
{
  "originAssetId": "nep141:wrap.near",
  "destinationAssetId": "nep141:usdc.near",
  "amountIn": "1.25",
  "recipient": "destination.near",
  "refundTo": "refund.near"
}
```

The API converts `amountIn` to atomic units using the catalogue decimals and sends `dry: true`, `swapType: EXACT_INPUT`, and the recipient/refund addresses to the provider. The live quote is not stored in the local simulation repository. The response has `mode: provider_dry_quote`, provider output/minimum amounts where returned, and no expiry or deposit instructions. A dry quote does not create a usable deposit address. Address format validation and transaction construction are not implemented.

Demo-mode request:

```json
{
  "originAssetId": "demo:near:near",
  "destinationAssetId": "demo:base:usdc",
  "amountIn": "10"
}
```

The amount is a base-10 decimal string, never a floating-point JSON number. The demo service rejects non-positive values, exponent notation, unknown demo IDs, identical asset IDs, and precision beyond the source asset's fixture decimals.

Demo response status is `201`. The quote includes integer atomic amounts as strings, a formatted estimate, a minimum display estimate, fixture mode, fixed demo assumptions, a two-minute local expiry, and a warning. It contains no deposit address, memo, refund instruction, provider quote ID, or executable transaction. Live response status is `200` and remains dry-only.

Provider rate limits, unavailable responses, invalid JSON, unexpected quote shapes, and timeout conditions return typed, sanitized errors; upstream response bodies and credentials are not returned.

## Live market data

`MARKET_DATA_MODE=live` enables server-side, read-only requests to DeFiLlama's public chain TVL, DEX overview, fees/revenue overview, and stablecoin-chain endpoints. The snapshot includes NEAR TVL, up to 90 recent TVL observations, 24-hour DEX volume, stablecoin supply, protocol fees, and protocol revenue. Each metric carries `source`, `sourceUrl`, `retrievedAt`, `observedAt` when supplied, `status`, and `methodologyVersion`. A source error produces a `null` value or preserves a last-good value marked `stale`; fixture values are never substituted. These API responses are not immutable research snapshots and are not sufficient by themselves for a formal investment thesis.

Markets and Intelligence provide client-side JSON and CSV downloads of the current snapshot. JSON retains the complete response, including the series and provenance. CSV emits metric and series records with source and snapshot status; text cells are escaped to reduce spreadsheet formula-injection risk. Downloads are user-triggered and are not stored by the service. Fixture snapshots remain explicitly marked as illustrative.

### `POST /api/v1/swaps`

Requires an `Idempotency-Key` header from 8 to 128 characters and a JSON body:

```json
{ "quoteId": "qte_..." }
```

Response status is `201` when a record is created and `200` when the same idempotency key is replayed. It creates a `QUOTE_READY` simulation record. An expired quote is rejected. The request does not accept a deposit destination override.

In `live` mode, this route fails closed with `LIVE_EXECUTION_UNAVAILABLE`.

### `POST /api/v1/swaps/{id}/simulate`

Advances exactly one local state:

```text
QUOTE_READY -> SIMULATED_DEPOSIT
SIMULATED_DEPOSIT -> SIMULATED_PROCESSING
SIMULATED_PROCESSING -> SIMULATION_COMPLETE
```

This is a demo control, not a deposit notification or provider status update. It is disabled when `DEMO_MODE=false`, `NODE_ENV=production`, or `SWAP_QUOTE_MODE=live`.

## Current error codes

`INVALID_JSON`, `BODY_TOO_LARGE`, `INVALID_AMOUNT`, `INVALID_DECIMALS`, `TOO_MANY_DECIMALS`, `AMOUNT_TOO_SMALL`, `UNKNOWN_ASSET`, `SAME_ASSET`, `QUOTE_EXPIRED`, `QUOTE_NOT_FOUND`, `SWAP_NOT_FOUND`, `IDEMPOTENCY_KEY_REQUIRED`, `TERMINAL_STATE`, `DEMO_DISABLED`, `LIVE_EXECUTION_UNAVAILABLE`, `PROVIDER_NOT_CONFIGURED`, `PROVIDER_AUTH_FAILED`, `PROVIDER_RATE_LIMITED`, `PROVIDER_UNAVAILABLE`, `PROVIDER_TIMEOUT`, `PROVIDER_NETWORK_ERROR`, `PROVIDER_INVALID_RESPONSE`, `PROVIDER_INVALID_CATALOGUE`, `PROVIDER_INVALID_QUOTE`, `PROVIDER_UNEXPECTED_DEPOSIT_DATA`, `PROVIDER_RESPONSE_TOO_LARGE`, `RATE_LIMITED`, and `NOT_FOUND`.
