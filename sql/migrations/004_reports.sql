-- 004_reports.sql
-- Workstream 3: Persist generated reports for retrieval and delivery.

CREATE TABLE IF NOT EXISTS reports (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type          text NOT NULL,  -- 'daily' | 'weekly' | 'monthly' | 'on_demand'
    title         text NOT NULL,
    content       text NOT NULL,  -- markdown or HTML
    citations     jsonb NOT NULL DEFAULT '[]',
    focus         text,
    schedule_id   uuid REFERENCES report_schedules(id) ON DELETE SET NULL,
    generated_at  timestamptz NOT NULL DEFAULT now(),
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_reports_tenant_date ON reports (tenant_id, generated_at DESC);
CREATE INDEX IF NOT EXISTS idx_reports_schedule_id ON reports (schedule_id) WHERE schedule_id IS NOT NULL;
