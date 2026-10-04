import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { reactive } from 'vue'
import { useForumStore } from '@/stores/forum'
import App from '@/App.vue'
import AppHeader from '@/components/layout/AppHeader.vue'

const auth = reactive({ isAuthenticated: true, isAdmin: false, isSharedQuota: false, user: { id: 1 } as { id: number } | null })
const route = reactive({ path: '/dashboard', fullPath: '/dashboard', meta: {} })
vi.mock('vue-router', () => ({
  RouterView: { template: '<div><AppHeader /><main data-route-page>Page content</main></div>' },
  useRoute: () => route,
  useRouter: () => ({ afterEach: vi.fn(), replace: vi.fn() }),
}))
vi.mock('@/stores', () => ({
  useAuthStore: () => auth,
  useAppStore: () => ({ docUrl: '', contactInfo: '', fetchPublicSettings: vi.fn().mockResolvedValue(undefined) }),
  useSubscriptionStore: () => ({ clear: vi.fn(), fetchActiveSubscriptions: vi.fn().mockResolvedValue(undefined), startPolling: vi.fn() }),
  useAnnouncementStore: () => ({ fetchAnnouncements: vi.fn(), reset: vi.fn() }),
  useAdminSettingsStore: () => ({}),
}))
vi.mock('@/api/setup', () => ({ getSetupStatus: vi.fn().mockResolvedValue({ needs_setup: false }) }))
vi.mock('@/utils/featureFlags', () => ({ FeatureFlags: {}, isFeatureFlagEnabled: () => false }))
vi.mock('@/utils/siteBillingMode', () => ({ resolveSiteBillingMode: () => 'balance' }))
vi.mock('@/utils/branding', () => ({ updateFavicon: vi.fn() }))
vi.mock('@/router/title', () => ({ resolveRouteDocumentTitle: () => 'Sub2API', resolveRouteMetaKeys: () => ({}) }))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({}) }))
vi.mock('vue-i18n', async (importOriginal) => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/common/Toast.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/common/NavigationProgress.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/common/AnnouncementPopup.vue', () => ({ default: { template: '<div />' } }))
vi.mock('../ForumBroadcast.vue', () => ({ default: { template: '<div />' } }))
vi.mock('@/components/common/BaseDialog.vue', () => ({ default: { template: '<div />' } }))

const pins = [1, 2, 3].map(id => ({
  id, author: { user_id: 2, username: 'Author', avatar_url: '', actual_cost: 0 },
  content: `Pinned ${id}`, rank: 0, pinned: true, created_at: '2026-10-03T00:00:00Z',
}))
let wrapper: ReturnType<typeof mount> | undefined
function render() {
  return mount(App, { global: {
    components: { AppHeader },
    stubs: {
      LocaleSwitcher: true, SubscriptionProgressMini: true, RouterLink: true,
      AnnouncementBell: { template: '<button data-announcement-bell>Announcements</button>' },
    },
  } })
}
beforeEach(() => {
  vi.useFakeTimers()
  setActivePinia(createPinia())
  auth.user = { id: 1 }
  auth.isAuthenticated = true
})
afterEach(() => { wrapper?.unmount(); vi.useRealTimers() })

describe('Global forum pins', () => {
  it('renders inside the same header row as announcements without a separate top banner', async () => {
    wrapper = render()
    await flushPromises()
    expect(wrapper.find('[data-global-forum-pins]').exists()).toBe(false)
    useForumStore().setPinned(pins.slice(0, 1))
    await flushPromises()
    const banner = wrapper.get('[data-global-forum-pins]')
    expect(banner.text()).toContain('Pinned 1')
    const row = banner.element.parentElement
    expect(row?.classList.contains('h-16')).toBe(true)
    expect(row?.querySelector('[data-announcement-bell]')).not.toBeNull()
    expect(banner.element.closest('header')).not.toBeNull()
    expect(wrapper.find('aside.forum-global-pins').exists()).toBe(false)
    expect(banner.classes()).toContain('pins-compact')
    route.path = route.fullPath = '/forum'
    await flushPromises()
    expect(wrapper.findAll('.pin-capsule')).toHaveLength(1)
    expect(wrapper.get('[data-global-forum-pins]').text()).toContain('Pinned 1')
    route.path = route.fullPath = '/keys'
    await flushPromises()
    expect(wrapper.get('[data-global-forum-pins]').text()).toContain('Pinned 1')
    useForumStore().setPinned(pins)
    await flushPromises()
    expect(banner.findAll('.pin-capsule')).toHaveLength(1)
    expect(wrapper.findAll('[data-pin-id]')).toHaveLength(1)
    expect(banner.find('[data-pin-pause]').exists()).toBe(false)
    await banner.get('.pin-open').trigger('click')
    expect(banner.findAll('[data-pin-detail-id]')).toHaveLength(3)
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    await banner.get('.pin-open').trigger('click')
    expect(banner.find('[data-pin-detail]').exists()).toBe(false)
    await wrapper.get('[aria-label="common.userMenu"]').trigger('click')
    expect(wrapper.find('[data-mobile-header-extras]').exists()).toBe(true)
  })
  it('clears old pins on account changes and hides them on logout', async () => {
    wrapper = render()
    await flushPromises()
    useForumStore().setPinned(pins)
    await flushPromises()
    auth.user = { id: 2 }
    await flushPromises()
    expect(useForumStore().pinned).toEqual([])
    expect(wrapper.find('[data-global-forum-pins]').exists()).toBe(false)
    useForumStore().setPinned(pins)
    await flushPromises()
    auth.isAuthenticated = false
    auth.user = null
    await flushPromises()
    expect(useForumStore().pinned).toEqual([])
    expect(wrapper.find('[data-global-forum-pins]').exists()).toBe(false)
  })
})
