import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import ChannelQuotasView from '../ChannelQuotasView.vue'
import ChannelQuotaSummary from '@/components/contributions/ChannelQuotaSummary.vue'

const mock = vi.hoisted(() => ({ quotas: vi.fn() }))
vi.mock('@/api/channelContributions', () => ({ channelContributionsAPI: mock }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
let wrapper: VueWrapper | undefined
const row = { id: 4, site_name: '提供站点', platform: 'openai', contributor_name: '贡献者', groups: [{ id: 2, name: '共享组' }], status: 'active', independent: true, rpm: 30, concurrency: 5, daily_limit: 20, total_limit: 100, daily_used: 3, total_used: 12, models: ['gpt-5'] }
beforeEach(() => { vi.useFakeTimers(); mock.quotas.mockReset() })
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.useRealTimers() })

describe('渠道额度', () => {
  const unavailable = ['removed', 'disabled', 'inactive', 'error', 'limited', 'quota_exhausted', 'pending'].map((status, index) => ({ ...row, id: 10 + index, status, site_name: `隐藏-${status}` }))
  it('页面不为不可用渠道渲染卡片', async () => {
    mock.quotas.mockResolvedValue({ items: [...unavailable, row], shared_quota: null, has_more: false })
    wrapper = mount(ChannelQuotasView)
    await flushPromises()
    expect(wrapper.findAll('[data-channel]')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('隐藏-')
  })

  it('侧栏先过滤再取前三条，全部不可用时整个栏目消失', async () => {
    mock.quotas.mockResolvedValue({ items: [...unavailable, row, { ...row, id: 5 }, { ...row, id: 6 }], shared_quota: null, has_more: false })
    wrapper = mount(ChannelQuotaSummary, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
    await flushPromises()
    expect(wrapper.findAll('a')).toHaveLength(4)
    expect(wrapper.text()).not.toContain('隐藏-')
    mock.quotas.mockResolvedValue({ items: unavailable, shared_quota: null, has_more: false })
    await vi.advanceTimersByTimeAsync(30000); await flushPromises()
    expect(wrapper.find('section').exists()).toBe(false)
  })

  it('展示提供商、贡献者、分组、状态及独立额度，未配置时展示全站额度', async () => {
    mock.quotas.mockResolvedValue({ items: [row, { ...row, id: 5, site_name: '全站渠道', independent: false, daily_limit: 0, total_limit: 0 }], shared_quota: { remaining: 80, total: 100 }, has_more: false })
    wrapper = mount(ChannelQuotasView)
    await flushPromises()
    const independent = wrapper.get('[data-channel="4"]')
    expect(independent.text()).toContain('OpenAI')
    expect(independent.text()).toContain('贡献者')
    expect(independent.text()).toContain('共享组')
    expect(independent.text()).toContain('contributions.states.active')
    expect(independent.text()).toContain('$3.00 / $20.00')
    expect(independent.text()).toContain('$12.00 / $100.00')
    expect(wrapper.get('[data-channel="5"]').text()).toContain('common.sharedQuota$80.00')
  })

  it('刷新时忽略旧响应，卸载后停止轮询', async () => {
    let resolveOld!: (value: unknown) => void
    mock.quotas.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
    wrapper = mount(ChannelQuotasView)
    await flushPromises()
    wrapper.unmount(); wrapper = undefined
    const signal = mock.quotas.mock.calls[0][1] as AbortSignal
    expect(signal.aborted).toBe(true)
    resolveOld({ items: [row], shared_quota: null, has_more: false })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(60000)
    expect(mock.quotas).toHaveBeenCalledTimes(1)
  })
})
