-- 003_schedules.sql
-- Workstream 2: Report schedules for cron-driven report generation.

CREATE TABLE IF NOT EXISTS report_schedules (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type       text NOT NULL,  -- 'daily' | 'weekly' | 'monthly'
    cron_expr  text NOT NULL,  -- e.g. '0 9 * * *' for daily at 9am
    focus      text,
    format     text NOT NULL DEFAULT 'standard',
    enabled    boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, type)
);

CREATE INDEX IF NOT EXISTS idx_schedules_tenant_id ON report_schedules (tenant_id);
CREATE INDEX IF NOT EXISTS idx_schedules_enabled ON report_schedules (enabled) WHERE enabled = true;