-- Report persistence queries.

-- name: CreateReport :one
INSERT INTO reports (tenant_id, type, title, content, citations, focus, schedule_id, generated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, tenant_id, type, title, content, citations, focus, schedule_id, generated_at, created_at;

-- name: GetReportByID :one
SELECT id, tenant_id, type, title, content, citations, focus, schedule_id, generated_at, created_at
FROM reports
WHERE id = $1;

-- name: ListReportsByTenant :many
SELECT id, tenant_id, type, title, content, citations, focus, schedule_id, generated_at, created_at
FROM reports
WHERE tenant_id = $1
ORDER BY generated_at DESC
LIMIT $2 OFFSET $3;

-- name: CountReportsByTenant :one
SELECT COUNT(*)::int
FROM reports
WHERE tenant_id = $1;

-- name: DeleteReport :exec
DELETE FROM reports
WHERE id = $1;

-- name: ListReportsBySchedule :many
SELECT id, tenant_id, type, title, content, citations, focus, schedule_id, generated_at, created_at
FROM reports
WHERE schedule_id = $1
ORDER BY generated_at DESC;
