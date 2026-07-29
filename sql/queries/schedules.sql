-- Schedule queries for the scheduling module.

-- name: CreateSchedule :one
INSERT INTO report_schedules (tenant_id, type, cron_expr, focus, format)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, tenant_id, type, cron_expr, focus, format, enabled, created_at, updated_at;

-- name: GetScheduleByID :one
SELECT id, tenant_id, type, cron_expr, focus, format, enabled, created_at, updated_at
FROM report_schedules
WHERE id = $1;

-- name: ListSchedulesByTenant :many
SELECT id, tenant_id, type, cron_expr, focus, format, enabled, created_at, updated_at
FROM report_schedules
WHERE tenant_id = $1
ORDER BY created_at DESC;

-- name: UpdateSchedule :one
UPDATE report_schedules
SET type = $2,
    cron_expr = $3,
    focus = $4,
    format = $5,
    updated_at = now()
WHERE id = $1
RETURNING id, tenant_id, type, cron_expr, focus, format, enabled, created_at, updated_at;

-- name: DeleteSchedule :exec
DELETE FROM report_schedules
WHERE id = $1;

-- name: ToggleSchedule :one
UPDATE report_schedules
SET enabled = NOT enabled,
    updated_at = now()
WHERE id = $1
RETURNING id, tenant_id, type, cron_expr, focus, format, enabled, created_at, updated_at;

-- name: ListAllEnabledSchedules :many
SELECT id, tenant_id, type, cron_expr, focus, format, enabled, created_at, updated_at
FROM report_schedules
WHERE enabled = true
ORDER BY tenant_id, type;