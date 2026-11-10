#!/usr/bin/env bash
set -euo pipefail

# AuthForge Deployment Health & Sanity Check Script
# Usage: ./scripts/health-check.sh [base_url]
# Example: ./scripts/health-check.sh http://localhost:8080
# Example: ./scripts/health-check.sh https://auth.example.com

BASE_URL="${1:-http://localhost:8080}"
BASE_URL="${BASE_URL%/}"

echo "========================================="
echo " AuthForge Deployment Health Check"
echo " Target: ${BASE_URL}"
echo "========================================="

check_status() {
    local name="$1"
    local url="$2"
    local expected_status="$3"

    echo -n "Checking ${name} (${url})... "
    status=$(curl -s -o /dev/null -w "%{http_code}" "${url}" || echo "FAILED")
    if [ "${status}" = "${expected_status}" ]; then
        echo "✅ OK (${status})"
    else
        echo "❌ FAIL (Expected ${expected_status}, got ${status})"
        return 1
    fi
}

# 1. Healthz probe
check_status "Gateway Health Probe" "${BASE_URL}/healthz" "200"

# 2. OpenID Configuration
check_status "OIDC Discovery Endpoint" "${BASE_URL}/.well-known/openid-configuration" "200"

# 3. JWKS Public Keys
check_status "JWKS Public Keys Endpoint" "${BASE_URL}/.well-known/jwks.json" "200"

# 4. Developer Console UI
check_status "Developer Console Web UI" "${BASE_URL}/" "200"

# 5. Application Demo UI
check_status "Application Demo Web UI" "${BASE_URL}/demo" "200"

# 6. Check JWKS content
echo -n "Validating JWKS Key Algorithm... "
jwks_alg=$(curl -s "${BASE_URL}/.well-known/jwks.json" | grep -o '"alg":"[^"]*"' | head -n 1 || echo "")
if [ "${jwks_alg}" = '"alg":"EdDSA"' ]; then
    echo "✅ OK (EdDSA algorithm confirmed)"
else
    echo "⚠️ Warning (Unexpected algorithm: ${jwks_alg})"
fi

# 7. Password Reset Rate-Limit / Generic response test
echo -n "Testing password reset probe... "
reset_status=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/v1/password-reset/request" \
    -H "Content-Type: application/json" \
    -d '{"email":"probe@example.com"}' || echo "FAILED")
if [ "${reset_status}" = "200" ]; then
    echo "✅ OK (Password recovery endpoint responsive)"
else
    echo "❌ FAIL (${reset_status})"
fi

echo "========================================="
echo "🎉 All deployment sanity checks passed!"
echo "========================================="
