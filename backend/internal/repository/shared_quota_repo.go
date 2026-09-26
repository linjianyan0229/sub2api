package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/pluginapi/sharedquota"
	"github.com/lib/pq"
)

type sharedQuotaRepository struct{ db *sql.DB }

func NewSharedQuotaRepository(db *sql.DB) service.SharedQuotaRepository {
	return &sharedQuotaRepository{db: db}
}

func (r *sharedQuotaRepository) ActivePolicy(ctx context.Context) (*service.SharedQuotaPolicy, error) {
	var p service.SharedQuotaPolicy
	err := r.db.QueryRowContext(ctx, `SELECT p.plugin_key, p.config_encrypted FROM sub2api_plugin_installations p
	JOIN sub2api_plugin_bindings b ON b.plugin_id = p.id
	WHERE b.capability = $1 AND b.platform = 'all' AND b.account_type = 'all' AND b.enabled = TRUE`, sharedquota.Capability).Scan(&p.PluginKey, &p.ConfigEncrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}

const sharedQuotaColumns = `id, quota, used, held, GREATEST(0, quota - used - held), ends_at`

type sharedQuotaQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func ensureSharedQuotaWindow(ctx context.Context, db sharedQuotaQueryer, key string, start, end time.Time, total float64) (*service.SharedQuotaSnapshot, error) {
	return ensureSharedQuotaWindowWithGroups(ctx, db, key, start, end, total, nil)
}

func sharedGroupLimitsJSON(limits map[int64]float64) ([]byte, error) {
	values := map[int64]float64{}
	for id, limit := range limits {
		values[id] = service.QuantizeUsageBillingAmount(limit)
	}
	return json.Marshal(values)
}

func ensureSharedQuotaWindowWithGroups(ctx context.Context, db sharedQuotaQueryer, key string, start, end time.Time, total float64, limits map[int64]float64) (*service.SharedQuotaSnapshot, error) {
	raw, err := sharedGroupLimitsJSON(limits)
	if err != nil {
		return nil, err
	}
	// 并发创建只采用首个账期，查询不会修改已存在账期的额度。
	var w service.SharedQuotaSnapshot
	err = db.QueryRowContext(ctx, `INSERT INTO shared_quota_windows (plugin_key, starts_at, ends_at, quota, group_limits)
	VALUES ($1,$2,$3,$4,$5) ON CONFLICT (plugin_key, starts_at) DO UPDATE SET plugin_key = EXCLUDED.plugin_key
	RETURNING `+sharedQuotaColumns, key, start, end, total, string(raw)).Scan(&w.WindowID, &w.Total, &w.Used, &w.Held, &w.Remaining, &w.ResetsAt)
	return &w, err
}

func currentSharedQuotaWindow(ctx context.Context, db sharedQuotaQueryer, key string, now time.Time) (*service.SharedQuotaSnapshot, error) {
	var w service.SharedQuotaSnapshot
	err := db.QueryRowContext(ctx, `SELECT `+sharedQuotaColumns+` FROM shared_quota_windows
		WHERE plugin_key=$1 AND starts_at <= $2 ORDER BY starts_at DESC LIMIT 1`, key, now).
		Scan(&w.WindowID, &w.Total, &w.Used, &w.Held, &w.Remaining, &w.ResetsAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// 修改刷新时间可能留下旧账期，不能在最新账期结束后重新启用更旧的额度。
	if !now.Before(w.ResetsAt) {
		return nil, nil
	}
	return &w, nil
}

func (r *sharedQuotaRepository) Window(ctx context.Context, policy *service.SharedQuotaPolicy, now, start, end time.Time, total float64) (*service.SharedQuotaSnapshot, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var encrypted string
	err = tx.QueryRowContext(ctx, `SELECT p.config_encrypted FROM sub2api_plugin_installations p
		JOIN sub2api_plugin_bindings b ON b.plugin_id=p.id
		WHERE p.plugin_key=$1 AND b.capability=$2 AND b.platform='all' AND b.account_type='all' AND b.enabled=TRUE
		FOR SHARE OF p,b`, policy.PluginKey, sharedquota.Capability).Scan(&encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSharedQuotaPolicyChanged
	}
	if err != nil {
		return nil, err
	}
	if encrypted != policy.ConfigEncrypted {
		return nil, service.ErrSharedQuotaPolicyChanged
	}
	w, err := currentSharedQuotaWindow(ctx, tx, policy.PluginKey, now)
	if err != nil {
		return nil, err
	}
	if w == nil {
		w, err = ensureSharedQuotaWindowWithGroups(ctx, tx, policy.PluginKey, start, end, total, policy.GroupLimits)
		if err != nil {
			return nil, err
		}
	}
	if policy.GroupID > 0 {
		w, err = sharedQuotaGroupWindow(ctx, tx, w.WindowID, policy.GroupID)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return w, nil
}

// 配置与当前账期原子保存，避免下一次查询提前采用待生效额度或覆盖刚保存的配置。
func (r *pluginRepository) UpdateSharedQuotaConfig(ctx context.Context, update *service.SharedQuotaConfigUpdate) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var key, encrypted, binary string
	err = tx.QueryRowContext(ctx, `SELECT plugin_key,config_encrypted,binary_sha256 FROM sub2api_plugin_installations WHERE id=$1 FOR UPDATE`, update.PluginID).
		Scan(&key, &encrypted, &binary)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrPluginStateChanged
	}
	if err != nil {
		return err
	}
	if encrypted != update.ExpectedEncrypted || binary != update.ExpectedBinarySHA256 {
		return service.ErrPluginStateChanged
	}
	if len(update.Next.GroupLimits) > 0 {
		ids := make([]int64, 0, len(update.Next.GroupLimits))
		for id := range update.Next.GroupLimits {
			ids = append(ids, id)
		}
		if err := lockLiveGroups(ctx, tx, ids); err != nil {
			return err
		}
	}
	w, err := currentSharedQuotaWindow(ctx, tx, key, update.Now)
	if err != nil {
		return err
	}
	if w == nil {
		initial := update.Next
		if update.Previous != nil {
			initial = *update.Previous
		}
		start, end := initial.Window(update.Now, update.Location)
		w, err = ensureSharedQuotaWindowWithGroups(ctx, tx, key, start, end, service.QuantizeUsageBillingAmount(initial.DailyQuota), initial.GroupLimits)
		if err != nil {
			return err
		}
	}
	total := w.Total
	if update.Previous == nil || update.Next.ApplyMode != sharedquota.ApplyNextReset {
		total = service.QuantizeUsageBillingAmount(update.Next.DailyQuota)
		raw, err := sharedGroupLimitsJSON(update.Next.GroupLimits)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE shared_quota_windows SET group_limits=$2 WHERE id=$1`, w.WindowID, string(raw)); err != nil {
			return err
		}
	}
	_, nextReset := update.Next.Window(update.Now, update.Location)
	// 修改刷新时间保留当前用量，以新时间的下一次到点作为账期结束。
	if _, err := tx.ExecContext(ctx, `UPDATE shared_quota_windows SET quota=$2,ends_at=$3 WHERE id=$1`, w.WindowID, total, nextReset); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sub2api_plugin_installations SET config_encrypted=$2,updated_at=NOW() WHERE id=$1`, update.PluginID, update.Encrypted); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *sharedQuotaRepository) GetWindow(ctx context.Context, id int64) (*service.SharedQuotaSnapshot, error) {
	var w service.SharedQuotaSnapshot
	err := r.db.QueryRowContext(ctx, `SELECT `+sharedQuotaColumns+` FROM shared_quota_windows WHERE id=$1`, id).Scan(&w.WindowID, &w.Total, &w.Used, &w.Held, &w.Remaining, &w.ResetsAt)
	return &w, err
}

func (r *sharedQuotaRepository) GetGroupWindow(ctx context.Context, id, groupID int64) (*service.SharedQuotaSnapshot, error) {
	return sharedQuotaGroupWindow(ctx, r.db, id, groupID)
}

func (r *sharedQuotaRepository) GetGroupQuotas(ctx context.Context, windowID int64, groupIDs []int64) (map[int64]service.SharedGroupQuota, error) {
	quotas := make(map[int64]service.SharedGroupQuota)
	if len(groupIDs) == 0 {
		return quotas, nil
	}
	rows, err := r.db.QueryContext(ctx, `SELECT limits.key::bigint,limits.value::numeric,
        COALESCE(g.used,0),COALESCE(g.held,0),
        GREATEST(0,limits.value::numeric-COALESCE(g.used,0)-COALESCE(g.held,0))
        FROM shared_quota_windows w CROSS JOIN LATERAL jsonb_each_text(w.group_limits) limits
        LEFT JOIN shared_quota_group_usage g ON g.window_id=w.id AND g.group_id=limits.key::bigint
        WHERE w.id=$1 AND limits.key::bigint=ANY($2)`, windowID, pq.Array(groupIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var quota service.SharedGroupQuota
		if err := rows.Scan(&id, &quota.Limit, &quota.Used, &quota.Held, &quota.Remaining); err != nil {
			return nil, err
		}
		quotas[id] = quota
	}
	return quotas, rows.Err()
}

func sharedQuotaGroupWindow(ctx context.Context, db sharedQuotaQueryer, id, groupID int64) (*service.SharedQuotaSnapshot, error) {
	w := service.SharedQuotaSnapshot{GroupID: groupID}
	var limit *float64
	var used, held float64
	err := db.QueryRowContext(ctx, `SELECT w.id,w.quota,w.used,w.held,GREATEST(0,w.quota-w.used-w.held),w.ends_at,
        (w.group_limits->>($2::bigint)::text)::numeric,COALESCE(g.used,0),COALESCE(g.held,0)
        FROM shared_quota_windows w LEFT JOIN shared_quota_group_usage g ON g.window_id=w.id AND g.group_id=$2
        WHERE w.id=$1`, id, groupID).Scan(&w.WindowID, &w.Total, &w.Used, &w.Held, &w.Remaining, &w.ResetsAt, &limit, &used, &held)
	if err != nil {
		return nil, err
	}
	if limit != nil {
		w.GroupQuota = &service.SharedGroupQuota{Limit: *limit, Used: used, Held: held, Remaining: math.Max(0, service.QuantizeUsageBillingAmount(*limit-used-held))}
	}
	return &w, nil
}

func applySharedGroupUsage(ctx context.Context, tx *sql.Tx, window, group int64, used, held float64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO shared_quota_group_usage(window_id,group_id,used,held) VALUES($1,$2,$3,$4)
        ON CONFLICT(window_id,group_id) DO UPDATE SET used=shared_quota_group_usage.used+$3,held=shared_quota_group_usage.held+$4`, window, group, used, held)
	return err
}

func applySharedQuotaUsage(ctx context.Context, tx *sql.Tx, id int64, amount float64) error {
	res, err := tx.ExecContext(ctx, `UPDATE shared_quota_windows SET used=used+$1 WHERE id=$2`, amount, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("共享额度账期不存在")
	}
	return nil
}

func reserveSharedQuotaBatch(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand, q *service.SharedQuotaSnapshot) (*service.BatchImageBalanceHoldResult, error) {
	amount := service.QuantizeUsageBillingAmount(cmd.HoldAmount)
	if amount < 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return nil, errors.New("共享冻结金额无效")
	}
	res, err := tx.ExecContext(ctx, `UPDATE shared_quota_windows SET held=held+$1 WHERE id=$2 AND quota-used-held >= $1`, amount, q.WindowID)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n != 1 {
		return nil, service.ErrBatchImageInsufficientBalance
	}
	if q.GroupID > 0 {
		// 全站行锁之后读取分组实时上限；任一冻结失败会回滚整个事务。
		current, err := sharedQuotaGroupWindow(ctx, tx, q.WindowID, q.GroupID)
		if err != nil {
			return nil, err
		}
		if current.GroupQuota != nil && current.GroupQuota.Remaining < amount {
			return nil, service.ErrBatchImageInsufficientBalance
		}
		if err := applySharedGroupUsage(ctx, tx, q.WindowID, q.GroupID, 0, amount); err != nil {
			return nil, err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO shared_quota_batch_holds (batch_id,window_id,amount,group_id) VALUES ($1,$2,$3,$4)`, cmd.BatchID, q.WindowID, amount, q.GroupID)
	return &service.BatchImageBalanceHoldResult{}, err
}

// 从持久化冻结记录找原账期，跨日或停用插件不会改扣个人余额。
func settleSharedQuotaBatch(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand, capture bool) (*service.BatchImageBalanceHoldResult, bool, error) {
	var window int64
	var held float64
	var settled bool
	var groupID int64
	err := tx.QueryRowContext(ctx, `SELECT window_id,amount,settled,group_id FROM shared_quota_batch_holds WHERE batch_id=$1 FOR UPDATE`, cmd.BatchID).Scan(&window, &held, &settled, &groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	if settled {
		return &service.BatchImageBalanceHoldResult{}, true, nil
	}
	actual := 0.0
	if capture {
		actual = service.QuantizeUsageBillingAmount(cmd.ActualAmount)
		if actual < 0 || math.IsNaN(actual) || math.IsInf(actual, 0) {
			return nil, true, errors.New("共享结算金额无效")
		}
		if actual-held > 0.00000001 {
			return nil, true, service.ErrBatchImageSettlementCostExceedsHold
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE shared_quota_windows SET held=held-$1,used=used+$2 WHERE id=$3`, held, actual, window)
	if err != nil {
		return nil, true, err
	}
	if groupID > 0 {
		result, err := tx.ExecContext(ctx, `UPDATE shared_quota_group_usage SET held=held-$1,used=used+$2 WHERE window_id=$3 AND group_id=$4`, held, actual, window, groupID)
		if err != nil {
			return nil, true, err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return nil, true, err
		}
		if rows != 1 {
			return nil, true, errors.New("共享额度分组冻结记录不存在")
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE shared_quota_batch_holds SET settled=TRUE WHERE batch_id=$1`, cmd.BatchID)
	return &service.BatchImageBalanceHoldResult{}, true, err
}

func (r *usageBillingRepository) BatchSharedQuota(ctx context.Context, batchID string) (*service.SharedQuotaSnapshot, error) {
	var id, groupID int64
	err := r.db.QueryRowContext(ctx, `SELECT window_id,group_id FROM shared_quota_batch_holds WHERE batch_id=$1`, batchID).Scan(&id, &groupID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return sharedQuotaGroupWindow(ctx, r.db, id, groupID)
}

func (r *usageBillingRepository) StoreMediaBillingSnapshot(ctx context.Context, key string, payload []byte) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO async_media_billing_snapshots (request_key,payload) VALUES ($1,$2) ON CONFLICT (request_key) DO NOTHING`, key, string(payload))
	return err
}
func (r *usageBillingRepository) LoadMediaBillingSnapshot(ctx context.Context, key string) ([]byte, error) {
	var payload []byte
	err := r.db.QueryRowContext(ctx, `SELECT payload FROM async_media_billing_snapshots WHERE request_key=$1`, key).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return payload, err
}
