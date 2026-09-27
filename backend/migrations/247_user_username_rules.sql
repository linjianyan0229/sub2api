-- 空昵称仍表示未设置；软删除账号释放昵称。迁移整体在事务中执行，
-- 如有历史重名，唯一索引创建失败会回滚，避免擅自覆盖历史昵称。
LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE;

CREATE FUNCTION enforce_user_username() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    suffix bigint := 0;
BEGIN
    NEW.username := regexp_replace(NEW.username, '^[[:space:]]+|[[:space:]]+$', '', 'g');
    IF NEW.deleted_at IS NOT NULL OR NEW.username = '' THEN
        RETURN NEW;
    END IF;

    -- 所有昵称写入共用事务锁，自动分配“小丑”时也不会被普通改名抢占。
    PERFORM pg_advisory_xact_lock(hashtextextended('users:username', 0));
    IF NEW.role <> 'admin' AND NEW.username = '管理员' THEN
        NEW.username := '小丑';
        WHILE EXISTS (SELECT 1 FROM users WHERE deleted_at IS NULL
                      AND lower(username) = lower(NEW.username) AND id <> NEW.id) LOOP
            suffix := suffix + 1;
            NEW.username := '小丑_' || suffix;
        END LOOP;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER users_username_rules
    BEFORE INSERT OR UPDATE OF username, role, deleted_at ON users
    FOR EACH ROW EXECUTE FUNCTION enforce_user_username();

UPDATE users SET username = regexp_replace(username, '^[[:space:]]+|[[:space:]]+$', '', 'g')
WHERE username <> regexp_replace(username, '^[[:space:]]+|[[:space:]]+$', '', 'g');
UPDATE users SET username = username
WHERE deleted_at IS NULL AND role <> 'admin' AND username = '管理员';

CREATE UNIQUE INDEX users_username_normalized_unique ON users (lower(username))
    WHERE deleted_at IS NULL AND username <> '';
