import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import ContributionStatusPanel from '@/components/contributions/ContributionStatusPanel.vue'

const mocks = vi.hoisted(() => ({ statuses: vi.fn() }))
vi.mock('@/api/channelContributions', () => ({ channelContributionsAPI: mocks }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const row = { id: 7, site_name: '<b>提供站点</b>', platform: 'openai', contributor_name: '贡献者', groups: [{ id: 2, name: '共享组' }], status: 'active', independent: true, rpm: 30, concurrency: 5, daily_limit: 20, total_limit: 100, daily_used: 3, total_used: 12, models: ['gpt-5'] }
let wrapper: VueWrapper | undefined
beforeEach(() => { vi.useFakeTimers(); mocks.statuses.mockReset() })
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.useRealTimers() })

describe('贡献渠道状态', () => {
  it('显示贡献者和可用状态，停用后仍可在状态页查看，并支持翻页与刷新', async () => {
    mocks.statuses.mockResolvedValue({ items: [row], has_more: true })
    wrapper = mount(ContributionStatusPanel)
    await flushPromises()
    expect(wrapper.text()).toContain('贡献者')
    expect(wrapper.text()).toContain('共享组')
    expect(wrapper.text()).toContain('$3.00 / $20.00')
    expect(wrapper.find('b').exists()).toBe(false)
    mocks.statuses.mockResolvedValue({ items: [{ ...row, id: 6, status: 'disabled' }], has_more: false })
    await wrapper.findAll('button').find(b => b.text() === 'contributions.older')!.trigger('click'); await flushPromises()
    expect(mocks.statuses).toHaveBeenLastCalledWith(7, expect.any(AbortSignal))
    expect(wrapper.text()).toContain('contributions.states.disabled')
    await vi.advanceTimersByTimeAsync(30000)
    expect(mocks.statuses).toHaveBeenCalledTimes(3)
    wrapper.unmount(); wrapper = undefined
    await vi.advanceTimersByTimeAsync(60000)
    expect(mocks.statuses).toHaveBeenCalledTimes(3)
  })

  it('未发布任何渠道时不占位', async () => {
    mocks.statuses.mockResolvedValue({ items: [], has_more: false })
    wrapper = mount(ContributionStatusPanel)
    await flushPromises()
    expect(wrapper.find('section').exists()).toBe(false)
  })
})
