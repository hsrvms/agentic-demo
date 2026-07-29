-- Usage tracking queries.

-- name: CreateUsageEvent :one
INSERT INTO usage_events (tenant_id, event_type, payload)
VALUES ($1, $2, $3)
RETURNING id, tenant_id, event_type, payload, created_at;

-- name: ListUsageEvents :many
SELECT id, tenant_id, event_type, payload, created_at
FROM usage_events
WHERE tenant_id = $1
  AND ($4::timestamptz IS NULL OR created_at >= $4)
  AND ($5::timestamptz IS NULL OR created_at <= $5)
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountUsageEvents :one
SELECT COUNT(*)::int
FROM usage_events
WHERE tenant_id = $1
  AND ($2::timestamptz IS NULL OR created_at >= $2)
  AND ($3::timestamptz IS NULL OR created_at <= $3);

-- name: UpsertUsageDaily :one
INSERT INTO usage_daily (tenant_id, date, llm_model, input_tokens, output_tokens, tool_calls, embedding_tokens, estimated_cost_usd, reports_generated)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (tenant_id, date, llm_model) DO UPDATE SET
    input_tokens        = usage_daily.input_tokens + EXCLUDED.input_tokens,
    output_tokens       = usage_daily.output_tokens + EXCLUDED.output_tokens,
    tool_calls          = usage_daily.tool_calls + EXCLUDED.tool_calls,
    embedding_tokens    = usage_daily.embedding_tokens + EXCLUDED.embedding_tokens,
    estimated_cost_usd  = usage_daily.estimated_cost_usd + EXCLUDED.estimated_cost_usd,
    reports_generated   = usage_daily.reports_generated + EXCLUDED.reports_generated
RETURNING id, tenant_id, date, llm_model, input_tokens, output_tokens, tool_calls, embedding_tokens, estimated_cost_usd, reports_generated;

-- name: GetUsageDailySummary :many
SELECT tenant_id, llm_model, date,
       SUM(input_tokens)::bigint AS input_tokens,
       SUM(output_tokens)::bigint AS output_tokens,
       SUM(tool_calls)::int AS tool_calls,
       SUM(embedding_tokens)::bigint AS embedding_tokens,
       SUM(estimated_cost_usd)::numeric AS estimated_cost_usd,
       SUM(reports_generated)::int AS reports_generated
FROM usage_daily
WHERE tenant_id = $1
  AND ($2::date IS NULL OR date >= $2)
  AND ($3::date IS NULL OR date <= $3)
GROUP BY tenant_id, llm_model, date
ORDER BY date DESC, llm_model;

-- name: CountUsageDaily :one
SELECT COUNT(*)::int
FROM usage_daily
WHERE tenant_id = $1
  AND ($2::date IS NULL OR date >= $2)
  AND ($3::date IS NULL OR date <= $3);