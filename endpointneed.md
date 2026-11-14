# Endpoint and integration work: implementation prompt

> Use this file as the task brief for a developer or an AI coding agent. It lists every endpoint and flow still missing between the Next.js frontend and the Go backend, with the contract to build, where the code goes, and how to verify it.

---

## Prompt

You are working in the `near` monorepo:

- **Backend:** Go module in `backend/`. Routes are registered in `backend/internal/httpapi/server.go` → `Routes()`. Handlers live in `backend/internal/httpapi/*.go` and business logic in `backend/internal/*`.
- **Frontend:** Next.js in `frontend/`. The browser only calls `/api/*`. The catch-all proxy `frontend/src/app/api/[...path]/route.ts` forwards to the Go API, but only for routes in its allowlist and only for query keys in its forward list.
- **Client:** every UI call goes through `frontend/src/lib/api-client.ts`. Types live in `frontend/src/lib/contracts.ts` and `frontend/src/lib/trading-contracts.ts`.
- **API docs:** `docs/openapi.yaml`. `TestOpenAPIMatchesRoutes` fails if a route is registered but not documented, or the reverse.

Implement the items below. Rules for every item:

1. **Do not delete or rename any existing endpoint.** Add new ones and extend responses only with new optional fields.
2. For a new route, wire all of these layers:
   - the Go handler and its entry in `Routes()`,
   - `docs/openapi.yaml`,
   - the proxy allowlist (and the forward list for any new query keys),
   - an `api-client.ts` method and the TypeScript types,
   - the UI that uses it.
3. **Never present invented data as real.** When data isn't available, return `null` with `quality: "unavailable"`, or a typed error. Mock data is allowed only behind `NEXT_PUBLIC_USE_MOCKS=true` with a TEST DATA tag.
4. **Errors use the envelope** `{ "error": { "code", "message" }, "requestId" }`. Reuse codes from `backend/internal/swap/domain/errors.go`, and add new codes there with an HTTP status.
5. **Money amounts are integer strings** in the smallest unit (`*big.Int` in Go). Never use floats for amounts.
6. **Ownership:** anything tied to a wallet uses `middleware.Principal(r)` and must not leak other wallets' data.
7. **Tests:**
   - Go unit and handler tests for every change.
   - Frontend `npm test` and `npx tsc --noEmit`.
   - `go test -race ./...` in CI.
   - Keep `go mod tidy -diff` clean.
8. Finish each item by running `scripts/smoke.sh` against a local API and calling the new route through the proxy (`http://127.0.0.1:3000/api/...`).

---

## 1. Current inventory (all connected and working)

