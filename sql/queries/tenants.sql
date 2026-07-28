-- Tenant queries for the control plane / tenant module.

-- name: CreateTenant :one
INSERT INTO tenants (id, name)
VALUES ($1, $2)
RETURNING id, name, status, settings, created_at, updated_at;

-- name: GetTenantByID :one
SELECT id, name, status, settings, created_at, updated_at
FROM tenants
WHERE id = $1;

-- name: ListTenantsByUser :many
SELECT t.id, t.name, t.status, t.settings, t.created_at, t.updated_at
FROM tenants t
JOIN tenant_memberships tm ON tm.tenant_id = t.id
WHERE tm.user_id = $1
ORDER BY t.created_at DESC;

-- name: UpdateTenantStatus :exec
UPDATE tenants
SET status = $2, updated_at = now()
WHERE id = $1;
