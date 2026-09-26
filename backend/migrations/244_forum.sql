CREATE TABLE forum_settings (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    muted_until TIMESTAMPTZ,
    retention_days INTEGER NOT NULL DEFAULT 30 CHECK (retention_days BETWEEN 0 AND 3650),
    last_message_id BIGINT NOT NULL DEFAULT 0
);
INSERT INTO forum_settings (id) VALUES (1);

CREATE TABLE forum_members (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    muted_until TIMESTAMPTZ,
    last_post_at TIMESTAMPTZ
);

CREATE TABLE forum_messages (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    content TEXT NOT NULL CHECK (char_length(content) BETWEEN 1 AND 2000),
    rank_at_send INTEGER NOT NULL DEFAULT 0 CHECK (rank_at_send BETWEEN 0 AND 20),
    pinned BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX forum_messages_created_at_idx ON forum_messages(created_at);
CREATE INDEX forum_messages_pinned_idx ON forum_messages(id DESC) WHERE pinned;
CREATE INDEX forum_messages_event_idx ON forum_messages(id) WHERE rank_at_send > 0;