| Method | Path | Backend | Proxy | Client method | Used by UI |
|---|---|---|---|---|---|
| GET | `/api/v1/health` | ✅ | ✅ | `health` | ⚠️ only a warm-up ping in the hero |
| GET | `/api/v1/chains` | ✅ | ✅ | `chains` | ✅ |
| GET | `/api/v1/assets` | ✅ | ✅ | `assets` | ✅ |
| GET | `/api/v1/markets/overview` | ✅ | ✅ | `markets` | ✅ |
| GET | `/api/v1/intelligence/near` | ✅ | ✅ | `intelligence` | ✅ |
| GET | `/api/v1/research/scenarios` | ✅ | ✅ | `research` | ⚠️ always treated as test data |
| GET | `/api/v1/risk/status` | ✅ | ✅ | `risk` | ✅ |
| GET | `/api/v1/portfolio` | ✅ real on-chain | ✅ | `portfolio` | ✅ |
| GET | `/api/v1/trading/overview` | ✅ | ✅ | `tradingOverview` | ✅ |
| GET | `/api/v1/trading/signals` | ✅ | ✅ | `tradingSignals` | ✅ |
| GET | `/api/v1/trading/top-movers` | ✅ | ✅ | `tradingTopMovers` | ❌ no UI |
| GET | `/api/v1/trading/history` | ✅ | ✅ | `tradingHistory` | ✅ |
| GET | `/api/v1/trading/backtests` | ✅ | ✅ | `tradingBacktests` | ✅ |
| GET | `/api/v1/trading/assets/{id}` | ✅ | ✅ | `tradingAsset` | ✅ |
| GET | `/api/v1/trading/orderblocks/{id}` | ✅ | ✅ | `tradingStructure` | ✅ |
| GET | `/api/v1/trading/liquidity/{id}` | ✅ | ✅ | `tradingLiquidity` | ✅ |
| GET | `/api/v1/trading/scenarios/{id}` | ✅ | ✅ | `tradingScenario` | ✅ |
| POST | `/api/v1/auth/nonce` | ✅ | ✅ | `authNonce` | ✅ wallet menu |
| POST | `/api/v1/auth/verify` | ✅ | ✅ | `authVerify` | ✅ wallet menu (EVM only) |
| GET | `/api/v1/auth/session` | ✅ | ✅ | `authSession` | ✅ restore on load |
| POST | `/api/v1/auth/logout` | ✅ | ✅ | `authLogout` | ✅ |
| GET | `/api/v1/activity` | ✅ | ✅ | `activity` | ✅ (no pagination UI) |
| POST | `/api/v1/quotes` | ✅ | ✅ | `quote` | ✅ |
| POST | `/api/v1/swaps` | ✅ | ✅ | `createLiveSwap`, `createSimulation` | ✅ |
| GET | `/api/v1/swaps/{id}` | ✅ | ✅ | `swap` | ✅ polled every 5 s |
| POST | `/api/v1/swaps/{id}/deposit` | ✅ | ✅ | `recordDeposit` | ✅ |
| POST | `/api/v1/swaps/{id}/cancel` | ✅ | ✅ | `cancelSwap` | ✅ |
| POST | `/api/v1/swaps/{id}/simulate` | ✅ | ✅ | `advanceSimulation` | ✅ |
| GET | `/healthz`, `/readyz`, `/metrics` | ✅ | ❌ (by design) | — | ops only |

---

## 2. Frontend-only work (backend already exists)

### 2.1 Capability banner from `/api/v1/health`
- **Why:** the UI doesn't know whether the server is in demo mode, whether sign-in is required, or whether live execution is on. Users only see "unavailable" states.
- **Build:**
  - Load `api.health()` in `workspace-context.tsx` and expose `capabilities: { quoteMode, demoMode, marketDataMode, executionEnabled, authRequired }`.
  - Show a banner in `shell.tsx`:
    - "Demo mode: swaps are simulations" when `demoMode`.
    - "Live execution is off" when `quoteMode === "live" && !executionEnabled`.
- **Acceptance:** the banner text follows the backend config, with no hard-coded mode.

### 2.2 Sign-in prompt when auth is required
- **Why:** with `AUTH_REQUIRED=true`, quotes, swaps and activity return `401 UNAUTHORIZED`, and pages currently show a generic error.
- **Build:**
  - In `apiRequest`, surface `401` as a typed `UnauthorizedError`.
  - `ViewStateBlock` and the Swap page show "Sign in with your wallet" with a button that calls `wallet.signIn()`. Show it on connect when `capabilities.authRequired && !wallet.session`.
- **Acceptance:** with `AUTH_REQUIRED=true`, a user can connect, sign in, quote and swap without leaving the page.

### 2.3 Top movers panel
- **Endpoint:** `GET /api/v1/trading/top-movers?timeframe=4h|1d` → `{ source, asOf, quality, items: Signal[] }` (the top 10 by absolute 24h change).
- **Build:** a "Top movers" card on `pages/trading.tsx` (or `explore.tsx`) using `api.tradingTopMovers()`, with symbol, chain, 24h change, direction and a link to `/workspace/trading/{assetId}`.
- **Acceptance:** renders live data with a provenance badge; shows the unavailable state when `MARKET_DATA_MODE=fixture`.

