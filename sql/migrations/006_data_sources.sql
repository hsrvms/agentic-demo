-- 006_data_sources.sql
-- Data Source Registry: per-tenant data source configurations with encrypted credentials.

CREATE TABLE IF NOT EXISTS data_source_configs (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    source_type     text NOT NULL CHECK (source_type IN ('file_upload', 'website', 'crm_hubspot', 'crm_salesforce')),
    name            text NOT NULL,
    config          jsonb NOT NULL DEFAULT '{}',
    credentials     bytea,
    status          text NOT NULL DEFAULT 'inactive' CHECK (status IN ('active', 'inactive', 'error')),
    last_sync_at    timestamptz,
    last_sync_status text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_data_source_configs_tenant ON data_source_configs (tenant_id);
CREATE INDEX idx_data_source_configs_type ON data_source_configs (tenant_id, source_type);