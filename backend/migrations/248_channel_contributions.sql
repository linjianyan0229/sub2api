CREATE TABLE channel_contributions (
    id bigserial PRIMARY KEY,
    user_id bigint REFERENCES users(id) ON DELETE SET NULL,
    site_name varchar(100) NOT NULL,
    base_url varchar(2048) NOT NULL,
    platform varchar(20) NOT NULL CHECK (platform IN ('openai','anthropic','gemini')),
    key_encrypted text NOT NULL,
    requested_group_id bigint REFERENCES groups(id) ON DELETE SET NULL,
    new_group_name varchar(100) NOT NULL DEFAULT '',
    status varchar(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
    review_note varchar(500) NOT NULL DEFAULT '',
    reviewed_by bigint REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at timestamptz,
    group_id bigint REFERENCES groups(id) ON DELETE SET NULL,
    account_id bigint UNIQUE REFERENCES accounts(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW()
);
CREATE INDEX channel_contributions_user_idx ON channel_contributions(user_id,id DESC);
CREATE INDEX channel_contributions_pending_idx ON channel_contributions(id DESC) WHERE status='pending';
CREATE INDEX channel_contributions_group_idx ON channel_contributions(group_id) WHERE status='approved';
