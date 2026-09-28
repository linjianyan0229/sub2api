//go:build unit

package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

type contributionAdmissionStub struct {
	allowed bool
	calls   int
}

func (s *contributionAdmissionStub) AdmitContributionAccount(context.Context, int64, *SharedQuotaSnapshot) (bool, error) {
	s.calls++
	return s.allowed, nil
}

func TestContributionAdmissionReleasesSlotsAndCannotBypassWithUnlimitedConcurrency(t *testing.T) {
	for _, concurrency := range []int{0, 5} {
		cache := &stubConcurrencyCacheForTest{acquireResult: true}
		admission := &contributionAdmissionStub{}
		svc := NewConcurrencyService(cache)
		svc.contributionAdmission = admission
		result, err := svc.AcquireAccountSlot(context.Background(), 42, concurrency)
		require.NoError(t, err)
		require.False(t, result.Acquired)
		require.Equal(t, 1, admission.calls)
		if concurrency > 0 {
			require.Equal(t, []int64{42}, cache.releasedAccountIDs)
		}
	}
}
