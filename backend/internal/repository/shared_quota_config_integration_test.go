//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/pkg/pluginapi/sharedquota"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type sharedQuotaConfigFixture struct {
	repo     *pluginRepository
	shared   *sharedQuotaRepository
	plugin   *service.PluginInstallation
	location *time.Location
	now      time.Time
}

func newSharedQuotaConfigFixture(t *testing.T) *sharedQuotaConfigFixture {
	t.Helper()
	ctx := context.Background()
	r := &pluginRepository{db: integrationDB}
	key := "test.shared." + uuid.NewString()
	plugin, err := r.Install(ctx, &service.PluginInstallation{
		PluginKey: key, Name: "共享额度配置测试", Version: "0.1.3", Manifest: service.PluginManifest{SchemaVersion: 1, ID: key, Name: "测试插件", Version: "0.1.3"},
		ArtifactData: []byte("test"), ArtifactPath: "/tmp/test.s2plugin", InstallPath: "/tmp/test", BinaryPath: "/tmp/test/plugin",
		BinarySHA256: strings.Repeat("a", 64), SignatureStatus: service.PluginSignatureTrusted, State: service.PluginStateDisabled,
	}, []service.PluginBinding{{Capability: sharedquota.Capability, Platform: "all", AccountType: "all", RolloutPercent: 100}})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := integrationDB.ExecContext(ctx, `DELETE FROM sub2api_plugin_installations WHERE id=$1`, plugin.ID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(ctx, `DELETE FROM shared_quota_windows WHERE plugin_key=$1`, key)
		require.NoError(t, err)
	})
	_, err = integrationDB.ExecContext(ctx, `UPDATE sub2api_plugin_bindings SET enabled=TRUE WHERE plugin_id=$1`, plugin.ID)
	require.NoError(t, err)
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	return &sharedQuotaConfigFixture{repo: r, shared: &sharedQuotaRepository{db: integrationDB}, plugin: plugin, location: loc, now: time.Date(2026, 9, 26, 10, 15, 0, 0, loc)}
}

func (f *sharedQuotaConfigFixture) save(t *testing.T, total float64, reset, mode string) {
	f.saveGroups(t, total, reset, mode, nil)
}

func (f *sharedQuotaConfigFixture) saveGroups(t *testing.T, total float64, reset, mode string, groups map[int64]float64) {
	t.Helper()
	ctx := context.Background()
	p, err := f.repo.GetByID(ctx, f.plugin.ID)
	require.NoError(t, err)
	c := sharedquota.Config{DailyQuota: total, ResetTime: reset, ApplyMode: mode, GroupLimits: groups}
	raw, err := json.Marshal(c)
	require.NoError(t, err)
	update := &service.SharedQuotaConfigUpdate{PluginID: p.ID, Encrypted: string(raw), ExpectedEncrypted: p.ConfigEncrypted, ExpectedBinarySHA256: p.BinarySHA256, Next: c, Now: f.now, Location: f.location}
	if old, err := sharedquota.Parse([]byte(p.ConfigEncrypted)); err == nil {
		update.Previous = &old
	}
	require.NoError(t, f.repo.UpdateSharedQuotaConfig(ctx, update))
}

func (f *sharedQuotaConfigFixture) snapshot(t *testing.T, now time.Time) *service.SharedQuotaSnapshot {
	t.Helper()
	ctx := context.Background()
	p, err := f.shared.ActivePolicy(ctx)
	require.NoError(t, err)
	require.NotNil(t, p)
	c, err := sharedquota.Parse([]byte(p.ConfigEncrypted))
	require.NoError(t, err)
	start, end := c.Window(now, f.location)
	p.GroupLimits = c.GroupLimits
	q, err := f.shared.Window(ctx, p, now, start, end, c.DailyQuota)
	require.NoError(t, err)
	return q
}

func TestSharedQuotaConfigImmediateAndScheduled(t *testing.T) {
	f := newSharedQuotaConfigFixture(t)
	ctx := context.Background()
	f.save(t, 100, "00:00", sharedquota.ApplyImmediately)
	q := f.snapshot(t, f.now)
	_, err := integrationDB.ExecContext(ctx, `UPDATE shared_quota_windows SET used=20,held=10 WHERE id=$1`, q.WindowID)
	require.NoError(t, err)
	f.save(t, 200, "00:00", sharedquota.ApplyNextReset)
	current := f.snapshot(t, f.now)
	require.Equal(t, q.WindowID, current.WindowID)
	require.Equal(t, 100.0, current.Total)
	require.Equal(t, 70.0, current.Remaining)
	// 连续修改待生效值和进程重建都不改变当前总额。
	f.save(t, 300, "00:00", sharedquota.ApplyNextReset)
	f.shared = &sharedQuotaRepository{db: integrationDB}
	require.Equal(t, 100.0, f.snapshot(t, current.ResetsAt.Add(-time.Second)).Total)
	f.save(t, 150, "00:00", sharedquota.ApplyImmediately)
	// 保存返回时数据库已更新，不需要下一次用户查询触发。
	current, err = f.shared.GetWindow(ctx, q.WindowID)
	require.NoError(t, err)
	require.Equal(t, 150.0, current.Total)
	require.Equal(t, 20.0, current.Used)
	require.Equal(t, 10.0, current.Held)
	require.Equal(t, 120.0, current.Remaining)
	next := f.snapshot(t, current.ResetsAt)
	require.NotEqual(t, current.WindowID, next.WindowID)
	require.Equal(t, 150.0, next.Total)
	require.Zero(t, next.Used)
	require.Zero(t, next.Held)
	old, err := f.shared.GetWindow(ctx, current.WindowID)
	require.NoError(t, err)
	require.Equal(t, 20.0, old.Used)
	require.Equal(t, 10.0, old.Held)
}

