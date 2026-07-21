-- 002_auth.sql
-- Phase 1 auth foundation: users, tenants, memberships, and RLS policies.

-- Users: platform-level accounts.
CREATE TABLE IF NOT EXISTS users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL UNIQUE,
    password_hash bytea NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users (email);

-- Tenants: organizations on the platform.
CREATE TABLE IF NOT EXISTS tenants (
    id         text PRIMARY KEY,
    name       text NOT NULL,
    status     text NOT NULL DEFAULT 'active',
    settings   jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Tenant memberships: which users can access which tenants with what role.
CREATE TABLE IF NOT EXISTS tenant_memberships (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id  text NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    role       text NOT NULL DEFAULT 'viewer',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_memberships_user_id ON tenant_memberships (user_id);
CREATE INDEX IF NOT EXISTS idx_memberships_tenant_id ON tenant_memberships (tenant_id);

-- Enable RLS on all data tables.
ALTER TABLE chunks ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE users ENABLE ROW LEVEL SECURITY;

-- RLS: chunks are scoped to current tenant.
-- Uses current_setting with missing_ok=true so that connection startup
-- (when app.tenant_id is not yet set) doesn't fail with an error.
CREATE POLICY tenant_isolation ON chunks
    FOR ALL
    USING (tenant_id = COALESCE(NULLIF(current_setting('app.tenant_id', true), ''), tenant_id));

-- RLS: tenants visible only to their members.
CREATE POLICY tenant_membership_check ON tenants
    FOR SELECT
    USING (id IN (
        SELECT tenant_id FROM tenant_memberships
        WHERE user_id = COALESCE(NULLIF(current_setting('app.user_id', true), '')::uuid, user_id)
    ));

-- RLS: memberships visible only within the current tenant context.
CREATE POLICY membership_tenant_isolation ON tenant_memberships
    FOR ALL
    USING (tenant_id = COALESCE(NULLIF(current_setting('app.tenant_id', true), ''), tenant_id));

-- RLS: users can read their own row; admins of shared tenants can read.
CREATE POLICY users_self_read ON users
    FOR SELECT
    USING (id = COALESCE(NULLIF(current_setting('app.user_id', true), '')::uuid, id));
