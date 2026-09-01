-- Initial Schema Migration for TertiusEye ITAM SaaS Platform

CREATE TABLE IF NOT EXISTS tenants (
    tenant_id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS devices (
    device_id UUID PRIMARY KEY,
    tenant_id UUID REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    hostname VARCHAR(255),
    hardware_specs JSONB,
    last_sync TIMESTAMP WITH TIME ZONE
);

CREATE TABLE IF NOT EXISTS oauth_tokens (
    token_id UUID PRIMARY KEY,
    tenant_id UUID REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    provider VARCHAR(50) NOT NULL,
    encrypted_dek BYTEA NOT NULL,
    token_ciphertext BYTEA NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS cloud_credentials (
    credential_id UUID PRIMARY KEY,
    tenant_id UUID REFERENCES tenants(tenant_id) ON DELETE CASCADE,
    role_arn VARCHAR(255) NOT NULL,
    external_id VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexing Strategy (LLD Section 5.2)
CREATE INDEX IF NOT EXISTS idx_devices_hardware_specs ON devices USING GIN (hardware_specs);
CREATE INDEX IF NOT EXISTS idx_devices_tenant_id ON devices (tenant_id);

-- Multi-Tenant Row-Level Security (RLS) (LLD Section 5.3)
ALTER TABLE devices ENABLE ROW LEVEL SECURITY;
ALTER TABLE oauth_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE cloud_credentials ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_policy ON devices;
CREATE POLICY tenant_isolation_policy ON devices 
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::UUID);

DROP POLICY IF EXISTS tenant_isolation_policy_tokens ON oauth_tokens;
CREATE POLICY tenant_isolation_policy_tokens ON oauth_tokens 
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::UUID);

DROP POLICY IF EXISTS tenant_isolation_policy_cloud ON cloud_credentials;
CREATE POLICY tenant_isolation_policy_cloud ON cloud_credentials 
    USING (tenant_id = NULLIF(current_setting('app.current_tenant_id', true), '')::UUID);
