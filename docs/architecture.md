# Architecture

## Production System Architecture

Universal DeFi Intelligence is built as a production-grade, distributed architecture separating HTTP traffic, background worker tasks, persistence, and external blockchain integrations:

```mermaid
flowchart TD
  subgraph Client Tier
    Browser["Next.js Web Client (Port 3000)"]
    Wallets["User Wallets (EVM, Solana, NEAR)"]
  end

  subgraph API Gateway / Ingress
    Proxy["Next.js Route Handler (/api/*)"]
  end

  subgraph Backend Tier (Go 1.26)
    API["Go HTTP API (:8081)"]
    Worker["Go Background Worker"]
    Migrate["Migration Runner (goose)"]
  end

  subgraph Core Engines
    StateMachine["Swap State Machine & Idempotency"]
    RiskEngine["Real-time Risk & Circuit Breaker"]
    AuthManager["Multi-Chain Auth (SIWE / NEP-413 / Ed25519)"]
    ScenarioEngine["Deterministic Research Scenarios"]
  end

  subgraph Storage Tier
    Postgres[("PostgreSQL 16 (Durable Stores)")]
    Redis[("Redis 7 (Distributed Locks & Cache)")]
  end

  subgraph External Providers
    OneClick["NEAR Intents 1Click Provider"]
    DeFiLlama["DeFiLlama Analytics API"]
    ChainsRPC["Multi-Chain RPCs (EVM, Solana, Stellar, Aptos, NEAR)"]
  end

  Browser --> Proxy
  Proxy -->|UDI_API_ORIGIN| API
  Wallets -.->|Non-Custodial Client Sign| ChainsRPC

  API --> StateMachine
  API --> RiskEngine
  API --> AuthManager
  API --> ScenarioEngine

  API --> Postgres
  API --> Redis
  Worker --> Postgres
  Worker --> Redis
  Migrate --> Postgres

  API --> OneClick
  API --> DeFiLlama
  Worker --> OneClick
  Worker --> ChainsRPC
```

---

## 1. Component Boundaries

### Web Frontend (`frontend/`)
- **Technology**: Next.js 14+ (App Router), React, TypeScript.
- **Responsibility**: User experience, non-custodial wallet connection, interactive swap interface, market analytics visualization, and deterministic research calculator.
- **Security**: Never receives or stores server provider credentials (`ONECLICK_JWT`). All backend calls are routed through the same-origin `/api/*` proxy.

### Go HTTP API (`backend/cmd/api`, `internal/httpapi`)
- **Technology**: Go 1.26 statically linked binary running in a distroless container.
- **Middleware Chain**:
  `RequestID` → `Logger` (redacting JWT, secrets, and addresses) → `Recoverer` → `CORS` → `SecurityHeaders` → `RateLimit` (Token Bucket) → `Auth` → Handlers.
- **Contract Parity**: Strict adherence to `openapi.yaml` and TypeScript DTOs defined in `contracts.ts`.
- **Endpoints**:
  - `/healthz`, `/readyz`: Liveness and readiness probes.
  - `/api/v1/health`: Operational flags and mode reflection.
  - `/api/v1/chains`, `/api/v1/assets`: Chain registry and token catalogue.
  - `/api/v1/markets/overview`, `/api/v1/intelligence/near`: Ecosystem analytics.
  - `/api/v1/research/scenarios`: Golden sensitivity scenarios ($536k Bear, $3.62M Base, $15.13M Bull).
  - `/api/v1/risk/status`: Provider error rate, circuit breaker state, catalogue freshness.
  - `/api/v1/quotes`, `/api/v1/swaps`, `/api/v1/swaps/{id}`: Non-custodial swap lifecycle.
  - `/api/v1/auth/*`: Nonce generation, wallet signature verification, and session management.

### Background Worker Daemon (`backend/cmd/worker`, `internal/worker`)
- **Technology**: Go daemon running concurrent worker errgroups with graceful shutdown.
- **Jobs**:
  1. **Swap Status Poller**: Polls external provider status for active swaps, advancing state machine safely (`DEPOSIT_SUBMITTED` → `PROCESSING` → `SUCCEEDED` / `REFUNDED` / `FAILED`).
  2. **Asset Catalogue Refresher**: Periodically synchronizes token lists from 1Click and updates local repository under a Redis distributed lock.
  3. **Stuck Swap Janitor**: Identifies timed-out un-deposited quotes and marks them expired.

### Multi-Chain Registry & Wallet Builders (`internal/chains/`, `internal/wallets/`)
- Supports 5 distinct blockchain families:
  - **EVM**: EIP-55 address normalization, ERC-20 calldata builders, JSON-RPC receipt verifiers.
  - **Solana**: Base58 validation, Associated Token Account (ATA) derivation, SPL-token transfers, Ed25519 verification.
  - **Stellar**: StrKey public address encoding, Muxed accounts, operation validation.
  - **Aptos**: Move module address validation and BCS payload construction.
  - **NEAR**: Named account validation (`alice.near`), implicit 64-char hex addresses, NEP-413 signing.
- **Policy**: Server strictly functions non-custodially. It constructs unsigned payloads; user wallets sign and broadcast independently.

---

## 2. Swap State Machine

All cross-chain swaps adhere to an immutable, strictly validated state machine:

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

- **Idempotency**: All swap creation requests require an `Idempotency-Key` header (8-128 chars). Repeated requests return the existing record without side effects.
- **Deposit Accelerator**: `POST /swaps/{id}/deposit` records the user's source transaction hash and initiates an async provider notification coordinated via `sync.WaitGroup` to guarantee shutdown safety.

---

## 3. Storage & Concurrency Model

- **PostgreSQL 16**: Primary source of truth.
  - Amounts stored using `NUMERIC(78,0)` for lossless handling of full uint256 blockchain integers.
  - Keyset cursor pagination for efficient, non-drifting activity feeds.
  - Event log: append-only `swap_events` table tracking historical state transitions.
- **Redis 7**:
  - Ephemeral cache for token catalogues and market observations.
  - Distributed redlock for worker catalogue refresh synchronization.

---

## 4. Security & Hardening Boundaries

1. **Distroless Container Runtime**: `gcr.io/distroless/static-debian12:nonroot` contains zero shells or system package managers; execution runs as unprivileged user `nonroot`.
2. **Log Redaction**: Automatic sanitization regex cleans JWT tokens, private hex keys, session cookies, and masks user wallet addresses.
3. **Graceful Shutdown**: Traps `SIGINT`/`SIGTERM` with 10-second bounded timeouts, draining HTTP connections and awaiting in-flight provider accelerator tasks.
4. **Circuit Breaker**: Detects provider 429/5xx error spikes, enters `HALF_OPEN` / `OPEN` states to protect infrastructure, and surfaces health metrics via `/api/v1/risk/status`.
