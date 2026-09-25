-- 账期和冻结记录独立于插件安装记录，停用、卸载后在途任务仍能完成结算。
CREATE TABLE IF NOT EXISTS shared_quota_windows (
    id BIGSERIAL PRIMARY KEY,
    plugin_key VARCHAR(160) NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    quota NUMERIC(20,8) NOT NULL CHECK (quota >= 0),
    used NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (used >= 0),
    held NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (held >= 0),
    UNIQUE (plugin_key, starts_at)
);
CREATE TABLE IF NOT EXISTS shared_quota_batch_holds (
    batch_id VARCHAR(128) PRIMARY KEY,
    window_id BIGINT NOT NULL REFERENCES shared_quota_windows(id),
    amount NUMERIC(20,8) NOT NULL CHECK (amount >= 0),
    settled BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS async_media_billing_snapshots (
    request_key TEXT PRIMARY KEY,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