### 2.4 Activity pagination
- **Endpoint:** `GET /api/v1/activity?limit=50&cursor=<nextCursor>` → `{ swaps, nextCursor? }`.
- **Build:** the client returns `{ swaps, nextCursor }`, and `pages/activity.tsx` adds a "Load more" button.
- **Acceptance:** more than 50 swaps can be browsed, with no duplicates and no gaps.

### 2.5 Research scenarios as real model output
- **Why:** `api.research()` always throws `UnavailableError` unless mocks are on, even though the backend computes the scenarios (with fictional inputs labelled in `note`).
- **Build:** show them with an "Illustrative inputs" tag instead of hiding them. Keep the TEST DATA tag only for mocks.
- **Acceptance:** the Research page shows Bear/Base/Bull with `projectedAnnualFeesUsd` from the API.

### 2.6 Demo catalogue usable without mocks
- **Why:** in `SWAP_QUOTE_MODE=demo`, every asset has a `demo:` id, so `api.assets()` throws, and Swap/Explore are unavailable unless `NEXT_PUBLIC_USE_MOCKS=true`.
- **Build:** when `capabilities.demoMode` is true, accept fixture assets and tag the page "Simulation". Never allow a live deposit from a simulation quote; `runSwap` already branches on `quote.mode === "simulation"`.
- **Acceptance:** a demo deployment can run a full simulated swap from the UI with no mock flag.

---

## 3. Backend and frontend work (new or extended endpoints)

### 3.1 Server-sent swap updates (replaces 5-second polling)
- **Endpoint:** `GET /api/v1/swaps/{id}/events` with `Accept: text/event-stream`.
  - Each event is `event: swap` with `data: <SwapRecord JSON>`, sent on every state change, plus a keep-alive comment every 15 s.
  - The stream closes after a terminal state. Ownership is checked as for `GET /swaps/{id}`.
- **Backend:**
  - Add an in-process pub/sub (`internal/swap/events`), publishing from `service.UpdateSwapState` / `RecordDeposit` and from the worker poller.
  - For multiple instances, either publish via Postgres `LISTEN/NOTIFY` (channel `swap_events`), or let the handler poll the DB every 2 s and emit only on change.
  - This route must not be wrapped by the gzip middleware or a WriteTimeout. Use `http.ResponseController` to disable the write deadline.
- **Proxy:** allow `GET /api/v1/swaps/{id}/events` and stream the body through unbuffered. Keep the `text/event-stream` content type, and skip the body-size limit and the 12 s timeout for this route.
- **Frontend:** in `swap-runner.ts`, use `EventSource` when available and fall back to the existing `getSwap` polling.
- **Acceptance:** UI progress updates within about 1 s of a state change, and the polling fallback still works.

### 3.2 Token balances beyond native + USDC
- **Endpoint (extend):** `GET /api/v1/portfolio?address=&chain=&tokens=catalogue`. Add a new optional `notice` that states coverage.
- **Backend:**
  - For EVM chains, read ERC-20 `balanceOf` for every catalogue asset with a `contractAddress` on that chain, using one Multicall3 call per chain (`0xcA11bde05977b3631167028862bE2a173976CA11`, `aggregate3`).
  - Skip zero balances.
  - Price via DeFiLlama `coins.llama.fi/prices/current/{chain}:{contract}`.
  - Cache per address+chain for 30 s, and cap the number of contracts per call (for example 200).
  - Solana SPL tokens: `getTokenAccountsByOwner` with `jsonParsed`.
- **Frontend:** no contract change; the portfolio page already lists `balances[]`. Show `notice`.
- **Acceptance:** a wallet holding catalogue tokens sees them with exact balances, and tokens without a price show "Not available".

