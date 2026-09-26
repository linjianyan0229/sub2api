//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type visibilityUserRepo struct {
	UserRepository
	user *User
	err  error
}

func (r *visibilityUserRepo) GetByID(context.Context, int64) (*User, error) { return r.user, r.err }

type visibilitySubRepo struct {
	UserSubscriptionRepository
	subscriptions []UserSubscription
	err           error
	calls         int
}

func (r *visibilitySubRepo) ListActiveByUserID(_ context.Context, userID int64) ([]UserSubscription, error) {
	r.calls++
	// Match the repository's active-status and expiry predicates.
	active := make([]UserSubscription, 0)
	for _, sub := range r.subscriptions {
		if sub.UserID == userID && sub.IsActive() {
			active = append(active, sub)
		}
	}
	return active, r.err
}

type visibilityGroupRepo struct {
	GroupRepository
	groups []Group
}

func (r *visibilityGroupRepo) ListActive(context.Context) ([]Group, error) { return r.groups, nil }

type sharedGroupVisibilityRepo struct {
	sharedQuotaTestRepo
	queried []int64
	err     error
}

func (r *sharedGroupVisibilityRepo) GetGroupQuotas(_ context.Context, windowID int64, ids []int64) (map[int64]SharedGroupQuota, error) {
	r.queried = append([]int64(nil), ids...)
	return map[int64]SharedGroupQuota{7: {Limit: 10, Used: 2, Held: 1, Remaining: 7}, 8: {Limit: 0}}, r.err
}

func TestSharedQuotaAvailableGroupsOnlyExposeAuthorizedCurrentQuotas(t *testing.T) {
	ctx := context.Background()
	repo := &sharedGroupVisibilityRepo{sharedQuotaTestRepo: sharedQuotaTestRepo{
		policy: &SharedQuotaPolicy{PluginKey: "test.shared", ConfigEncrypted: `{"daily_quota":100,"reset_time":"00:00"}`},
		quota:  SharedQuotaSnapshot{WindowID: 1, Remaining: 100},
	}}
	shared, err := NewSharedQuotaService(repo, sharedQuotaTestEncryptor{}, &config.Config{Timezone: "UTC"})
	require.NoError(t, err)
	groups := &visibilityGroupRepo{groups: []Group{{ID: 7}, {ID: 8, IsExclusive: true}, {ID: 9}}}
	user := &User{ID: 1, Role: RoleUser}
	svc := &APIKeyService{sharedQuota: shared, userRepo: &visibilityUserRepo{user: user}, groupRepo: groups, userSubRepo: &visibilitySubRepo{}}
	available, err := svc.GetAvailableGroups(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, []int64{7, 9}, repo.queried)
	require.Len(t, available, 2)
	require.Equal(t, &SharedGroupQuota{Limit: 10, Used: 2, Held: 1, Remaining: 7}, available[0].SharedQuota)
	require.Nil(t, available[1].SharedQuota, "未配置分组上限不应显示零额度")
	require.Nil(t, groups.groups[0].SharedQuota, "展示数据不应污染分组仓储返回值")

	user.RestrictPublicGroups, user.AllowedGroups = true, []int64{8}
	available, err = svc.GetAvailableGroups(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, []int64{8}, repo.queried)
	require.Len(t, available, 1)
	require.NotNil(t, available[0].SharedQuota, "零上限必须显示")
	require.Zero(t, available[0].SharedQuota.Limit)

	repo.err = errors.New("共享账本读取失败")
	available, err = svc.GetAvailableGroups(ctx, 1)
	require.ErrorIs(t, err, repo.err)
	require.Nil(t, available)
	repo.queried = nil
	user.Role = RoleAdmin
	_, err = svc.GetAvailableGroups(ctx, 1)
	require.NoError(t, err)
	require.Nil(t, repo.queried, "管理员沿用原额度展示")
	user.Role, repo.policy = RoleUser, nil
	_, err = svc.GetAvailableGroups(ctx, 1)
	require.NoError(t, err)
	require.Nil(t, repo.queried, "插件停用不应返回共享分组额度")
}

func TestGetUserGroupVisibilityIncludesActiveSubscriptions(t *testing.T) {
	for _, restricted := range []bool{false, true} {
		t.Run(map[bool]string{false: "unrestricted", true: "restricted"}[restricted], func(t *testing.T) {
			now := time.Now()
			subs := &visibilitySubRepo{subscriptions: []UserSubscription{
				{UserID: 1, GroupID: 42, Status: SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour)},
				{UserID: 1, GroupID: 43, Status: SubscriptionStatusActive, ExpiresAt: now.Add(-time.Hour)},
				{UserID: 1, GroupID: 44, Status: "expired", ExpiresAt: now.Add(time.Hour)},
				{UserID: 2, GroupID: 45, Status: SubscriptionStatusActive, ExpiresAt: now.Add(time.Hour)},
			}}
			svc := &APIKeyService{
				userRepo:    &visibilityUserRepo{user: &User{ID: 1, AllowedGroups: []int64{7}, RestrictPublicGroups: restricted}},
				userSubRepo: subs,
				groupRepo:   &visibilityGroupRepo{groups: []Group{{ID: 42, IsExclusive: true, SubscriptionType: "subscription"}}},
			}
			available, err := svc.GetAvailableGroups(context.Background(), 1)
			require.NoError(t, err)
			require.Len(t, available, 1)
			visible, restrict, err := svc.GetUserGroupVisibility(context.Background(), 1)
			require.NoError(t, err)
			require.Equal(t, restricted, restrict)
			require.Equal(t, map[int64]struct{}{7: {}, 42: {}}, visible)
			require.Contains(t, visible, available[0].ID, "a subscribed group that can be bound must be visible")
		})
	}
}

func TestGetUserGroupVisibilityEmptyAndErrors(t *testing.T) {
	failure := errors.New("repository unavailable")
	for _, tc := range []struct {
		name            string
		userErr, subErr error
	}{
		{name: "empty"}, {name: "user failure", userErr: failure}, {name: "subscription failure", subErr: failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subs := &visibilitySubRepo{err: tc.subErr}
			svc := &APIKeyService{userRepo: &visibilityUserRepo{user: &User{ID: 1}, err: tc.userErr}, userSubRepo: subs}
			got, _, err := svc.GetUserGroupVisibility(context.Background(), 1)
			if tc.userErr != nil || tc.subErr != nil {
				require.ErrorIs(t, err, failure)
				require.Nil(t, got, "repository failures must not become anonymous visibility")
			} else {
				require.NoError(t, err)
				require.NotNil(t, got, "an empty logged-in user must not become anonymous")
				require.Empty(t, got)
			}
			if tc.userErr != nil {
				require.Zero(t, subs.calls)
			}
		})
	}
}
