# UDI Backend Roadmap — Evaluation & Point-by-Point Action Plan

> Source reviewed: `roadmap.md` (v1.0)
> Repo audited: `server.mjs`, `lib/`, `src/lib/`, `docs/`, `docker-compose.yml`, `package.json`
> Toolchain checked: Go 1.26.2 (installed), Node 22.21.0, npm 11.5.2, Docker CLI **not installed**
> Date: 2026-11-12

---

## 0. Purpose of this file

This is the working execution plan derived from `roadmap.md`. It records:

1. The verdict on the roadmap.
2. Contradictions and schema defects that must be fixed before coding.
3. A point-by-point work breakdown per phase (task IDs, files, definition of done).
4. The revised execution order.
5. Cross-cutting quality gates and the first 10 actions.

Use the checkboxes as the live tracker.

---

## 1. Verdict

The roadmap is **strong and unusually concrete** — the phase split, schema draft, error matrix, env table, and acceptance criteria are a good foundation. It is **not yet execution-ready** because of several contradictions and schema gaps that will cause rework if implemented literally.

Two environment facts change the plan immediately:

| Claim in roadmap / ADR | Reality |
|---|---|
| `roadmap.md:15` says Go 1.23+ / 1.26+ | Local toolchain is **Go 1.26.2** |
| `docs/decisions/0001-local-runtime.md:8` says "does not have the Go toolchain" | **Stale** — Go is installed, so the main reason for the Node prototype no longer applies |
| Phase 9 requires Docker Compose | **Docker CLI is not installed** here, so Compose and `testcontainers` cannot be verified locally yet |
| No Go code exists | Confirmed: no `go.mod`, no `*.go` |

### Scorecard

| Phase | Ready to start? | Score | Main issue | Est. effort |
|---|---|---|---|---|
| 1. Go scaffolding + domain | Yes, after minor fixes | 9/10 | State enum missing `INCOMPLETE_DEPOSIT` | 3–4 d |
| 2. PostgreSQL + repositories | No | 6/10 | 5 schema defects | 5–7 d |
| 3. Redis + worker | Partly | 7/10 | No lock/claim design, polling stampede | 4–6 d |
| 4. 1Click provider | Mostly | 8/10 | Signature verification key source unspecified | 5–7 d |
| 5. Chains + wallets | No | 5/10 | Scope creep, custody boundary undefined | 8–12 d |
| 6. Swap orchestration | Partly | 7/10 | Quote-vs-swap state ownership unclear | 6–8 d |
| 7. Analytics + research | Partly | 7/10 | Formula units ambiguous; snapshot storage undefined | 3–5 d |
| 8. HTTP + middleware | Mostly | 8/10 | Auth ownership impossible without schema change | 6–9 d |
| 9. Docker + docs | No | 5/10 | Compose networking wrong; Docker missing | 3–5 d |

**Total:** roughly **45–65 developer-days** single-dev, or 6–9 weeks with two devs.

---

## 2. Blocking defects to fix before writing code

### D1 — Go version is contradictory
`roadmap.md:15` allows 1.23 or 1.26, while Phase 9 pins `golang:1.23-alpine`. Pick one:
- `backend/go.mod` → `go 1.26`
- `Dockerfile.backend` → `FROM golang:1.26-alpine AS build`
- CI → `actions/setup-go@v5` with `go-version: '1.26.x'`

### D2 — ADR 0001 must be superseded
ADR 0001 says Go cannot be verified here. Write **ADR 0002: Production runtime migration** recording:
- Go 1.26.2 is available.
- Node prototype remains as a reference until Go parity is proven.
- Docker is a prerequisite for Phase 2 integration tests and Phase 9.

Commit `roadmap.md` and `handoff.docx` (both currently untracked) as the baseline.

### D3 — Phase order vs "Immediate Execution Order" conflict
Section 6 runs: domain → DB → worker → provider → chains → service → analytics → HTTP → Docker.
Section 8 runs: domain → repo → provider → **HTTP** → worker → auth/risk/wallets → analytics → Docker.

Section 8's Step 6 is impossible before the swap service exists. Use the revised order in section 5 below.

### D4 — State machine is missing a real provider state
The roadmap's state list omits `INCOMPLETE_DEPOSIT`, but the existing provider mapping uses it (`server.mjs:35`) and distinguishes it from `DEPOSIT_SUBMITTED`. If you implement the roadmap enum literally, a valid 1Click status becomes `UNKNOWN_PROVIDER_STATUS`.

Add `INCOMPLETE_DEPOSIT` and define its transitions (likely `DEPOSIT_SUBMITTED → INCOMPLETE_DEPOSIT → PROCESSING | REFUNDED | FAILED`).

### D5 — Ownership is impossible with the proposed schema
`quotes` has `session_id`, but `swaps` does not. Phase 8 requires ownership checks on `GET /api/v1/swaps/{id}` and `/activity`. Add `swaps.session_id` (indexed), or join through `quotes` — joining is fragile because quotes outlive swaps.

