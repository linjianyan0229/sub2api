import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import { useForumStore } from '@/stores/forum'
import ForumView from '../ForumView.vue'

const mocks = vi.hoisted(() => ({ page: vi.fn(), send: vi.fn(), pin: vi.fn(), mute: vi.fn(), mutes: vi.fn(), settings: vi.fn(), clear: vi.fn(), success: vi.fn(), error: vi.fn(), auth: { isAdmin: false, user: { id: 1 } } }))
vi.mock('@/api/forum', () => ({ forumAPI: mocks }))
vi.mock('@/stores', () => ({ useAuthStore: () => mocks.auth, useAppStore: () => ({ showSuccess: mocks.success, showError: mocks.error }) }))
vi.mock('vue-i18n', async (importOriginal) => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key, locale: ref('zh-CN') }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('@/components/common/BaseDialog.vue', () => ({ default: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' } }))
vi.mock('@/components/forum/ForumDrawer.vue', () => ({ default: { props: ['show'], template: '<div v-if="show"><slot /></div>' } }))
vi.mock('@/components/common/ConfirmDialog.vue', () => ({ default: { props: ['show'], emits: ['confirm', 'cancel'], template: '<div v-if="show"><button data-confirm @click="$emit(\'confirm\')">Confirm</button></div>' } }))

function snapshot(muted: string | null = null) {
  return {
    messages: [{ id: 1, author: { user_id: 2, username: '昵称', avatar_url: 'https://example.com/a.png', actual_cost: 12.5 }, content: '<script>bad()</script>', rank: 2, pinned: false, created_at: new Date().toISOString() }],
    pinned: [], settings: { muted_until: muted, retention_days: 30 }, muted_until: null, has_more: false
  }
}
beforeEach(() => { setActivePinia(createPinia()); vi.useFakeTimers(); vi.clearAllMocks(); mocks.auth.isAdmin = false; mocks.page.mockResolvedValue(snapshot()); mocks.mutes.mockResolvedValue([]) })
afterEach(() => { vi.useRealTimers() })

describe('论坛讨论', () => {
  it('自己的消息使用右侧布局，其他消息使用左侧布局并保持时间顺序', async () => {
    const result = snapshot()
    const other = result.messages[0]
    const own = { ...other, id: 2, author: { ...other.author, user_id: 1, username: '自己' } }
    mocks.page.mockResolvedValue({ ...result, messages: [own, other] })
    const wrapper = mount(ForumView)
    await flushPromises()
    const messages = wrapper.findAll('.forum-message')
    expect(messages.map(message => message.attributes('data-message-id'))).toEqual(['1', '2'])
    expect(messages[0].classes()).not.toContain('own-message')
    expect(messages[1].classes()).toContain('own-message')
    wrapper.unmount()
  })
  it('正文不重复展示置顶区域，仍同步导航栏状态并保留消息的取消置顶操作', async () => {
    mocks.auth.isAdmin = true
    const result = snapshot()
    const message = { ...result.messages[0], pinned: true }
    mocks.page.mockResolvedValue({ ...result, messages: [message], pinned: [message] })
    const wrapper = mount(ForumView)
    await flushPromises()
    expect(wrapper.find('.forum-pinned').exists()).toBe(false)
    expect(wrapper.find('.pin-capsule').exists()).toBe(false)
    expect(wrapper.find('[data-pin-detail]').exists()).toBe(false)
    expect(useForumStore().pinned).toHaveLength(1)
    mocks.page.mockResolvedValue(result)
    await wrapper.get('.message-meta .forum-action').trigger('click')
    await flushPromises()
    expect(mocks.pin).toHaveBeenCalledWith(1, false)
    expect(useForumStore().pinned).toEqual([])
    expect(wrapper.find('.forum-pinned').exists()).toBe(false)
    wrapper.unmount()
  })
  it('展示昵称、累计 USD 和头像，发言作为纯文本输出，普通用户没有管理入口', async () => {
    const wrapper = mount(ForumView)
    await flushPromises()
    expect(wrapper.text()).toContain('昵称')
    expect(wrapper.text()).toContain('$12.5000 USD')
    expect(wrapper.get('img').attributes('src')).toBe('https://example.com/a.png')
    expect(wrapper.text()).toContain('<script>bad()</script>')
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.find('.forum-management').exists()).toBe(false)
    expect(wrapper.find('textarea').exists()).toBe(false)
    await wrapper.get('[data-composer-trigger]').trigger('mouseenter')
    await wrapper.get('textarea').setValue('讨论内容')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(mocks.send).toHaveBeenCalledWith('讨论内容')
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('')
    wrapper.unmount()
  })
  it('禁言期间禁用发送，发送失败保留草稿', async () => {
    mocks.page.mockResolvedValueOnce(snapshot(new Date(Date.now() + 60000).toISOString()))
    const wrapper = mount(ForumView)
    await flushPromises()
    await wrapper.get('[data-composer-trigger]').trigger('mouseenter')
    expect(wrapper.get('textarea').attributes('disabled')).toBeDefined()
    await wrapper.get('header button').trigger('click'); await flushPromises()
    mocks.send.mockRejectedValueOnce(new Error('FORUM_MUTED'))
    await wrapper.get('textarea').setValue('保留草稿')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('保留草稿')
    expect(mocks.error).toHaveBeenCalledWith('FORUM_MUTED')
    wrapper.unmount()
  })
  it('管理员称号在发言中展示，普通账号同样昵称也不能获得称号', async () => {
    const result = snapshot()
    const message = { ...result.messages[0], author: { ...result.messages[0].author, username: '管理员', is_admin: true } }
    mocks.page.mockResolvedValue({ ...result, messages: [message], pinned: [message] })
    const wrapper = mount(ForumView)
    await flushPromises()
    expect(wrapper.findAllComponents({ name: 'ForumAdminBadge' })).toHaveLength(1)
    mocks.page.mockResolvedValue({ ...result, messages: [{ ...message, author: { ...message.author, is_admin: false } }], pinned: [] })
    await wrapper.get('header button').trigger('click')
    await flushPromises()
    expect(wrapper.findComponent({ name: 'ForumAdminBadge' }).exists()).toBe(false)
    expect(wrapper.text()).toContain('管理员')
    wrapper.unmount()
  })
  it('管理员清空必须先确认，清空调用不携带用户可控身份', async () => {
    mocks.auth.isAdmin = true
    const wrapper = mount(ForumView)
    await flushPromises()
    await wrapper.get('.forum-manage-button').trigger('click')
    const clear = wrapper.findAll('button').find(button => button.text() === 'forum.clear')!
    await clear.trigger('click')
    expect(mocks.clear).not.toHaveBeenCalled()
    await wrapper.get('[data-confirm]').trigger('click'); await flushPromises()
    expect(mocks.clear).toHaveBeenCalledOnce()
    wrapper.unmount()
  })
})
