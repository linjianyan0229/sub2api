-- 分组上限是全站共享账期内的附加约束，不产生独立余额。
ALTER TABLE shared_quota_windows ADD COLUMN group_limits JSONB NOT NULL DEFAULT '{}';
CREATE TABLE shared_quota_group_usage (
    window_id BIGINT NOT NULL REFERENCES shared_quota_windows(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL CHECK (group_id > 0),
    used NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (used >= 0),
    held NUMERIC(20,8) NOT NULL DEFAULT 0 CHECK (held >= 0),
    PRIMARY KEY (window_id, group_id)
);
-- 保留提交时分组，跨日、修改 Key 分组或停用插件后仍按原记录结算。
ALTER TABLE shared_quota_batch_holds ADD COLUMN group_id BIGINT NOT NULL DEFAULT 0 CHECK (group_id >= 0);
