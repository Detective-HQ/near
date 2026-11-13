#!/usr/bin/env bash
# End-to-end smoke test for the Go API in demo mode.
# Usage: scripts/smoke.sh [base-url]   (requires curl and python3)
set -euo pipefail

BASE_URL="${1:-http://127.0.0.1:8081}"
step=0

json() { python3 -c "import sys, json; d=json.load(sys.stdin); print($1)"; }
ok() { echo "OK${1:+ ($1)}"; }
begin() { step=$((step + 1)); echo -n "$step. $1... "; }

echo "============================================="
echo "E2E smoke test against $BASE_URL"
echo "============================================="

begin "GET /healthz";  curl -sf "$BASE_URL/healthz" > /dev/null; ok
begin "GET /readyz";   curl -sf "$BASE_URL/readyz" > /dev/null; ok
begin "GET /api/v1/health"
MODE=$(curl -sf "$BASE_URL/api/v1/health" | json "d['quoteMode']"); ok "quoteMode=$MODE"
begin "GET /api/v1/chains"
CHAINS=$(curl -sf "$BASE_URL/api/v1/chains" | json "len(d['chains'])"); ok "$CHAINS chains"
begin "GET /api/v1/assets?chain=eth (provider alias)"
ASSETS=$(curl -sf "$BASE_URL/api/v1/assets?chain=eth" | json "len(d['assets'])"); ok "$ASSETS assets"
begin "GET /api/v1/research/scenarios"
curl -sf "$BASE_URL/api/v1/research/scenarios" | json "d['scenarios'][1]['projectedAnnualFeesUsd']" > /dev/null; ok

begin "POST /api/v1/quotes"
QUOTE_ID=$(curl -sf -X POST "$BASE_URL/api/v1/quotes" -H "Content-Type: application/json" \
  -d '{"originAssetId":"demo:ethereum:usdc","destinationAssetId":"demo:base:usdc","amountIn":"25"}' \
  | json "d['quote']['id']")
ok "$QUOTE_ID"

IDEMP_KEY="smoke-$(date +%s)-$RANDOM"
begin "POST /api/v1/swaps"
SWAP_ID=$(curl -sf -X POST "$BASE_URL/api/v1/swaps" -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEMP_KEY" -d "{\"quoteId\":\"$QUOTE_ID\"}" | json "d['swap']['id']")
ok "$SWAP_ID"

begin "replay POST /api/v1/swaps (idempotent)"
REPLAY=$(curl -sf -X POST "$BASE_URL/api/v1/swaps" -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEMP_KEY" -d "{\"quoteId\":\"$QUOTE_ID\"}" | json "d['swap']['id'] + ' ' + str(d['created'])")
[ "$REPLAY" = "$SWAP_ID False" ] || { echo "FAILED: replay returned '$REPLAY'"; exit 1; }
ok

begin "POST /api/v1/swaps/{id}/deposit"
TX_HASH="0x$(printf '%064x' "$RANDOM$RANDOM$RANDOM")"
STATE=$(curl -sf -X POST "$BASE_URL/api/v1/swaps/$SWAP_ID/deposit" -H "Content-Type: application/json" \
  -d "{\"txHash\":\"$TX_HASH\",\"chainId\":\"1\"}" | json "d['swap']['state']")
[ "$STATE" = "DEPOSIT_SUBMITTED" ] || { echo "FAILED: state $STATE"; exit 1; }
ok "$STATE"

for want in PROCESSING SUCCEEDED; do
  begin "POST /api/v1/swaps/{id}/simulate -> $want"
  STATE=$(curl -sf -X POST "$BASE_URL/api/v1/swaps/$SWAP_ID/simulate" -H "Content-Type: application/json" -d '{}' \
    | json "d['swap']['state'] + ' ' + str(d['swap']['realSettlement'])")
  [ "$STATE" = "$want False" ] || { echo "FAILED: got '$STATE'"; exit 1; }
  ok
done

begin "cancel after success is rejected"
CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/api/v1/swaps/$SWAP_ID/cancel" -H "Content-Type: application/json" -d '{}')
[ "$CODE" = "409" ] || { echo "FAILED: HTTP $CODE"; exit 1; }
ok

begin "GET /api/v1/swaps/{id}"
EVENTS=$(curl -sf "$BASE_URL/api/v1/swaps/$SWAP_ID" | json "len(d['swap']['events'])"); ok "$EVENTS events"
begin "GET /api/v1/activity"
curl -sf "$BASE_URL/api/v1/activity" | json "len(d['swaps'])" > /dev/null; ok
begin "GET /api/v1/risk/status"
RISK=$(curl -sf "$BASE_URL/api/v1/risk/status" | json "d['status']"); ok "$RISK"

echo "============================================="
echo "All E2E smoke tests passed."
echo "============================================="
