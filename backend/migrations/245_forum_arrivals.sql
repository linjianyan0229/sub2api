-- 上线记录独立于发言。同一登录会话永久去重，刷新和令牌续期不产生新通报。
CREATE TABLE forum_arrivals (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL,
    rank_at_arrival INTEGER NOT NULL CHECK (rank_at_arrival BETWEEN 0 AND 20),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, session_id)
);
CREATE INDEX forum_arrivals_event_idx ON forum_arrivals(id) WHERE rank_at_arrival > 0;
ALTER TABLE forum_settings ADD COLUMN last_arrival_id BIGINT NOT NULL DEFAULT 0;
