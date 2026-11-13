#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${1:-http://127.0.0.1:8081}"

echo "============================================="
echo "Running E2E Smoke Test against $BASE_URL"
echo "============================================="

# 1. /healthz
echo -n "1. Checking /healthz... "
curl -sf "$BASE_URL/healthz" > /dev/null
echo "OK"

# 2. /readyz
echo -n "2. Checking /readyz... "
curl -sf "$BASE_URL/readyz" > /dev/null
echo "OK"

# 3. /api/v1/health
echo -n "3. Checking /api/v1/health... "
curl -sf "$BASE_URL/api/v1/health" > /dev/null
echo "OK"

# 4. /api/v1/chains
echo -n "4. Checking /api/v1/chains... "
curl -sf "$BASE_URL/api/v1/chains" > /dev/null
echo "OK"

# 5. /api/v1/assets
echo -n "5. Checking /api/v1/assets?chain=near... "
curl -sf "$BASE_URL/api/v1/assets?chain=near" > /dev/null
echo "OK"

# 6. /api/v1/quotes
echo -n "6. Creating quote via POST /api/v1/quotes... "
QUOTE_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/quotes" \
  -H "Content-Type: application/json" \
  -d '{"originAssetId":"demo:near:near","destinationAssetId":"demo:base:usdc","amountIn":"10"}')
QUOTE_ID=$(echo "$QUOTE_RESP" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
if [ -z "$QUOTE_ID" ]; then
  echo "FAILED: could not extract quoteId"
  exit 1
fi
echo "OK (Quote ID: $QUOTE_ID)"

# 7. /api/v1/swaps
IDEMP_KEY="smoke-idemp-$(date +%s)-$RANDOM"
echo -n "7. Creating swap via POST /api/v1/swaps... "
SWAP_RESP=$(curl -sf -X POST "$BASE_URL/api/v1/swaps" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: $IDEMP_KEY" \
  -d "{\"quoteId\":\"$QUOTE_ID\"}")
SWAP_ID=$(echo "$SWAP_RESP" | grep -o '"id":"[^"]*' | cut -d'"' -f4)
if [ -z "$SWAP_ID" ]; then
  echo "FAILED: could not extract swapId"
  exit 1
fi
echo "OK (Swap ID: $SWAP_ID)"

# 8. /api/v1/swaps/{id}/deposit
echo -n "8. Submitting deposit via POST /api/v1/swaps/$SWAP_ID/deposit... "
curl -sf -X POST "$BASE_URL/api/v1/swaps/$SWAP_ID/deposit" \
  -H "Content-Type: application/json" \
  -d '{"txHash":"0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef","chainId":"8453"}' > /dev/null
echo "OK"

# 9. /api/v1/swaps/{id}
echo -n "9. Verifying swap state via GET /api/v1/swaps/$SWAP_ID... "
STATE_RESP=$(curl -sf "$BASE_URL/api/v1/swaps/$SWAP_ID")
if echo "$STATE_RESP" | grep -q "DEPOSIT_SUBMITTED"; then
  echo "OK (State: DEPOSIT_SUBMITTED)"
else
  echo "FAILED: State is not DEPOSIT_SUBMITTED"
  exit 1
fi

# 10. /api/v1/swaps/{id}/simulate
echo -n "10. Advancing simulation via POST /api/v1/swaps/$SWAP_ID/simulate... "
curl -sf -X POST "$BASE_URL/api/v1/swaps/$SWAP_ID/simulate" > /dev/null
echo "OK"

# 11. /api/v1/activity
echo -n "11. Checking activity via GET /api/v1/activity... "
curl -sf "$BASE_URL/api/v1/activity" > /dev/null
echo "OK"

# 12. /api/v1/risk/status
echo -n "12. Checking risk engine status via GET /api/v1/risk/status... "
RISK_RESP=$(curl -sf "$BASE_URL/api/v1/risk/status")
if echo "$RISK_RESP" | grep -q "HEALTHY"; then
  echo "OK (Status: HEALTHY)"
else
  echo "FAILED: Risk status is not HEALTHY"
  exit 1
fi

echo "============================================="
echo "All E2E Smoke Tests PASSED successfully!"
echo "============================================="