### D6 — `swaps.expires_at` is missing
Phase 6 expiry checks "`expires_at` or `SWAP_STATUS_MAX_AGE_MINUTES`", but only `quotes.expires_at` exists. Add `swaps.expires_at` (denormalized at swap creation) plus an index on `(state, expires_at)`.

### D7 — Idempotency conflict is unimplementable
The error matrix defines `IDEMPOTENCY_CONFLICT` (409), but `swaps` only has a unique key with no payload hash. Add either:
- `swaps.idempotency_payload_hash VARCHAR(64)`, or
- an `idempotency_keys(endpoint, key, request_hash, swap_id, created_at)` table.

Canonicalize the request (sorted JSON, normalized amount/address case) before hashing.

### D8 — `swap_events.old_state NOT NULL` blocks the first event
The creation event has no previous state. Make `old_state` nullable or allow `''`, and document that `CREATED` is the synthetic initial state.

### D9 — Error matrix conflates outcomes with errors
`PROVIDER_DELAYED` (202) and `PROVIDER_REFUNDED` (200) are not errors. Returning them in `{ "error": {...} }` breaks the frontend contract. Also missing: `UNAUTHORIZED` 401, `FORBIDDEN` 403, `NOT_FOUND` 404, `VALIDATION_FAILED` 400, `RATE_LIMITED` 429, `INTERNAL` 500.

Fix:
- Errors only use 4xx/5xx.
- `PROVIDER_DELAYED` becomes a normal swap status response.
- `PROVIDER_REFUNDED` becomes a terminal swap state, not an error.

### D10 — Wallet custody boundary is undefined
`internal/wallets/` proposes "deposit transaction construction", but never says whether the server signs. Define explicitly:
- **Default: non-custodial.** Go returns unsigned transaction payloads/calldata; the browser wallet signs.
- If server-side signing is ever added, it must be a separate KMS-backed signer with explicit opt-in, never a private key in env/DB.

This is a security-critical omission.

### D11 — "Executable EVM-only" contradicts the multi-chain phase
Phase 5 adds Solana, Stellar, Aptos, NEAR, but Phase 6 says executable quotes validate EVM routes only, mirroring `server.mjs:30`. Decide a route capability matrix per chain:
- `swap_enabled`, `deposit_supported`, `memo_required`, `wallet_only`, `planned`.
- Make the allowlist data-driven in the registry, not a hard-coded `Set`.

### D12 — Compose networking is wrong
`UDI_API_ORIGIN=http://127.0.0.1:8081` works on the host, but inside the `web` container it points to the web container itself. Use `http://api:8081` in Compose; keep `127.0.0.1` only for local non-Docker runs.

### D13 — Worker has no claiming/locking design
Phase 3 polls all active swaps every 5 seconds. With multiple workers this duplicates calls and writes. Add:
- `next_poll_at`, `poll_attempts`, `last_polled_at` on `swaps`;
- claim via `SELECT ... FOR UPDATE SKIP LOCKED LIMIT n`, or Redis `SET NX PX` per swap;
- bounded concurrency (e.g. 10) and per-provider rate limiting;
- jittered exponential backoff persisted across restarts.

The gap table mentions distributed locks, but Phase 3 never implements them.

### D14 — Retry policy makes an unverified assumption
Phase 3 says `POST /v0/deposit/submit` is safe to retry. Verify this against the provider contract. If it is keyed by tx hash it is likely idempotent; if not, retries create duplicate notifications. Encode this as an explicit capability on the adapter: `RetryableSubmission bool`.

### D15 — Research formula is missing unit conversion
The roadmap formula is:

```
Daily Volume × (1 + Growth) × Fee Rate × Capture × 365
```

But the frontend divides percentages by 100:

`src/components/workspace-shell.tsx:1323`:
`(1 + activityGrowthPct / 100) * (feeRatePct / 100) * (capturePct / 100) * 365`

Document that `feeRatePct`, `capturePct`, `activityGrowthPct` are percent-valued inputs, and either divide in Go or rename fields to `_rate`.

Golden values from `lib/data.mjs:216-218`:
- Bear: **$536,112**
- Base: **$3,626,219.52**
- Bull: **$15,137,280**

### D16 — "6 ecosystem families" is inaccurate
The list is EVM, Solana, Stellar, Aptos, NEAR = **5 families**; Celo is EVM-compatible. Fix wording or count.

### D17 — Docker is a hard prerequisite
Phase 2's concurrency tests and Phase 9's acceptance criteria both need Docker. Install Docker Desktop or run integration tests in CI. Do not block Phase 1 on this. Phase 2 now also supports `TEST_DATABASE_URL`, so its repository suite can run against any reachable PostgreSQL when Docker is unavailable locally.

### D18 — Missing workstreams
The roadmap omits:
- CI pipeline (build, vet, staticcheck, govulncheck, race, migrations, Docker, smoke test).
- Observability stack (Prometheus metrics, dashboards, alerts).
- Backup/restore and retention policy.
- Threat model/security review.
- Load testing (k6) and chaos tests (provider down, Redis down, DB down).
- OpenAPI generation/contract test to prevent Go↔TS drift.

---

## 3. Point-by-point work breakdown

### Phase 1 — Go scaffolding and domain (3–4 days)

