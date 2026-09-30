//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type contributionFixture struct {
	ctx                context.Context
	repo               service.ChannelContributionRepository
	svc                *service.ChannelContributionService
	admin, user, other int64
	prefix             string
}

func newContributionFixture(t *testing.T) contributionFixture {
	t.Helper()
	f := contributionFixture{ctx: context.Background(), prefix: uuid.NewString()}
	f.repo = NewChannelContributionRepository(integrationEntClient, integrationDB, nil)
	users := NewUserRepository(integrationEntClient, integrationDB)
	keys := service.NewAPIKeyService(nil, users, NewGroupRepository(integrationEntClient, integrationDB), NewUserSubscriptionRepository(integrationEntClient), nil, nil, &config.Config{})
	admin := service.NewAdminService(&config.Config{}, users, NewAdminGroupRepository(integrationEntClient, integrationDB), NewAccountRepository(integrationEntClient, integrationDB, nil).(service.AdminAccountRepository),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, integrationEntClient, nil, nil, NewUserSubscriptionRepository(integrationEntClient), nil, nil, nil, nil, nil, nil)
	f.svc = service.ProvideChannelContributionService(f.repo, &AESEncryptor{key: make([]byte, 32)}, keys, nil, admin, &config.Config{Timezone: "UTC"})
	ids := []*int64{&f.admin, &f.user, &f.other}
	for i, id := range ids {
		role := service.RoleUser
		if i == 0 {
			role = service.RoleAdmin
		}
		u := &service.User{Email: fmt.Sprintf("%s-%d@contribution.test", f.prefix, i), Username: fmt.Sprintf("贡献者-%s-%d", f.prefix, i), PasswordHash: "hash", Role: role, Status: service.StatusActive, Concurrency: 5}
		require.NoError(t, users.Create(f.ctx, u))
		*id = u.ID
	}
	t.Cleanup(func() {
		for _, query := range []string{
			`DELETE FROM channel_monitors WHERE extra_headers->>'sub2api:contribution_id' IN (SELECT id::text FROM channel_contributions WHERE site_name LIKE $1)`,
			`DELETE FROM channel_contribution_batch_holds WHERE account_id IN (SELECT id FROM accounts WHERE name LIKE $1)`,
			`DELETE FROM batch_image_jobs WHERE account_id IN (SELECT id FROM accounts WHERE name LIKE $1)`,
			`DELETE FROM api_keys WHERE user_id IN (SELECT id FROM users WHERE email LIKE $1)`,
			`DELETE FROM channel_contributions WHERE site_name LIKE $1`,
			`DELETE FROM account_groups WHERE account_id IN (SELECT id FROM accounts WHERE name LIKE $1)`,
			`DELETE FROM accounts WHERE name LIKE $1`,
			`DELETE FROM groups WHERE name LIKE $1`,
			`DELETE FROM users WHERE email LIKE $1`,
		} {
			_, err := integrationDB.ExecContext(f.ctx, query, f.prefix+"%")
			require.NoError(t, err)
		}
	})
	return f
}

func (f contributionFixture) input(suffix string) service.ChannelContributionInput {
	return service.ChannelContributionInput{SiteName: f.prefix + suffix, BaseURL: "https://api.example.com/v1", APIKey: "sk-private-test-value", Platform: service.PlatformOpenAI, NewGroupName: f.prefix + suffix}
}

