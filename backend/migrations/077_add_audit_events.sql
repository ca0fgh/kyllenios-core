CREATE TABLE IF NOT EXISTS audit_events (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    request_id VARCHAR(128) NOT NULL DEFAULT '',
    client_request_id VARCHAR(128) NOT NULL DEFAULT '',
    user_id BIGINT NULL,
    api_key_id BIGINT NULL,
    account_id BIGINT NULL,
    group_id BIGINT NULL,
    platform VARCHAR(32) NOT NULL DEFAULT '',
    request_type SMALLINT NOT NULL DEFAULT 0,
    method VARCHAR(16) NOT NULL DEFAULT '',
    path VARCHAR(255) NOT NULL DEFAULT '',
    inbound_endpoint VARCHAR(128) NOT NULL DEFAULT '',
    upstream_endpoint VARCHAR(128) NOT NULL DEFAULT '',
    upstream_target VARCHAR(255) NOT NULL DEFAULT '',
    status_code INTEGER NOT NULL DEFAULT 0,
    requested_model VARCHAR(128) NOT NULL DEFAULT '',
    effective_model VARCHAR(128) NOT NULL DEFAULT '',
    upstream_model VARCHAR(128) NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    request_hash VARCHAR(80) NOT NULL DEFAULT '',
    response_hash VARCHAR(80) NOT NULL DEFAULT '',
    request_bytes INTEGER NOT NULL DEFAULT 0,
    response_bytes INTEGER NOT NULL DEFAULT 0,
    request_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    response_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    has_tool_calls BOOLEAN NOT NULL DEFAULT FALSE,
    tool_count INTEGER NOT NULL DEFAULT 0,
    tool_calls_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    tool_hashes_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    risk_flags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    risk_level VARCHAR(16) NOT NULL DEFAULT 'low',
    canary_injected BOOLEAN NOT NULL DEFAULT FALSE,
    canary_labels_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    alert_sent_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_events_created_at ON audit_events (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_request_id ON audit_events (request_id);
CREATE INDEX IF NOT EXISTS idx_audit_events_client_request_id ON audit_events (client_request_id);
CREATE INDEX IF NOT EXISTS idx_audit_events_platform_created_at ON audit_events (platform, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_user_created_at ON audit_events (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_account_created_at ON audit_events (account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_risk_level_created_at ON audit_events (risk_level, created_at DESC);
