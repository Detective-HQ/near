# Architecture

## Current local prototype

The local prototype runs from the repository's locked Node.js dependencies:

```text
Browser
  ├─ Next.js App Router (TypeScript/TSX)
  └─ same-origin /api proxy
       └─ Node.js HTTP API
           ├─ exact decimal conversion and local fixture simulation
           ├─ NEAR Intents 1Click catalogue, dry/executable quote and status adapter
           ├─ browser-injected EVM wallet transaction flow
           ├─ optional DeFiLlama public NEAR metrics
           ├─ in-process live-swap status poller
           └─ atomic JSON repository at data/udi.json
```

The frontend is a TypeScript Next.js application; `scripts/run-services.mjs` starts Next.js and the API as separate local processes. The Next.js route handler proxies same-origin `/api/*` requests to the Node API. `server.mjs` applies response security headers, rejects oversized bodies, limits request rates, validates API inputs, and emits structured logs without request bodies.

The JSON repository is suitable only for a single-instance local prototype. It has no database transactions, multi-instance coordination, owner authorization, schema migration system, or operational backup. Swap records contain transaction and wallet destination references; there is no user authentication or per-user access control.

## Safety boundary

- `lib/data.mjs` labels its catalogue and metrics as demo fixtures. Demo asset identifiers are not provider IDs.
- `lib/domain.mjs` creates estimates from fixture prices and a fixed fee/slippage assumption; it does not fetch routes.
- `lib/providers/nearoneclick.mjs` fetches/caches the provider catalogue, requests dry and executable exact-input quotes, verifies executable quote signatures/request fields, and normalizes provider lifecycle data. The partner JWT stays server-side; response sizes, timeouts, shapes, and upstream errors are bounded or validated.
- `lib/providers/defillama.mjs` optionally fetches NEAR TVL history, DEX volume, stablecoin supply, protocol fees, and protocol revenue. It preserves per-metric source, retrieval time, methodology version, and stale/unavailable states. It does not replace a failed observation with fixture data.
- Live catalogue assets retain exact provider `assetId` values. Catalogue presence does not prove a route; each pair needs a fresh quote.
- Executable quotes require `SWAP_QUOTE_MODE=live`, `ONECLICK_JWT`, and `ENABLE_LIVE_EXECUTION=true`. The first implementation supports EVM-to-EVM routes on Ethereum, Base, Arbitrum, Polygon, Avalanche, and BNB Chain only.
- The browser builds native-asset or ERC-20 source transfers and requests wallet signature/broadcast. It never receives provider credentials or handles a seed phrase/private key. EVM deposit memos are rejected by this path.
- The API records the wallet-reported source transaction hash and asks 1Click for status. It does not independently verify the source receipt on an RPC; provider status controls the tracked success/refund/failure state. Unknown statuses are not success.
- `DEMO_MODE=false` disables demo quote and simulation writes. Production `NODE_ENV` also disables demo writes; it does not enable live execution.
- Simulation states are prefixed with `SIMULATED_` or `SIMULATION_`; `realSettlement` remains false for them.
- The DeFiLlama integration is read-only. It does not provide active-account/transaction indexing, portfolio balances, persisted snapshots, or a live investment model.

## Production boundary

The original product direction calls for a Go API, PostgreSQL, Redis-backed workers, multiple chain-specific wallet clients, provider adapters, and separate analytics connectors. This prototype currently uses Next.js, a Node API, and local JSON persistence. It does not have PostgreSQL, Redis, a durable worker, user auth/ownership, broad-chain wallet integrations, or production monitoring. Split these concerns into independently deployable web, API, worker, and analytics services before production. Keep provider payloads behind adapters and retain the original provider status alongside a versioned internal state mapping.

Suggested production path:

1. Finalize the OpenAPI contract and implement authentication plus strict user ownership for every quote, swap, activity row, and saved research record.
2. Add PostgreSQL migrations and durable queue/worker support for assets, quotes, swaps, transaction hashes, append-only events, observations, and model snapshots.
3. Contract-test the provider adapter against the current official API schema; do not infer asset IDs or route support.
4. Independently verify source-chain receipts and destination/refund transactions; add chain-specific validation and wallet flows only after separate review.
5. Add bounded retries, durable status polling, stale-data alerts, production monitoring, rate-limit policy, and tested backup/restore.
6. Complete threat modeling and security review; verify every route/asset pair before enabling it.

## Design record

The app screen uses the dealt Achromatic ledger style, seed `729774780`: a blue-grey ground, neutral navigation, Geist text, tabular numbers, and a single blue hue on the principal chart. Its key finding is the capability boundary: executable swaps are opt-in and EVM-only; all other chains remain visibly wallet-only, feature-flagged, or planned.
