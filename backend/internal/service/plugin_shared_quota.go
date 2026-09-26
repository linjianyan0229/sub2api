package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/pluginapi/sharedquota"
)

type SharedQuotaConfigUpdate struct {
	PluginID             int64
	Encrypted            string
	ExpectedEncrypted    string
	ExpectedBinarySHA256 string
	Previous             *sharedquota.Config
	Next                 sharedquota.Config
	Now                  time.Time
	Location             *time.Location
}

type SharedQuotaConfigRepository interface {
	UpdateSharedQuotaConfig(context.Context, *SharedQuotaConfigUpdate) error
}

func (m *PluginManager) savePluginConfig(ctx context.Context, installation *PluginInstallation, previous, next json.RawMessage, encrypted string) error {
	if !isSharedQuotaPlugin(installation) {
		return m.repo.UpdateConfig(ctx, installation.ID, encrypted, installation.BinarySHA256)
	}
	repo, ok := m.repo.(SharedQuotaConfigRepository)
	if !ok {
		return errors.New("宿主不支持共享额度生效方式")
	}
	loc, err := time.LoadLocation(m.cfg.Timezone)
	if err != nil {
		return err
	}
	config, err := sharedquota.Parse(next)
	if err != nil {
		return err
	}
	update := &SharedQuotaConfigUpdate{
		PluginID: installation.ID, Encrypted: encrypted, ExpectedEncrypted: installation.ConfigEncrypted,
		ExpectedBinarySHA256: installation.BinarySHA256, Next: config, Now: time.Now(), Location: loc,
	}
	if old, err := sharedquota.Parse(previous); err == nil {
		update.Previous = &old
	}
	return repo.UpdateSharedQuotaConfig(ctx, update)
}

func isSharedQuotaPlugin(p *PluginInstallation) bool {
	if p == nil {
		return false
	}
	for _, c := range p.Manifest.Capabilities {
		if c.ID == sharedquota.Capability {
			return true
		}
	}
	return false
}

func hasEnabledSharedQuotaBinding(bindings []PluginBinding) bool {
	for _, b := range bindings {
		if b.Capability == sharedquota.Capability && b.Enabled {
			return true
		}
	}
	return false
}

func (m *PluginManager) validateSharedQuotaConfig(p *PluginInstallation, raw []byte) error {
	if !isSharedQuotaPlugin(p) {
		return nil
	}
	_, err := sharedquota.Parse(raw)
	return err
}

// 计费策略与网络传输插件独立运行；它不获得上游账号或凭据访问权限。
func (m *PluginManager) reconcileSharedQuota(ctx context.Context, installations []*PluginInstallation) error {
	desired := map[int64]bool{}
	var errs error
	for _, p := range installations {
		if !isSharedQuotaPlugin(p) || !hasEnabledSharedQuotaBinding(p.Bindings) {
			continue
		}
		desired[p.ID] = true
		m.mu.Lock()
		r := m.runtimes[p.ID]
		m.mu.Unlock()
		if r != nil && !r.client.Exited() && r.installation.ConfigEncrypted == p.ConfigEncrypted && r.installation.BinarySHA256 == p.BinarySHA256 {
			continue
		}
		local, err := m.ensureLocalInstallation(ctx, p)
		if err == nil {
			r, err = m.prepareRuntime(ctx, local, true)
		}
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		latest, err := m.repo.GetByID(ctx, p.ID)
		if err != nil || latest == nil || !hasEnabledSharedQuotaBinding(latest.Bindings) || latest.ConfigEncrypted != p.ConfigEncrypted || latest.BinarySHA256 != p.BinarySHA256 {
			r.kill()
			errs = errors.Join(errs, err)
			continue
		}
		if err := m.repo.MarkRuntimeHealthy(ctx, p.ID, p.BinarySHA256, p.ConfigEncrypted); err != nil {
			r.kill()
			errs = errors.Join(errs, err)
			continue
		}
		m.mu.Lock()
		m.publishRuntimeLocked(local, r)
		m.mu.Unlock()
	}
	m.mu.Lock()
	var stale []*pluginRuntime
	for id, r := range m.runtimes {
		if isSharedQuotaPlugin(r.installation) && !desired[id] {
			stale = append(stale, r)
			delete(m.runtimes, id)
		}
	}
	m.mu.Unlock()
	for _, r := range stale {
		r.drain(10 * time.Second)
	}
	return errs
}
