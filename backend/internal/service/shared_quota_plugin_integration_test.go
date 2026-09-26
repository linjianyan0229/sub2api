package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type sharedPluginMemoryRepo struct {
	PluginRepository
	p *PluginInstallation
}

func (r *sharedPluginMemoryRepo) GetByID(context.Context, int64) (*PluginInstallation, error) {
	p := *r.p
	p.Bindings = append([]PluginBinding(nil), r.p.Bindings...)
	return &p, nil
}
func (r *sharedPluginMemoryRepo) List(ctx context.Context) ([]*PluginInstallation, error) {
	p, e := r.GetByID(ctx, r.p.ID)
	return []*PluginInstallation{p}, e
}
func (r *sharedPluginMemoryRepo) UpdateConfig(_ context.Context, _ int64, cipher, _ string) error {
	r.p.ConfigEncrypted = cipher
	return nil
}
func (r *sharedPluginMemoryRepo) UpdateSharedQuotaConfig(ctx context.Context, update *SharedQuotaConfigUpdate) error {
	return r.UpdateConfig(ctx, update.PluginID, update.Encrypted, update.ExpectedBinarySHA256)
}

func (r *sharedPluginMemoryRepo) BeginEnable(context.Context, int64, string, string) error {
	r.p.State = PluginStateStarting
	return nil
}
func (r *sharedPluginMemoryRepo) UpdateState(_ context.Context, _ int64, state, last string, _ *time.Time, _, _ string) error {
	r.p.State = state
	r.p.LastError = last
	return nil
}
func (r *sharedPluginMemoryRepo) UpdateBindingsAndState(_ context.Context, _ int64, bindings []PluginBinding, state, last string, _ *time.Time, _, _ string) error {
	r.p.Bindings = append([]PluginBinding(nil), bindings...)
	r.p.State = state
	r.p.LastError = last
	return nil
}
func (r *sharedPluginMemoryRepo) MarkRuntimeHealthy(context.Context, int64, string, string) error {
	r.p.State = PluginStateEnabled
	return nil
}

func TestSharedQuotaSignedPackageLifecycle(t *testing.T) {
	packagePath := os.Getenv("SUB2API_TEST_SHARED_QUOTA_PACKAGE")
	if packagePath == "" {
		t.Skip("未提供共享额度插件包，跳过真实插件进程验证")
	}
	cfg := testPluginConfig(t.TempDir(), false)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(packagePath), "trusted-publisher.yaml"))
	require.NoError(t, err)
	var trust struct {
		Plugins struct {
			Publishers map[string]string `yaml:"trusted_publishers"`
		} `yaml:"plugins"`
	}
	require.NoError(t, yaml.Unmarshal(data, &trust))
	cfg.Plugins.TrustedPublishers = trust.Plugins.Publishers
	host := PluginHostInfo{Version: "0.2.9", BuildType: "release"}
	installer := NewPluginPackageInstaller(cfg, host)
	f, err := os.Open(packagePath)
	require.NoError(t, err)
	defer f.Close()
	installed, err := installer.Install(context.Background(), f, nil)
	require.NoError(t, err)
	require.Equal(t, PluginSignatureTrusted, installed.SignatureStatus)
	require.True(t, isSharedQuotaPlugin(installed))
	require.True(t, pluginAccountScopeFromManifest(installed.Manifest).Empty())
	installed.ID = 1
	installed.Bindings = []PluginBinding{{PluginID: 1, Capability: PluginCapabilitySharedQuota, Platform: "all", AccountType: "all", RolloutPercent: 100}}
	repo := &sharedPluginMemoryRepo{p: installed}
	manager := NewPluginManager(repo, sharedQuotaTestEncryptor{}, cfg, host, nil)
	defer manager.Stop()
	route := &pluginRoute{pluginID: 99, rolloutPercent: 100}
	manager.route.Store(route)
	ctx := context.Background()
	_, err = manager.SaveConfig(ctx, 1, []byte(`{"daily_quota":20,"reset_time":"04:30"}`))
	require.NoError(t, err)
	_, err = manager.SaveConfig(ctx, 1, []byte(`{"daily_quota":-1,"reset_time":"04:30"}`))
	require.Error(t, err)
	_, err = manager.Enable(ctx, 1, true, 10)
	require.ErrorContains(t, err, "必须全站启用")
	enabled, err := manager.Enable(ctx, 1, true, 100)
	require.NoError(t, err)
	require.True(t, enabled.RuntimeHealthy)
	require.Same(t, route, manager.route.Load())
	runtime := manager.runtimes[1]
	updated, err := manager.SaveConfig(ctx, 1, []byte(`{"daily_quota":30,"reset_time":"04:30","apply_mode":"next_reset"}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"daily_quota":30,"reset_time":"04:30","apply_mode":"next_reset"}`, string(updated))
	runtime.kill()
	items, err := repo.List(ctx)
	require.NoError(t, err)
	require.NoError(t, manager.reconcileSharedQuota(ctx, items))
	require.NotSame(t, runtime, manager.runtimes[1])
	require.Same(t, route, manager.route.Load())
	disabled, err := manager.Disable(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, PluginStateDisabled, disabled.State)
	require.False(t, hasEnabledSharedQuotaBinding(disabled.Bindings))
	require.Same(t, route, manager.route.Load())
}
