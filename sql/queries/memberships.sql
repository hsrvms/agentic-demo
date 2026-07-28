-- Membership queries for the tenant module.

-- name: CreateMembership :one
INSERT INTO tenant_memberships (user_id, tenant_id, role)
VALUES ($1, $2, $3)
RETURNING id, user_id, tenant_id, role, created_at;

-- name: GetMembership :one
SELECT id, user_id, tenant_id, role, created_at
FROM tenant_memberships
WHERE user_id = $1 AND tenant_id = $2;

-- name: ListMembershipsByTenant :many
SELECT id, user_id, tenant_id, role, created_at
FROM tenant_memberships
WHERE tenant_id = $1
ORDER BY created_at ASC;

-- name: ListMembershipsByUser :many
SELECT id, user_id, tenant_id, role, created_at
FROM tenant_memberships
WHERE user_id = $1
ORDER BY created_at ASC;

-- name: DeleteMembership :exec
DELETE FROM tenant_memberships
WHERE user_id = $1 AND tenant_id = $2;
