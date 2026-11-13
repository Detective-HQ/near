# ADR 0002: Production Go Backend Architecture, Multi-Chain Bridge, and Observability

- Status: Accepted
- Date: 2026-11-13
- Supersedes: ADR 0001

## Context

ADR 0001 established a provisional Node.js mock API for fast prototyping without external toolchains. As the platform matured into production, the system required:
1. High-throughput, concurrent cross-chain swap orchestration across EVM, Solana, NEAR, Aptos, and Stellar.
2. Idempotent state transitions and strict non-custodial deposit lifecycle management.
3. Resilience against external provider rate limits, network timeouts, and partition faults.
4. Production PostgreSQL persistence with reversible schema migrations and Redis caching.
5. Standardized error taxonomy (RFC-compliant) and real-time risk observability.

## Decision

We replace the single-process prototype with a hardened, statically compiled Go backend architecture structured into clear domain boundaries:

1. **Domain State Machine & Data Models (`internal/swap/domain`)**:
   - Explicit state transitions: `CREATED` → `QUOTING` → `QUOTED` → `AWAITING_USER_DEPOSIT` → `DEPOSIT_SUBMITTED` → `PROCESSING` → `SUCCEEDED` / `FAILED` / `REFUNDED` / `CANCELLED` / `EXPIRED`.
   - Idempotent deduplication hashing on `(endpoint, idempotency_key, request_payload_hash)`.
   - Numeric(78,0) lossless representation for atomic blockchain amounts up to uint256.

2. **Database Migrations & Persistence (`migrations/`, `internal/storage/`)**:
   - Embed goose-managed reversible SQL migrations directly into Go binaries (`migrations.FS`).
   - Pure pgx/v5 connection pooling and key-set cursor pagination for swap records and activity feeds.

3. **External Provider Integration (`internal/swap/providers/`)**:
   - Primary 1Click adapter (`nearoneclick`) using server-side signed JWTs, 2MB bounded streams, ed25519 signature checks, and fallback adapter resilience.
   - Independent background poller daemon (`cmd/worker`) decoupling provider status synchronizations from user HTTP requests.

4. **Multi-Chain Registry & Wallet Builders (`internal/chains/`, `internal/wallets/`)**:
   - Explicit family drivers: EVM (EIP-55/ERC-20), Solana (SPL/ATA/base58), Stellar (StrKey/Muxed), Aptos (BCS/Move), NEAR (implicit/named).
   - Client-side transaction encoding: the server NEVER holds user private keys or performs custodial signing.

5. **HTTP API Hardening & Middleware Chain (`internal/httpapi/`)**:
   - Strict middleware order: `RequestID` → `Logger` (with sensitive redaction) → `Recoverer` → `CORS` → `SecurityHeaders` → `RateLimit` → `Auth` → Handler.
   - Standardized error mapping mapping domain errors to exact HTTP status codes.
   - Real-time risk monitoring (`internal/risk`) reporting slippage risk, catalogue staleness, and circuit breaker status at `/api/v1/risk/status`.

6. **Deployment Packaging (`Dockerfile.backend`, `docker-compose.yml`)**:
   - Multi-stage build producing static, stripped binaries (`CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w"`).
   - Distroless non-root runtime container (`gcr.io/distroless/static-debian12:nonroot`) with custom `api healthcheck` and `worker healthcheck` commands.

## Consequences

- Zero data loss: All state transitions are guarded by database transactions and background poller recovery.
- Minimal attack surface: Distroless container contains no shell, package manager, or unnecessary binaries.
- Strict security: Sensitive data (JWTs, private keys, authorization tokens) are sanitized in all log streams.
- Operational clarity: Health (`/healthz`, `/readyz`), risk (`/api/v1/risk/status`), and migration tooling (`cmd/migrate`) provide comprehensive observability.
