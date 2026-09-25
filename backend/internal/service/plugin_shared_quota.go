package service

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/pkg/pluginapi/sharedquota"
)

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
