#!/usr/bin/env bash
# Manual test script for auth + RLS feature.
#
# Prerequisites:
#   1. Server running: go run ./cmd/server
#   2. Migrations applied: migrate -path sql/migrations -database "$DATABASE_URL" up
#   3. jq installed (for JSON parsing)
#
# Usage:
#   ./scripts/test-auth.sh

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:3000}"
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m'

pass=0
fail=0

check() {
    local label="$1"
    local expected_code="$2"
    local actual_code="$3"

    if [ "$actual_code" = "$expected_code" ]; then
        echo -e "${GREEN}✓ PASS${NC}: $label (HTTP $actual_code)"
        pass=$((pass + 1))
    else
        echo -e "${RED}✗ FAIL${NC}: $label (expected HTTP $expected_code, got HTTP $actual_code)"
        fail=$((fail + 1))
    fi
}

echo "=============================================="
echo " Auth + Tenant API Manual Test"
echo " Server: $BASE_URL"
echo "=============================================="
echo

# 1. Health check
echo -e "${YELLOW}--- Health Check ---${NC}"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/health")
check "Health endpoint" "200" "$HTTP_CODE"
echo

# 2. Register a user
echo -e "${YELLOW}--- Register User ---${NC}"
REGISTER_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/auth/register" \
    -H "Content-Type: application/json" \
    -d '{"email": "alice@example.com", "password": "securepass123"}')
HTTP_CODE=$(echo "$REGISTER_RESPONSE" | tail -1)
BODY=$(echo "$REGISTER_RESPONSE" | head -n -1)
check "Register new user" "201" "$HTTP_CODE"
echo "Response: $BODY"

TOKEN=$(echo "$BODY" | jq -r '.token // empty')
USER_ID=$(echo "$BODY" | jq -r '.user_id // empty')
echo "Token: ${TOKEN:0:50}..."
echo "User ID: $USER_ID"
echo

# 3. Register duplicate user
echo -e "${YELLOW}--- Register Duplicate ---${NC}"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/api/auth/register" \
    -H "Content-Type: application/json" \
    -d '{"email": "alice@example.com", "password": "anotherpass"}')
check "Reject duplicate email" "409" "$HTTP_CODE"
echo

# 4. Register with weak password
echo -e "${YELLOW}--- Register Weak Password ---${NC}"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/api/auth/register" \
    -H "Content-Type: application/json" \
    -d '{"email": "bob@example.com", "password": "short"}')
check "Reject weak password" "400" "$HTTP_CODE"
echo

# 5. Login
echo -e "${YELLOW}--- Login ---${NC}"
LOGIN_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"email": "alice@example.com", "password": "securepass123"}')
HTTP_CODE=$(echo "$LOGIN_RESPONSE" | tail -1)
BODY=$(echo "$LOGIN_RESPONSE" | head -n -1)
check "Login with valid credentials" "200" "$HTTP_CODE"
TOKEN=$(echo "$BODY" | jq -r '.token // empty')
echo "New token: ${TOKEN:0:50}..."
echo

# 6. Login with wrong password
echo -e "${YELLOW}--- Login Wrong Password ---${NC}"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/api/auth/login" \
    -H "Content-Type: application/json" \
    -d '{"email": "alice@example.com", "password": "wrongpassword"}')
check "Reject wrong password" "401" "$HTTP_CODE"
echo

# 7. Auth rejection tests
echo -e "${YELLOW}--- Auth Rejection ---${NC}"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/auth/me")
check "Reject missing auth token" "401" "$HTTP_CODE"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/auth/me" \
    -H "Authorization: Bearer invalid-token" \
    -H "X-Tenant-ID: t-fake")
check "Reject invalid token" "401" "$HTTP_CODE"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/auth/me" \
    -H "Authorization: Bearer $TOKEN")
check "Reject missing X-Tenant-ID" "400" "$HTTP_CODE"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/auth/me" \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Tenant-ID: t-nonexistent")
check "Reject nonexistent tenant" "404" "$HTTP_CODE"
echo