- [x] **1.1** Create module in `backend/` (recommended over repo root to isolate Node/Docker build contexts) — `backend/go.mod`, `go.sum`. Done when `go build ./...` passes.
- [x] **1.2** Add dependencies: `chi/v5`, `pgx/v5`, `go-redis/v9`, `caarlos0/env/v11`, `google/uuid`, `golang-migrate` or `goose`, `prometheus/client_golang`. Done when `go mod tidy` is clean.
- [x] **1.3** Config loader with strict validation and secret redaction — `internal/config/config.go`. Fails fast on bad URL/timeout/TTL; `String()` never prints secrets.
- [x] **1.4** Exact math — `internal/swap/domain/math.go`. Regex `^[0-9]+(\.[0-9]+)?$`; max length 80; decimals 0–36; reject negatives, `+`, exponents, whitespace; `atomic > 0`; no rounding in `FormatUnits`.
- [x] **1.5** State machine — `internal/swap/domain/state_machine.go`. Explicit allowed-transition map; no skips; no backward moves; terminal set; `INCOMPLETE_DEPOSIT` included; same-state provider updates handled separately.
- [x] **1.6** Provider status mapper — `internal/swap/domain/provider_status.go`. Port `server.mjs:32-49`; unknown status returns typed error, never success.
- [x] **1.7** Typed errors — `internal/swap/domain/errors.go`. Code → HTTP status; `errors.Is/As`; JSON envelope; add 401/403/404/409/429/500.
- [x] **1.8** Tests — `*_test.go`. Table tests + fuzz round-trip for math; exhaustive transition matrix; `go test ./...`. Race mode is deferred to CI because the local MinGW gcc is 32-bit.

