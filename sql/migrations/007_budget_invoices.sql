-- 007_budget_invoices: Budget enforcement and invoicing.
-- Adds monthly_budget_usd to tenants and creates the invoices table.

-- Add monthly budget cap to tenants (0 = no budget enforced).
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS monthly_budget_usd NUMERIC(12,6) NOT NULL DEFAULT 0;

-- Invoices: generated monthly billing records.
CREATE TABLE IF NOT EXISTS invoices (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      TEXT NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    period_start   DATE NOT NULL,
    period_end     DATE NOT NULL,
    total_cost_usd NUMERIC(12,6) NOT NULL,
    line_items     JSONB NOT NULL DEFAULT '[]',
    status         TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'issued', 'paid', 'overdue')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_invoices_tenant ON invoices (tenant_id, period_start DESC);