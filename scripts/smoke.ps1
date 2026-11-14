param(
    [string]$BaseUrl = "http://127.0.0.1:8081"
)

# End-to-end smoke test for the Go API in demo mode (Windows PowerShell 5.1+).
$ErrorActionPreference = "Stop"
$script:Step = 0

function Invoke-Step {
    param([string]$Name, [string]$Path, [string]$Method = "GET", [hashtable]$Headers = @{}, [string]$Body = $null)
    $script:Step++
    Write-Host -NoNewline "$script:Step. $Name... "
    $params = @{ Uri = "$BaseUrl$Path"; Method = $Method; Headers = $Headers; UseBasicParsing = $true }
    if ($Body) { $params.Body = $Body; $params.ContentType = "application/json" }
    try {
        $resp = Invoke-RestMethod @params
        Write-Host "OK"
        return $resp
    } catch {
        Write-Host "FAILED: $_"
        exit 1
    }
}

function Assert-Equal($Actual, $Expected, $Label) {
    if ($Actual -ne $Expected) {
        Write-Host "FAILED: $Label is '$Actual', expected '$Expected'"
        exit 1
    }
}

Write-Host "============================================="
Write-Host "E2E smoke test against $BaseUrl"
Write-Host "============================================="

Invoke-Step "GET /healthz" "/healthz" | Out-Null
Invoke-Step "GET /readyz" "/readyz" | Out-Null
$health = Invoke-Step "GET /api/v1/health" "/api/v1/health"
$chains = Invoke-Step "GET /api/v1/chains" "/api/v1/chains"
if ($chains.chains.Count -lt 1) { Write-Host "FAILED: no chains"; exit 1 }
Invoke-Step "GET /api/v1/assets?chain=eth" "/api/v1/assets?chain=eth" | Out-Null
Invoke-Step "GET /api/v1/research/scenarios" "/api/v1/research/scenarios" | Out-Null

$quote = Invoke-Step "POST /api/v1/quotes" "/api/v1/quotes" "POST" @{} '{"originAssetId":"demo:ethereum:usdc","destinationAssetId":"demo:base:usdc","amountIn":"25"}'
$quoteId = $quote.quote.id

$key = "smoke-" + [guid]::NewGuid().ToString("N")
$created = Invoke-Step "POST /api/v1/swaps" "/api/v1/swaps" "POST" @{ "Idempotency-Key" = $key } "{`"quoteId`":`"$quoteId`"}"
$swapId = $created.swap.id
$replay = Invoke-Step "replay POST /api/v1/swaps" "/api/v1/swaps" "POST" @{ "Idempotency-Key" = $key } "{`"quoteId`":`"$quoteId`"}"
Assert-Equal $replay.swap.id $swapId "replayed swap id"
Assert-Equal $replay.created $false "replay created flag"

$hash = "0x" + ([guid]::NewGuid().ToString("N") + [guid]::NewGuid().ToString("N"))
$deposit = Invoke-Step "POST /api/v1/swaps/{id}/deposit" "/api/v1/swaps/$swapId/deposit" "POST" @{} "{`"txHash`":`"$hash`",`"chainId`":`"1`"}"
Assert-Equal $deposit.swap.state "DEPOSIT_SUBMITTED" "state after deposit"

foreach ($want in @("PROCESSING", "SUCCEEDED")) {
    $sim = Invoke-Step "POST /api/v1/swaps/{id}/simulate -> $want" "/api/v1/swaps/$swapId/simulate" "POST" @{} "{}"
    Assert-Equal $sim.swap.state $want "simulated state"
    Assert-Equal $sim.swap.realSettlement $false "realSettlement"
}

Invoke-Step "GET /api/v1/swaps/{id}" "/api/v1/swaps/$swapId" | Out-Null
Invoke-Step "GET /api/v1/activity" "/api/v1/activity" | Out-Null
$risk = Invoke-Step "GET /api/v1/risk/status" "/api/v1/risk/status"
Write-Host "Risk status: $($risk.status)"

Write-Host "============================================="
Write-Host "All E2E smoke tests passed."
Write-Host "============================================="
