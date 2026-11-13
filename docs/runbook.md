# Local runbook

## Start

Requirements: Node.js 22 or newer.

```powershell
cd "D:\My Code\Near"
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
npm run dev
```

Open the Next.js frontend at `http://127.0.0.1:3000`. `npm run dev` starts the frontend and API together. The frontend proxies `/api/*` requests to the Node API at `http://127.0.0.1:8081`; local API records are written to `data/udi.json`.

The npm scripts load `.env` when present. Optional settings:

| Variable | Default | Purpose |
| --- | --- | --- |
| `API_HOST` | `127.0.0.1` | API bind address. |
| `API_PORT` | `8081` | Direct API port. |
| `NEXT_HOST` | `127.0.0.1` | Next.js bind address. |
| `NEXT_PORT` | `3000` | Frontend port. |
| `UDI_API_ORIGIN` | `http://127.0.0.1:8081` | API origin used by the Next.js proxy. |
| `DATA_FILE` | `./data/udi.json` | Local prototype persistence file. |
| `DEMO_MODE` | `true` | Set to `false` to disable demo write endpoints. |
| `SWAP_QUOTE_MODE` | `demo` | `demo` uses local fixtures; `live` uses the 1Click catalogue and dry quote API only. |
| `MARKET_DATA_MODE` | `fixture` | `fixture` uses local sample data; `live` fetches read-only NEAR analytics from public DeFiLlama APIs. |
| `MARKET_DATA_TTL_SECONDS` | `300` | In-memory live analytics cache lifetime. |
| `MARKET_DATA_TIMEOUT_MS` | `10000` | Per-request analytics source timeout. |
| `ONECLICK_BASE_URL` | `https://1click.chaindefuser.com` | Provider API base URL. |
| `ONECLICK_JWT` | empty | Server-side NEAR Intents partner JWT; required for live quote requests and never returned by the API. |
| `ONECLICK_TIMEOUT_MS` | `10000` | Per-request provider timeout. |
| `ASSET_CATALOGUE_TTL_SECONDS` | `300` | In-memory live catalogue cache TTL. |
| `ONECLICK_QUOTE_DEADLINE_SECONDS` | `180` | Requested executable quote lifetime; server clamps it to 60–600 seconds. |
| `ONECLICK_QUOTE_PUBLIC_KEY` | empty | Base58 Ed25519 key used to verify quote signatures (the `ed25519:` prefix is accepted). When empty, the backend still verifies the echoed request fields but reports `signatureVerified=false`. |
| `SWAP_STATUS_POLL_SECONDS` | `5` | In-process status polling interval; server clamps it to 1–60 seconds. |
| `ENABLE_LIVE_EXECUTION` | `false` | Explicit server gate for executable quotes and live EVM deposit tracking. |
| `LOG_LEVEL` | `info` | Reserved for future log filtering; currently emits info and errors. |

To request the live provider catalogue and dry quotes, set `SWAP_QUOTE_MODE=live` and configure `ONECLICK_JWT` in `.env`. The catalogue may load without the JWT; quote requests will not. These settings alone do not enable deposits.

The backend verifies quote signatures against `ONECLICK_QUOTE_PUBLIC_KEY`. If the key is not configured, it validates the echoed request fields but never claims the signature was verified (`signatureVerified=false`). Set the key in production so a tampered quote fails closed with `PROVIDER_SIGNATURE_INVALID`.

The worker daemon (`backend/cmd/worker`) uses the same 1Click adapter. Without `ONECLICK_JWT` it can still refresh the catalogue, but swap status polling pauses with `PROVIDER_NOT_CONFIGURED` instead of failing swaps. The fallback provider entry is a stub that returns `NO_ROUTE` until a second provider is integrated.

The EVM deposit path requires all three conditions: `SWAP_QUOTE_MODE=live`, a configured `ONECLICK_JWT`, and `ENABLE_LIVE_EXECUTION=true`. Keep the execution gate false by default. When enabled, the app can ask an injected EVM wallet to broadcast a real source-chain transaction for a provider-issued executable quote. Confirm the exact route, asset contract, amount, recipient, refund address, expiry, and wallet transaction. The automated lifecycle tests use a mock provider; no live route or real-fund transaction has been verified in this repository.

To run with live market observations, configure `MARKET_DATA_MODE=live`. The service makes read-only requests to DeFiLlama and its stablecoin API. It requires outbound network access; no credentials are needed. Open Markets and use “Refresh data” to bypass the in-memory cache. Each metric reports its source and retrieval time; missing data remains a dash, and stale observations are labelled.

## Verify

```powershell
npm test
npm run check
Invoke-RestMethod http://127.0.0.1:3000/api/v1/health
Invoke-RestMethod http://127.0.0.1:3000/api/v1/chains
```

For direct API access, use port `8081`. The health route reports local process status only; it does not check a provider, database, indexer, wallet, or chain.

## Logs and data

Operational logs are one JSON object per line on stdout/stderr. They include request and swap IDs and the source transaction hash, but not request bodies, wallet signatures, API keys, or wallet addresses.

Simulation and live-prototype records are persisted in `data/udi.json`. Back up the file before clearing local history if the records are needed. Do not use this file as a production audit record. The API has no authentication or owner-level access control; all local users share one repository.

## Docker

```powershell
docker compose up --build
```

The Compose configuration exposes the Next.js app on port 3000 and stores API records in a named volume. It is a local prototype setup, not a production deployment template.

## Incident boundary

If an unexpected asset, route, destination, provider status, or wallet transaction appears, stop before signing or sending. Do not infer route support from the chain list, token catalogue, or an earlier quote; request and review a fresh quote for each pair. A wallet broadcast only records the source transaction and is not settlement. Disable `ENABLE_LIVE_EXECUTION` if provider behavior is uncertain. Before public or production deployment, add authentication and data ownership, durable storage and worker coordination, independent source-chain receipt verification, route-specific tests, operational alerts, and security review.
