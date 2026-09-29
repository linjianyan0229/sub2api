//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type contributionEditCache struct {
	service.SchedulerCache
	account *service.Account
}

func (c *contributionEditCache) SetAccount(_ context.Context, account *service.Account) error {
	c.account = account
	return nil
}

func TestContributionOwnerEditing(t *testing.T) {
	f := newContributionFixture(t)
	cache := &contributionEditCache{}
	f.repo.(*channelContributionRepository).schedulerCache = cache
	in := f.input("-编辑")
	in.DailyLimit, in.TotalLimit, in.Models = 10, 50, []string{"gpt-5"}
	c, err := f.svc.Submit(f.ctx, f.user, in)
	require.NoError(t, err)
	update := service.ContributionAccountInput{SiteName: f.prefix + "-新名称", BaseURL: "https://api.example.com/v2/", RPM: 60, Concurrency: 8, DailyLimit: 8, TotalLimit: 40, Models: []string{"gpt-5", "gpt-5-mini"}, ShowInStatus: true}
	_, err = f.svc.OwnedAccount(f.ctx, f.user, c.ID)
	require.ErrorIs(t, err, service.ErrContributionNotFound)
	require.ErrorIs(t, f.svc.UpdateOwnedAccount(f.ctx, f.user, c.ID, update), service.ErrContributionNotFound)
	require.NoError(t, f.svc.Review(f.ctx, f.admin, c.ID, true, ""))
	c, err = f.repo.GetForReview(f.ctx, f.admin, c.ID)
	require.NoError(t, err)
	for _, actor := range []int64{f.other, f.admin} {
		_, err := f.svc.OwnedAccount(f.ctx, actor, c.ID)
		require.ErrorIs(t, err, service.ErrContributionNotFound)
		require.ErrorIs(t, f.svc.UpdateOwnedAccount(f.ctx, actor, c.ID, update), service.ErrContributionNotFound)
	}
	start := time.Now().UTC().Format(time.RFC3339)
	_, err = integrationDB.Exec(`UPDATE accounts SET status='error',extra=extra || jsonb_build_object('quota_used',12,'quota_daily_used',3,'quota_daily_start',$2::text,'admin_only','kept'),credentials=credentials || '{"model_mapping":{"gpt-5":"upstream-gpt"},"admin_credential":"kept"}' WHERE id=$1`, *c.AccountID, start)
	require.NoError(t, err)
	// 模拟创建事件已被消费，避免 outbox 合并同类事件影响断言。
	_, err = integrationDB.Exec(`DELETE FROM scheduler_outbox WHERE account_id=$1`, *c.AccountID)
	require.NoError(t, err)
	var outboxBefore int
	require.NoError(t, integrationDB.QueryRow(`SELECT count(*) FROM scheduler_outbox WHERE account_id=$1`, *c.AccountID).Scan(&outboxBefore))
	require.NoError(t, f.svc.UpdateOwnedAccount(f.ctx, f.user, c.ID, update))
	require.NotNil(t, cache.account)
	require.Equal(t, update.SiteName, cache.account.Name)
	require.Equal(t, update.RPM, cache.account.GetBaseRPM())
	accounts := NewAccountRepository(integrationEntClient, integrationDB, nil)
	a, err := accounts.GetByID(f.ctx, *c.AccountID)
	require.NoError(t, err)
	require.Equal(t, "sk-private-test-value", a.Credentials["api_key"])
	require.Equal(t, "https://api.example.com/v2", a.Credentials["base_url"])
	require.Equal(t, "kept", a.Credentials["admin_credential"])
	require.Equal(t, "upstream-gpt", a.GetModelMapping()["gpt-5"])
	require.Equal(t, 12.0, a.GetQuotaUsed())
	require.Equal(t, 3.0, a.GetQuotaDailyUsed())
	require.Equal(t, start, a.Extra["quota_daily_start"])
	require.Equal(t, "kept", a.Extra["admin_only"])
	require.Equal(t, "error", a.Status)
	require.Equal(t, []int64{*c.GroupID}, a.GroupIDs)
	require.Equal(t, 8, a.Concurrency)
	require.Equal(t, 60, a.GetBaseRPM())
	require.Equal(t, 8.0, a.GetQuotaDailyLimit())
	require.Equal(t, 40.0, a.GetQuotaLimit())
	var outboxAfter int
	require.NoError(t, integrationDB.QueryRow(`SELECT count(*) FROM scheduler_outbox WHERE account_id=$1`, *c.AccountID).Scan(&outboxAfter))
	require.Greater(t, outboxAfter, outboxBefore)
	view, err := f.svc.OwnedAccount(f.ctx, f.user, c.ID)
	require.NoError(t, err)
	require.True(t, view.ShowInStatus)
	require.Equal(t, update.SiteName, view.SiteName)
	blob, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(blob), "api_key")
	require.NotContains(t, string(blob), "sk-private")
	require.NotContains(t, string(blob), "admin_credential")
	mine, err := f.svc.List(f.ctx, f.user, false, "", 0)
	require.NoError(t, err)
	require.True(t, mine[0].CanEdit)
	require.Equal(t, update.SiteName, mine[0].SiteName)
	require.True(t, mine[0].ShowInStatus)
	statuses, err := f.svc.Statuses(f.ctx, f.other, 0)
	require.NoError(t, err)
	require.Len(t, statuses, 1)
	require.Equal(t, "error", statuses[0].Status)
	blob, err = json.Marshal(statuses)
	require.NoError(t, err)
	require.NotContains(t, string(blob), "api_key")
	require.NotContains(t, string(blob), "api.example.com")
	quotas, _, err := f.svc.Quotas(f.ctx, f.other, 0)
	require.NoError(t, err)
	require.Empty(t, quotas)
	update.APIKey, update.Models, update.ShowInStatus = "sk-new-private", []string{}, false
	require.NoError(t, f.svc.UpdateOwnedAccount(f.ctx, f.user, c.ID, update))
	a, err = accounts.GetByID(f.ctx, *c.AccountID)
	require.NoError(t, err)
	require.Equal(t, "sk-new-private", a.Credentials["api_key"])
	require.NotContains(t, a.Credentials, "model_mapping")
	statuses, err = f.svc.Statuses(f.ctx, f.other, 0)
	require.NoError(t, err)
	require.Empty(t, statuses)
	update.BaseURL = "https://127.0.0.1"
	require.Error(t, f.svc.UpdateOwnedAccount(f.ctx, f.user, c.ID, update))
	update.BaseURL = in.BaseURL
	update.TotalLimit = -1
	require.Error(t, f.svc.UpdateOwnedAccount(f.ctx, f.user, c.ID, update))
	update.TotalLimit = 40
	_, err = integrationDB.Exec(`UPDATE users SET status='inactive' WHERE id=$1`, f.user)
	require.NoError(t, err)
	require.ErrorIs(t, f.svc.UpdateOwnedAccount(f.ctx, f.user, c.ID, update), service.ErrContributionNotFound)
	_, err = integrationDB.Exec(`UPDATE users SET status='active' WHERE id=$1`, f.user)
	require.NoError(t, err)
	_, err = integrationDB.Exec(`UPDATE accounts SET deleted_at=NOW() WHERE id=$1`, *c.AccountID)
	require.NoError(t, err)
	_, err = f.svc.OwnedAccount(f.ctx, f.user, c.ID)
	require.ErrorIs(t, err, service.ErrContributionNotFound)
	require.ErrorIs(t, f.svc.UpdateOwnedAccount(f.ctx, f.user, c.ID, update), service.ErrContributionNotFound)
	mine, err = f.svc.List(f.ctx, f.user, false, "", 0)
	require.NoError(t, err)
	require.False(t, mine[0].CanEdit)
}

