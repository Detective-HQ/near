# Universal DeFi Intelligence

A Next.js App Router frontend written in TypeScript/TSX, backed by a Node.js HTTP API. The local prototype includes a responsive landing page and research workspace, source-attributed analytics, a scenario calculator, and two clearly separated swap paths: local fixture simulation and an opt-in NEAR Intents 1Click execution flow for supported EVM-to-EVM pairs. The live path requests dry and executable quotes, asks an injected EVM wallet to sign and broadcast the source transfer, records the transaction hash, and tracks provider status through success, refund, or failure.

Live execution is disabled by default and has not been verified against a real provider route or wallet transaction in this repository. This is not an audited or production-ready custody, exchange, or settlement service. The API persists to a local JSON file and has no user authentication or per-user data isolation. Never use fixture quotes for transfers; never enable live execution on a public deployment without route-specific testing, security review, durable storage, access control, and operational monitoring.

## Run locally

Requirements: Node.js 22 or newer.

```powershell
cd "D:\My Code\Near"
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
npm run dev
```

Open `http://127.0.0.1:3000`. Next.js serves the frontend and proxies API requests to the Node API on port `8081`; `npm run dev` starts both services. Local demo records are written to `data/udi.json`.

For an optimized build, run `npm run typecheck`, `npm test`, `npm run build`, then `npm start`. Production mode disables simulated swap writes. Docker Compose serves the app on port `3000` and persists local prototype records in a named volume; it is not a production deployment template.

## API credentials

- `ONECLICK_JWT` is the only required external credential for live 1Click quotes and executable quotes. It is a server-side partner JWT sent as a bearer credential. Keep it in `.env` or a secret manager; never use a `NEXT_PUBLIC_` variable or commit it.
- DeFiLlama's public endpoints used for optional read-only market metrics require no API key in this integration. Set `MARKET_DATA_MODE=live` to request them.
- No wallet API key is needed. The browser uses an injected EVM wallet for read-only balance checks and, only when the server execution gate is enabled, for user-reviewed transaction signing.
- Hosted RPC credentials are not currently used. They become necessary if a future implementation adds hosted RPC/indexer reads or supports wallets without a usable injected provider.
- Additional routing providers will require their own credentials if they are selected to cover chains/assets outside the current 1Click EVM slice.

To enable the live provider catalogue and dry quotes, set these values in `.env`:

```powershell
$env:SWAP_QUOTE_MODE = "live"
$env:ONECLICK_JWT = "<server-side partner JWT>"
npm start
```

The token catalogue can load without the JWT, but live quote requests cannot. Live execution additionally requires `ENABLE_LIVE_EXECUTION=true`; it remains `false` in `.env.example` and in the default Compose configuration. Enabling it exposes a real-funds EVM wallet flow when a valid provider route and wallet are available. Only enable it in a controlled environment after reviewing the pair, token contract, recipient, refund address, quote expiry, and transaction. No real route has been tested as part of this handoff.

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
- In live quote mode, `GET /api/v1/assets` retrieves and briefly caches the 1Click token catalogue, preserving each provider `assetId`; `POST /api/v1/quotes` requests an authenticated exact-input dry quote.
- With `SWAP_QUOTE_MODE=live`, a configured `ONECLICK_JWT`, and `ENABLE_LIVE_EXECUTION=true`, the first real-transfer slice supports EVM source and destination assets on Ethereum, Base, Arbitrum, Polygon, Avalanche, and BNB Chain when the provider returns a valid route and the asset has verified transfer metadata.
- The live swap flow obtains an executable quote, checks its signed request fields and deadline, builds a native-asset or ERC-20 source transaction in the browser, asks the connected wallet to sign/broadcast, records the EVM transaction hash, optionally notifies 1Click, and polls provider status. The provider's terminal outcome—not the wallet callback—determines success/refund/failure.
- Activity retains source, destination, and refund transaction references separately. Unfunded executable instructions can be discarded; unknown provider statuses are not treated as success.
- In opt-in market-data mode, read-only NEAR TVL history, DEX volume, stablecoin supply, protocol fees, and protocol revenue are fetched from DeFiLlama with per-metric provenance and bounded caching.
- Markets and Intelligence can export the current metric snapshot and series as JSON or CSV, retaining source, timestamps, methodology, and live-versus-fixture status.
- `POST /api/v1/swaps` stores a local demo record and requires an idempotency key.
- `POST /api/v1/swaps/{id}/simulate` advances only through named simulated states. It never creates transaction hashes or provider statuses.
- Atomic local JSON persistence, JSON structured operational logs, request-size limits, simple per-IP rate limiting, same-origin API behavior, and security response headers.
- Sensitivity controls recalculate bear/base/bull annual fee scenarios from visible assumptions and a displayed formula.

## Prototype limits and remaining production work

- Only the EVM wallet transaction path is implemented. Solana, Stellar, Celo, Aptos, and NEAR remain planned or wallet-only; they are not enabled by this flow.
- EVM addresses are format-checked, but the API does not independently verify a submitted source transaction receipt on-chain. The source hash is user-reported; provider status supplies the tracked settlement outcome.
- The local JSON repository and in-process status poller are single-instance prototype infrastructure. PostgreSQL, Redis, a durable worker/queue, migrations, backups, and multi-instance coordination are not implemented.
- There is no user authentication or owner-level access control. Do not expose this API to multiple users or the public while it stores activity in a shared repository.
- Broad token portfolio balances, protocol positions, generalized chain RPC/indexing, a second route provider, production alerting, deployment hardening, and independent security/audit review remain outstanding.
- Demo asset IDs start with `demo:` and are never executable. A provider catalogue entry still does not guarantee a live route; each pair must return a fresh quote.
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
    nearoneclick.mjs     cached catalogue, quote, and provider-status adapter
    defillama.mjs        optional live NEAR public-metrics adapter
  repository.mjs         atomic local JSON persistence
scripts/                  local multi-service runner and checks
test/                     domain, repository, and HTTP API tests
docs/                     architecture, contract, methods, runbook, decisions
```
