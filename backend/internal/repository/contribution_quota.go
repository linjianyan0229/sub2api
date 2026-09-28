package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func NewContributionAdmissionRepository(db *sql.DB) service.ContributionAdmissionRepository {
	return &contributionAdmissionRepository{db: db}
}

type contributionAdmissionRepository struct{ db *sql.DB }

func (r *sharedQuotaRepository) IndependentContributionAccounts(ctx context.Context, group int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT a.id FROM accounts a JOIN channel_contributions c ON c.account_id=a.id AND c.status='approved'
JOIN account_groups ag ON ag.account_id=a.id WHERE ag.group_id=$1 AND a.deleted_at IS NULL AND a.status='active' AND a.schedulable
AND (COALESCE((a.extra->>'quota_limit')::numeric,0)>0 OR COALESCE((a.extra->>'quota_daily_limit')::numeric,0)>0)`, group)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// 在拿到并发槽之后读取实时金额，并原子占用 RPM；调度缓存滞后不能绕过限制。
func (r *contributionAdmissionRepository) AdmitContributionAccount(ctx context.Context, id int64, q *service.SharedQuotaSnapshot) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	a := &service.Account{ID: id, Type: service.AccountTypeAPIKey}
	var extra []byte
	err = tx.QueryRowContext(ctx, `SELECT a.status,a.schedulable,a.extra,a.expires_at,a.auto_pause_on_expired,a.rate_limit_reset_at,a.overload_until,a.temp_unschedulable_until
FROM accounts a JOIN channel_contributions c ON c.account_id=a.id AND c.status='approved' WHERE a.id=$1 AND a.deleted_at IS NULL FOR UPDATE OF a`, id).Scan(&a.Status, &a.Schedulable, &extra, &a.ExpiresAt, &a.AutoPauseOnExpired, &a.RateLimitResetAt, &a.OverloadUntil, &a.TempUnschedulableUntil)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	isContribution := err == nil
	if isContribution {
		if err := json.Unmarshal(extra, &a.Extra); err != nil {
			return false, err
		}
		var held float64
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(sum(amount),0) FROM channel_contribution_batch_holds WHERE account_id=$1 AND NOT settled`, id).Scan(&held); err != nil {
			return false, err
		}
		if !contributionAvailable(a, held, 0.00000001) {
			return false, nil
		}
		if !a.IsSchedulable() {
			return false, nil
		}
	}
	if q != nil && !(isContribution && a.HasIndependentContributionQuota()) {
		current, err := sharedQuotaGroupWindow(ctx, tx, q.WindowID, q.GroupID)
		if err != nil {
			return false, err
		}
		if current.CheckPoolRemaining() != nil {
			return false, nil
		}
	}
	if isContribution && a.GetBaseRPM() > 0 {
		result, err := tx.ExecContext(ctx, `INSERT INTO channel_contribution_rpm(account_id,window_start,used) VALUES($1,date_trunc('minute',NOW()),1)
ON CONFLICT(account_id) DO UPDATE SET window_start=date_trunc('minute',NOW()),used=CASE WHEN channel_contribution_rpm.window_start < date_trunc('minute',NOW()) THEN 1 ELSE channel_contribution_rpm.used+1 END
WHERE channel_contribution_rpm.window_start < date_trunc('minute',NOW()) OR channel_contribution_rpm.used < $2`, id, a.GetBaseRPM())
		if err != nil {
			return false, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if n == 0 {
			return false, nil
		}
	}
	return true, tx.Commit()
}
