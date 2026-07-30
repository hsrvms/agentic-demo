-- 005_usage: Usage tracking tables for audit trail and aggregated dashboards.

-- usage_events: raw audit trail of every LLM call, tool invocation, and embedding operation.
CREATE TABLE IF NOT EXISTS usage_events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  TEXT NOT NULL,
    event_type TEXT NOT NULL,  -- 'llm_usage' | 'tool_usage' | 'embedding_usage'
    payload    JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_usage_events_tenant_time ON usage_events (tenant_id, created_at DESC);

-- usage_daily: hourly roll-ups for fast dashboard queries and budget enforcement.
-- One row per tenant per date per model. Upserted by the UsageCollector.
CREATE TABLE IF NOT EXISTS usage_daily (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           TEXT NOT NULL,
    date                DATE NOT NULL,
    llm_model           TEXT NOT NULL,
    input_tokens        BIGINT NOT NULL DEFAULT 0,
    output_tokens       BIGINT NOT NULL DEFAULT 0,
    tool_calls          INTEGER NOT NULL DEFAULT 0,
    embedding_tokens    BIGINT NOT NULL DEFAULT 0,
    estimated_cost_usd  NUMERIC(12,6) NOT NULL DEFAULT 0,
    reports_generated   INTEGER NOT NULL DEFAULT 0,
    UNIQUE (tenant_id, date, llm_model)
);

CREATE INDEX idx_usage_daily_tenant_date ON usage_daily (tenant_id, date DESC);