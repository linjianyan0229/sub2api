package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSharedQuotaApplyIsAtomicAndNeverUpdatesUser(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "扣费成功", true: "事务失败回滚"}[fail], func(t *testing.T) {
			db, m, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			m.ExpectBegin()
			m.ExpectQuery("INSERT INTO usage_billing_dedup").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
			m.ExpectQuery("FROM usage_billing_dedup_archive").WillReturnError(sql.ErrNoRows)
			update := m.ExpectExec(regexp.QuoteMeta("UPDATE shared_quota_windows SET used=used+$1 WHERE id=$2")).WithArgs(1.25, int64(7))
			if fail {
				update.WillReturnError(errors.New("offline"))
				m.ExpectRollback()
			} else {
				update.WillReturnResult(sqlmock.NewResult(0, 1))
				m.ExpectCommit()
			}
			result, err := (&usageBillingRepository{db: db}).Apply(context.Background(), &service.UsageBillingCommand{RequestID: "test", APIKeyID: 1, UserID: 1, SharedQuotaWindowID: 7, SharedQuotaCost: 1.25})
			if fail {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.True(t, result.Applied)
				require.Nil(t, result.NewBalance)
			}
			require.NoError(t, m.ExpectationsWereMet())
		})
	}
}

func TestSharedQuotaBatchSettlementUsesPersistedHold(t *testing.T) {
	db, m, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	m.ExpectBegin()
	tx, err := db.Begin()
	require.NoError(t, err)
	m.ExpectQuery("SELECT window_id,amount,settled").WithArgs("batch-1").WillReturnRows(sqlmock.NewRows([]string{"window_id", "amount", "settled", "group_id"}).AddRow(7, 10, false, 0))
	m.ExpectExec(regexp.QuoteMeta("UPDATE shared_quota_windows SET held=held-$1,used=used+$2 WHERE id=$3")).WithArgs(10.0, 4.0, int64(7)).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("UPDATE shared_quota_batch_holds SET settled=TRUE").WithArgs("batch-1").WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectCommit()
	// cmd 的 hold 值不可信；释放金额只能采用账本中保存的 10。
	_, handled, err := settleSharedQuotaBatch(context.Background(), tx, &service.BatchImageBalanceHoldCommand{BatchID: "batch-1", HoldAmount: 999, ActualAmount: 4}, true)
	require.NoError(t, err)
	require.True(t, handled)
	require.NoError(t, tx.Commit())
	require.NoError(t, m.ExpectationsWereMet())
}

func TestSharedQuotaRejectsMixedPersonalCosts(t *testing.T) {
	err := (&usageBillingRepository{}).applyUsageBillingEffects(context.Background(), nil, &service.UsageBillingCommand{SharedQuotaWindowID: 7, SharedQuotaCost: 1, BalanceCost: 1}, &service.UsageBillingApplyResult{})
	require.ErrorContains(t, err, "不能同时扣除个人额度")
}

func TestSharedQuotaGroupFailureRollsBackSiteCharge(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO usage_billing_dedup").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("FROM usage_billing_dedup_archive").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("UPDATE shared_quota_windows SET used").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO shared_quota_group_usage").WithArgs(int64(7), int64(8), 1.25, 0.0).WillReturnError(errors.New("分组账本不可用"))
	mock.ExpectRollback()
	_, err = (&usageBillingRepository{db: db}).Apply(context.Background(), &service.UsageBillingCommand{RequestID: "atomic-group", APIKeyID: 1, UserID: 1, SharedQuotaWindowID: 7, SharedQuotaGroupID: 8, SharedQuotaCost: 1.25})
	require.ErrorContains(t, err, "分组账本不可用")
	require.NoError(t, mock.ExpectationsWereMet())
}
