-- name: CreateTenant :one
INSERT INTO tenants (id, name)
VALUES ($1, $2)
RETURNING id, name, status, settings, created_at, updated_at, monthly_budget_usd;

-- name: GetTenantByID :one
SELECT id, name, status, settings, created_at, updated_at, monthly_budget_usd
FROM tenants
WHERE id = $1;

-- name: ListTenantsByUser :many
SELECT t.id, t.name, t.status, t.settings, t.created_at, t.updated_at, t.monthly_budget_usd
FROM tenants t
JOIN tenant_memberships tm ON tm.tenant_id = t.id
WHERE tm.user_id = $1
ORDER BY t.created_at DESC;

-- name: UpdateTenantStatus :exec
UPDATE tenants
SET status = $2, updated_at = now()
WHERE id = $1;

-- name: DeleteTenant :exec
UPDATE tenants
SET status = 'deleted', updated_at = now()
WHERE id = $1;