func TestChannelContributionLifecycle(t *testing.T) {
	f := newContributionFixture(t)
	c, err := f.svc.Submit(f.ctx, f.user, f.input("-新组"))
	require.NoError(t, err)
	require.Equal(t, "pending", c.Status)
	require.NotEqual(t, "sk-private-test-value", c.KeyEncrypted)
	require.NotEmpty(t, c.KeyEncrypted)
	blob, err := json.Marshal(c)
	require.NoError(t, err)
	require.NotContains(t, string(blob), "sk-private-test-value")
	require.NotContains(t, string(blob), c.KeyEncrypted)
	require.NotContains(t, string(blob), "@contribution.test")
	var count int
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT count(*) FROM accounts WHERE name=$1`, c.SiteName).Scan(&count))
	require.Zero(t, count)
	_, err = f.svc.List(f.ctx, f.user, true, "", 0)
	require.ErrorIs(t, err, service.ErrContributionAdmin)
	require.ErrorIs(t, f.svc.Review(f.ctx, f.user, c.ID, true, ""), service.ErrContributionAdmin)
	others, err := f.svc.List(f.ctx, f.other, false, "", 0)
	require.NoError(t, err)
	require.Empty(t, others)
	mine, err := f.svc.List(f.ctx, f.user, false, "", 0)
	require.NoError(t, err)
	require.Len(t, mine, 1)
	require.Empty(t, mine[0].KeyEncrypted)

	// 同一个申请并发批准只能创建一份组和账号。
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- f.svc.Review(f.ctx, f.admin, c.ID, true, "已通过") }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			require.ErrorIs(t, err, service.ErrContributionReviewed)
		}
	}
	require.Equal(t, 1, success)
	approved, err := f.repo.GetForReview(f.ctx, f.admin, c.ID)
	require.NoError(t, err)
	require.Equal(t, "approved", approved.Status)
	require.Empty(t, approved.KeyEncrypted)
	require.NotNil(t, approved.AccountID)
	require.NotNil(t, approved.GroupID)
	account, err := NewAccountRepository(integrationEntClient, integrationDB, nil).GetByID(f.ctx, *approved.AccountID)
	require.NoError(t, err)
	require.Equal(t, "sk-private-test-value", account.Credentials["api_key"])
	require.Equal(t, c.BaseURL, account.Credentials["base_url"])
	require.True(t, account.Schedulable)
	require.Equal(t, service.AccountTypeAPIKey, account.Type)
	require.Contains(t, account.GroupIDs, *approved.GroupID)
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT count(*) FROM scheduler_outbox WHERE account_id=$1`, account.ID).Scan(&count))
	require.Positive(t, count)
	groups, err := f.svc.Groups(f.ctx, f.other)
	require.NoError(t, err)
	require.NoError(t, f.svc.EnrichGroups(f.ctx, groups))
	found := false
	for _, group := range groups {
		if group.ID == *approved.GroupID {
			found = true
			require.Len(t, group.Contributions, 1)
			require.Equal(t, c.ContributorName, group.Contributions[0].ContributorName)
			require.Equal(t, c.SiteName, group.Contributions[0].AccountName)
		}
	}
	require.True(t, found)
	_, err = integrationDB.ExecContext(f.ctx, `UPDATE users SET username=$2 WHERE id=$1`, f.user, "改名-"+f.prefix)
	require.NoError(t, err)
	contributors, err := f.repo.Contributors(f.ctx, []int64{*approved.GroupID})
	require.NoError(t, err)
	require.Equal(t, "改名-"+f.prefix, contributors[*approved.GroupID][0].ContributorName)
	_, err = integrationDB.ExecContext(f.ctx, `DELETE FROM account_groups WHERE account_id=$1`, account.ID)
	require.NoError(t, err)
	contributors, err = f.repo.Contributors(f.ctx, []int64{*approved.GroupID})
	require.NoError(t, err)
	require.Empty(t, contributors)

	rejected, err := f.svc.Submit(f.ctx, f.user, f.input("-拒绝"))
	require.NoError(t, err)
	require.NoError(t, f.svc.Review(f.ctx, f.admin, rejected.ID, false, "暂不接入"))
	rejected, err = f.repo.GetForReview(f.ctx, f.admin, rejected.ID)
	require.NoError(t, err)
	require.Equal(t, "rejected", rejected.Status)
	require.Equal(t, "暂不接入", rejected.ReviewNote)
	require.Empty(t, rejected.KeyEncrypted)
	require.Nil(t, rejected.AccountID)
	require.Nil(t, rejected.GroupID)
}

