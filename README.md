# Universal DeFi Intelligence (Zonyx)

[![CI Pipeline](https://github.com/Detective-HQ/near/actions/workflows/ci.yml/badge.svg)](https://github.com/Detective-HQ/near/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Next.js](https://img.shields.io/badge/Next.js-16.4-black?logo=next.js&logoColor=white)](https://nextjs.org/)
[![TypeScript](https://img.shields.io/badge/TypeScript-5.x-3178C6?logo=typescript&logoColor=white)](https://www.typescriptlang.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-4169E1?logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-7-DC382D?logo=redis&logoColor=white)](https://redis.io/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> **Non-custodial cross-chain swap orchestration, intent-based settlement, and source-attributed DeFi intelligence.**

---

## 1. Executive Overview

**Universal DeFi Intelligence (Zonyx)** solves two fundamental challenges in decentralized finance:

1. **Liquidity Fragmentation & Bridge Friction:** Traditional cross-chain bridges rely on wrapped assets, custodial pool risks, and high slippage. Zonyx coordinates **intent-based swaps** powered by the **NEAR Intents (1Click / Defuse Protocol)** framework, enabling users to swap native tokens across distinct blockchains directly from their own wallets.
2. **Opaque & Unverifiable Market Data:** Most trading dashboards present aggregated metrics without attribution or methodology. Zonyx enforces **verifiable data provenance**—every TVL observation, DEX volume figure, and fee calculation is cryptographically captured, timestamped, and linked to its original upstream source (e.g. DeFiLlama).

---

## 2. Core Architectural Principles

```
┌────────────────────────────────────────────────────────────────────────┐
│                        CLIENT / USER WALLET                            │
│  (Injected EVM / Solana / NEAR Wallets · Client-Side Transaction Sign) │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │ HTTP / WebSocket (Client Calls)
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                       NEXT.JS GATEWAY & PROXY                          │
│     (App Router · Header Forwarding · Edge Caching · Static Bento)     │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │ Reverse Proxy (/api/v1/*)
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                        PRODUCTION GO BACKEND                           │
│  ┌───────────────────────┬──────────────────────┬───────────────────┐  │
│  │   HTTP API Server     │   Risk & Circuit     │  Idempotent State │  │
│  │   (chi/v5 Router)     │   Breaker Engine     │  Machine Core     │  │
│  └───────────┬───────────┴──────────┬───────────┴───────────┬───────┘  │
│              │                      │                       │          │
│              ▼                      ▼                       ▼          │
│  ┌───────────────────────┬──────────────────────┬───────────────────┐  │
│  │ Non-Custodial Multi-  │ PostgreSQL 16 Store  │ Redis 7 Lock &    │  │
│  │ Chain Drivers         │ (pgx/v5 Connection)  │ Queue Engine      │  │
│  └───────────────────────┴──────────────────────┴───────────────────┘  │
└───────────────────────────────────┬────────────────────────────────────┘
                                    │ Outbound Orchestration
                                    ▼
┌────────────────────────────────────────────────────────────────────────┐
│                     EXTERNAL SETTLEMENT & TELEMETRY                    │
│   • NEAR Intents (1Click API)      • Public DeFiLlama Connectors       │
│   • Multi-Chain RPC Verifiers      • Distributed Solvers & Makers      │
└────────────────────────────────────────────────────────────────────────┘
```

### 1. Non-Custodial by Design
The backend **never touches or holds user private keys**. 
- Deposit transactions, ERC-20 approvals, and cross-chain transfers are generated as unsigned transaction payloads (`to`, `value`, `data`, `memo`).
- Transactions are signed and broadcast strictly inside the user's browser wallet (MetaMask, Rabby, Phantom, etc.).
- Settlement confirmation is driven by on-chain receipt verifiers and cryptographic solver status polling.

### 2. Lossless Precision & Exact Arithmetic
Blockchain asset values span up to 256 bits (e.g. 18 to 24 decimals). Floating-point math is strictly prohibited across the codebase.
- Atomic amounts are handled as arbitrary-precision big integers (`big.Int`) and persisted as `NUMERIC(78, 0)` in PostgreSQL.
- Deterministic decimal string formatting ensures mathematical consistency between Go backends and TypeScript frontends.

### 3. Strict Idempotency & Finite State Machine
All state transitions follow an explicit, unidirectional finite state machine:
$$\text{CREATED} \longrightarrow \text{QUOTED} \longrightarrow \text{AWAITING\_USER\_DEPOSIT} \longrightarrow \text{DEPOSIT\_SUBMITTED} \longrightarrow \text{PROCESSING} \longrightarrow \begin{cases} \text{SUCCEEDED} \\ \text{REFUNDED} \\ \text{FAILED} \end{cases}$$
- Duplicate calls with identical `Idempotency-Key` and matching payload hashes return cached results safely.
- Hash mismatches on repeated keys trigger an `IDEMPOTENCY_CONFLICT` (HTTP 409).
- Unfunded quotes cleanly transition to `EXPIRED` or `CANCELLED`.

### 4. Cryptographic Quote Verification
Quotes received from 1Click are verified using **Ed25519 signature checks**:
$$\text{Message} = \text{Base58}\Big(\text{SHA-256}\big(\text{canonicalJSON}(\text{QuoteRequest} \cup \text{QuoteResponse} \cup \text{Timestamp})\big)\Big)$$
- Signatures are verified against the provider's Ed25519 public key.
- If unverified, the quote is flagged with `signatureVerified: false` to alert downstream consumers.

---

## 3. Platform Capabilities

### 🔀 Intent-Based Cross-Chain Swaps
- **Single-click execution:** Route swaps between disparate chains without wrapping tokens or manual bridging steps.
- **Supported Ecosystems:**
  - **EVM:** Ethereum, Arbitrum, Base, Polygon, Avalanche, BNB Chain.
  - **Non-EVM:** Solana (SPL/ATA derivation), Stellar (StrKey & strict memo boundaries), Aptos (Move/BCS validation), and NEAR Protocol (named/implicit accounts).
- **Dual Execution Modes:**
  - **Simulation Mode:** Deterministic fixture pricing and local state machine simulation for integration testing.
  - **Live Mode:** Production quotes, real-time solver liquidity, and on-chain deposit monitoring.

### 📊 Source-Attributed Telemetry & Intelligence
- **DeFiLlama Live Ingestion:** Automated ingestion of TVL history, 24-hour DEX volume, stablecoin supply, protocol fees, and annualized revenue.
- **Data Provenance:** Every metric record includes source URLs, retrieval timestamps, methodology versioning, and an immutable SHA-256 payload checksum.
- **Export Engine:** Snapshot export to CSV and JSON formats preserving provenance headers.

### 📈 Trading Intelligence & Setups
- **Evidence-Based Ranking:** Bullish and bearish market setups scored across market structure, order book liquidity, and derivatives.
- **Transparent Zones:** Published Order Block (OB), Break of Structure (BOS), Change of Character (CHoCH), and Fair Value Gap (FVG) detection rules.

### 🔬 Scenario Sensitivity Calculator
- **Interactive Modeling:** Sensitivity calculators modeling fee capture and annual protocol revenue across Bear, Base, and Bull market regimes using exact formulas:
$$\text{Annual Fee Capture} = \text{Daily Volume} \times (1 + \text{Growth Rate}) \times \text{Fee Rate} \times \text{Capture Rate} \times 365$$

### 🛡️ Risk & Circuit Breaker Engine
- **Continuous Health Checks:** Real-time monitoring of slippage limits (default warning at 200 bps, hard limit at 500 bps), token catalogue freshness, and RPC node synchronization status.

---

## 4. Technology Stack

| Layer | Technology | Purpose |
|---|---|---|
| **Frontend Framework** | **Next.js 16** (App Router, Turbopack) | High-performance React 19 UI, server-side rendering, and API proxying. |
| **Styling & 3D** | **Tailwind CSS**, **Three.js** | Custom night-valley design system, WebGL procedural hero, and CSS bento grids. |
| **Backend Core** | **Go 1.26** (`cmd/api`, `cmd/worker`) | High-concurrency compiled backend, domain state machine, and async polling worker. |
| **HTTP Routing** | **chi/v5** | Minimal, zero-allocation HTTP routing with standardized middleware chains. |
| **Database** | **PostgreSQL 16** via **pgx/v5** | Relational storage with reversible goose migrations and `NUMERIC(78, 0)` precision. |
| **Caching & Locks** | **Redis 7** via **go-redis/v9** | Distributed locking (`SET NX PX`), token catalogue caching, and rate limiting. |
| **Cryptography** | **Ed25519**, **Base58**, **SHA-256** | Quote signature validation, address checksums (EIP-55, Base58, StrKey). |
| **Observability** | **slog** (Structured JSON) | Correlation IDs (`request_id`, `swap_id`), latency tracking, sensitive data redaction. |
| **Packaging** | **Docker** (Distroless non-root) | Minimal multi-stage builds (`Dockerfile.backend`) with custom healthcheck CLI. |

---

## 5. Repository Structure

```text
├── backend/
│   ├── cmd/
│   │   ├── api/                 # Go HTTP REST API server binary entrypoint
│   │   ├── migrate/             # Database migration CLI (up, down, status, check)
│   │   └── worker/              # Background worker daemon for swap polling
│   ├── internal/
│   │   ├── analytics/           # DeFiLlama connectors & metric observation persistence
│   │   ├── auth/                # Session manager & SIWE/wallet signature validation
│   │   ├── chains/              # Ecosystem drivers (EVM, Solana, Stellar, Aptos, NEAR)
│   │   │   └── registry/        # Data-driven chain capability registry (chains.yaml)
│   │   ├── config/              # Environment config loader with secret redaction
│   │   ├── httpapi/             # Routes, handlers, and security middleware
│   │   ├── research/            # Financial formula models and scenario calculation
│   │   ├── risk/                # Real-time slippage, stale-data, and circuit breaker engine
│   │   ├── storage/             # PostgreSQL (pgx/v5) and Redis client pools
│   │   ├── swap/                # Swap orchestration service, domain state machine, and math
│   │   │   ├── domain/          # Unidirectional state transitions & exact integer math
│   │   │   ├── providers/       # NEAR Intents 1Click adapter & alternative provider chain
│   │   │   └── repository/      # Idempotent swap and quote persistence
│   │   └── wallets/             # Non-custodial unsigned transaction constructors
│   └── migrations/              # Embedded reversible SQL migrations (goose)
├── frontend/
│   ├── src/
│   │   ├── app/                 # Next.js App Router (marketing landing, workspace, proxy)
│   │   ├── components/
│   │   │   ├── landing/         # Bento grid, chain slider, ribbons, and feature modules
│   │   │   └── workspace/       # Modular workspace pages (swap, markets, portfolio, etc.)
│   │   └── lib/                 # API client, decimal math, view-state models, and formatting
│   └── test/                    # Frontend unit, copy, and property tests
├── docs/
│   ├── architecture.md          # Architectural decisions & domain boundaries
│   ├── api-contract.md          # Complete REST API payload specifications
│   ├── runbook.md               # Operations, deployment, backup, and on-call runbook
│   └── decisions/               # Architecture Decision Records (ADR 0001, ADR 0002)
├── scripts/                     # Smoke tests (PowerShell / Bash) and local runners
├── docker-compose.yml           # Production stack (Postgres, Redis, API, Worker, Web)
└── Dockerfile.backend           # Distroless multi-stage container specification
```

---

## 6. Security & Non-Custodial Guarantees

- **Zero Custody:** Private keys never enter the system. The server only generates unsigned calldata; users authorize transactions through their preferred browser wallet.
- **Strict Network Boundaries:** All database and Redis access is parameterized. Outbound RPC and provider endpoints are protected with timeout deadlines and size-bounded streaming (`2MB` max).
- **Log Sanitation:** Structured logs scrub authorization tokens, JWT bearer credentials, private nonces, and cookie payloads.
- **Replay Protection:** Unique constraint enforcement on `(endpoint, idempotency_key, request_payload_hash)` prevents duplicate broadcast execution.

---

## 7. Documentation & References

- [Architecture Overview](docs/architecture.md) — Comprehensive system design and domain boundaries.
- [REST API Contract](docs/api-contract.md) — Endpoint specifications, DTOs, and error code taxonomy.
- [ADR 0002: Production Go Architecture](docs/decisions/0002-backend-architecture.md) — Decision rationale for the Go migration and multi-chain bridge.
- [Operational Runbook](docs/runbook.md) — Deployment instructions, database migrations, disaster recovery, and monitoring guides.
- [Research Methodology](docs/research-methodology.md) — Valuation models and mathematical definitions for scenario calculators.

---

## 8. License

This project is licensed under the [MIT License](LICENSE).
