package service

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContributionSettingsValidation(t *testing.T) {
	s := ContributionSettings{Models: []string{" gpt-5 ", "gpt-5", "gemini-2.5-pro"}, DailyLimit: 1.234567891}
	require.NoError(t, validateContributionSettings(&s))
	require.Equal(t, 5, s.Concurrency)
	require.Equal(t, []string{"gpt-5", "gemini-2.5-pro"}, s.Models)
	require.Equal(t, 1.23456789, s.DailyLimit)
	for _, invalid := range []ContributionSettings{
		{RPM: -1}, {RPM: 100001}, {Concurrency: -1}, {Concurrency: 1001},
		{DailyLimit: -1}, {DailyLimit: math.NaN()}, {TotalLimit: math.Inf(1)}, {TotalLimit: 1e10}, {TotalLimit: 1e-10},
		{Models: []string{"gpt-*"}}, {Models: []string{"a\nb"}}, {Models: []string{""}},
	} {
		require.Error(t, validateContributionSettings(&invalid))
	}
}

func TestContributionBillingUsesSelectedAccount(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		extra                                map[string]any
		admin                                bool
		kind                                 int8
		accountCost, sharedCost, balanceCost float64
	}{
		{"独立每日限额", map[string]any{"contribution_id": 1, "quota_daily_limit": 10}, false, BillingTypeContribution, 3, 0, 0},
		{"独立累计限额", map[string]any{"contribution_id": 1, "quota_limit": 10}, false, BillingTypeContribution, 3, 0, 0},
		{"未配置渠道限额", map[string]any{"contribution_id": 1}, false, BillingTypeSharedQuota, 0, 4, 0},
		{"普通账号不豁免", map[string]any{"quota_limit": 10}, false, BillingTypeSharedQuota, 3, 4, 0},
		{"管理员维持个人计费", map[string]any{"contribution_id": 1, "quota_limit": 10}, true, BillingTypeBalance, 3, 0, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			user := &User{ID: 1, Role: RoleUser, SharedQuota: &SharedQuotaSnapshot{WindowID: 8, GroupID: 3}}
			if tc.admin {
				user.Role = RoleAdmin
			}
			log := &UsageLog{}
			cmd := buildUsageBillingCommand("contribution", log, &postUsageBillingParams{
				User: user, APIKey: &APIKey{ID: 2}, Account: &Account{ID: 4, Type: AccountTypeAPIKey, Extra: tc.extra},
				Cost: &CostBreakdown{TotalCost: 2, ActualCost: 4}, AccountRateMultiplier: 1.5,
			})
			require.Equal(t, tc.kind, cmd.BillingType)
			require.Equal(t, tc.accountCost, cmd.AccountQuotaCost)
			require.Equal(t, tc.sharedCost, cmd.SharedQuotaCost)
			require.Equal(t, tc.balanceCost, cmd.BalanceCost)
			require.Zero(t, cmd.APIKeyQuotaCost)
			require.Zero(t, cmd.SubscriptionCost)
			if tc.kind == BillingTypeContribution {
				require.Equal(t, tc.accountCost, log.ActualCost)
			}
			if !tc.admin {
				require.Equal(t, tc.kind, log.BillingType)
			}
		})
	}
}

func TestContributionSchedulingWhenSharedQuotaExhausted(t *testing.T) {
	accounts := []Account{{ID: 1}, {ID: 2, Extra: map[string]any{"contribution_id": 1, "quota_limit": 10}}, {ID: 3, Extra: map[string]any{"contribution_id": 2, "quota_daily_limit": 10}}}
	q := &SharedQuotaSnapshot{Remaining: 0, IndependentAccountIDs: []int64{2}}
	ctx := WithSharedQuota(context.Background(), q)
	require.NoError(t, q.CheckRemaining())
	require.ErrorIs(t, q.CheckPoolRemaining(), ErrSharedQuotaExhausted)
	filtered := filterSharedQuotaAccounts(ctx, accounts)
	require.Len(t, filtered, 1)
	require.Equal(t, int64(2), filtered[0].ID)
	require.Equal(t, int64(1), accounts[0].ID, "不得修改调度缓存切片")
	q.Remaining = 10
	require.Len(t, filterSharedQuotaAccounts(ctx, accounts), 3)
	q.GroupQuota = &SharedGroupQuota{Remaining: 0}
	require.Len(t, filterSharedQuotaAccounts(ctx, accounts), 1)
}

type contributionModelRepo struct {
	ChannelContributionRepository
	item *ChannelContribution
	err  error
}

func (r *contributionModelRepo) GetForReview(context.Context, int64, int64) (*ChannelContribution, error) {
	return r.item, r.err
}

type contributionModelFetcher struct {
	account *Account
	err     error
}

func (f *contributionModelFetcher) FetchUpstreamSupportedModels(_ context.Context, a *Account) ([]string, error) {
	f.account = a
	return []string{"gpt-test"}, f.err
}

type contributionTestCipher struct{}

func (contributionTestCipher) Encrypt(s string) (string, error) { return s, nil }
func (contributionTestCipher) Decrypt(s string) (string, error) { return s, nil }

func TestContributionModelsRequireAdminAndHideUpstreamErrors(t *testing.T) {
	r := &contributionModelRepo{err: ErrContributionAdmin}
	f := &contributionModelFetcher{}
	s := NewChannelContributionService(r, contributionTestCipher{}, nil)
	s.modelFetcher = f
	_, err := s.Models(context.Background(), 1, 1)
	require.ErrorIs(t, err, ErrContributionAdmin)
	require.Nil(t, f.account)
	r.err = nil
	r.item = &ChannelContribution{ID: 1, SiteName: "测试站", Platform: PlatformOpenAI, BaseURL: "https://api.example.com", KeyEncrypted: "sk-private", Status: "pending"}
	models, err := s.Models(context.Background(), 2, 1)
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-test"}, models)
	require.Equal(t, "sk-private", f.account.Credentials["api_key"])
	f.err = errors.New("failed: sk-private")
	_, err = s.Models(context.Background(), 2, 1)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "sk-private")
	r.item.Status = "approved"
	_, err = s.Models(context.Background(), 2, 1)
	require.ErrorIs(t, err, ErrContributionReviewed)
}
