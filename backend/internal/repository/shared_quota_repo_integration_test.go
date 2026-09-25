//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSharedQuotaRepositoryConcurrentBillingAndBatchSettlement(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := NewUsageBillingRepository(client, integrationDB)
	shared := NewSharedQuotaRepository(integrationDB)
	start := time.Now().UTC().Truncate(time.Second)
	window, err := shared.Window(ctx, "test."+uuid.NewString(), start, start.Add(24*time.Hour), 100)
	require.NoError(t, err)
	var keys []*service.APIKey
	for i := 0; i < 2; i++ {
		user := mustCreateUser(t, client, &service.User{Email: uuid.NewString() + "@example.com", PasswordHash: "hash", Balance: 20})
		key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-" + uuid.NewString(), Name: "shared", Quota: 1, RateLimit5h: 1})
		keys = append(keys, key)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			k := keys[i%2]
			cmd := &service.UsageBillingCommand{RequestID: fmt.Sprintf("%d-%d", window.WindowID, i), APIKeyID: k.ID, UserID: k.UserID, SharedQuotaWindowID: window.WindowID, SharedQuotaCost: 1.25, BillingType: service.BillingTypeSharedQuota}
			_, e := repo.Apply(ctx, cmd)
			if e == nil {
				_, e = repo.Apply(ctx, cmd)
			}
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	q, err := shared.GetWindow(ctx, window.WindowID)
	require.NoError(t, err)
	require.Equal(t, 25.0, q.Used)
	require.Equal(t, 75.0, q.Remaining)
	for _, k := range keys {
		var balance, used, rate float64
		require.NoError(t, integrationDB.QueryRow(`SELECT u.balance,k.quota_used,k.usage_5h FROM users u JOIN api_keys k ON k.user_id=u.id WHERE k.id=$1`, k.ID).Scan(&balance, &used, &rate))
		require.Equal(t, 20.0, balance)
		require.Zero(t, used)
		require.Zero(t, rate)
	}
	key := keys[0]
	hold := &service.BatchImageBalanceHoldCommand{RequestID: uuid.NewString(), APIKeyID: key.ID, UserID: key.UserID, BatchID: uuid.NewString(), HoldAmount: 30}
	_, err = repo.ReserveBatchImageBalance(service.WithSharedQuota(ctx, window), hold)
	require.NoError(t, err)
	q, err = shared.GetWindow(ctx, window.WindowID)
	require.NoError(t, err)
	require.Equal(t, 30.0, q.Held)
	require.Equal(t, 45.0, q.Remaining)
	// 不带共享 context 模拟跨日、重启、停用后的后台结算。
	capture := *hold
	capture.RequestID = uuid.NewString()
	capture.RequestFingerprint = ""
	capture.ActualAmount = 12
	_, err = repo.CaptureBatchImageBalance(ctx, &capture)
	require.NoError(t, err)
	_, err = repo.CaptureBatchImageBalance(ctx, &capture)
	require.NoError(t, err)
	release := *hold
	release.RequestID = uuid.NewString()
	release.RequestFingerprint = ""
	_, err = repo.ReleaseBatchImageBalance(ctx, &release)
	require.NoError(t, err)
	q, err = shared.GetWindow(ctx, window.WindowID)
	require.NoError(t, err)
	require.Zero(t, q.Held)
	require.Equal(t, 37.0, q.Used)
	oversized := *hold
	oversized.BatchID = uuid.NewString()
	oversized.RequestID = uuid.NewString()
	oversized.RequestFingerprint = ""
	oversized.HoldAmount = 100
	_, err = repo.ReserveBatchImageBalance(service.WithSharedQuota(ctx, window), &oversized)
	require.ErrorIs(t, err, service.ErrBatchImageInsufficientBalance)
	mixed := &service.UsageBillingCommand{RequestID: uuid.NewString(), APIKeyID: key.ID, UserID: key.UserID, SharedQuotaWindowID: window.WindowID, SharedQuotaCost: 1, BalanceCost: 1}
	_, err = repo.Apply(ctx, mixed)
	require.Error(t, err)
	q, err = shared.GetWindow(ctx, window.WindowID)
	require.NoError(t, err)
	require.Equal(t, 37.0, q.Used)
}