# 8. Create first tenant (auth-only, no X-Tenant-ID required)
echo -e "${YELLOW}--- Create First Tenant ---${NC}"
CREATE_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/tenants" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name": "Acme Corp"}')
HTTP_CODE=$(echo "$CREATE_RESPONSE" | tail -1)
BODY=$(echo "$CREATE_RESPONSE" | head -n -1)
check "Create first tenant" "201" "$HTTP_CODE"
echo "Response: $BODY"

TENANT_ID=$(echo "$BODY" | jq -r '.id // empty')
echo "Tenant ID: $TENANT_ID"
echo

# 9. Reject empty tenant name
echo -e "${YELLOW}--- Reject Empty Name ---${NC}"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/api/tenants" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name": ""}')
check "Reject empty tenant name" "400" "$HTTP_CODE"
echo

# 10. Access /me with valid token + tenant (tenant-scoped)
echo -e "${YELLOW}--- Protected Endpoints ---${NC}"
ME_RESPONSE=$(curl -s -w "\n%{http_code}" "$BASE_URL/api/auth/me" \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Tenant-ID: $TENANT_ID")
HTTP_CODE=$(echo "$ME_RESPONSE" | tail -1)
BODY=$(echo "$ME_RESPONSE" | head -n -1)
check "Access /me with valid auth" "200" "$HTTP_CODE"
echo "Response: $BODY"
echo

# 11. List tenants (auth-only)
echo -e "${YELLOW}--- List Tenants ---${NC}"
TENANTS_RESPONSE=$(curl -s -w "\n%{http_code}" "$BASE_URL/api/tenants" \
    -H "Authorization: Bearer $TOKEN")
HTTP_CODE=$(echo "$TENANTS_RESPONSE" | tail -1)
BODY=$(echo "$TENANTS_RESPONSE" | head -n -1)
check "List tenants" "200" "$HTTP_CODE"
TENANT_COUNT=$(echo "$BODY" | jq 'length')
echo "Tenant count: $TENANT_COUNT"
echo "Response: $BODY"
echo

# 12. Create a second tenant
echo -e "${YELLOW}--- Create Second Tenant ---${NC}"
CREATE_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/tenants" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name": "Beta Inc"}')
HTTP_CODE=$(echo "$CREATE_RESPONSE" | tail -1)
BODY=$(echo "$CREATE_RESPONSE" | head -n -1)
check "Create second tenant" "201" "$HTTP_CODE"
echo "Response: $BODY"
echo

# 13. List tenants again (should have 2)
echo -e "${YELLOW}--- List After Create ---${NC}"
TENANTS_RESPONSE=$(curl -s -w "\n%{http_code}" "$BASE_URL/api/tenants" \
    -H "Authorization: Bearer $TOKEN")
HTTP_CODE=$(echo "$TENANTS_RESPONSE" | tail -1)
BODY=$(echo "$TENANTS_RESPONSE" | head -n -1)
check "List tenants after create" "200" "$HTTP_CODE"
TENANT_COUNT=$(echo "$BODY" | jq 'length')
echo "Tenant count: $TENANT_COUNT"
echo "Response: $BODY"
echo

# 14. Cross-tenant access: user not member of a random tenant
echo -e "${YELLOW}--- Cross-Tenant Isolation ---${NC}"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/auth/me" \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Tenant-ID: t-someone-else")
check "Reject non-member tenant access" "404" "$HTTP_CODE"
echo

# 15. Register a second user and verify they can't access first user's tenant
echo -e "${YELLOW}--- Multi-User Isolation ---${NC}"
REGISTER2_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/auth/register" \
    -H "Content-Type: application/json" \
    -d '{"email": "bob@example.com", "password": "securepass123"}')
TOKEN2=$(echo "$REGISTER2_RESPONSE" | head -n -1 | jq -r '.token // empty')

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/auth/me" \
    -H "Authorization: Bearer $TOKEN2" \
    -H "X-Tenant-ID: $TENANT_ID")
check "Reject non-member user access to other tenant" "403" "$HTTP_CODE"
echo

# Summary
echo "=============================================="
echo -e " Results: ${GREEN}$pass passed${NC}, ${RED}$fail failed${NC}"
echo "=============================================="

if [ "$fail" -gt 0 ]; then
    exit 1
fi