func TestChannelContributionGroupsAndLimit(t *testing.T) {
	f := newContributionFixture(t)
	public, err := integrationEntClient.Group.Create().SetName(f.prefix + "-public").SetPlatform(service.PlatformOpenAI).Save(f.ctx)
	require.NoError(t, err)
	private, err := integrationEntClient.Group.Create().SetName(f.prefix + "-private").SetPlatform(service.PlatformOpenAI).SetIsExclusive(true).Save(f.ctx)
	require.NoError(t, err)
	in := f.input("-existing")
	in.NewGroupName = ""
	in.GroupID = &private.ID
	_, err = f.svc.Submit(f.ctx, f.user, in)
	require.ErrorIs(t, err, service.ErrContributionGroup)
	in.GroupID, in.Platform = &public.ID, service.PlatformGemini
	_, err = f.svc.Submit(f.ctx, f.user, in)
	require.ErrorIs(t, err, service.ErrContributionGroup)
	in.Platform = service.PlatformOpenAI
	c, err := f.svc.Submit(f.ctx, f.user, in)
	require.NoError(t, err)
	require.NoError(t, f.svc.Review(f.ctx, f.admin, c.ID, true, ""))
	c, err = f.repo.GetForReview(f.ctx, f.admin, c.ID)
	require.NoError(t, err)
	require.Equal(t, public.ID, *c.GroupID)
	c, err = f.svc.Submit(f.ctx, f.user, in)
	require.NoError(t, err)
	_, err = integrationEntClient.Group.UpdateOneID(public.ID).SetStatus("inactive").Save(f.ctx)
	require.NoError(t, err)
	require.ErrorIs(t, f.svc.Review(f.ctx, f.admin, c.ID, true, ""), service.ErrContributionGroup)
	require.NoError(t, f.svc.Review(f.ctx, f.admin, c.ID, false, "已停用"))

	results := make(chan error, 25)
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := f.svc.Submit(f.ctx, f.other, f.input(fmt.Sprintf("-limit-%d", i)))
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	success, limited := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, service.ErrContributionLimit) {
			limited++
		} else {
			require.NoError(t, err)
		}
	}
	require.Equal(t, 20, success)
	require.Equal(t, 5, limited)
}

func TestChannelContributionRollback(t *testing.T) {
	f := newContributionFixture(t)
	in := f.input("-collision")
	_, err := integrationEntClient.Group.Create().SetName(in.NewGroupName).Save(f.ctx)
	require.NoError(t, err)
	c, err := f.svc.Submit(f.ctx, f.user, in)
	require.NoError(t, err)
	require.ErrorIs(t, f.svc.Review(f.ctx, f.admin, c.ID, true, ""), service.ErrGroupExists)
	c, err = f.repo.GetForReview(f.ctx, f.admin, c.ID)
	require.NoError(t, err)
	require.Equal(t, "pending", c.Status)
	require.NotEmpty(t, c.KeyEncrypted)
	c, err = f.svc.Submit(f.ctx, f.user, f.input("-rollback"))
	require.NoError(t, err)
	// 强制账号校验失败，验证先创建的分组也一同回滚。
	require.Error(t, f.repo.Review(f.ctx, f.admin, c, true, "", &service.Account{Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey}))
	var count int
	require.NoError(t, integrationDB.QueryRowContext(f.ctx, `SELECT count(*) FROM groups WHERE name=$1`, c.NewGroupName).Scan(&count))
	require.Zero(t, count)
	c, err = f.repo.GetForReview(f.ctx, f.admin, c.ID)
	require.NoError(t, err)
	require.Equal(t, "pending", c.Status)
	_, err = integrationDB.ExecContext(f.ctx, `UPDATE users SET status='disabled' WHERE id=$1`, f.user)
	require.NoError(t, err)
	require.ErrorIs(t, f.svc.Review(f.ctx, f.admin, c.ID, true, ""), service.ErrUserNotFound)
	require.NoError(t, f.svc.Review(f.ctx, f.admin, c.ID, false, "贡献者已停用"))
}
