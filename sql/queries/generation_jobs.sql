-- Report generation job tracking queries.

-- name: CreateGenerationJob :one
INSERT INTO report_generation_jobs (tenant_id, task_id, report_type, focus)
VALUES ($1, $2, $3, $4)
RETURNING id, tenant_id, task_id, report_type, focus, status, error, enqueued_at, finished_at, created_at;

-- name: ListGenerationJobsByTenant :many
SELECT id, tenant_id, task_id, report_type, focus, status, error, enqueued_at, finished_at, created_at
FROM report_generation_jobs
WHERE tenant_id = $1
ORDER BY enqueued_at DESC
LIMIT $2;

-- name: UpdateGenerationJob :exec
UPDATE report_generation_jobs
SET status = $2, error = $3, finished_at = $4
WHERE task_id = $1;
