package repository

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/pluginapi/sharedquota"
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

func (r *sharedQuotaRepository) Window(ctx context.Context, key string, start, end time.Time, total float64) (*service.SharedQuotaSnapshot, error) {
	// 同一账期保存配置不会清空已用或冻结金额；旧账期保留供在途请求结算。
	var w service.SharedQuotaSnapshot
	err := r.db.QueryRowContext(ctx, `INSERT INTO shared_quota_windows (plugin_key, starts_at, ends_at, quota)
	VALUES ($1,$2,$3,$4) ON CONFLICT (plugin_key, starts_at) DO UPDATE SET quota = EXCLUDED.quota
	RETURNING `+sharedQuotaColumns, key, start, end, total).Scan(&w.WindowID, &w.Total, &w.Used, &w.Held, &w.Remaining, &w.ResetsAt)
	return &w, err
}

func (r *sharedQuotaRepository) GetWindow(ctx context.Context, id int64) (*service.SharedQuotaSnapshot, error) {
	var w service.SharedQuotaSnapshot
	err := r.db.QueryRowContext(ctx, `SELECT `+sharedQuotaColumns+` FROM shared_quota_windows WHERE id=$1`, id).Scan(&w.WindowID, &w.Total, &w.Used, &w.Held, &w.Remaining, &w.ResetsAt)
	return &w, err
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
	_, err = tx.ExecContext(ctx, `INSERT INTO shared_quota_batch_holds (batch_id,window_id,amount) VALUES ($1,$2,$3)`, cmd.BatchID, q.WindowID, amount)
	return &service.BatchImageBalanceHoldResult{}, err
}

// 从持久化冻结记录找原账期，跨日或停用插件不会改扣个人余额。
func settleSharedQuotaBatch(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand, capture bool) (*service.BatchImageBalanceHoldResult, bool, error) {
	var window int64
	var held float64
	var settled bool
	err := tx.QueryRowContext(ctx, `SELECT window_id,amount,settled FROM shared_quota_batch_holds WHERE batch_id=$1 FOR UPDATE`, cmd.BatchID).Scan(&window, &held, &settled)
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
	_, err = tx.ExecContext(ctx, `UPDATE shared_quota_batch_holds SET settled=TRUE WHERE batch_id=$1`, cmd.BatchID)
	return &service.BatchImageBalanceHoldResult{}, true, err
}

func (r *usageBillingRepository) BatchSharedQuota(ctx context.Context, batchID string) (*service.SharedQuotaSnapshot, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT window_id FROM shared_quota_batch_holds WHERE batch_id=$1`, batchID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return (&sharedQuotaRepository{db: r.db}).GetWindow(ctx, id)
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
