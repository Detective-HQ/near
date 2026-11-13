param(
    [string]$BaseUrl = "http://127.0.0.1:8081"
)

$ErrorActionPreference = "Stop"

Write-Host "============================================="
Write-Host "Running E2E Smoke Test against $BaseUrl"
Write-Host "============================================="

function Test-Endpoint {
    param([string]$Name, [string]$Url, [string]$Method = "GET", [hashtable]$Headers = @{}, [string]$Body = $null)
    Write-Host -NoNewline "$Name... "
    $params = @{
        Uri = $Url
        Method = $Method
        Headers = $Headers
    }
    if ($Body) {
        $params.Body = $Body
        $params.ContentType = "application/json"
    }
    try {
        $resp = Invoke-RestMethod @params
        Write-Host "OK"
        return $resp
    } catch {
        Write-Host "FAILED: $_"
        exit 1
    }
}

# 1. /healthz
$null = Test-Endpoint -Name "1. Checking /healthz" -Url "$BaseUrl/healthz"

# 2. /readyz
$null = Test-Endpoint -Name "2. Checking /readyz" -Url "$BaseUrl/readyz"

# 3. /api/v1/health
$null = Test-Endpoint -Name "3. Checking /api/v1/health" -Url "$BaseUrl/api/v1/health"

# 4. /api/v1/chains
$null = Test-Endpoint -Name "4. Checking /api/v1/chains" -Url "$BaseUrl/api/v1/chains"

# 5. /api/v1/assets
$null = Test-Endpoint -Name "5. Checking /api/v1/assets?chain=near" -Url "$BaseUrl/api/v1/assets?chain=near"

# 6. /api/v1/quotes
$quoteBody = @{
    originAssetId = "demo:near:near"
    destinationAssetId = "demo:base:usdc"
    amountIn = "10"
} | ConvertTo-Json
$quote = Test-Endpoint -Name "6. Creating quote via POST /api/v1/quotes" -Url "$BaseUrl/api/v1/quotes" -Method "POST" -Body $quoteBody
$quoteId = $quote.id
if (-not $quoteId) {
    Write-Host "FAILED: quoteId not found"
    exit 1
}

# 7. /api/v1/swaps
$idempKey = "smoke-idemp-" + [System.Guid]::NewGuid().ToString()
$swapBody = @{ quoteId = $quoteId } | ConvertTo-Json
$swap = Test-Endpoint -Name "7. Creating swap via POST /api/v1/swaps" -Url "$BaseUrl/api/v1/swaps" -Method "POST" -Headers @{ "Idempotency-Key" = $idempKey } -Body $swapBody
$swapId = $swap.id
if (-not $swapId) {
    Write-Host "FAILED: swapId not found"
    exit 1
}

# 8. /api/v1/swaps/{id}/deposit
$depBody = @{
    txHash = "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
    chainId = "8453"
} | ConvertTo-Json
$null = Test-Endpoint -Name "8. Submitting deposit via POST /api/v1/swaps/$swapId/deposit" -Url "$BaseUrl/api/v1/swaps/$swapId/deposit" -Method "POST" -Body $depBody

# 9. /api/v1/swaps/{id}
$swapState = Test-Endpoint -Name "9. Verifying swap state via GET /api/v1/swaps/$swapId" -Url "$BaseUrl/api/v1/swaps/$swapId"
if ($swapState.state -ne "DEPOSIT_SUBMITTED") {
    Write-Host "FAILED: Expected DEPOSIT_SUBMITTED, got $($swapState.state)"
    exit 1
}

# 10. /api/v1/swaps/{id}/simulate
$null = Test-Endpoint -Name "10. Advancing simulation via POST /api/v1/swaps/$swapId/simulate" -Url "$BaseUrl/api/v1/swaps/$swapId/simulate" -Method "POST"

# 11. /api/v1/activity
$null = Test-Endpoint -Name "11. Checking activity via GET /api/v1/activity" -Url "$BaseUrl/api/v1/activity"

# 12. /api/v1/risk/status
$risk = Test-Endpoint -Name "12. Checking risk status via GET /api/v1/risk/status" -Url "$BaseUrl/api/v1/risk/status"
if ($risk.status -ne "HEALTHY") {
    Write-Host "FAILED: Risk status is $($risk.status)"
    exit 1
}

Write-Host "============================================="
Write-Host "All E2E Smoke Tests PASSED successfully!"
Write-Host "============================================="
