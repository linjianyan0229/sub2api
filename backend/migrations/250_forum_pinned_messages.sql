-- Pins retain a copy of the message, without a foreign key to chat history.
CREATE TABLE IF NOT EXISTS forum_pinned_messages (
    id BIGINT PRIMARY KEY,
    user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    content TEXT NOT NULL CHECK (char_length(content) BETWEEN 1 AND 2000),
    rank_at_send INTEGER NOT NULL DEFAULT 0 CHECK (rank_at_send BETWEEN 0 AND 20),
    created_at TIMESTAMPTZ NOT NULL
);

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_attribute
        WHERE attrelid = 'forum_messages'::regclass
          AND attname = 'pinned' AND NOT attisdropped
    ) THEN
        INSERT INTO forum_pinned_messages (id, user_id, content, rank_at_send, created_at)
        SELECT id, user_id, content, rank_at_send, created_at
        FROM forum_messages WHERE pinned
        ON CONFLICT (id) DO NOTHING;
    END IF;
END $$;

ALTER TABLE forum_messages DROP COLUMN IF EXISTS pinned;
