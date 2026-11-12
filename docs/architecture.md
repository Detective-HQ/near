# Architecture

## Current local prototype

The current application is intentionally small enough to run in the provided environment without installing packages:

```text
Browser
  └─ same-origin JSON and static assets
          └─ Node 22 HTTP API
              ├─ domain rules: exact decimal conversion, fixture quote, demo states
              ├─ local fixture catalogue and metric observations
              ├─ optional NEAR Intents 1Click token catalogue and dry-quote adapter
              ├─ optional DeFiLlama public NEAR metrics adapter
              └─ atomic JSON repository at data/udi.json
```

The UI is a static JavaScript application. The API and UI share a single process, so the browser does not need a separately configured API URL. `server.mjs` serves only files within `public/`, applies response security headers, rejects oversized bodies, limits request rates, validates payloads in the domain layer, and emits structured logs without request bodies.

The persistence file is suitable only for a single-process local prototype. It has no database transactions, multi-instance coordination, owner authorization, schema migration system, or operational backup. No users or wallet addresses are stored.

## Safety boundary

- `lib/data.mjs` labels its catalogue and metrics as demo fixtures. Demo asset identifiers are not provider IDs.
- `lib/domain.mjs` creates estimates from fixture prices and a fixed fee/slippage assumption; it does not fetch routes.
- `lib/providers/nearoneclick.mjs` optionally fetches/caches the provider catalogue and requests only `dry: true` exact-input quotes. The API key stays server-side; response sizes, timeouts, shapes, and upstream errors are bounded or validated.
- `lib/providers/defillama.mjs` optionally fetches NEAR TVL history, DEX volume, stablecoin supply, protocol fees, and protocol revenue. It preserves per-metric source, retrieval time, methodology version, and stale/unavailable states. It does not replace a failed observation with fixture data.
- Live catalogue assets retain exact provider `assetId` values, but `walletSupported` and `swapEnabled` remain false. Token availability does not prove route availability.
- Live dry quotes are not persisted as simulation swaps and contain no deposit address. Swap creation and simulation advance routes fail closed in live mode.
- Simulation states are prefixed with `SIMULATED_` or `SIMULATION_`. `realSettlement` remains false, provider status is null, and transaction references remain null.
- `DEMO_MODE=false` disables demo quote and simulation writes. Production `NODE_ENV` also disables demo writes; it does not enable execution.
- The browser contains no private key, seed phrase, signing, transaction construction, or broadcast code.
- The DeFiLlama integration is read-only. It does not provide active-account/transaction indexing, portfolio balances, persisted snapshots, or a live investment model.

## Intended production boundary

The original product direction calls for a Go API, PostgreSQL, Redis-backed workers, chain-specific wallet clients, provider adapters, and separate analytics connectors. Those components are not present in this prototype. Before production, split the code into independently deployable web, API, worker, and analytics services. Keep provider payloads behind adapters and retain the original provider status alongside a versioned internal state mapping.

Suggested production path:

1. Define and review an OpenAPI contract and ownership/authentication model.
2. Add PostgreSQL migrations for assets, quotes, swaps, transactions, append-only events, observations, and saved research inputs.
3. Extend provider adapter contract tests against the current official API schema; do not infer asset IDs.
4. Add chain-specific wallet transaction construction and destination-specific address validation.
5. Add bounded status polling, idempotent event ingestion, stale-data alerts, and operational dashboards.
6. Complete security review and independently verify route availability before enabling any chain/asset pair.

## Design record

The app screen uses the dealt Achromatic ledger style, seed `729774780`: a blue-grey ground, neutral navigation, Geist text, tabular numbers, and a single blue hue on the principal chart. The screen's main finding is the capability boundary: every chain is configured as not wallet-supported and not swap-enabled.
