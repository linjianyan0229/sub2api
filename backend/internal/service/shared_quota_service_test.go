package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/pkg/pluginapi/sharedquota"
	"github.com/stretchr/testify/require"
)

type sharedQuotaTestRepo struct {
	policy *SharedQuotaPolicy
	quota  SharedQuotaSnapshot
	err    error
	reads  int
}

func (r *sharedQuotaTestRepo) ActivePolicy(context.Context) (*SharedQuotaPolicy, error) {
	r.reads++
	return r.policy, r.err
}
func (r *sharedQuotaTestRepo) Window(_ context.Context, _ string, _, end time.Time, total float64) (*SharedQuotaSnapshot, error) {
	q := r.quota
	q.Total = total
	q.ResetsAt = end
	return &q, r.err
}
func (r *sharedQuotaTestRepo) GetWindow(context.Context, int64) (*SharedQuotaSnapshot, error) {
	q := r.quota
	return &q, r.err
}

type sharedQuotaTestEncryptor struct{}

func (sharedQuotaTestEncryptor) Encrypt(s string) (string, error) { return s, nil }
func (sharedQuotaTestEncryptor) Decrypt(s string) (string, error) { return s, nil }

func TestSharedQuotaSnapshotAndRequestIsolation(t *testing.T) {
	repo := &sharedQuotaTestRepo{policy: &SharedQuotaPolicy{PluginKey: "test.shared", ConfigEncrypted: `{"daily_quota":100,"reset_time":"03:30"}`}, quota: SharedQuotaSnapshot{WindowID: 7, Used: 12, Held: 3, Remaining: 85}}
	svc, err := NewSharedQuotaService(repo, sharedQuotaTestEncryptor{}, &config.Config{Timezone: "Asia/Shanghai"})
	require.NoError(t, err)
	keys := &APIKeyService{sharedQuota: svc}
	original := &APIKey{User: &User{ID: 1, Role: RoleUser, Balance: 9, Concurrency: 2}, Quota: 1, QuotaUsed: 1, Status: StatusAPIKeyQuotaExhausted}
	key, err := keys.BindSharedQuota(context.Background(), original)
	require.NoError(t, err)
	require.NotSame(t, original, key)
	require.NotSame(t, original.User, key.User)
	require.Nil(t, original.User.SharedQuota)
	require.Equal(t, 9.0, original.User.Balance)
	require.Equal(t, 85.0, key.User.SharedQuota.Remaining)
	require.Equal(t, "Asia/Shanghai", key.User.SharedQuota.Timezone)
	other, err := svc.Snapshot(context.Background(), &User{ID: 2, Role: RoleUser})
	require.NoError(t, err)
	require.Equal(t, key.User.SharedQuota.WindowID, other.WindowID)
	before := repo.reads
	admin, err := svc.Snapshot(context.Background(), &User{Role: RoleAdmin})
	require.NoError(t, err)
	require.Nil(t, admin)
	require.Equal(t, before, repo.reads)
	repo.policy = nil
	disabled, err := keys.BindSharedQuota(context.Background(), original)
	require.NoError(t, err)
	require.Nil(t, disabled.User.SharedQuota)
	require.Equal(t, 9.0, disabled.User.Balance)
	// 已接收快照不随插件停用而变化。
	require.Equal(t, int64(7), key.User.SharedQuota.WindowID)
}

func TestSharedQuotaFailsClosedAndChecksLiveRemaining(t *testing.T) {
	repo := &sharedQuotaTestRepo{quota: SharedQuotaSnapshot{WindowID: 1, Remaining: 0}}
	svc := &SharedQuotaService{repo: repo}
	require.ErrorIs(t, svc.Check(context.Background(), &SharedQuotaSnapshot{WindowID: 1, Remaining: 100}), ErrSharedQuotaExhausted)
	repo.err = errors.New("db offline")
	_, err := svc.Snapshot(context.Background(), &User{Role: RoleUser})
	require.ErrorIs(t, err, ErrBillingServiceUnavailable)
	require.ErrorIs(t, svc.Check(context.Background(), &SharedQuotaSnapshot{WindowID: 1}), ErrBillingServiceUnavailable)
}

func TestSharedQuotaCommandNeverBillsPersonalSources(t *testing.T) {
	user := &User{ID: 1, Role: RoleUser, SharedQuota: &SharedQuotaSnapshot{WindowID: 42}}
	key := &APIKey{ID: 2, User: user, Quota: 1, RateLimit5h: 1}
	params := &postUsageBillingParams{User: user, APIKey: key, Account: &Account{ID: 3, Type: AccountTypeOAuth}, Cost: &CostBreakdown{TotalCost: 1.25, ActualCost: 2.5}, Subscription: &UserSubscription{ID: 4}, IsSubscriptionBill: true, SimpleModeKeyRateLimitOnly: true}
	log := &UsageLog{BillingType: BillingTypeSubscription, SubscriptionID: &params.Subscription.ID}
	cmd := buildUsageBillingCommand("request", log, params)
	require.Equal(t, int64(42), cmd.SharedQuotaWindowID)
	require.Equal(t, 2.5, cmd.SharedQuotaCost)
	require.Zero(t, cmd.BalanceCost)
	require.Zero(t, cmd.SubscriptionCost)
	require.Zero(t, cmd.APIKeyQuotaCost)
	require.Zero(t, cmd.APIKeyRateLimitCost)
	require.Nil(t, cmd.SubscriptionID)
	require.Equal(t, BillingTypeSharedQuota, log.BillingType)
	require.Nil(t, log.SubscriptionID)
	require.False(t, simpleModeKeyRateLimitBillingEnabled(&config.Config{RunMode: config.RunModeSimple, SimpleModeKeyRateLimitEnabled: true}, key))
	_, err := applyUsageBilling(context.Background(), "request", log, params, &billingDeps{}, nil)
	require.Error(t, err)
	user.Role = RoleAdmin
	params.SimpleModeKeyRateLimitOnly = false
	cmd = buildUsageBillingCommand("admin", &UsageLog{}, params)
	require.Zero(t, cmd.SharedQuotaWindowID)
	require.Equal(t, 2.5, cmd.SubscriptionCost)
}

func TestSharedQuotaPluginDoesNotDisplaceTransportOrGrantCredentials(t *testing.T) {
	installation := &PluginInstallation{ID: 2, Manifest: PluginManifest{Capabilities: []PluginCapability{{ID: sharedquota.Capability, Platform: "all", AccountType: "all"}}}}
	manager := &PluginManager{runtimes: map[int64]*pluginRuntime{}}
	route := &pluginRoute{pluginID: 1, rolloutPercent: 80}
	manager.route.Store(route)
	runtime := &pluginRuntime{installation: installation}
	manager.publishRuntimeLocked(installation, runtime)
	require.Same(t, route, manager.route.Load())
	require.Same(t, runtime, manager.runtimes[2])
	require.Empty(t, manager.detachAllRuntimes())
	require.Same(t, runtime, manager.runtimes[2])
	manager.publishUnavailableRoute(1, 100, "transport down")
	require.Same(t, runtime, manager.runtimes[2])
	require.True(t, pluginAccountScopeFromManifest(installation.Manifest).Empty())
	require.Error(t, manager.validateSharedQuotaConfig(installation, []byte(`{}`)))
	require.NoError(t, manager.validateSharedQuotaConfig(installation, []byte(`{"daily_quota":0,"reset_time":"00:00"}`)))
}
