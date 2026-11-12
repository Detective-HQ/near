# Universal DeFi Intelligence

A Next.js App Router frontend written in TypeScript/TSX, backed by a Node.js HTTP API. It includes a responsive landing page and research workspace, exact decimal quote arithmetic, a persisted mock lifecycle, metric provenance fields, and a reproducible scenario calculator. An opt-in NEAR Intents 1Click adapter can load its current token catalogue and request provider dry quotes.

This is not a production DeFi application. Live quote mode is strictly dry-only: it does not request or return deposit instructions, build wallet transactions, sign, broadcast, or verify settlement. Fixture quotes are not executable and must never be used to send funds. Market data can optionally use public DeFiLlama endpoints; wallet portfolio balances and scenario assumptions remain fixtures/unavailable.

## Run locally

Requirements: Node.js 22 or newer.

```powershell
cd "D:\My Code\Near"
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
npm run dev
```

Open `http://127.0.0.1:3000`. Next.js serves the frontend and proxies API requests to the Node API on port `8081`; `npm run dev` starts both services. Local demo records are written to `data/udi.json`.

For an optimized build, run `npm run typecheck`, `npm test`, `npm run build`, then `npm start`. Production mode disables simulated swap writes. The Docker image and Compose setup also serve the app on port `3000`; production simulation writes are disabled there as well.

## API credentials

- `ONECLICK_API_KEY` is optional in demo mode. In this implementation, it holds the NEAR Intents partner JWT required for live 1Click dry quotes; the Node backend sends it in `X-API-Key`. Never expose it with a `NEXT_PUBLIC_` variable or commit it.
- DeFiLlama’s public endpoints in the current integration require no API key. Set `MARKET_DATA_MODE=live` to opt into live read-only metrics.
- No wallet API key is needed. The portfolio screen can connect to an injected EVM wallet for read-only native-balance checks; it does not sign or broadcast transactions.
- Hosted RPC credentials are not used; they would only be needed if a future feature adds a hosted RPC or indexer.

To enable the live provider catalogue and dry quotes, set these values in `.env`:

```powershell
$env:SWAP_QUOTE_MODE = "live"
$env:ONECLICK_API_KEY = "<server-side key>"
npm start
```

Keep the key in a server-side secret store; never put it in frontend code or commit it. The token catalogue may load without the key, but quote requests return a configuration error until a key is set. Live-mode recipient and refund addresses are sent to the provider for quote generation, but this prototype does not validate them with chain-specific wallet adapters. A live quote is not permission or instructions to send funds.

To enable read-only NEAR market metrics, set `MARKET_DATA_MODE=live`. This uses public DeFiLlama APIs and requires outbound network access, but no API key. Metrics are cached in memory, include source/retrieval metadata, and remain separate from the illustrative scenario assumptions. A failed source is shown as unavailable or stale; fixtures are not substituted.

To run the checks:

```powershell
npm run typecheck
npm test
npm run check
npm run build
```

`npm run check` performs strict TypeScript checking, JavaScript syntax checks, and the test suite.

If Docker Compose is installed, `docker compose up --build` starts the production-configured app. It persists API records in the `udi-data` named volume.

## What works

- The public landing page has a source-attributed NEAR metric selector and links into the workspace; it labels live data separately from illustrative fixtures.
- Overview, swap sandbox, activity, portfolio, markets, NEAR intelligence, scenario research, and risk screens.
- Portfolio can read an injected EVM wallet's native balance without signing; manual refresh keeps the last reading visible and labels it if the next lookup fails.
- `GET /api/v1/chains`, `GET /api/v1/assets`, `GET /api/v1/markets/overview`, `GET /api/v1/intelligence/near`, `GET /api/v1/research/scenarios`, and `GET /api/v1/activity`.
- In default demo mode, `POST /api/v1/quotes` calculates an illustrative exact-input output from local fixture prices using integer arithmetic.
- In opt-in live mode, `GET /api/v1/assets` retrieves and briefly caches the 1Click token catalogue, preserving each `assetId`; `POST /api/v1/quotes` requests an authenticated exact-input dry quote.
- In opt-in market-data mode, read-only NEAR TVL history, DEX volume, stablecoin supply, protocol fees, and protocol revenue are fetched from DeFiLlama with per-metric provenance and bounded caching.
- Markets and Intelligence can export the current metric snapshot and series as JSON or CSV, retaining source, timestamps, methodology, and live-versus-fixture status.
- `POST /api/v1/swaps` stores a local demo record and requires an idempotency key.
- `POST /api/v1/swaps/{id}/simulate` advances only through named simulated states. It never creates transaction hashes or provider statuses.
- Atomic local JSON persistence, JSON structured operational logs, request-size limits, simple per-IP rate limiting, same-origin API behavior, and security response headers.
- Sensitivity controls recalculate bear/base/bull annual fee scenarios from visible assumptions and a displayed formula.

## What is not connected

- No executable provider quote, deposit instructions, wallet transaction builder, signing, broadcast, refund workflow, provider status polling, or settlement verification.
- No wallet adapter or chain-specific address validation, transaction construction, user signature, or chain broadcast.
- No RPC or indexer for active accounts/transactions, wallet balance source, PostgreSQL, Redis, worker, authentication, or owner-level access control.
- Demo asset IDs start with `demo:`. In live quote mode the current provider asset IDs are returned unchanged, but chain entries still have `walletSupported: false` and `swapEnabled: false`; catalogue presence does not guarantee a route.
- Default market and NEAR metrics are illustrative fixtures. In `MARKET_DATA_MODE=live`, only explicitly sourced DeFiLlama metrics are live; active accounts, transactions, portfolio balances, and scenario inputs remain unavailable or illustrative.

The API disables demo quote and simulation writes when `NODE_ENV=production`. Do not deploy this prototype as a production execution service. See [the architecture](docs/architecture.md), [the API contract](docs/api-contract.md), and [the runbook](docs/runbook.md).

## Repository map

```text
src/
  app/                  Next.js App Router pages, layouts, and API proxy
  components/           Typed landing and workspace React components
  lib/                  API contracts/client, formatting, wallet, and exports
lib/
  data.mjs               fixture catalogue, metrics, and scenario inputs
  domain.mjs             exact decimal math and simulation state machine
  providers/
    nearoneclick.mjs     cached catalogue and dry-quote adapter
    defillama.mjs        optional live NEAR public-metrics adapter
  repository.mjs         atomic local JSON persistence
scripts/                  local multi-service runner
test/                     domain, repository, and HTTP API tests
docs/                     architecture, contract, methods, runbook, decisions
```
