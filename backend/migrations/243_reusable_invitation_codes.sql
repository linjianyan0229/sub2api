-- 已有邀请码继续保留单人上限，管理员可按需调大。
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS max_uses INTEGER NOT NULL DEFAULT 1 CHECK (max_uses > 0);
ALTER TABLE redeem_codes ADD COLUMN IF NOT EXISTS used_count INTEGER NOT NULL DEFAULT 0 CHECK (used_count >= 0);

-- 不设级联外键，删除邀请码或用户后仍保留历史和全站 IP 占用。
CREATE TABLE IF NOT EXISTS invitation_usages (
    id BIGSERIAL PRIMARY KEY,
    redeem_code_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    username VARCHAR NOT NULL DEFAULT '',
    email VARCHAR NOT NULL DEFAULT '',
    ip VARCHAR(45) UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (redeem_code_id, user_id)
);
CREATE INDEX IF NOT EXISTS invitation_usages_code_id_idx ON invitation_usages (redeem_code_id, id);

INSERT INTO invitation_usages (redeem_code_id, user_id, username, email, created_at)
SELECT r.id, r.used_by, COALESCE(u.username, ''), COALESCE(u.email, ''), COALESCE(r.used_at, r.created_at)
FROM redeem_codes r LEFT JOIN users u ON u.id = r.used_by
WHERE r.type = 'invitation' AND r.used_by IS NOT NULL
ON CONFLICT (redeem_code_id, user_id) DO NOTHING;

UPDATE redeem_codes SET used_count = 1
WHERE type = 'invitation' AND (used_by IS NOT NULL OR status = 'used');
