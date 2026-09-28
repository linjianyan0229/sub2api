//go:build integration

package repository

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestContributionConfiguredGroupCreatedOnlyAfterApproval(t *testing.T) {
	f := newContributionFixture(t)
	in := f.input("-配置组")
	price, margin := 0.2, 0.15
	in.ContributionSettings = service.ContributionSettings{RPM: 12, Concurrency: 3, DailyLimit: 8, TotalLimit: 40, Models: []string{"gpt-5"}, NewGroupConfig: &service.CreateGroupInput{
		Name: "不能覆盖主申请组名", Platform: "gemini", Description: "自建配置", RateMultiplier: 1.5, RPMLimit: 20,
		AllowImageGeneration: true, ImagePrice1K: &price, ForceOpenAIFast: true, AllowLive: true,
		ProfitControlEnabled: true, ProfitMinMargin: &margin, MaxReasoningEffort: "high",
		ModelAllowlist: service.GroupModelAllowlist{Enabled: true, Models: []string{"gpt-*"}},
	}}
	c, err := f.svc.Submit(f.ctx, f.user, in)
	require.NoError(t, err)
	var count int
	require.NoError(t, integrationDB.QueryRow(`SELECT count(*) FROM groups WHERE name=$1`, in.NewGroupName).Scan(&count))
	require.Zero(t, count)
	rows, _, err := f.svc.Quotas(f.ctx, f.user, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, in.NewGroupName, rows[0].Groups[0].Name)
	require.Equal(t, "pending", rows[0].Status)
	rows, _, err = f.svc.Quotas(f.ctx, f.other, 0)
	require.NoError(t, err)
	require.Empty(t, rows)
	models := []string{"gpt-5", "gpt-5-mini"}
	require.NoError(t, f.svc.ReviewWithModels(f.ctx, f.admin, c.ID, true, "", &models))
	c, err = f.repo.GetForReview(f.ctx, f.admin, c.ID)
	require.NoError(t, err)
	g, err := NewGroupRepository(integrationEntClient, integrationDB).GetByID(f.ctx, *c.GroupID)
	require.NoError(t, err)
	require.Equal(t, in.NewGroupName, g.Name)
	require.Equal(t, service.PlatformOpenAI, g.Platform)
	require.Equal(t, 1.5, g.RateMultiplier)
	require.Equal(t, 20, g.RPMLimit)
	require.True(t, g.AllowImageGeneration)
	require.True(t, g.ForceOpenAIFast)
	require.True(t, g.AllowLive)
	require.True(t, g.ProfitControlEnabled)
	require.Equal(t, margin, g.ProfitMinMargin)
	require.Equal(t, price, *g.ImagePrice1K)
	require.Equal(t, "high", g.MaxReasoningEffort)
	require.Equal(t, []string{"gpt-*"}, g.ModelAllowlist.Models)
	a, err := NewAccountRepository(integrationEntClient, integrationDB, nil).GetByID(f.ctx, *c.AccountID)
	require.NoError(t, err)
	require.Equal(t, 3, a.Concurrency)
	require.Equal(t, 12, a.GetBaseRPM())
	require.Equal(t, 8.0, a.GetQuotaDailyLimit())
	require.Equal(t, 40.0, a.GetQuotaLimit())
	require.True(t, a.HasIndependentContributionQuota())
	require.True(t, a.IsModelSupported("gpt-5-mini"))
	require.False(t, a.IsModelSupported("not-selected"))
	rows, _, err = f.svc.Quotas(f.ctx, f.other, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "active", rows[0].Status)
	require.Equal(t, *c.GroupID, rows[0].Groups[0].ID)
	blob, err := json.Marshal(rows)
	require.NoError(t, err)
	require.NotContains(t, string(blob), "sk-private")
	require.NotContains(t, string(blob), "api.example.com")
	require.NotContains(t, string(blob), "@contribution.test")
	// 改为私有分组后，无权限用户不能再查看该渠道。
	_, err = integrationDB.Exec(`UPDATE groups SET is_exclusive=true WHERE id=$1`, *c.GroupID)
	require.NoError(t, err)
	rows, _, err = f.svc.Quotas(f.ctx, f.other, 0)
	require.NoError(t, err)
	require.Empty(t, rows)
	rows, _, err = f.svc.Quotas(f.ctx, f.admin, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestContributionGroupReferencesRecheckedAndRollback(t *testing.T) {
	f := newContributionFixture(t)
	group, err := integrationEntClient.Group.Create().SetName(f.prefix + "-源组").SetPlatform(service.PlatformOpenAI).Save(f.ctx)
	require.NoError(t, err)
	in := f.input("-引用")
	in.NewGroupConfig = &service.CreateGroupInput{RateMultiplier: 1, CopyAccountsFromGroupIDs: []int64{group.ID}}
	c, err := f.svc.Submit(f.ctx, f.user, in)
	require.NoError(t, err)
	_, err = integrationDB.Exec(`UPDATE groups SET is_exclusive=true WHERE id=$1`, group.ID)
	require.NoError(t, err)
	require.ErrorIs(t, f.svc.Review(f.ctx, f.admin, c.ID, true, ""), service.ErrContributionGroup)
	in.SiteName, in.NewGroupName = f.prefix+"-越权", f.prefix+"-越权"
	_, err = f.svc.Submit(f.ctx, f.user, in)
	require.ErrorIs(t, err, service.ErrContributionGroup)
	in.NewGroupConfig.CopyAccountsFromGroupIDs = nil
	in.NewGroupConfig.ModelRouting = map[string][]int64{"gpt-*": {999999999}}
	_, err = f.svc.Submit(f.ctx, f.user, in)
	require.ErrorIs(t, err, service.ErrContributionGroup)
	in.NewGroupConfig.ModelRouting = nil
	c, err = f.svc.Submit(f.ctx, f.user, in)
	require.NoError(t, err)
	c.PreparedGroup = &service.Group{Name: in.NewGroupName, Platform: service.PlatformOpenAI, RateMultiplier: 1, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard}
	require.Error(t, f.repo.Review(f.ctx, f.admin, c, true, "", &service.Account{Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}))
	var count int
	require.NoError(t, integrationDB.QueryRow(`SELECT count(*) FROM groups WHERE name=$1`, in.NewGroupName).Scan(&count))
	require.Zero(t, count)
}

func TestContributionIndependentQuotaAdmissionBillingAndBatch(t *testing.T) {
	f := newContributionFixture(t)
	in := f.input("-独立额度")
	in.RPM, in.DailyLimit, in.TotalLimit = 4, 10, 50
	c, err := f.svc.Submit(f.ctx, f.user, in)
	require.NoError(t, err)
	require.NoError(t, f.svc.Review(f.ctx, f.admin, c.ID, true, ""))
	c, err = f.repo.GetForReview(f.ctx, f.admin, c.ID)
	require.NoError(t, err)
	accountID := *c.AccountID
	key := mustCreateApiKey(t, integrationEntClient, &service.APIKey{UserID: f.user, Key: "sk-" + uuid.NewString(), Name: "贡献限额测试", Quota: 1, RateLimit5h: 1})
	_, err = integrationDB.Exec(`UPDATE users SET balance=20 WHERE id=$1`, f.user)
	require.NoError(t, err)
	start := time.Now().UTC()
	q, err := ensureSharedQuotaWindow(f.ctx, integrationDB, f.prefix, start, start.Add(24*time.Hour), 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := integrationDB.Exec(`DELETE FROM shared_quota_windows WHERE id=$1`, q.WindowID)
		require.NoError(t, err)
	})
	q.GroupID = *c.GroupID
	shared := &sharedQuotaRepository{db: integrationDB}
	ids, err := shared.IndependentContributionAccounts(f.ctx, q.GroupID)
	require.NoError(t, err)
	require.Equal(t, []int64{accountID}, ids)
	admission := NewContributionAdmissionRepository(integrationDB)
	// 固定窗口设在未来一分钟，避免测试跨分钟边界产生假失败。
	_, err = integrationDB.Exec(`INSERT INTO channel_contribution_rpm(account_id,window_start,used) VALUES($1,date_trunc('minute',NOW()) + interval '1 minute',0)`, accountID)
	require.NoError(t, err)
	var wg sync.WaitGroup
	results := make(chan bool, 12)
	errors := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := admission.AdmitContributionAccount(f.ctx, accountID, q)
			results <- ok
			errors <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	accepted := 0
	for ok := range results {
		if ok {
			accepted++
		}
	}
	require.Equal(t, 4, accepted)
	_, err = integrationDB.Exec(`UPDATE accounts SET extra=extra || '{"base_rpm":0}' WHERE id=$1`, accountID)
	require.NoError(t, err)
	billing := &usageBillingRepository{db: integrationDB}
	cmd := &service.UsageBillingCommand{RequestID: uuid.NewString(), APIKeyID: key.ID, UserID: f.user, AccountID: accountID, AccountType: service.AccountTypeAPIKey, BillingType: service.BillingTypeContribution, AccountQuotaCost: 2}
	result, err := billing.Apply(f.ctx, cmd)
	require.NoError(t, err)
	require.True(t, result.Applied)
	result, err = billing.Apply(f.ctx, cmd)
	require.NoError(t, err)
	require.False(t, result.Applied)
	var balance, keyUsed, windowUsed, accountUsed float64
	require.NoError(t, integrationDB.QueryRow(`SELECT u.balance,k.quota_used FROM users u JOIN api_keys k ON k.user_id=u.id WHERE k.id=$1`, key.ID).Scan(&balance, &keyUsed))
	require.Equal(t, 20.0, balance)
	require.Zero(t, keyUsed)
	require.NoError(t, integrationDB.QueryRow(`SELECT used FROM shared_quota_windows WHERE id=$1`, q.WindowID).Scan(&windowUsed))
	require.Zero(t, windowUsed)
	require.NoError(t, integrationDB.QueryRow(`SELECT (extra->>'quota_used')::numeric FROM accounts WHERE id=$1`, accountID).Scan(&accountUsed))
	require.Equal(t, 2.0, accountUsed)
	// 混合分组里的普通贡献渠道仍受全站额度限制。
	ordinary := f.input("-全站额度")
	ordinary.NewGroupName = ""
	ordinary.GroupID = c.GroupID
	o, err := f.svc.Submit(f.ctx, f.user, ordinary)
	require.NoError(t, err)
	require.NoError(t, f.svc.Review(f.ctx, f.admin, o.ID, true, ""))
	o, err = f.repo.GetForReview(f.ctx, f.admin, o.ID)
	require.NoError(t, err)
	ok, err := admission.AdmitContributionAccount(f.ctx, *o.AccountID, q)
	require.NoError(t, err)
	require.False(t, ok)
	// 独立渠道批量冻结占用自己的额度，不依赖全站余额。
	batchID := uuid.NewString()
	_, err = integrationDB.Exec(`INSERT INTO batch_image_jobs(batch_id,user_id,api_key_id,account_id,provider,model,item_count) VALUES($1,$2,$3,$4,'gemini_api','test',1)`, batchID, f.user, key.ID, accountID)
	require.NoError(t, err)
	hold := &service.BatchImageBalanceHoldCommand{RequestID: uuid.NewString(), APIKeyID: key.ID, UserID: f.user, BatchID: batchID, HoldAmount: 8}
	_, err = billing.ReserveBatchImageBalance(service.WithSharedQuota(f.ctx, q), hold)
	require.NoError(t, err)
	ok, err = admission.AdmitContributionAccount(f.ctx, accountID, q)
	require.NoError(t, err)
	require.False(t, ok)
	snapshot, err := billing.BatchSharedQuota(f.ctx, batchID)
	require.NoError(t, err)
	require.Equal(t, accountID, snapshot.ContributionAccountID)
	capture := *hold
	capture.RequestID = uuid.NewString()
	capture.RequestFingerprint = ""
	capture.ActualAmount = 3
	_, err = billing.CaptureBatchImageBalance(f.ctx, &capture)
	require.NoError(t, err)
	_, err = billing.CaptureBatchImageBalance(f.ctx, &capture)
	require.NoError(t, err)
	require.NoError(t, integrationDB.QueryRow(`SELECT (extra->>'quota_used')::numeric FROM accounts WHERE id=$1`, accountID).Scan(&accountUsed))
	require.Equal(t, 5.0, accountUsed)
	ok, err = admission.AdmitContributionAccount(f.ctx, accountID, q)
	require.NoError(t, err)
	require.True(t, ok)
	// 日额度跨日自动恢复，累计额度不重置。
	_, err = integrationDB.Exec(`UPDATE accounts SET extra=extra || jsonb_build_object('quota_daily_used',10,'quota_daily_start',$2::text) WHERE id=$1`, accountID, time.Now().UTC().Add(-48*time.Hour).Format(time.RFC3339))
	require.NoError(t, err)
	ok, err = admission.AdmitContributionAccount(f.ctx, accountID, q)
	require.NoError(t, err)
	require.True(t, ok)
	_, err = integrationDB.Exec(`UPDATE accounts SET extra=extra || '{"quota_used":50}' WHERE id=$1`, accountID)
	require.NoError(t, err)
	ok, err = admission.AdmitContributionAccount(f.ctx, accountID, q)
	require.NoError(t, err)
	require.False(t, ok)
}
