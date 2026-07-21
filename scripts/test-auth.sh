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

# 7. Access /me without token
echo -e "${YELLOW}--- Auth Rejection ---${NC}"
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/auth/me")
check "Reject missing auth token" "401" "$HTTP_CODE"
echo

# 8. Access /me with invalid token
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/auth/me" \
    -H "Authorization: Bearer invalid-token" \
    -H "X-Tenant-ID: t-fake")
check "Reject invalid token" "401" "$HTTP_CODE"
echo

# 9. Access /me with valid token but missing X-Tenant-ID
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/auth/me" \
    -H "Authorization: Bearer $TOKEN")
check "Reject missing X-Tenant-ID" "400" "$HTTP_CODE"
echo

# 10. Access /me with valid token but nonexistent tenant
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/auth/me" \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Tenant-ID: t-nonexistent")
check "Reject nonexistent tenant" "404" "$HTTP_CODE"
echo

# 11. Access /me with valid token but user not member of tenant
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/api/auth/me" \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Tenant-ID: t-unknown")
check "Reject non-member access" "404" "$HTTP_CODE"
echo

# 12. Create a tenant (need to bypass tenant-scoped auth first)
# We'll create the tenant via a separate registration flow.
# Since /api/tenants requires auth with a tenant, we need to seed
# the first tenant directly or use a bootstrap endpoint.
#
# For now, let's insert directly into the DB using psql.
echo -e "${YELLOW}--- Bootstrap First Tenant ---${NC}"
TENANT_ID="t-$(cat /proc/sys/kernel/random/uuid | cut -c1-8)"
PGDATABASE="${PGDATABASE:-app}"
PGHOST="${PGHOST:-localhost}"
PGPORT="${PGPORT:-5432}"
PGUSER="${PGUSER:-app}"
PGPASSWORD="${PGPASSWORD:-app}"

psql "postgresql://$PGUSER:$PGPASSWORD@$PGHOST:$PGPORT/$PGDATABASE?sslmode=disable" \
    -c "INSERT INTO tenants (id, name) VALUES ('$TENANT_ID', 'Acme Corp') ON CONFLICT DO NOTHING;" 2>/dev/null || true
psql "postgresql://$PGUSER:$PGPASSWORD@$PGHOST:$PGPORT/$PGDATABASE?sslmode=disable" \
    -c "INSERT INTO tenant_memberships (user_id, tenant_id, role) VALUES ('$USER_ID', '$TENANT_ID', 'admin') ON CONFLICT DO NOTHING;" 2>/dev/null || true
echo "Created tenant: $TENANT_ID"
echo

# 13. Access /me with valid token and valid tenant
echo -e "${YELLOW}--- Protected Endpoints ---${NC}"
ME_RESPONSE=$(curl -s -w "\n%{http_code}" "$BASE_URL/api/auth/me" \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Tenant-ID: $TENANT_ID")
HTTP_CODE=$(echo "$ME_RESPONSE" | tail -1)
BODY=$(echo "$ME_RESPONSE" | head -n -1)
check "Access /me with valid auth" "200" "$HTTP_CODE"
echo "Response: $BODY"
echo

# 14. List tenants
TENANTS_RESPONSE=$(curl -s -w "\n%{http_code}" "$BASE_URL/api/tenants" \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Tenant-ID: $TENANT_ID")
HTTP_CODE=$(echo "$TENANTS_RESPONSE" | tail -1)
BODY=$(echo "$TENANTS_RESPONSE" | head -n -1)
check "List tenants" "200" "$HTTP_CODE"
echo "Response: $BODY"
echo

# 15. Create another tenant
CREATE_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "$BASE_URL/api/tenants" \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Tenant-ID: $TENANT_ID" \
    -H "Content-Type: application/json" \
    -d '{"name": "Beta Inc"}')
HTTP_CODE=$(echo "$CREATE_RESPONSE" | tail -1)
BODY=$(echo "$CREATE_RESPONSE" | head -n -1)
check "Create new tenant" "201" "$HTTP_CODE"
echo "Response: $BODY"
echo

# 16. Create tenant with empty name
HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/api/tenants" \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Tenant-ID: $TENANT_ID" \
    -H "Content-Type: application/json" \
    -d '{"name": ""}')
check "Reject empty tenant name" "400" "$HTTP_CODE"
echo

# 17. List tenants again (should have 2+)
TENANTS_RESPONSE=$(curl -s -w "\n%{http_code}" "$BASE_URL/api/tenants" \
    -H "Authorization: Bearer $TOKEN" \
    -H "X-Tenant-ID: $TENANT_ID")
HTTP_CODE=$(echo "$TENANTS_RESPONSE" | tail -1)
BODY=$(echo "$TENANTS_RESPONSE" | head -n -1)
check "List tenants after create" "200" "$HTTP_CODE"
TENANT_COUNT=$(echo "$BODY" | jq 'length')
echo "Tenant count: $TENANT_COUNT"
echo "Response: $BODY"
echo

# Summary
echo "=============================================="
echo -e " Results: ${GREEN}$pass passed${NC}, ${RED}$fail failed${NC}"
echo "=============================================="

if [ "$fail" -gt 0 ]; then
    exit 1
fi
