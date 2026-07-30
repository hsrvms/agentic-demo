-- Budget and invoice queries.

-- name: GetTenantBudget :one
SELECT monthly_budget_usd
FROM tenants
WHERE id = $1;

-- name: UpdateTenantBudget :exec
UPDATE tenants
SET monthly_budget_usd = $2, updated_at = now()
WHERE id = $1;

-- name: CreateInvoice :one
INSERT INTO invoices (tenant_id, period_start, period_end, total_cost_usd, line_items, status)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, tenant_id, period_start, period_end, total_cost_usd, line_items, status, created_at;

-- name: GetInvoiceByID :one
SELECT id, tenant_id, period_start, period_end, total_cost_usd, line_items, status, created_at
FROM invoices
WHERE id = $1;

-- name: ListInvoicesByTenant :many
SELECT id, tenant_id, period_start, period_end, total_cost_usd, line_items, status, created_at
FROM invoices
WHERE tenant_id = $1
ORDER BY period_start DESC
LIMIT $2 OFFSET $3;

-- name: CountInvoicesByTenant :one
SELECT COUNT(*)::int
FROM invoices
WHERE tenant_id = $1;