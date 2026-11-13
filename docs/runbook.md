# Universal DeFi Intelligence — Operational Runbook

## 1. Quick Start

### Local Development (Go Backend + Next.js Frontend)

Requirements: Go 1.26+, Node.js 22+, PostgreSQL 16+, Redis 7+.

```powershell
# 1. Run database migrations
cd backend
go run ./cmd/migrate up

# 2. Start Go API server (Port 8081)
go run ./cmd/api

# 3. Start Background Worker (Separate terminal)
go run ./cmd/worker

# 4. Start Next.js Frontend (Port 3000)
cd ../frontend
npm run dev
```

### Production / Docker Compose Stack

```bash
docker compose up --build -d
```
Services managed:
- `postgres:16-alpine`: Port 5432 with persistent volume `postgres_data`
- `redis:7-alpine`: Port 6379 with persistent volume `redis_data`
- `migrate`: Runs embedded schema migrations to current version and terminates
- `api`: Statically compiled Go API in distroless non-root container (Port 8081)
- `worker`: Background swap status poller and catalogue refresher
- `web`: Next.js web application (Port 3000) connecting to `http://api:8081`

---

## 2. Database Migrations & Recovery

Embedded goose migrations are managed via `backend/cmd/migrate`:

```bash
# Check current migration version and pending files
go run ./cmd/migrate status

# Apply all pending migrations
go run ./cmd/migrate up

# Rollback the last migration
go run ./cmd/migrate down

# Rollback to a specific version (e.g. version 2)
go run ./cmd/migrate down-to 2

# Verify schema drift
go run ./cmd/migrate check
```

### Backup & Restore

```bash
# Create a binary dump of PostgreSQL database
docker compose exec postgres pg_dump -U udi -d udi -F c -f /tmp/udi_backup.dump
docker compose cp postgres:/tmp/udi_backup.dump ./udi_backup.dump

# Restore from a backup
docker compose cp ./udi_backup.dump postgres:/tmp/udi_backup.dump
docker compose exec postgres pg_restore -U udi -d udi -c /tmp/udi_backup.dump
```

---

## 3. Health & Readiness Monitoring

The Go server exposes dedicated health endpoints for Kubernetes and Docker probes:

- **Liveness Probe**: `GET /healthz`
  - Returns `200 OK` (`{"status":"ok"}`) when the server process is responsive.
- **Readiness Probe**: `GET /readyz`
  - Validates PostgreSQL connectivity, Redis availability, and catalogue freshness.
- **Operational Status**: `GET /api/v1/health`
  - Returns current mode (`live` vs `demo`), execution gate status, and data flags.
- **Risk & Circuit Breaker**: `GET /api/v1/risk/status`
  - Reports provider 429/5xx error rates, circuit breaker state (`CLOSED`, `HALF_OPEN`, `OPEN`), catalogue age, and active alerts.

### CLI Healthchecks (For Shell-less Distroless Containers)

```bash
# Test API server health
docker compose exec api /bin/api healthcheck

# Test Worker health (validates DB pool connectivity)
docker compose exec worker /bin/worker healthcheck
```

---

## 4. End-to-End Smoke Testing

Run the automated smoke test suite to verify the entire quote, swap, deposit, and status lifecycle:

```bash
# Linux / macOS / Git Bash:
bash scripts/smoke.sh http://127.0.0.1:8081

# Windows PowerShell:
.\scripts\smoke.ps1 -BaseUrl http://127.0.0.1:8081
```

---

## 5. Configuration Reference

Environment variables configure the runtime safely:

| Variable | Default | Purpose |
| --- | --- | --- |
| `APP_ENV` | `local` | `local`, `staging`, or `production`. |
| `HTTP_ADDR` | `:8081` | Address and port for the Go HTTP API. |
| `DATABASE_URL` | `postgres://udi:udi@localhost:5432/udi` | PostgreSQL connection string. |
| `REDIS_URL` | `redis://localhost:6379` | Redis connection string for caching and distributed locks. |
| `SWAP_QUOTE_MODE` | `live` | `demo` (local simulation) or `live` (1Click provider). |
| `ENABLE_LIVE_EXECUTION` | `false` | Master safety gate for executable quotes and deposit tracking. |
| `ONECLICK_JWT` | empty | Server-side NEAR Intents partner JWT for live execution. |
| `AUTH_REQUIRED` | `false` | When true, enforces wallet session ownership on `/swaps/{id}` and `/activity`. |
| `LOG_LEVEL` | `info` | Logging verbosity (`debug`, `info`, `warn`, `error`). |

---

## 6. Incident Mitigation & On-Call Runbook

### High Provider Error Rate / Circuit Breaker Tripped (`OPEN`)
1. Check `/api/v1/risk/status` to inspect error rate and active alerts.
2. If the provider is returning 429s, increase `SWAP_STATUS_POLL_SECONDS` or temporarily toggle `SWAP_QUOTE_MODE=demo`.
3. The circuit breaker will automatically transition to `HALF_OPEN` and `CLOSED` as provider calls succeed.

### Stale Catalogue Alert
1. Inspect worker logs: `docker compose logs -f worker`.
2. Check if the worker is unable to acquire the distributed lock in Redis.
3. Force a manual catalogue refresh or restart the worker: `docker compose restart worker`.

### In-Flight Shutdown Protection
The API coordinates background accelerator goroutines using `SwapService.Wait(ctx)`. In-flight deposit notifications to the provider are given up to 10 seconds to complete gracefully before the process exits.
