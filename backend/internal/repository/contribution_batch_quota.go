package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func contributionAvailable(a *service.Account, held, amount float64) bool {
	if limit := a.GetQuotaLimit(); limit > 0 && limit-a.GetQuotaUsed()-held < amount {
		return false
	}
	daily := a.GetQuotaDailyUsed()
	if a.IsDailyQuotaPeriodExpired() {
		daily = 0
	}
	if limit := a.GetQuotaDailyLimit(); limit > 0 && limit-daily-held < amount {
		return false
	}
	return true
}

func reserveContributionBatch(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, bool, error) {
	a := &service.Account{Type: service.AccountTypeAPIKey}
	var extra []byte
	err := tx.QueryRowContext(ctx, `SELECT a.id,a.extra,a.status,a.schedulable,a.expires_at,a.auto_pause_on_expired,a.rate_limit_reset_at,a.overload_until,a.temp_unschedulable_until FROM batch_image_jobs j JOIN accounts a ON a.id=j.account_id JOIN channel_contributions c ON c.account_id=a.id AND c.status='approved' WHERE j.batch_id=$1 AND j.user_id=$2 AND j.api_key_id=$3 AND a.deleted_at IS NULL FOR UPDATE OF a`, cmd.BatchID, cmd.UserID, cmd.APIKeyID).Scan(&a.ID, &extra, &a.Status, &a.Schedulable, &a.ExpiresAt, &a.AutoPauseOnExpired, &a.RateLimitResetAt, &a.OverloadUntil, &a.TempUnschedulableUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	if err = json.Unmarshal(extra, &a.Extra); err != nil {
		return nil, true, err
	}
	if !a.HasIndependentContributionQuota() {
		return nil, false, nil
	}
	if !a.IsSchedulable() {
		return nil, true, service.ErrBatchImageNoAccountAvailable
	}
	amount := service.QuantizeUsageBillingAmount(cmd.HoldAmount)
	if amount < 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return nil, true, errors.New("渠道冻结金额无效")
	}
	var held float64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(sum(amount),0) FROM channel_contribution_batch_holds WHERE account_id=$1 AND NOT settled`, a.ID).Scan(&held); err != nil {
		return nil, true, err
	}
	if !contributionAvailable(a, held, amount) {
		return nil, true, service.ErrBatchImageInsufficientBalance
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO channel_contribution_batch_holds(batch_id,account_id,amount) VALUES($1,$2,$3)`, cmd.BatchID, a.ID, amount)
	return &service.BatchImageBalanceHoldResult{}, true, err
}

func settleContributionBatch(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand, capture bool) (*service.BatchImageBalanceHoldResult, bool, error) {
	var id int64
	var held float64
	var settled bool
	err := tx.QueryRowContext(ctx, `SELECT account_id FROM channel_contribution_batch_holds WHERE batch_id=$1`, cmd.BatchID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	// 与冻结、普通扣费保持账号在前的锁顺序，避免结算与新任务相互等待。
	if err := tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, id).Scan(&id); err != nil {
		return nil, true, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT amount,settled FROM channel_contribution_batch_holds WHERE batch_id=$1 FOR UPDATE`, cmd.BatchID).Scan(&held, &settled); err != nil {
		return nil, true, err
	}
	if settled {
		return &service.BatchImageBalanceHoldResult{}, true, nil
	}
	if capture {
		amount := service.QuantizeUsageBillingAmount(cmd.ActualAmount)
		if amount < 0 || math.IsNaN(amount) || math.IsInf(amount, 0) || amount-held > 0.00000001 {
			return nil, true, service.ErrBatchImageSettlementCostExceedsHold
		}
		if amount > 0 {
			if _, err = incrementUsageBillingAccountQuota(ctx, tx, id, amount); err != nil {
				return nil, true, err
			}
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE channel_contribution_batch_holds SET settled=true WHERE batch_id=$1`, cmd.BatchID)
	return &service.BatchImageBalanceHoldResult{}, true, err
}
