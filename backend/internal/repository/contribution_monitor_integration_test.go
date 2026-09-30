//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type contributionMonitorRuntime struct{}
type contributionMonitorScheduler struct{ last *service.ChannelMonitor }

func (s *contributionMonitorScheduler) Schedule(m *service.ChannelMonitor) { s.last = m }
func (*contributionMonitorScheduler) Unschedule(int64)                     {}

func (contributionMonitorRuntime) GetChannelMonitorRuntime(context.Context) service.ChannelMonitorRuntime {
	return service.ChannelMonitorRuntime{Enabled: true, Mode: service.ChannelMonitorModeV1}
}

func TestContributionMonitorHistoryAndPermissions(t *testing.T) {
	f := newContributionFixture(t)
	repo := f.repo.(*channelContributionRepository)
	monitorRepo := NewChannelMonitorRepository(integrationEntClient, integrationDB)
	monitors := service.NewChannelMonitorService(monitorRepo, &AESEncryptor{key: make([]byte, 32)})
	monitors.SetRuntimeReader(contributionMonitorRuntime{})
	scheduler := &contributionMonitorScheduler{}
	monitors.SetScheduler(scheduler)
	f.svc.SetMonitorService(monitors)
	in := f.input("-监控")
	in.Models = []string{"gpt-5"}
	c, err := f.svc.Submit(f.ctx, f.user, in)
	require.NoError(t, err)
	require.NoError(t, f.svc.Review(f.ctx, f.admin, c.ID, true, ""))
	config := service.ContributionAccountInput{SiteName: in.SiteName, BaseURL: in.BaseURL, Models: in.Models, ShowInStatus: true, MonitorModel: "gpt-5"}
	require.NoError(t, f.svc.UpdateOwnedAccount(f.ctx, f.user, c.ID, config))
	items, err := repo.ContributionMonitors(f.ctx, []int64{c.ID})
	require.NoError(t, err)
	require.Len(t, items, 1)
	m := items[0]
	require.NotNil(t, scheduler.last)
	require.Equal(t, m.ID, scheduler.last.ID)
	require.True(t, scheduler.last.Enabled)
	require.True(t, m.Enabled)
	require.Empty(t, m.ExtraHeaders)
	require.Equal(t, c.ID, m.ContributionID)
	read, err := monitors.Get(f.ctx, m.ID)
	require.NoError(t, err)
	require.Empty(t, read.APIKey)
	require.False(t, read.APIKeyDecryptFailed)
	views, err := f.svc.MonitorViews(f.ctx, f.other, 0)
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Empty(t, views[0].Monitor.PrimaryStatus)
	require.Empty(t, views[0].Monitor.Timeline)
	require.Nil(t, views[0].Monitor.PrimaryLatencyMs)
	// 仅在隔离测试库写入历史 fixture，验证逐时段延迟没有被账号状态替代。
	latency1, latency2 := 145, 860
	now := time.Now().UTC()
	require.NoError(t, monitorRepo.InsertHistoryBatch(f.ctx, []*service.ChannelMonitorHistoryRow{
		{MonitorID: m.ID, Model: "gpt-5", Status: service.MonitorStatusOperational, LatencyMs: &latency1, CheckedAt: now.Add(-time.Minute)},
		{MonitorID: m.ID, Model: "gpt-5", Status: service.MonitorStatusDegraded, LatencyMs: &latency2, CheckedAt: now},
	}))
	require.NoError(t, f.svc.UpdateOwnedAccount(f.ctx, f.user, c.ID, config))
	require.NoError(t, f.svc.InitializeMonitors(f.ctx))
	items, err = repo.ContributionMonitors(f.ctx, []int64{c.ID})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, m.ID, items[0].ID)
	views, err = f.svc.MonitorViews(f.ctx, f.other, 0)
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.Len(t, views[0].Monitor.Timeline, 2)
	require.Equal(t, 860, *views[0].Monitor.Timeline[0].LatencyMs)
	require.Equal(t, 145, *views[0].Monitor.Timeline[1].LatencyMs)
	require.Equal(t, service.MonitorStatusDegraded, views[0].Monitor.PrimaryStatus)
	detail, err := f.svc.MonitorDetail(f.ctx, f.other, c.ID)
	require.NoError(t, err)
	require.Len(t, detail.Models, 1)
	require.Equal(t, 860, *detail.Models[0].LatestLatencyMs)
	blob, err := json.Marshal(views)
	require.NoError(t, err)
	require.NotContains(t, string(blob), "sk-private")
	require.NotContains(t, string(blob), "api.example.com")
	public, err := monitors.ListUserView(f.ctx)
	require.NoError(t, err)
	for _, v := range public {
		require.NotEqual(t, m.ID, v.ID)
	}
	_, err = monitors.GetUserDetail(f.ctx, m.ID)
	require.ErrorIs(t, err, service.ErrChannelMonitorNotFound)
	a, err := repo.ContributionProbeAccount(f.ctx, c.ID)
	require.NoError(t, err)
	require.Equal(t, "sk-private-test-value", a.GetCredential("api_key"))
	_, err = integrationDB.Exec(`UPDATE groups SET is_exclusive=true WHERE id IN (SELECT group_id FROM account_groups WHERE account_id=$1)`, a.ID)
	require.NoError(t, err)
	views, err = f.svc.MonitorViews(f.ctx, f.other, 0)
	require.NoError(t, err)
	require.Empty(t, views)
	_, err = f.svc.MonitorDetail(f.ctx, f.other, c.ID)
	require.ErrorIs(t, err, service.ErrChannelMonitorNotFound)
	_, err = f.svc.MonitorDetail(f.ctx, f.admin, c.ID)
	require.NoError(t, err)
	config.ShowInStatus = false
	require.NoError(t, f.svc.UpdateOwnedAccount(f.ctx, f.user, c.ID, config))
	items, err = repo.ContributionMonitors(f.ctx, []int64{c.ID})
	require.NoError(t, err)
	require.False(t, items[0].Enabled)
	require.False(t, scheduler.last.Enabled)
	_, err = repo.ContributionProbeAccount(f.ctx, c.ID)
	require.ErrorIs(t, err, service.ErrContributionNotFound)
	_, err = f.svc.MonitorDetail(f.ctx, f.admin, c.ID)
	require.ErrorIs(t, err, service.ErrChannelMonitorNotFound)
	config.ShowInStatus = true
	require.NoError(t, f.svc.UpdateOwnedAccount(f.ctx, f.user, c.ID, config))
	detail, err = f.svc.MonitorDetail(f.ctx, f.admin, c.ID)
	require.NoError(t, err)
	require.Equal(t, 860, *detail.Models[0].LatestLatencyMs)
	_, err = integrationDB.Exec(`UPDATE accounts SET deleted_at=NOW() WHERE id=$1`, a.ID)
	require.NoError(t, err)
	_, err = repo.ContributionProbeAccount(f.ctx, c.ID)
	require.ErrorIs(t, err, service.ErrContributionNotFound)
	views, err = f.svc.MonitorViews(f.ctx, f.admin, 0)
	require.NoError(t, err)
	require.Empty(t, views)
}
