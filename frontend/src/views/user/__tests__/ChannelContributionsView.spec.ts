import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { ref } from 'vue'
import ChannelContributionsView from '../ChannelContributionsView.vue'
import GroupContributions from '@/components/common/GroupContributions.vue'

const mocks = vi.hoisted(() => ({ list: vi.fn(), groups: vi.fn(), submit: vi.fn(), review: vi.fn(), success: vi.fn(), error: vi.fn(), auth: { isAdmin: false } }))
vi.mock('@/api/channelContributions', () => ({ channelContributionsAPI: mocks }))
vi.mock('@/stores', () => ({ useAuthStore: () => mocks.auth, useAppStore: () => ({ showSuccess: mocks.success, showError: mocks.error }) }))
vi.mock('vue-i18n', async (original) => ({ ...await original<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key, locale: ref('zh-CN') }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('@/components/common/BaseDialog.vue', () => ({ default: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' } }))

const item = { id: 7, site_name: '共享站点', base_url: 'https://api.example.com', platform: 'openai', contributor_name: '贡献者', new_group_name: '共享组', status: 'pending', created_at: '2026-09-28T00:00:00Z' }
let wrapper: VueWrapper | undefined
beforeEach(() => {
  vi.clearAllMocks()
  mocks.auth.isAdmin = false
  mocks.list.mockResolvedValue({ items: [item], has_more: false })
  mocks.groups.mockResolvedValue([{ id: 3, name: 'OpenAI 分组', platform: 'openai' }, { id: 4, name: 'Gemini 分组', platform: 'gemini' }])
  mocks.submit.mockResolvedValue(item)
  mocks.review.mockResolvedValue(undefined)
})
afterEach(() => { wrapper?.unmount(); wrapper = undefined })

async function openForm() {
  wrapper = mount(ChannelContributionsView, { attachTo: document.body })
  await flushPromises()
  await wrapper.get('header button').trigger('click')
  await flushPromises()
  await wrapper.get('#contribution-site').setValue('新站点')
  await wrapper.get('#contribution-url').setValue('https://api.example.com')
  await wrapper.get('#contribution-key').setValue('sk-private-test')
}

describe('渠道提供', () => {
  it('普通用户仅有自己的申请，点击提交按钮可申请新组且成功后清空 Key', async () => {
    await openForm()
    expect(mocks.list).toHaveBeenCalledWith(false, '', 0, expect.any(AbortSignal))
    expect(wrapper!.find('[role="tablist"]').exists()).toBe(false)
    expect(wrapper!.find('.contribution-review-button').exists()).toBe(false)
    expect(wrapper!.get('#contribution-key').attributes('type')).toBe('password')
    await wrapper!.get('#contribution-group').setValue('new')
    await wrapper!.get('#contribution-new-group').setValue('申请的新组')
    await wrapper!.get('button[form="contribution-form"]').trigger('click')
    await flushPromises()
    expect(mocks.submit).toHaveBeenCalledWith({ site_name: '新站点', base_url: 'https://api.example.com', api_key: 'sk-private-test', platform: 'openai', group_id: null, new_group_name: '申请的新组' })
    expect(wrapper!.find('#contribution-key').exists()).toBe(false)
    await wrapper!.get('header button').trigger('click'); await flushPromises()
    expect((wrapper!.get('#contribution-key').element as HTMLInputElement).value).toBe('')
  })

  it('现有组按平台筛选，提交失败保留草稿，取消后清除 Key', async () => {
    await openForm()
    expect(wrapper!.get('#contribution-group').text()).toContain('OpenAI 分组')
    expect(wrapper!.get('#contribution-group').text()).not.toContain('Gemini 分组')
    await wrapper!.get('#contribution-platform').setValue('gemini')
    expect(wrapper!.get('#contribution-group').text()).not.toContain('OpenAI 分组')
    await wrapper!.get('#contribution-group').setValue('4')
    mocks.submit.mockRejectedValueOnce(new Error('提交失败'))
    await wrapper!.get('form').trigger('submit'); await flushPromises()
    expect(mocks.submit).toHaveBeenCalledWith(expect.objectContaining({ group_id: 4, new_group_name: '', platform: 'gemini' }))
    expect(mocks.error).toHaveBeenCalledWith('提交失败')
    expect((wrapper!.get('#contribution-key').element as HTMLInputElement).value).toBe('sk-private-test')
    await wrapper!.findAll('button').find(b => b.text() === 'common.cancel')!.trigger('click')
    await wrapper!.get('header button').trigger('click'); await flushPromises()
    expect((wrapper!.get('#contribution-key').element as HTMLInputElement).value).toBe('')
  })

  it.each(['approved', 'rejected'])('管理员可审核为 %s，并刷新列表', async decision => {
    mocks.auth.isAdmin = true
    wrapper = mount(ChannelContributionsView)
    await flushPromises()
    expect(mocks.list).toHaveBeenCalledWith(true, 'pending', 0, expect.any(AbortSignal))
    expect(wrapper.text()).toContain('贡献者')
    await wrapper.get('.contribution-review-button').trigger('click')
    await wrapper.get('#contribution-note').setValue('审核备注')
    const label = decision === 'approved' ? 'contributions.approve' : 'contributions.reject'
    await wrapper.findAll('button').find(b => b.text() === label)!.trigger('click')
    await flushPromises()
    expect(mocks.review).toHaveBeenCalledWith(7, decision, '审核备注')
    expect(mocks.list).toHaveBeenCalledTimes(2)
    expect(wrapper.find('#contribution-note').exists()).toBe(false)
  })

  it('切换审核与个人列表时忽略已过期响应', async () => {
    mocks.auth.isAdmin = true
    let resolveOld!: (value: unknown) => void
    mocks.list.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
    wrapper = mount(ChannelContributionsView)
    await wrapper.findAll('[role="tab"]')[1].trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('共享站点')
    resolveOld({ items: [{ ...item, site_name: '不应展示的审核记录' }], has_more: false })
    await flushPromises()
    expect(wrapper.text()).not.toContain('不应展示的审核记录')
  })

  it('分组内的账号与贡献者名称按文本展示', () => {
    wrapper = mount(GroupContributions, { props: { items: [{ account_id: 1, account_name: '共享站点', contributor_name: '<img src=x onerror=alert(1)>' }] } })
    expect(wrapper.text()).toContain('共享站点')
    expect(wrapper.text()).toContain('<img src=x onerror=alert(1)>')
    expect(wrapper.find('img').exists()).toBe(false)
  })
})