func TestContributionCatalogFiltersBeforePagination(t *testing.T) {
	f := newContributionFixture(t)
	var groupID int64
	for i := 0; i < 110; i++ {
		in := f.input(fmt.Sprintf("-%03d", i))
		if groupID > 0 {
			in.NewGroupName, in.GroupID = "", &groupID
		}
		in.DailyLimit, in.TotalLimit, in.ShowInStatus = 10, 20, true
		c, err := f.svc.Submit(f.ctx, f.user, in)
		require.NoError(t, err)
		require.NoError(t, f.svc.Review(f.ctx, f.admin, c.ID, true, ""))
		c, err = f.repo.GetForReview(f.ctx, f.admin, c.ID)
		require.NoError(t, err)
		groupID = *c.GroupID
		if i >= 52 {
			_, err = integrationDB.Exec(`UPDATE accounts SET schedulable=false WHERE id=$1`, *c.AccountID)
			require.NoError(t, err)
		}
	}
	first, _, err := f.svc.Quotas(f.ctx, f.other, 0)
	require.NoError(t, err)
	require.Len(t, first, 51)
	require.Equal(t, f.prefix+"-051", first[0].SiteName)
	// 第 51 条只作 has_more 探针，下一页应从第 50 条之后读取。
	second, _, err := f.svc.Quotas(f.ctx, f.other, first[49].ID)
	require.NoError(t, err)
	require.Len(t, second, 2)
	require.Equal(t, first[50].ID, second[0].ID)
	require.Equal(t, f.prefix+"-000", second[1].SiteName)
	statuses, err := f.svc.Statuses(f.ctx, f.other, 0)
	require.NoError(t, err)
	require.Len(t, statuses, 51)
	require.Equal(t, "disabled", statuses[0].Status)
	// 每种不可用状态都从额度列表中消失。
	_, err = integrationDB.Exec(`UPDATE accounts SET schedulable=false WHERE name LIKE $1`, f.prefix+"%")
	require.NoError(t, err)
	var accountID int64
	require.NoError(t, integrationDB.QueryRow(`SELECT account_id FROM channel_contributions WHERE id=$1`, first[0].ID).Scan(&accountID))
	for _, state := range []string{
		`status='error'`, `status='inactive'`, `schedulable=false`, `rate_limit_reset_at=NOW()+interval '1 hour'`,
		`overload_until=NOW()+interval '1 hour'`, `temp_unschedulable_until=NOW()+interval '1 hour'`,
		`auto_pause_on_expired=true,expires_at=NOW()-interval '1 hour'`, `extra=extra || '{"quota_used":20}'`,
		`extra=extra || jsonb_build_object('quota_daily_used',10,'quota_daily_start',NOW())`, `deleted_at=NOW()`,
	} {
		_, err := integrationDB.Exec(`UPDATE accounts SET status='active',schedulable=true,rate_limit_reset_at=NULL,overload_until=NULL,temp_unschedulable_until=NULL,expires_at=NULL,deleted_at=NULL,extra=extra || '{"quota_used":0,"quota_daily_used":0}' WHERE id=$1`, accountID)
		require.NoError(t, err)
		_, err = integrationDB.Exec(`UPDATE accounts SET `+state+` WHERE id=$1`, accountID)
		require.NoError(t, err)
		items, _, err := f.svc.Quotas(f.ctx, f.other, 0)
		require.NoError(t, err)
		require.Empty(t, items, state)
	}
	_, err = integrationDB.Exec(`UPDATE accounts SET deleted_at=NULL,status='active',schedulable=true,extra=extra || '{"quota_used":0,"quota_daily_used":10,"quota_daily_start":"2000-01-01T00:00:00Z"}' WHERE id=$1`, accountID)
	require.NoError(t, err)
	items, _, err := f.svc.Quotas(f.ctx, f.other, 0)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Zero(t, items[0].DailyUsed)
	_, err = integrationDB.Exec(`UPDATE groups SET is_exclusive=true WHERE id=$1`, groupID)
	require.NoError(t, err)
	statuses, err = f.svc.Statuses(f.ctx, f.other, 0)
	require.NoError(t, err)
	require.Empty(t, statuses)
	_, err = integrationDB.Exec(`UPDATE groups SET status='inactive' WHERE id=$1`, groupID)
	require.NoError(t, err)
	items, _, err = f.svc.Quotas(f.ctx, f.admin, 0)
	require.NoError(t, err)
	require.Empty(t, items)
}