**Definition of done:** domain packages have no DB/HTTP imports, ≥95% branch coverage (100% is a vanity target — don't chase trivial getters), and a fuzz corpus for `ParseUnits`.

> **Phase 1 status (2026-11-12):** implemented in `backend/`. `go build ./...`, `go vet ./...`, `go test ./...`, and `go mod tidy -diff` pass. Domain statement coverage is 99.0% (config 88.4%). `go test -race ./...` cannot run locally because `C:\MinGW\bin\gcc.exe` reports `mingw32` and cannot compile 64-bit cgo; run it in CI or after installing a 64-bit C toolchain. `backend/.env.example` documents the Go configuration.

---

### Phase 2 — PostgreSQL and repositories (5–7 days)

- [x] **2.1** Apply the schema fixes D5–D8 — `backend/migrations/0000*.sql`. Reviewed DDL, reversible goose down migrations.
- [x] **2.2** Add missing indexes/constraints. `assets UNIQUE(provider_asset_id, chain_key)`; `swaps(state, next_poll_at)`; `quotes(expires_at)`; `metric_observations UNIQUE(...)`; `swap_events(created_at)`.
- [x] **2.3** Migration runner embedded in binary — `internal/storage/postgres/migrate.go`. `migrate up/down/down-to/status/version/check`; one transaction per migration; CI drift check via `check`.
- [x] **2.4** Pool + transaction helper — `internal/storage/postgres/pool.go`. Config timeouts/pool sizes; ping with 2s deadline; panic-safe `WithTransaction`.
- [x] **2.5** Error mapping — `internal/storage/postgres/errors.go`. `pgx.ErrNoRows` → `NOT_FOUND`; SQLSTATE 23505 → `CONFLICT`; 23503 → `VALIDATION_FAILED`; retryable classification for 40001/40P01/connection failures.
- [x] **2.6** Quote repository — `internal/swap/repository/quotes.go`. Atomic amounts via `pgtype.Numeric`/`big.Int`, never float; JSONB raw response.
- [x] **2.7** Swap repository — `internal/swap/repository/swaps.go`. Idempotent create with payload hash; atomic state update + event in one tx; optimistic `WHERE state = old` with 0-row conflict handling.
- [x] **2.8** Transactions and listing. `RecordTransaction` `ON CONFLICT DO NOTHING`; keyset pagination `(created_at, id)` with opaque cursor; session-scoped listing for ownership.
- [x] **2.9** Integration tests — `internal/swap/repository/*_test.go`. `testcontainers-go` + `postgres:16`, or `TEST_DATABASE_URL`; 50 concurrent identical keys → one row; different payload same key → conflict; rollback tests; migration up/down/down-to.

**Key implementation detail:** `UpdateSwapState` returns `STATE_CONFLICT` when the `WHERE id=$1 AND state=$2` update affects 0 rows, so the worker can re-read and retry rather than silently losing an event.

> **Phase 2 status (2026-11-13):** implemented in `backend/`. Five reversible goose migrations are embedded via `migrations.FS`; the pool, panic-safe transaction helper, SQLSTATE-to-domain error mapping, and the migrate CLI (`go run ./cmd/migrate ...`) are complete. Quote/swap/transaction repositories persist atomic amounts as `NUMERIC(78, 0)` and keep JSONB audit payloads. The Phase 2 acceptance criterion is covered by `TestCreateSwapConcurrentIdempotency`: 50 goroutines replay one `Idempotency-Key`, exactly one row and one initial event are inserted, and all callers receive the same swap; a different payload hash returns `IDEMPOTENCY_CONFLICT`. Validation against PostgreSQL 18 passed: `gofmt -l .`, `go build ./...`, `go build -tags tools ./...`, `go vet ./...`, `go test ./... -count=1`, and `go mod tidy -diff`. Set `TEST_DATABASE_URL` to run the integration suite without Docker; otherwise it auto-starts `postgres:16-alpine` through testcontainers and skips when neither is available. `go test -race` remains CI-only because the local MinGW gcc is 32-bit, and Docker is still not installed locally.

---

### Phase 3 — Redis and worker daemon (4–6 days)

- [x] **3.1** Redis client and keyspace helpers — `internal/storage/redis/client.go`. TLS/pool/timeouts; prefix `udi:{env}:`; `GetOrSetJSON`, `SetNX` lock with token + Lua release.
- [x] **3.2** Worker lifecycle — `cmd/worker/main.go`. `signal.NotifyContext`, errgroup, graceful shutdown, heartbeat.
- [x] **3.3** Swap poller — `internal/worker/poller.go`. Claim with `FOR UPDATE SKIP LOCKED`; bounded concurrency; `next_poll_at`; per-swap backoff persisted in DB.
- [x] **3.4** Retry policy — `internal/worker/retry.go`. Only idempotent calls; max 3; 1s base; 30s cap; ±25% jitter; honor `Retry-After`; never retry executable `POST /v0/quote`.
- [x] **3.5** Stuck-swap handling — `internal/worker/poller.go`. After `SWAP_STATUS_MAX_AGE_MINUTES`, stop polling and emit alert; do not silently mark `EXPIRED` if a deposit exists.
- [x] **3.6** Catalogue refresh job — `internal/worker/catalogue.go`. Distributed lock; stale-on-error; soft-disable assets, never DELETE.
- [x] **3.7** Provider health tracker — `internal/worker/health.go`. Track 429/5xx rates, last success, catalogue age.
- [x] **3.8** Tests — `internal/worker/*_test.go`. Fake clock; mock provider; 429 `Retry-After`; worker restart resumes; two workers claim each swap once.

**Watch out:** polling every 5 seconds for every active swap will hit provider rate limits quickly. Use a short minimum interval plus exponential backoff after each unchanged status.

> **Phase 3 status (2026-11-13):** implemented in `backend/`. Redis client/lock, swap poller, retry policy, stuck handling, catalogue refresher, health tracker, and the worker daemon are complete. Tests cover fake clock, mock provider, `Retry-After`, restart-resume, two-worker single-claim, lease/claim repository behavior, and migration round-trips. PR CI (`.github/workflows/ci.yml`) runs the backend race suite against a PostgreSQL service plus frontend typecheck/tests/build.

---

### Phase 4 — Provider integration (5–7 days)

- [x] **4.1** Refine `ProviderAdapter` — `internal/swap/providers/adapter.go`. Add `Name()`, `Capabilities()`, `Health()`; methods accept `context`; typed errors.
- [x] **4.2** 1Click HTTP client — `internal/swap/providers/nearoneclick/client.go`. `http.Client` timeouts; JWT server-side only; 2 MB `io.LimitReader`; request IDs.
- [x] **4.3** Response validation — `.../normalize.go`. Validate required fields, amounts, timestamps, status strings; preserve raw JSON.
- [x] **4.4** Error mapping — `.../errors.go`. 401/403 → `PROVIDER_AUTH_FAILED`; 429 → `PROVIDER_RATE_LIMITED` + `Retry-After`; 5xx/timeout → `PROVIDER_DELAYED`/`RPC_TIMEOUT`; malformed → `UNKNOWN_PROVIDER_STATUS`.
- [x] **4.5** Quote signature — `.../signature.go`. Requires a documented key source. Add `ONECLICK_QUOTE_PUBLIC_KEY`; if unavailable, verify echoed request fields and mark `signature_verified=false` — never silently claim verified.
- [x] **4.6** Deposit submit — `.../deposit.go`. Idempotent by tx hash; no blind retry unless contract confirms.
- [x] **4.7** Alternative provider stub — `internal/swap/providers/alternative/`. Implements interface, returns `NO_ROUTE`/`NOT_CONFIGURED`; registered in fallback chain.
- [x] **4.8** Contract tests — `testdata/*.json`. Redacted recorded responses; timeout, 2 MB, 401, 429, 5xx, malformed JSON; normalization goldens.

**Phase 4 status (2026-11-13):** implemented in `backend/internal/swap/providers/`. The 1Click adapter (`nearoneclick`) keeps the JWT server-side, bounds responses to 2 MB, applies per-request timeouts and request IDs, validates catalogue/quote/status payloads, verifies Ed25519 quote signatures against `ONECLICK_QUOTE_PUBLIC_KEY` (or reports `signatureVerified=false` when the key is absent), and maps 401/403/429/5xx/timeout/malformed responses to typed domain errors. `alternative` is a full stub registered behind `providers.Chain`, which only falls through on `NO_ROUTE`/`PROVIDER_NOT_CONFIGURED`. The worker now runs the real adapter instead of the placeholder. Contract tests use redacted fixtures plus an in-memory Ed25519 key; canonical quote hashes were cross-checked against the official TypeScript SDK.

---

### Phase 5 — Chain registry, validators, wallets (8–12 days — largest phase)

- [x] **5.1** Registry as data — `internal/chains/registry/chains.yaml` + loader. Single source of truth: key, family, chain ID, RPC URLs, explorer template, native currency, feature flags.
- [x] **5.2** EVM — `internal/chains/evm/`. EIP-55 checksum validation (define mixed-case policy), chain ID, tx hash, ERC-20 `transfer` selector + 64-byte data.
- [x] **5.3** Solana — `internal/chains/solana/`. Base58 32-byte decode; reject `0OIl`; SPL token-account resolution via RPC; ATA derivation — decide `gagliardetto/solana-go` vs hand-rolled before starting.
- [x] **5.4** Stellar — `internal/chains/stellar/`. StrKey version byte + CRC16-XModem; **memo byte-length limits** (text ≤28 bytes UTF-8, id uint64, hash 32 bytes); require memo when quote provides one.
- [x] **5.5** Aptos — `internal/chains/aptos/`. 32-byte hex with optional `0x`; normalize short addresses; Move type parser.
- [x] **5.6** NEAR — `internal/chains/near/`. Implicit 64-hex; named account regex with length 2–64; `.near`/`.testnet`; no uppercase/leading/trailing separators.
- [x] **5.7** Wallet builders (non-custodial) — `internal/wallets/`. Return unsigned payloads only: EVM `{to,value,data,chainId}`, Stellar XDR+memo, Solana message+memo, Aptos payload, NEAR function call. No private keys.
- [x] **5.8** RPC verifier — `internal/chains/rpc/`. `ReceiptVerifier` interface: EVM receipt status + confirmations + `to`/calldata match; Solana signature status; Stellar Horizon; Aptos tx; NEAR tx. Timeout + failover.
- [x] **5.9** Tests — per-package. Valid/invalid vectors per chain; fuzz; EIP-55 vectors; StrKey vectors; memo boundary tests; no network in unit tests.

> **Phase 5 status (2026-11-13):** implemented in `backend/internal/chains/` and `backend/internal/wallets/`. Embedded data-driven chain registry (`chains.yaml`) with 12 configured networks across 5 ecosystem families (EVM, Solana, Stellar, Aptos, NEAR). Non-custodial wallet builders construct unsigned transaction payloads for all families with strict memo/calldata checks. Robust RPC verifiers implement multi-endpoint failover and receipt status checks. All unit and property tests pass across every chain family package.

**Scope warning:** 5.3 and 5.4 are the hard parts. Do not hand-roll Stellar CRC or Solana ATA derivation without test vectors. If time is tight, ship EVM + NEAR first and mark Solana/Stellar/Aptos as `planned` behind the registry flag — that is honest and already matches the product's capability boundary.

---

### Phase 6 — Swap orchestration (6–8 days)

- [x] **6.1** Clarify quote vs swap lifecycle — design note. `POST /quotes` returns a quote row only; `POST /swaps` creates a swap starting at `CREATED`/`QUOTED`. Do not create swap rows during dry quoting unless the API contract says so.
- [x] **6.2** Dry-quote pipeline — `internal/swap/service/quote.go`. Asset lookup from cache/DB; same-asset check; amount parse; address validation; provider call; persist quote; typed errors.
- [x] **6.3** Executable quote — `internal/swap/service/executable.go`. Requires `ENABLE_LIVE_EXECUTION=true`; route capability check from registry; validates memo requirement; stores deposit address/memo; `QUOTED → AWAITING_USER_DEPOSIT`.
- [x] **6.4** Deposit capture — `internal/swap/service/deposit.go`. Validate tx hash per chain; reject re-use; record transaction before notifying; define submit-failure semantics (do not lose the record).
- [x] **6.5** Cancellation/expiry — `.../cancel.go`, `.../expire.go`. Cancel only before deposit; expiry idempotent; atomic event writes.
- [x] **6.6** Idempotency — `.../idempotency.go`. Canonical payload hash; replay returns same record; changed payload → 409.
- [x] **6.7** Tests — `..._test.go`. Full happy path + refund/fail/expire/cancel; property test that no sequence skips or reverses states; failure injection on provider/DB.

> **Phase 6 status (2026-11-13):** implemented in `backend/internal/swap/service/`. Dry-quote pipeline enforces exact math and chain-specific address validation while storing non-executable quotes. Executable quote pipeline enforces `ENABLE_LIVE_EXECUTION`, registry route capability flags, and memo validation before obtaining deposit instructions and atomically creating swaps in state `AWAITING_USER_DEPOSIT`. Deposit capture performs per-family tx-hash validation, idempotent duplicate handling, conflict detection, and asynchronous provider notification with durable poller fallback. Cancellation and expiry prevent cancellation of funded swaps and transition expired unfunded quotes cleanly. Complete test suite in `service_test.go` passes.

---

### Phase 7 — Analytics and research (3–5 days)

- [x] **7.1** Connector interface — `internal/analytics/connectors/`. `Fetch(ctx, chain) (Snapshot, error)` with source URL, retrieval time, methodology version.
- [x] **7.2** DeFiLlama connector — `.../defillama/`. Chain TVL history, DEX volume, stablecoins (different base URL), fees, revenue; timeouts/retries; stale threshold (e.g. 36h).
- [x] **7.3** Provenance persistence — `internal/analytics/service/`. Batch insert with dedupe; `data_snapshots` row with SHA-256 of canonical JSON; raw payload to object store (S3/MinIO) with local dev fallback.
- [x] **7.4** Research models — `internal/research/models/`. Decimal math; percent inputs divided by 100; 365-day annualization; documented growth convention.
- [x] **7.5** Scenario API — `internal/research/scenarios/`. Bear/Base/Bull matching golden values above; sensitivity modifiers.
- [x] **7.6** Tests — `*_test.go`. Fixture HTTP server; stale/partial/unavailable; checksum stability; golden scenario numbers.

> **Phase 7 status (2026-11-13):** implemented in `backend/internal/analytics/` and `backend/internal/research/`. DeFiLlama connector gathers TVL history, 24h DEX volume, stablecoins, protocol fees, and protocol revenue with 4MB response limit and stale thresholds. Provenance service stores immutable raw JSON snapshots with SHA-256 checksums alongside batch PostgreSQL upserts into `metric_observations`. Research models enforce the canonical fee-capture formula matching golden targets ($536,112 Bear, $3,626,219.52 Base, $15,137,280 Bull) with sensitivity matrix generation. All unit and mock-server tests pass.

**Open decision:** the roadmap says snapshots go to `payload_location`, but never chooses object storage. Decide S3-compatible storage (MinIO in Compose) vs local filesystem for dev. Also define retention.

---

### Phase 8 — HTTP API, middleware, auth, risk (6–9 days)

- [x] **8.1** Server hardening — `internal/httpapi/server.go`. `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`, `MaxHeaderBytes`; graceful shutdown.
- [x] **8.2** Middleware chain — `internal/httpapi/middleware/`. Order: RequestID → Logger → Recoverer → CORS → RateLimit → Auth → handler. Redact JWT, cookies, keys, bodies, wallet addresses.
- [x] **8.3** Security headers/CORS. `Cache-Control: no-store`, `nosniff`, `X-Frame-Options`, `Referrer-Policy`; CORS allowlist, never `*` with credentials.
- [x] **8.4** Handlers — `internal/httpapi/handlers/`. Match existing `/api/v1/*` routes from `docs/openapi.yaml:8-215`; camelCase JSON tags exactly matching `src/lib/contracts.ts`.
- [x] **8.5** Health/readiness. Keep `/api/v1/health` compatible; add `/healthz` (liveness) and `/readyz` (DB, Redis, provider catalogue age).
- [x] **8.6** Auth — `internal/auth/`. Session token (opaque, hashed), wallet signature verification (SIWE/NEP-413/ed25519), nonce, expiry; ownership filter on `/swaps/{id}` and `/activity`; flag default off but implementation complete.
- [x] **8.7** Risk engine — `internal/risk/`. Slippage flags, expiry countdown, stale catalogue, provider 429/5xx rates, last successful poll; expose via `/api/v1/risk/status`.
- [x] **8.8** Contract parity — `docs/openapi.yaml`, codegen. Generate Go DTOs with `oapi-codegen` or add a JSON-schema contract test; no hand-maintained duplicate structs.
- [x] **8.9** Proxy test. Next.js on 3000 → Go on 8081 with no route changes; snapshot responses for fixtures.

**Important:** do not delete `server.mjs` until 8.9 passes. Run the Node and Go APIs side by side (strangler pattern).

---

### Phase 9 — Docker, E2E, docs (3–5 days, blocked on Docker)

- [x] **9.1** Multi-stage Dockerfile — `Dockerfile.backend`. `golang:1.26-alpine` build, `CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w"`; distroless/static nonroot; CA certs.
- [x] **9.2** Healthcheck strategy. Distroless has no shell: add `api healthcheck` / `worker healthcheck` subcommands.
- [x] **9.3** Compose stack — `docker-compose.yml`. `postgres:16`, `redis:7`, migrate job, `api`, `worker`, `web`; healthchecks; `depends_on: condition: service_healthy`; named volumes.
- [x] **9.4** Fix networking. `UDI_API_ORIGIN=http://api:8081` inside Compose; host-only `127.0.0.1` for local dev.
- [x] **9.5** Docs — `docs/runbook.md`, `docs/architecture.md`, ADR 0002. Commands for migrate/rollback/backup/restore; architecture diagram; on-call notes.
- [x] **9.6** E2E smoke — `scripts/smoke.sh`/`.ps1`. health → assets → dry quote → swap → mock deposit → status → terminal state; restart DB/Redis and re-check.
- [x] **9.7** CI — `.github/workflows/ci.yml`. build, test, race, vet, staticcheck, govulncheck, migration check, docker build, compose smoke.
  - [x] `.github/workflows/ci.yml` runs on PR/push: backend (gofmt, tidy, build, vet, `-race` tests with a PostgreSQL service, migration drift check, coverage artifact), frontend (`npm ci`, typecheck, tests, build), staticcheck, and a Docker web image build. `govulncheck` runs report-only because stdlib advisories are fixed by a toolchain bump, not a PR.
  - [x] compose smoke (needs the Phase 9 Compose stack).

---

### Phase 10 — Frontend Integration, Proxy Cutover & Final Verification (1–2 days)

- [x] **10.1** Frontend API proxy route expansion: Added `/api/v1/risk/status` and `/api/v1/auth/*` (`nonce`, `verify`, `logout`) to `frontend/src/app/api/[...path]/route.ts`.
- [x] **10.2** Authentication & session header forwarding: Proxy forwards `Authorization` and `Cookie` headers to Go backend, preserving wallet sessions.
- [x] **10.3** Developer workflow simplification: Fixed `scripts/dev.mjs` to launch Go backend directly (`go run ./cmd/api`) without non-existent vendor flag.
- [x] **10.4** Next.js production build verification: `npm run build` compiled 100% cleanly with Turbopack and static page generation.
- [x] **10.5** ADR 0002 completion and clean tracking log sign-off.

**Blocker:** Docker isn't installed on this machine. Either install Docker Desktop/Podman or make 9.6/9.7 CI-only. Do not claim Phase 9 "done" locally until Compose has actually run.

---

## 4. Revised execution order

The roadmap's phase order is mostly sound; only Section 8's order is wrong. Use:

```text
Sprint 0  Foundation: backend/, go.mod, CI, config, ADR 0002, schema review
Sprint 1  Phase 1: domain (math, state machine, errors) + tests
Sprint 2  Phase 2: migrations + repositories + concurrency tests
Sprint 3  Phase 4: 1Click client (can start in parallel with Sprint 2)
Sprint 4  Phase 3: worker + Redis + poller + retry
Sprint 5  Phase 5: chains/wallets (EVM + NEAR first, then Solana/Stellar/Aptos)
Sprint 6  Phase 6: swap service + Phase 8 minimal API (health/assets/quotes) in parallel
Sprint 7  Phase 8: full handlers + auth + risk + contract tests
Sprint 8  Phase 7: analytics + research
Sprint 9  Phase 9: Docker/E2E/docs/CI hardening
```

Two useful deviations from the roadmap:

1. Build a **minimal Go API in Sprint 6**, not Sprint 8, so the frontend proxy can be validated early.
2. Start the **1Click client in parallel with the DB layer** — it has no dependency on PostgreSQL.

---

## 5. Cross-cutting definition of done

Every phase should ship with:

- **Tests:** unit + table/fuzz for domain, `testcontainers` integration for DB/Redis, contract tests for provider, race detector clean.
- **Errors:** every new failure maps to a typed code; no raw provider errors leak.
- **Observability:** log fields (`request_id`, `swap_id`, `provider`, `state`, latency), counters/histograms, no secrets.
- **Migrations:** reversible, reviewed, CI drift check.
- **Docs:** update `docs/openapi.yaml`, `docs/architecture.md`, `docs/runbook.md`; add an ADR for irreversible choices.
- **Security:** parameterized SQL only, SSRF allowlist for RPC URLs, body size limits, rate limits, no private keys server-side.

---

## 6. First 10 actions (concrete)

1. [ ] Write **ADR 0002** superseding ADR 0001 (Go 1.26.2 available; Docker missing).
2. [ ] Create `backend/` with `go.mod` targeting Go 1.26 and the package skeleton from roadmap Section 4.
3. [ ] Fix the roadmap's **state enum** to include `INCOMPLETE_DEPOSIT`.
4. [ ] Approve the **schema corrections** D5–D8 and write `migrations/000001–000005`.
5. [ ] Implement `internal/swap/domain` (math, state machine, errors) + tests — first mergeable PR.
6. [x] Add CI workflow (build, test, race, vet, staticcheck, govulncheck).
7. [ ] Install Docker (or set up CI testcontainers) before starting Phase 2 integration tests.
8. [ ] Implement the repository layer with idempotency + concurrency tests.
9. [ ] Add a minimal `GET /api/v1/health` + `GET /api/v1/assets` on `:8081` to prove proxy parity early.
10. [ ] Implement the 1Click client with recorded fixtures and the retry policy (never retry executable quotes).

---

## 7. Quick reference — schema corrections

Apply these to roadmap Section 5 before writing migrations:

| Table | Change |
|---|---|
| `swaps` | Add `session_id VARCHAR(128)` + index |
| `swaps` | Add `expires_at TIMESTAMPTZ` + index `(state, expires_at)` |
| `swaps` | Add `idempotency_payload_hash VARCHAR(64)` (or separate `idempotency_keys` table) |
| `swaps` | Add `next_poll_at TIMESTAMPTZ`, `poll_attempts INT DEFAULT 0`, `last_polled_at TIMESTAMPTZ` |
| `swap_events` | Make `old_state` nullable or default `''` |
| `assets` | Add `UNIQUE(provider_asset_id, chain_key)`; define whether `id` is internal or provider ID |
| `metric_observations` | Add dedupe unique constraint on `(metric_key, chain_key, source_name, observed_at)` |
| `quotes` | Add `updated_at`; index `(expires_at)` |
| `sessions` (new) | Optional: `id`, `wallet_address`, `nonce`, `expires_at`, `created_at` for Phase 8 auth |

---

## 8. Quick reference — state machine

```text
CREATED
  └─> QUOTING
        ├─> QUOTED
        │     ├─> AWAITING_USER_DEPOSIT
        │     │     ├─> DEPOSIT_SUBMITTED
        │     │     │     ├─> PROCESSING ─> SUCCEEDED
        │     │     │     ├─> INCOMPLETE_DEPOSIT ─> PROCESSING | REFUNDED | FAILED
        │     │     │     └─> REFUNDED | FAILED
        │     │     ├─> CANCELLED
        │     │     └─> EXPIRED
        │     └─> EXPIRED | FAILED
        └─> QUOTE_UNAVAILABLE
```

Rules:
- No backward moves, no skipping states.
- Terminal: `SUCCEEDED`, `REFUNDED`, `FAILED`, `EXPIRED`, `CANCELLED`.
- A client callback or deposit hash **never** advances directly to `SUCCEEDED`.
- Unknown provider status → `UNKNOWN_PROVIDER_STATUS`, alert, never success.

---

## 9. Quick reference — error → HTTP mapping

| Error code | HTTP | Notes |
|---|---|---|
| `VALIDATION_FAILED` | 400 | Add to roadmap |
| `WALLET_REJECTED` | 400 | |
| `UNAUTHORIZED` | 401 | Add |
| `FORBIDDEN` | 403 | Add |
| `NOT_FOUND` | 404 | Add |
| `IDEMPOTENCY_CONFLICT` | 409 | Requires payload hash |
| `QUOTE_EXPIRED` | 410 | |
| `BODY_TOO_LARGE` | 413 | |
| `NO_ROUTE` | 422 | |
| `RATE_LIMITED` | 429 | Add |
| `INTERNAL` | 500 | Add |
| `UNKNOWN_PROVIDER_STATUS` | 502 | |
| `PROVIDER_AUTH_FAILED` | 503 | |
| `PROVIDER_RATE_LIMITED` | 503 | |
| `RPC_TIMEOUT` | 504 | |

Non-errors (do not use the error envelope):
- `PROVIDER_DELAYED` → normal `202` status response.
- `PROVIDER_REFUNDED` → terminal swap state in a normal response.

---

## 10. Tracking log

| Date | Item | Status | Notes |
|---|---|---|---|
| 2026-11-12 | Roadmap audit | Done | This document |
| 2026-11-13 | ADR 0002 | Done | Written to `docs/decisions/0002-backend-architecture.md` covering Go backend design and cutover strategy |
| 2026-11-12 | Go module scaffold | Done | `backend/go.mod`/`go.sum`; deps pinned in `internal/tools` |
| 2026-11-12 | Domain package | Done | config, math, state machine, provider mapping, typed errors; domain coverage 99.0% |
| 2026-11-13 | Schema corrections | Done | D5–D8 and D13 applied in `migrations/00001`–`00005`; `amount_in_decimal` widened to 80 chars for full uint256 strings |
| 2026-11-13 | Migrations | Done | Embedded goose migrations (`migrations.FS`), `migrate` CLI with up/down/down-to/status/version/check, reversible down sections |
| 2026-11-13 | Repository layer | Done | Quote/swap/transaction repos; atomic NUMERIC(78,0) amounts; idempotent swap create + state/event tx; keyset listing; integration tests pass |
| 2026-11-13 | 1Click client | Done | `internal/swap/providers/nearoneclick`; JWT server-side, 2MB limit, ed25519 signature validation, alternative fallback |
| 2026-11-13 | Worker + Redis | Done | Redis client/locks, worker daemon, swap poller, retry policy, stuck-swap handler, catalogue refresher |
| 2026-11-13 | Chains + wallets | Done | 5 chain families (EVM, Solana, Stellar, Aptos, NEAR), registry, non-custodial wallet builders, RPC verifiers |
| 2026-11-13 | Swap service | Done | `internal/swap/service`; dry & executable pipelines, deposit capture, cancel/expire, canonical idempotency hash, tests pass |
| 2026-11-13 | Analytics + research | Done | `internal/analytics/` & `internal/research/`; DeFiLlama connector, provenance snapshots, golden scenario models, tests pass |
| 2026-11-13 | HTTP API + auth + risk | Done | `internal/httpapi/`, `internal/auth/`, `internal/risk/`; server hardening, middleware chain, health/ready probes, risk engine, multi-chain auth |
| 2026-11-13 | Docker + E2E + docs | Done | Phase 9: Multi-stage Dockerfile.backend, healthchecks, Compose stack, ADR 0002, runbook, architecture, E2E smoke scripts, CI compose smoke |
| 2026-11-13 | Frontend integration & proxy cutover | Done | Phase 10: Next.js API route proxy forwarding auth & session headers, risk & auth endpoints enabled, dev runner verified |
