-- Data source registry queries.

-- name: CreateDataSource :one
INSERT INTO data_source_configs (tenant_id, source_type, name, config, credentials, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, tenant_id, source_type, name, config, credentials, status, last_sync_at, last_sync_status, created_at, updated_at;

-- name: GetDataSourceByID :one
SELECT id, tenant_id, source_type, name, config, credentials, status, last_sync_at, last_sync_status, created_at, updated_at
FROM data_source_configs
WHERE id = $1;

-- name: ListDataSourcesByTenant :many
SELECT id, tenant_id, source_type, name, config, credentials, status, last_sync_at, last_sync_status, created_at, updated_at
FROM data_source_configs
WHERE tenant_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountDataSourcesByTenant :one
SELECT COUNT(*)::int
FROM data_source_configs
WHERE tenant_id = $1;

-- name: UpdateDataSource :one
UPDATE data_source_configs
SET name = $2,
    config = $3,
    credentials = $4,
    status = $5,
    updated_at = now()
WHERE id = $1
RETURNING id, tenant_id, source_type, name, config, credentials, status, last_sync_at, last_sync_status, created_at, updated_at;

-- name: DeleteDataSource :exec
DELETE FROM data_source_configs
WHERE id = $1;

-- name: UpdateDataSourceSyncStatus :one
UPDATE data_source_configs
SET last_sync_at = $2,
    last_sync_status = $3,
    status = $4,
    updated_at = now()
WHERE id = $1
RETURNING id, tenant_id, source_type, name, config, credentials, status, last_sync_at, last_sync_status, created_at, updated_at;