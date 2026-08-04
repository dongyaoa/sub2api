CREATE TABLE IF NOT EXISTS grok_video_generation_tasks (
    request_id VARCHAR(160) PRIMARY KEY,
    user_id BIGINT NOT NULL,
    api_key_id BIGINT NOT NULL,
    group_id BIGINT,
    account_id BIGINT NOT NULL,
    subscription_id BIGINT,
    operation VARCHAR(16) NOT NULL DEFAULT 'text',
    model VARCHAR(100) NOT NULL,
    upstream_model VARCHAR(100),
    prompt TEXT NOT NULL DEFAULT '',
    resolution VARCHAR(16),
    aspect_ratio VARCHAR(16),
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    request_payload_hash VARCHAR(64),
    status VARCHAR(24) NOT NULL DEFAULT 'processing',
    http_status INTEGER,
    task_error JSONB,
    last_upstream_error TEXT,
    last_checked_at TIMESTAMPTZ,
    video_url TEXT,
    content_type VARCHAR(100),
    byte_size BIGINT NOT NULL DEFAULT 0,
    browser_playable BOOLEAN NOT NULL DEFAULT FALSE,
    playback_format_version INTEGER NOT NULL DEFAULT 0,
    delivery_error TEXT,
    delivered_at TIMESTAMPTZ,
    billing_status VARCHAR(24) NOT NULL DEFAULT 'pending',
    billing_error TEXT,
    billed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    hidden_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_grok_video_tasks_user_created
    ON grok_video_generation_tasks (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_grok_video_tasks_api_key_created
    ON grok_video_generation_tasks (api_key_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_grok_video_tasks_account_created
    ON grok_video_generation_tasks (account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_grok_video_tasks_status_created
    ON grok_video_generation_tasks (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_grok_video_tasks_billing_created
    ON grok_video_generation_tasks (billing_status, created_at DESC);

COMMENT ON TABLE grok_video_generation_tasks IS
    'Durable Grok video generation, delivery, and billing reconciliation ledger';
