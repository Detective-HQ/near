# Production deployment

Three services and one database:

| Component | Image / build | Notes |
|---|---|---|
| API | `Dockerfile.backend`, entrypoint `/bin/api` | Port 8081. Health: `/healthz` (liveness), `/readyz` (DB check) |
| Worker | `Dockerfile.backend`, entrypoint `/bin/worker` | No port. Polls swaps, refreshes the catalogue, records trading signals |
| Migrations | `Dockerfile.backend`, entrypoint `/bin/migrate up` | Run once per release, before the API/worker start |
| Web | `Dockerfile` (Next.js) | Port 3000. Proxies `/api/*` to `UDI_API_ORIGIN` |
| Database | Managed PostgreSQL 16+ | TLS required |

All configuration is environment variables. The full list with production values is in `backend/.env.production.example`.

## 1. Create the cloud database

Any managed PostgreSQL works. Create a database and copy two connection strings:

- **Direct URL**: used only by `migrate` (DDL and advisory locks need a real session).
- **Pooled URL**: used by the API and worker. It's optional, but recommended on serverless databases.

| Provider | Direct | Pooled | Set |
|---|---|---|---|
| Neon | `ep-xxx.region.aws.neon.tech` | `ep-xxx-pooler.region.aws.neon.tech` | `DB_PGBOUNCER=true` with the pooled host |
| Supabase | port `5432` | port `6543` (transaction mode) | `DB_PGBOUNCER=true` with port 6543 |
| AWS RDS / Railway / Render / Fly Postgres | the given URL | the same URL | `DB_PGBOUNCER=false` |

Append `?sslmode=require` to both URLs. In `APP_ENV=production` the API and worker refuse to start with `sslmode=disable`, `allow`, `prefer`, or no `sslmode` at all. `verify-full` with the provider CA is stronger where supported.

## 2. Run migrations

```bash
DATABASE_URL="<direct url>?sslmode=require" APP_ENV=production /bin/migrate up
DATABASE_URL="<direct url>?sslmode=require" APP_ENV=production /bin/migrate status
```

The API checks the schema at startup and refuses to serve if any migration is pending, so a forgotten migration fails fast instead of failing on the first request. Roll back one step with `migrate down`, or to a version with `migrate down-to N`.

## 3. Deploy API and worker

Set on both:

```
APP_ENV=production
DATABASE_URL=<pooled url>?sslmode=require
DB_PGBOUNCER=true            # only for a PgBouncer pooled URL
MARKET_DATA_MODE=live        # real market data + trading analytics
SWAP_QUOTE_MODE=demo         # switch to live once ONECLICK_JWT is set
```

API only:

```
HTTP_ADDR=:8081
TRUSTED_PROXY_CIDRS=<your platform's proxy/load-balancer network>
AUTH_REQUIRED=true|false
```

- Keep the API private if your host allows it, so only the web service can reach it. Otherwise put it behind your platform's TLS load balancer.
- Run the worker as exactly one instance to start with. Swap claiming is multi-worker safe (`FOR UPDATE SKIP LOCKED`), so you can scale later.
- Sizing: the API is stateless; scale it horizontally. Keep `DB_MAX_CONNS × (API instances + worker instances)` below your database's connection limit, or use the pooled URL.

## 4. Deploy the web app

```
UDI_API_ORIGIN=https://<api internal or public URL>
NEXT_PUBLIC_USE_MOCKS=false
```

The browser only ever calls the web app; `/api/*` is proxied server-side, so no CORS setting is needed in that setup.

## 5. Enable live swaps (when ready)

1. Get a NEAR Intents 1Click partner JWT and set `ONECLICK_JWT` (server-side only).
2. Set `SWAP_QUOTE_MODE=live`. Dry quotes now come from the provider.
3. Set `ENABLE_LIVE_EXECUTION=true` only after reviewing the swap flow on a small amount. The config refuses this without `ONECLICK_JWT` and live quote mode.
4. Optionally set `ONECLICK_QUOTE_PUBLIC_KEY` so quote signatures are verified.

## 6. Verify the launch

```bash
bash scripts/smoke.sh https://<api-url>     # demo-mode end-to-end check
curl -s https://<api-url>/readyz            # {"checks":{"database":"ok"},...}
curl -s https://<api-url>/api/v1/health     # modes, executionEnabled, authRequired
```

## Render

| Render resource | Settings |
|---|---|
| PostgreSQL | Create it in the same region as the services. Use the **internal** URL with `DB_REQUIRE_TLS=false` (private network), or the external URL with `?sslmode=require`. |
| Web Service (API) | Runtime Docker, Dockerfile `Dockerfile.backend`, Docker command `/bin/api`, health check path `/healthz`, **pre-deploy command** `/bin/migrate up`. Paid instance (free instances sleep). |
| Background Worker | Same repo and Dockerfile, Docker command `/bin/worker`. Paid plan (required for workers). |
| Web Service (frontend) | Dockerfile `Dockerfile`, `UDI_API_ORIGIN=http://<api-service>:8081` (Render private network) or the API's public URL. |

Set `TRUSTED_PROXY_CIDRS=10.0.0.0/8` on the API, then check that the `clientIp` field in the API logs shows real visitor addresses. If it shows the same internal address for everyone, widen or adjust the range to cover Render's proxy addresses.

## Monitoring

- `GET /metrics` (Prometheus) on the API; set `METRICS_TOKEN` and scrape with `Authorization: Bearer <token>`. The worker serves the same at `WORKER_METRICS_ADDR` when set.
- Key series:
  - `udi_http_requests_total` / `udi_http_request_duration_seconds` (per route pattern)
  - `udi_provider_calls_total{outcome}`
  - `udi_swap_transitions_total{state}`
  - `udi_swaps_stuck_total` (alert when > 0)
  - `udi_trading_refresh_duration_seconds{result}`
- Without a Prometheus server, use the platform's log search on `level=ERROR` and the `/readyz` health check.

## Operations

- **Backups:** enable the provider's point-in-time recovery. The `swaps`, `swap_events`, `swap_transactions` and `trading_signals` tables are the audit trail; never truncate them.
- **Secrets:** `ONECLICK_JWT` and `DATABASE_URL` belong in the platform's secret store. The services never log them (config output is redacted).
- **Rollback:** deploy the previous image. If its schema is older, run `migrate down-to <version>` first; every migration is reversible.
- **Health:** alert on `/readyz` failures and on worker logs `swap exceeded max poll age` (swaps needing manual review).
