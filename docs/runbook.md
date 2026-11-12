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
| `ONECLICK_API_KEY` | empty | Server-side provider credential; required for live quote requests and never returned by the API. |
| `ONECLICK_TIMEOUT_MS` | `10000` | Per-request provider timeout. |
| `ASSET_CATALOGUE_TTL_SECONDS` | `300` | In-memory live catalogue cache TTL. |
| `LOG_LEVEL` | `info` | Reserved for future log filtering; currently emits info and errors. |

To run in live quote mode, set `SWAP_QUOTE_MODE=live` and the server-side `ONECLICK_API_KEY` in `.env`. `GET /api/v1/assets` may load the provider catalogue without a key. Live quotes are dry-only; this application cannot turn a quote into a swap.

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

Operational logs are one JSON object per line on stdout/stderr. They include a request ID and simulation IDs, but not request bodies, wallet signatures, API keys, or addresses. This prototype has no wallet data.

Simulation records are persisted in `data/udi.json`. To clear local demo history, stop the process, make a backup if the records are needed, then remove only that file. Do not use this local file as a production audit record.

## Docker

```powershell
docker compose up --build
```

The Compose configuration is a local demo setup. It exposes the Next.js app on port 3000 and stores API records in a named volume. It is not a production deployment template.

## Incident boundary

If someone attempts to use the prototype with real funds, stop the flow: even in live quote mode there is no executable quote or safe deposit address in this application. Do not infer route support from the chain list, token catalogue, quote result, or a simulation state. A real deployment requires reviewed chain-specific wallet transaction construction, address validation, route-specific testing, status tracking, settlement verification, and security review.