### 3.3 Non-EVM deposit transactions (unsigned payloads)
- **Why:** `backend/internal/wallets/` already builds unsigned payloads for EVM, Solana, Stellar (XDR + memo), Aptos and NEAR, but no endpoint exposes them. Live swaps are EVM-only because the frontend builds the EVM transfer itself.
- **Endpoint:** `GET /api/v1/swaps/{id}/deposit-tx?from=<sourceAddress>` → `{ family, chainKey, payload, memo?, expiresAt }`.
  - `payload` is family-specific and unsigned:
    - EVM: `{ to, value, data, chainId }`
    - Solana: a base64 message
    - Stellar: XDR
    - Aptos: an entry-function payload
    - NEAR: `{ receiverId, actions }`
  - Ownership is checked; this works only while the swap is in `AWAITING_USER_DEPOSIT`.
- **Backend:** a handler calls `internal/wallets` builders with the swap's deposit address, memo and amount.
- **Frontend:** add NEAR (wallet-selector) and Solana (wallet-standard) connectors next to EVM. The swap runner gets the payload, asks the wallet to sign and send, then calls `POST /swaps/{id}/deposit` with the hash.
- **Registry:** enable `swap_enabled` / `deposit_supported` for a chain only after an end-to-end test on mainnet with a small amount.
- **Acceptance:** one real NEAR → EVM swap completes. The server never sees a private key.

### 3.4 NEAR and Solana wallet sign-in in the UI
- **Backend:** already supported by `POST /auth/verify`:
  - `chainFamily: "near"` with `publicKey: "ed25519:..."` (the access key is verified on-chain).
  - `chainFamily: "solana"`.
- **Frontend:**
  - After connecting a NEAR or Solana wallet (item 3.3), call `authNonce`, have the wallet sign `message` (NEAR `signMessage` / Solana `signMessage`), then call `authVerify` with the matching family.
  - For NEAR, the backend expects an ed25519 signature over the raw message bytes. If the wallet only offers NEP-413 `signMessage`, add NEP-413 support in `internal/auth` first: borsh payload `{tag 2147484061, message, nonce[32], recipient, callbackUrl?}`, SHA-256, then verify.
- **Acceptance:** a NEAR or Solana user can sign in, and their swaps and portfolio are scoped to that wallet.

### 3.5 Claim anonymous swaps after sign-in
- **Why:** swaps created before signing in have no owner and disappear from Activity once the user signs in.
- **Endpoint:** `POST /api/v1/swaps/{id}/claim` (session required). It sets `session_id` on an unowned swap, but only if the swap's `refund_address` or `recipient_address` equals the signed-in wallet. Otherwise it returns `403 FORBIDDEN`.
- **Backend:** add a repository method `ClaimSwap(ctx, id, principal, wallet)` with an optimistic update `WHERE session_id IS NULL`, plus the memstore equivalent.
- **Frontend:** after sign-in, call claim for swaps the browser created (keep their ids in `localStorage`).
- **Acceptance:** sign in after a swap and it still appears in Activity. Another wallet cannot claim it.

### 3.6 On-chain deposit verification
- **Why:** `backend/internal/chains/rpc` (`MultiChainVerifier`) exists but isn't used. A reported tx hash is trusted until the provider confirms it.
- **Endpoint (extend):** `POST /api/v1/swaps/{id}/deposit` keeps its contract. Add optional response fields `verification: { status: "pending" | "confirmed" | "failed" | "mismatch", confirmations, checkedAt }`.
- **Backend:**
  - After recording, verify asynchronously that the transaction succeeded, paid the swap's `deposit_address`, and moved at least `amountInAtomic` of the origin asset.
  - On `failed` / `mismatch`, write an audit event and flag the swap for review. Don't auto-advance on this alone; provider status stays the source of truth.
- **Frontend:** show the verification status in the swap progress view.
- **Acceptance:** a wrong-address or reverted transaction is flagged within one poll cycle.