func TestSharedQuotaConfigDeferredWithoutPriorUsageAndResetTimeChange(t *testing.T) {
	f := newSharedQuotaConfigFixture(t)
	ctx := context.Background()
	// 模拟旧插件已经配置但尚未产生任何账期。
	require.NoError(t, f.repo.UpdateConfig(ctx, f.plugin.ID, `{"daily_quota":100,"reset_time":"00:00"}`, f.plugin.BinarySHA256))
	f.save(t, 200, "11:00", sharedquota.ApplyNextReset)
	current := f.snapshot(t, f.now)
	require.Equal(t, 100.0, current.Total)
	require.Equal(t, 11, current.ResetsAt.In(f.location).Hour())
	_, err := integrationDB.ExecContext(ctx, `UPDATE shared_quota_windows SET used=15 WHERE id=$1`, current.WindowID)
	require.NoError(t, err)
	f.save(t, 250, "12:00", sharedquota.ApplyNextReset)
	current = f.snapshot(t, f.now)
	require.Equal(t, 100.0, current.Total)
	require.Equal(t, 15.0, current.Used)
	require.Equal(t, 100.0, f.snapshot(t, current.ResetsAt.Add(-time.Second)).Total)
	next := f.snapshot(t, current.ResetsAt)
	require.Equal(t, 250.0, next.Total)
	require.Zero(t, next.Used)
}

func TestSharedQuotaConfigConcurrentSaveRejectsStalePolicy(t *testing.T) {
	f := newSharedQuotaConfigFixture(t)
	ctx := context.Background()
	f.save(t, 100, "00:00", sharedquota.ApplyImmediately)
	p, err := f.shared.ActivePolicy(ctx)
	require.NoError(t, err)
	old, _ := sharedquota.Parse([]byte(p.ConfigEncrypted))
	start, end := old.Window(f.now, f.location)
	q := f.snapshot(t, f.now)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, total := range []float64{200, 300} {
		wg.Add(1)
		go func(total float64) {
			defer wg.Done()
			c := sharedquota.Config{DailyQuota: total, ResetTime: "00:00", ApplyMode: sharedquota.ApplyImmediately}
			raw, _ := json.Marshal(c)
			results <- f.repo.UpdateSharedQuotaConfig(ctx, &service.SharedQuotaConfigUpdate{PluginID: f.plugin.ID, Encrypted: string(raw), ExpectedEncrypted: p.ConfigEncrypted, ExpectedBinarySHA256: f.plugin.BinarySHA256, Previous: &old, Next: c, Now: f.now, Location: f.location})
		}(total)
	}
	wg.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		} else {
			require.ErrorIs(t, err, service.ErrPluginStateChanged)
		}
	}
	require.Equal(t, 1, succeeded)
	_, err = f.shared.Window(ctx, p, f.now, start, end, 100)
	require.ErrorIs(t, err, service.ErrSharedQuotaPolicyChanged)
	latest := f.snapshot(t, f.now)
	require.Equal(t, q.WindowID, latest.WindowID)
	require.Contains(t, []float64{200, 300}, latest.Total)
	// 降低到已用+冻结以下仍保留用量，仅将剩余置零。
	_, err = integrationDB.ExecContext(ctx, `UPDATE shared_quota_windows SET used=50,held=10 WHERE id=$1`, q.WindowID)
	require.NoError(t, err)
	f.save(t, 10, "00:00", sharedquota.ApplyImmediately)
	latest = f.snapshot(t, f.now)
	require.Zero(t, latest.Remaining)
	require.Equal(t, 50.0, latest.Used)
	require.Equal(t, 10.0, latest.Held)
}

