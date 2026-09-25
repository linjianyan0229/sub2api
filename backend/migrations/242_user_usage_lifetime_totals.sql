-- 累计排行独立于明细日志保留周期；迁移前已清理的明细无法恢复。
-- 回填与触发器在同一迁移事务内完成，阻止并发写入造成漏计或重复累计。
LOCK TABLE usage_logs IN SHARE ROW EXCLUSIVE MODE;

CREATE TABLE user_usage_lifetime_totals (
    user_id BIGINT PRIMARY KEY,
    actual_cost NUMERIC(30, 10) NOT NULL DEFAULT 0,
    tokens BIGINT NOT NULL DEFAULT 0
);

INSERT INTO user_usage_lifetime_totals (user_id, actual_cost, tokens)
SELECT user_id, SUM(actual_cost),
       SUM(input_tokens::bigint + output_tokens + cache_creation_tokens + cache_read_tokens)
FROM usage_logs
GROUP BY user_id;

CREATE INDEX idx_user_usage_lifetime_ranking
    ON user_usage_lifetime_totals (actual_cost DESC, tokens DESC, user_id ASC);

CREATE FUNCTION accumulate_user_usage_after_insert()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO user_usage_lifetime_totals AS totals (user_id, actual_cost, tokens)
    SELECT user_id, SUM(actual_cost),
           SUM(input_tokens::bigint + output_tokens + cache_creation_tokens + cache_read_tokens)
    FROM inserted_usage_logs
    GROUP BY user_id
    ORDER BY user_id
    ON CONFLICT (user_id) DO UPDATE SET
        actual_cost = totals.actual_cost + EXCLUDED.actual_cost,
        tokens = totals.tokens + EXCLUDED.tokens;
    RETURN NULL;
END;
$$;

CREATE TRIGGER usage_logs_lifetime_insert
AFTER INSERT ON usage_logs
REFERENCING NEW TABLE AS inserted_usage_logs
FOR EACH STATEMENT EXECUTE FUNCTION accumulate_user_usage_after_insert();

-- 费用更正按差额累计；删除明细、清理分区不会扣减历史累计量。
CREATE FUNCTION correct_user_usage_after_update()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO user_usage_lifetime_totals AS totals (user_id, actual_cost, tokens)
    SELECT user_id, SUM(actual_cost), SUM(tokens)
    FROM (
        SELECT user_id, actual_cost,
               input_tokens::bigint + output_tokens + cache_creation_tokens + cache_read_tokens AS tokens
        FROM updated_usage_logs
        UNION ALL
        SELECT user_id, -actual_cost,
               -(input_tokens::bigint + output_tokens + cache_creation_tokens + cache_read_tokens) AS tokens
        FROM previous_usage_logs
    ) changes
    GROUP BY user_id
    HAVING SUM(actual_cost) <> 0 OR SUM(tokens) <> 0
    ORDER BY user_id
    ON CONFLICT (user_id) DO UPDATE SET
        actual_cost = totals.actual_cost + EXCLUDED.actual_cost,
        tokens = totals.tokens + EXCLUDED.tokens;
    RETURN NULL;
END;
$$;

CREATE TRIGGER usage_logs_lifetime_update
AFTER UPDATE ON usage_logs
REFERENCING OLD TABLE AS previous_usage_logs NEW TABLE AS updated_usage_logs
FOR EACH STATEMENT EXECUTE FUNCTION correct_user_usage_after_update();