### 3.7 Operations endpoints (protected)
- **Endpoints (admin-only, bearer `ADMIN_TOKEN`; not exposed through the public proxy):**
  - `GET /api/v1/admin/swaps?stuck=true&limit=` lists swaps with `stuck_at` set, with their last provider status and error code.
  - `POST /api/v1/admin/swaps/{id}/repoll` clears `stuck_at` and sets `next_poll_at = now()`.
  - `GET /api/v1/admin/provider/health` returns the worker's provider health (error rates, last success, catalogue age).
- **Backend:** add an `ADMIN_TOKEN` config value (required to enable these routes; compared in constant time). Log every admin action.
- **Acceptance:** an operator can find and re-poll a stuck swap without database access. The `udi_swaps_stuck_total` alert has a matching runbook action.

---

## 4. End-to-end flows to verify after implementation

### A. Live EVM swap
1. Connect the wallet, then sign in: `POST /auth/nonce` → `personal_sign` → `POST /auth/verify`. The `udi_session` cookie is set.
2. `GET /assets`, pick two EVM assets, then `POST /quotes` with `recipient` = `refundTo` = the wallet. The response is a `provider_dry_quote`.
3. Review, then `POST /swaps` with an `Idempotency-Key`. The swap is `AWAITING_USER_DEPOSIT` with a `depositAddress`.
4. The wallet sends the deposit, then `POST /swaps/{id}/deposit {txHash, chainId}` moves it to `DEPOSIT_SUBMITTED`.
5. `GET /swaps/{id}/events` (or polling) moves it through `PROCESSING` to `SUCCEEDED` / `REFUNDED`.
6. `GET /activity` shows it for this wallet only, and `GET /portfolio` reflects the new balances.

### B. Demo simulation
1. `POST /quotes` with demo assets returns a `simulation` quote.
2. `POST /swaps` with `{quoteId}` creates it in `AWAITING_USER_DEPOSIT`, flagged simulated.
3. `POST /swaps/{id}/simulate` three times walks it through `DEPOSIT_SUBMITTED`, `PROCESSING` and `SUCCEEDED` (`realSettlement` stays false).

### C. Non-EVM deposit (after 3.3)
1. Quote and create a swap from a NEAR, Solana, Stellar or Aptos asset.
2. `GET /swaps/{id}/deposit-tx?from=<addr>`, have the wallet sign and send, then `POST /swaps/{id}/deposit`.
3. Track status as in flow A.

### D. Portfolio
- `GET /portfolio?address=<wallet>[&chain=]` returns exact balances, with `valueUsd` when `MARKET_DATA_MODE=live`.
- Quality is `partial` when some chains fail; the failed chains are named in `notice`.

### E. Trading
- `GET /trading/overview`, `/signals`, `/top-movers` and `/assets/{id}`.
- `GET /orderblocks/{id}?timeframe=`, `/liquidity/{id}` and `/scenarios/{id}`.
- `GET /history` and `/backtests`.
- All require `MARKET_DATA_MODE=live`; otherwise they return `503 MARKET_DATA_UNAVAILABLE`.

---

## 5. Suggested order

1. **Quick UI wins:** 2.1, 2.2, 2.3, 2.4, 2.5, 2.6.
2. **Wallet scoping and safety:** 3.5 (claim anonymous swaps), then 3.6 (deposit verification).
3. **Latency:** 3.1 (server-sent swap updates).
4. **Coverage:** 3.2 (token balances).
5. **Multi-chain:** 3.3 (non-EVM deposits) together with 3.4 (NEAR/Solana sign-in).
6. **Operations:** 3.7.

Definition of done for the whole list:
- Every route is in `Routes()`, `openapi.yaml`, the proxy allowlist, `api-client.ts` and a UI.
- All tests pass, including `-race`.
- `scripts/smoke.sh` passes.
- No screen shows invented data outside mock mode.