func TestSharedQuotaGroupConfigImmediateAndScheduled(t *testing.T) {
	f := newSharedQuotaConfigFixture(t)
	ctx := context.Background()
	groupA := mustCreateGroup(t, integrationEntClient, &service.Group{Name: "共享上限A-" + uuid.NewString()})
	groupB := mustCreateGroup(t, integrationEntClient, &service.Group{Name: "共享上限B-" + uuid.NewString()})
	f.saveGroups(t, 100, "00:00", sharedquota.ApplyImmediately, map[int64]float64{groupA.ID: 10})
	q := f.snapshot(t, f.now)
	a, err := f.shared.GetGroupWindow(ctx, q.WindowID, groupA.ID)
	require.NoError(t, err)
	require.Equal(t, 100.0, a.Total)
	require.Equal(t, 10.0, a.GroupQuota.Limit)
	b, err := f.shared.GetGroupWindow(ctx, q.WindowID, groupB.ID)
	require.NoError(t, err)
	require.Nil(t, b.GroupQuota)
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO shared_quota_group_usage(window_id,group_id,used,held) VALUES($1,$2,5,2),($1,$3,4,0)`, q.WindowID, groupA.ID, groupB.ID)
	require.NoError(t, err)
	_, err = integrationDB.ExecContext(ctx, `UPDATE shared_quota_windows SET used=9,held=2 WHERE id=$1`, q.WindowID)
	require.NoError(t, err)
	// 立即新增、降低、移除上限，都保留分组已用和冻结；未选中的分组仍使用同一共享池。
	f.saveGroups(t, 100, "00:00", sharedquota.ApplyImmediately, map[int64]float64{groupA.ID: 6, groupB.ID: 3})
	a, err = f.shared.GetGroupWindow(ctx, q.WindowID, groupA.ID)
	require.NoError(t, err)
	require.Equal(t, 89.0, a.Remaining)
	require.Equal(t, 5.0, a.GroupQuota.Used)
	require.Equal(t, 2.0, a.GroupQuota.Held)
	require.Zero(t, a.GroupQuota.Remaining)
	b, err = f.shared.GetGroupWindow(ctx, q.WindowID, groupB.ID)
	require.NoError(t, err)
	require.Zero(t, b.GroupQuota.Remaining)
	f.saveGroups(t, 100, "00:00", sharedquota.ApplyImmediately, map[int64]float64{groupA.ID: 20})
	b, err = f.shared.GetGroupWindow(ctx, q.WindowID, groupB.ID)
	require.NoError(t, err)
	require.Nil(t, b.GroupQuota)
	require.Equal(t, a.WindowID, b.WindowID)
	// 延迟新增 B、移除 A：当前账期保持原配置，下一账期一起切换并重置用量。
	f.saveGroups(t, 200, "11:00", sharedquota.ApplyNextReset, map[int64]float64{groupB.ID: 30})
	f.shared = &sharedQuotaRepository{db: integrationDB}
	current := f.snapshot(t, f.now)
	a, err = f.shared.GetGroupWindow(ctx, current.WindowID, groupA.ID)
	require.NoError(t, err)
	require.Equal(t, 20.0, a.GroupQuota.Limit)
	require.Equal(t, 13.0, a.GroupQuota.Remaining)
	b, err = f.shared.GetGroupWindow(ctx, current.WindowID, groupB.ID)
	require.NoError(t, err)
	require.Nil(t, b.GroupQuota)
	next := f.snapshot(t, current.ResetsAt)
	require.NotEqual(t, current.WindowID, next.WindowID)
	a, err = f.shared.GetGroupWindow(ctx, next.WindowID, groupA.ID)
	require.NoError(t, err)
	require.Nil(t, a.GroupQuota)
	require.Equal(t, 200.0, a.Remaining)
	b, err = f.shared.GetGroupWindow(ctx, next.WindowID, groupB.ID)
	require.NoError(t, err)
	require.Equal(t, 30.0, b.GroupQuota.Remaining)
	require.Zero(t, b.GroupQuota.Used)
	// 旧账期保留用量，供在途任务结算。
	old, err := f.shared.GetGroupWindow(ctx, current.WindowID, groupA.ID)
	require.NoError(t, err)
	require.Equal(t, 5.0, old.GroupQuota.Used)
}

func TestSharedQuotaConfigRejectsUnknownGroup(t *testing.T) {
	f := newSharedQuotaConfigFixture(t)
	ctx := context.Background()
	f.save(t, 100, "00:00", sharedquota.ApplyImmediately)
	p, err := f.repo.GetByID(ctx, f.plugin.ID)
	require.NoError(t, err)
	previous, err := sharedquota.Parse([]byte(p.ConfigEncrypted))
	require.NoError(t, err)
	next := sharedquota.Config{DailyQuota: 200, ResetTime: "00:00", GroupLimits: map[int64]float64{9007199254740991: 10}}
	raw, err := json.Marshal(next)
	require.NoError(t, err)
	err = f.repo.UpdateSharedQuotaConfig(ctx, &service.SharedQuotaConfigUpdate{PluginID: p.ID, Encrypted: string(raw), ExpectedEncrypted: p.ConfigEncrypted, ExpectedBinarySHA256: p.BinarySHA256, Previous: &previous, Next: next, Now: f.now, Location: f.location})
	require.ErrorIs(t, err, service.ErrGroupNotFound)
	after, err := f.repo.GetByID(ctx, f.plugin.ID)
	require.NoError(t, err)
	require.Equal(t, p.ConfigEncrypted, after.ConfigEncrypted)
	require.Equal(t, 100.0, f.snapshot(t, f.now).Total)
}
