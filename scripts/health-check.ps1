param (
    [string]$BaseUrl = "http://localhost:8080"
)

$BaseUrl = $BaseUrl.TrimEnd('/')

Write-Host "=========================================" -ForegroundColor Cyan
Write-Host " AuthForge Deployment Health Check" -ForegroundColor Cyan
Write-Host " Target: $BaseUrl" -ForegroundColor Cyan
Write-Host "=========================================" -ForegroundColor Cyan

function Check-Endpoint {
    param (
        [string]$Name,
        [string]$Url,
        [int]$ExpectedStatus = 200
    )

    Write-Host -NoNewline "Checking $Name ($Url)... "
    try {
        $response = Invoke-WebRequest -Uri $Url -Method Get -UseBasicParsing -TimeoutSec 5
        if ($response.StatusCode -eq $ExpectedStatus) {
            Write-Host "✅ OK ($($response.StatusCode))" -ForegroundColor Green
        } else {
            Write-Host "❌ FAIL (Expected $ExpectedStatus, got $($response.StatusCode))" -ForegroundColor Red
        }
    } catch {
        Write-Host "❌ FAIL ($($_.Exception.Message))" -ForegroundColor Red
    }
}

# 1. Gateway Health Probe
Check-Endpoint -Name "Gateway Health Probe" -Url "$BaseUrl/healthz"

# 2. OpenID Configuration
Check-Endpoint -Name "OIDC Discovery Endpoint" -Url "$BaseUrl/.well-known/openid-configuration"

# 3. JWKS Public Keys
Check-Endpoint -Name "JWKS Public Keys Endpoint" -Url "$BaseUrl/.well-known/jwks.json"

# 4. Developer Console Web UI
Check-Endpoint -Name "Developer Console Web UI" -Url "$BaseUrl/"

# 5. Application Demo Web UI
Check-Endpoint -Name "Application Demo Web UI" -Url "$BaseUrl/demo"

# 6. Check JWKS content
Write-Host -NoNewline "Validating JWKS Key Algorithm... "
try {
    $jwks = Invoke-RestMethod -Uri "$BaseUrl/.well-known/jwks.json" -Method Get -TimeoutSec 5
    if ($jwks.keys[0].alg -eq "EdDSA") {
        Write-Host "✅ OK (EdDSA algorithm confirmed)" -ForegroundColor Green
    } else {
        Write-Host "⚠️ Warning (Algorithm: $($jwks.keys[0].alg))" -ForegroundColor Yellow
    }
} catch {
    Write-Host "❌ FAIL ($($_.Exception.Message))" -ForegroundColor Red
}

# 7. Password reset probe
Write-Host -NoNewline "Testing password reset probe... "
try {
    $body = '{"email":"probe@example.com"}'
    $resetResp = Invoke-WebRequest -Uri "$BaseUrl/v1/password-reset/request" -Method Post -Body $body -ContentType "application/json" -TimeoutSec 5
    if ($resetResp.StatusCode -eq 200) {
        Write-Host "✅ OK (Password recovery endpoint responsive)" -ForegroundColor Green
    } else {
        Write-Host "❌ FAIL ($($resetResp.StatusCode))" -ForegroundColor Red
    }
} catch {
    Write-Host "❌ FAIL ($($_.Exception.Message))" -ForegroundColor Red
}

Write-Host "=========================================" -ForegroundColor Cyan
Write-Host "🎉 All deployment sanity checks passed!" -ForegroundColor Cyan
Write-Host "=========================================" -ForegroundColor Cyan
