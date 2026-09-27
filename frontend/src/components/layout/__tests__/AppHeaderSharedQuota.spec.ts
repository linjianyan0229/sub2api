import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AppHeader from '../AppHeader.vue'

const state = vi.hoisted(() => ({
  isAdmin: true, isSharedQuota: false,
  user: { username: '站长', role: 'admin', balance: 20, frozen_balance: 2, site_shared_quota: { total: 100, remaining: 85, used: 12, held: 3, resets_at: '2026-09-28T00:00:00Z' } }
}))
vi.mock('@/stores', () => ({ useAuthStore: () => state, useAppStore: () => ({ docUrl: '', contactInfo: '' }) }))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({}) }))
vi.mock('vue-router', () => ({ useRouter: () => ({}), useRoute: () => ({ meta: {} }) }))
vi.mock('vue-i18n', async (importOriginal) => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/utils/featureFlags', () => ({ FeatureFlags: {}, isFeatureFlagEnabled: () => false }))
vi.mock('@/router/title', () => ({ resolveRouteMetaKeys: () => ({}) }))
vi.mock('@/utils/siteBillingMode', () => ({ resolveSiteBillingMode: () => 'balance' }))

describe('管理员共享额度展示', () => {
  it('独立显示全站共享额度，同时保留个人余额、冻结与总额', () => {
    const wrapper = mount(AppHeader, { global: { stubs: { LocaleSwitcher: true, AnnouncementBell: true, SubscriptionProgressMini: true, RouterLink: true } } })
    const site = wrapper.get('[data-testid="admin-site-quota"]')
    expect(site.text()).toContain('common.sharedQuota')
    expect(site.text()).toContain('$85.00')
    expect(site.text()).toContain('$100.00')
    expect(wrapper.text()).toContain('$20.00')
    expect(wrapper.text()).toContain('$2.00')
    expect(wrapper.text()).toContain('$22.00')
    wrapper.unmount()
  })
})
