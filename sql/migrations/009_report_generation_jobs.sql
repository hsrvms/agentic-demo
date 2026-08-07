-- 009_report_generation_jobs.sql
-- Track report generation jobs triggered from the web UI so users can see
-- the live status of an on-demand generation (queued / running / succeeded /
-- failed). The task_id links a row to its asynq task for live state lookups;
-- status/error are the durable fallback once the queue record expires.

CREATE TABLE IF NOT EXISTS report_generation_jobs (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    task_id      text NOT NULL UNIQUE,
    report_type  text NOT NULL,  -- 'daily' | 'weekly' | 'monthly' | 'on_demand'
    focus        text NOT NULL DEFAULT '',
    status       text NOT NULL DEFAULT 'queued',  -- 'queued' | 'running' | 'succeeded' | 'failed'
    error        text NOT NULL DEFAULT '',
    enqueued_at  timestamptz NOT NULL DEFAULT now(),
    finished_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_report_generation_jobs_tenant
    ON report_generation_jobs (tenant_id, enqueued_at DESC);
