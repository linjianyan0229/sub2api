import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { reactive } from 'vue'
import { useForumStore } from '@/stores/forum'
import App from '@/App.vue'
import AppHeader from '@/components/layout/AppHeader.vue'

const auth = reactive({ isAuthenticated: true, isAdmin: false, isSharedQuota: false, user: { id: 1 } as { id: number } | null })
const route = reactive({ path: '/dashboard', fullPath: '/dashboard', meta: {} })
const mocks = vi.hoisted(() => ({ pin: vi.fn(), recall: vi.fn(), success: vi.fn(), error: vi.fn() }))
vi.mock('@/api/forum', () => ({ forumAPI: { pin: mocks.pin, recall: mocks.recall } }))
vi.mock('vue-router', () => ({
  RouterView: { template: '<div><AppHeader /><main data-route-page>Page content</main></div>' },
  useRoute: () => route,
  useRouter: () => ({ afterEach: vi.fn(), replace: vi.fn() }),
}))
vi.mock('@/stores', () => ({
  useAuthStore: () => auth,
  useAppStore: () => ({ docUrl: '', contactInfo: '', fetchPublicSettings: vi.fn().mockResolvedValue(undefined), showSuccess: mocks.success, showError: mocks.error }),
  useSubscriptionStore: () => ({ clear: vi.fn(), fetchActiveSubscriptions: vi.fn().mockResolvedValue(undefined), startPolling: vi.fn() }),
  useAnnouncementStore: () => ({ fetchAnnouncements: vi.fn(), reset: vi.fn() }),
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
}))
vi.mock('@/api/setup', () => ({ getSetupStatus: vi.fn().mockResolvedValue({ needs_setup: false }) }))
vi.mock('@/utils/featureFlags', () => ({ FeatureFlags: {}, isFeatureFlagEnabled: () => false }))
vi.mock('@/utils/siteBillingMode', () => ({ resolveSiteBillingMode: () => 'balance' }))
vi.mock('@/utils/branding', () => ({ updateFavicon: vi.fn() }))
vi.mock('@/router/title', () => ({ resolveRouteDocumentTitle: () => 'Sub2API', resolveRouteMetaKeys: () => ({}) }))
vi.mock('@/stores/adminSettings', () => ({ useAdminSettingsStore: () => ({ customMenuItems: [] }) }))
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
  vi.clearAllMocks()
  mocks.pin.mockReset().mockResolvedValue(undefined)
  mocks.recall.mockReset().mockResolvedValue(undefined)
  setActivePinia(createPinia())
  auth.user = { id: 1 }
  auth.isAuthenticated = true
  auth.isAdmin = false
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
    expect(banner.find('.pin-unpin').exists()).toBe(false)
    expect(banner.find('[data-pin-recall]').exists()).toBe(false)
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    await banner.get('.pin-open').trigger('click')
    expect(banner.find('[data-pin-detail]').exists()).toBe(false)
    await wrapper.get('[aria-label="common.userMenu"]').trigger('click')
    expect(wrapper.find('[data-mobile-header-extras]').exists()).toBe(true)
  })
  it('lets admins unpin archived messages from the header without loading chat history', async () => {
    auth.isAdmin = true
    wrapper = render()
    await flushPromises()
    useForumStore().setPinned(pins)
    await flushPromises()
    await wrapper.get('.pin-open').trigger('click')
    let finish!: () => void
    mocks.pin.mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve }))
    await wrapper.get('[data-pin-detail-id="2"] .pin-unpin').trigger('click')
    expect(mocks.pin).toHaveBeenCalledOnce()
    expect(mocks.pin).toHaveBeenCalledWith(2, false)
    expect(wrapper.findAll('.pin-unpin').every(button => button.attributes('disabled') !== undefined)).toBe(true)
    expect(useForumStore().pinned).toHaveLength(3)
    finish()
    await flushPromises()
    expect(useForumStore().pinned.map(pin => pin.id)).toEqual([1, 3])
    expect(wrapper.find('[data-pin-detail-id="2"]').exists()).toBe(false)
    expect(mocks.success).toHaveBeenCalledWith('common.saved')
    await wrapper.get('[data-pin-detail-id="1"] .pin-unpin').trigger('click')
    await flushPromises()
    await wrapper.get('[data-pin-detail-id="3"] .pin-unpin').trigger('click')
    await flushPromises()
    expect(useForumStore().pinned).toEqual([])
    expect(wrapper.find('[data-global-forum-pins]').exists()).toBe(false)
  })
  it('keeps the pinned copy visible when unpinning fails', async () => {
    auth.isAdmin = true
    mocks.pin.mockRejectedValueOnce(new Error('Unpin failed'))
    wrapper = render()
    await flushPromises()
    useForumStore().setPinned(pins)
    await flushPromises()
    await wrapper.get('.pin-open').trigger('click')
    await wrapper.get('.pin-unpin').trigger('click')
    await flushPromises()
    expect(useForumStore().pinned).toEqual(pins)
    expect(mocks.error).toHaveBeenCalledWith('Unpin failed')
    expect(mocks.success).not.toHaveBeenCalled()
    expect(wrapper.get('.pin-unpin').attributes('disabled')).toBeUndefined()
  })
  it('lets admins recall archived pins and rejects stale polling snapshots', async () => {
    auth.isAdmin = true
    wrapper = render()
    await flushPromises()
    const store = useForumStore()
    store.setPinned(pins)
    await flushPromises()
    await wrapper.get('.pin-open').trigger('click')
    await wrapper.get('[data-pin-detail-id="2"] [data-pin-recall]').trigger('click')
    await flushPromises()
    expect(mocks.recall).toHaveBeenCalledWith(2)
    expect(mocks.pin).not.toHaveBeenCalled()
    expect(store.recalledIds.has(2)).toBe(true)
    expect(store.pinned.map(pin => pin.id)).toEqual([1, 3])
    store.setPinned(pins)
    await flushPromises()
    expect(wrapper.find('[data-pin-detail-id="2"]').exists()).toBe(false)
    expect(mocks.success).toHaveBeenCalledWith('forum.recalled')
    store.reset()
    expect(store.recalledIds.size).toBe(0)
  })
  it('keeps pins intact when recall fails', async () => {
    auth.isAdmin = true
    mocks.recall.mockRejectedValueOnce(new Error('Recall failed'))
    wrapper = render()
    await flushPromises()
    useForumStore().setPinned(pins)
    await flushPromises()
    await wrapper.get('.pin-open').trigger('click')
    await wrapper.get('[data-pin-recall]').trigger('click')
    await flushPromises()
    expect(useForumStore().pinned).toEqual(pins)
    expect(useForumStore().recalledIds.size).toBe(0)
    expect(mocks.error).toHaveBeenCalledWith('Recall failed')
    expect(wrapper.get('[data-pin-recall]').attributes('disabled')).toBeUndefined()
  })
  it('does not apply an old unpin response to another account', async () => {
    auth.isAdmin = true
    let finish!: () => void
    mocks.pin.mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve }))
    wrapper = render()
    await flushPromises()
    useForumStore().setPinned(pins)
    await flushPromises()
    await wrapper.get('.pin-open').trigger('click')
    await wrapper.get('.pin-unpin').trigger('click')
    auth.user = { id: 2 }
    await flushPromises()
    useForumStore().setPinned(pins)
    finish()
    await flushPromises()
    expect(useForumStore().pinned).toEqual(pins)
    expect(mocks.success).not.toHaveBeenCalled()
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
