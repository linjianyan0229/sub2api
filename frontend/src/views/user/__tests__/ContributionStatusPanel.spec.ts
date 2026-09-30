import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import ContributionStatusPanel from '@/components/contributions/ContributionStatusPanel.vue'
import MonitorCard from '@/components/user/monitor/MonitorCard.vue'
import MonitorDetailDialog from '@/components/user/MonitorDetailDialog.vue'

const mocks = vi.hoisted(() => ({ statuses: vi.fn(), monitorStatus: vi.fn(), showError: vi.fn() }))
vi.mock('@/api/channelContributions', () => ({ channelContributionsAPI: mocks }))
vi.mock('@/stores/app', () => ({ useAppStore: () => mocks }))
vi.mock('vue-i18n', async importOriginal => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))
const row = { id: 7, contributor_name: '贡献者', probing: true, monitor: { id: 91, name: '<b>提供站点</b>', provider: 'openai', group_name: '共享组', primary_model: 'gpt-5', primary_status: 'degraded', primary_latency_ms: 860, primary_ping_latency_ms: 20, availability_7d: 50, extra_models: [], timeline: [
  { status: 'degraded', latency_ms: 860, ping_latency_ms: 20, checked_at: '2026-09-30T03:01:00Z' },
  { status: 'operational', latency_ms: 145, ping_latency_ms: 18, checked_at: '2026-09-30T03:00:00Z' }
] } }
let wrapper: VueWrapper | undefined
beforeEach(() => { vi.useFakeTimers(); vi.clearAllMocks() })
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.useRealTimers() })
function render() { return mount(ContributionStatusPanel, { global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' } } } }) }

describe('贡献渠道 V1 监控', () => {
  it('展示每个时段的真实延迟，并通过贡献详情接口打开统计', async () => {
    mocks.statuses.mockResolvedValue({ items: [row], has_more: false })
    mocks.monitorStatus.mockResolvedValue({ id: 91, models: [{ model: 'gpt-5', latest_status: 'degraded', latest_latency_ms: 860, availability_7d: 50, availability_15d: 50, availability_30d: 50, avg_latency_7d_ms: 503 }] })
    wrapper = render(); await flushPromises()
    expect(wrapper.text()).toContain('贡献者')
    expect(wrapper.text()).toContain('共享组')
    expect(wrapper.text()).toContain('monitorCommon.status.degraded')
    expect(wrapper.find('b').exists()).toBe(false)
    const bars = wrapper.findAll('[title]').filter(bar => bar.attributes('title'))
    expect(bars.map(bar => bar.attributes('title'))).toEqual([expect.stringContaining('145ms'), expect.stringContaining('860ms')])
    await wrapper.findComponent(MonitorCard).trigger('click'); await flushPromises()
    expect(mocks.monitorStatus).toHaveBeenCalledWith(7)
    expect(wrapper.findComponent(MonitorDetailDialog).text()).toContain('503')
  })

  it('无检测历史时保持待检测和灰色时间条，不伪造可用状态', async () => {
    mocks.statuses.mockResolvedValue({ items: [{ ...row, probing: false, monitor: { ...row.monitor, primary_status: '', primary_latency_ms: null, primary_ping_latency_ms: null, timeline: [] } }], has_more: false })
    wrapper = render(); await flushPromises()
    expect(wrapper.text()).toContain('monitorCommon.status.unknown')
    expect(wrapper.text()).not.toContain('monitorCommon.status.operational')
    expect(wrapper.text()).not.toContain('monitorCommon.nextUpdateIn')
    expect(wrapper.findAll('[title]').filter(bar => bar.attributes('title'))).toHaveLength(0)
    expect(wrapper.findComponent(MonitorCard).props('availabilityValue')).toBeNull()
  })

  it('保留分页和自动刷新，卸载后停止请求', async () => {
    mocks.statuses.mockResolvedValue({ items: [row], has_more: true })
    wrapper = render(); await flushPromises()
    mocks.statuses.mockResolvedValue({ items: [{ ...row, id: 6, probing: false }], has_more: false })
    await wrapper.findAll('button').find(b => b.text() === 'contributions.older')!.trigger('click'); await flushPromises()
    expect(mocks.statuses).toHaveBeenLastCalledWith(7, expect.any(AbortSignal))
    expect(wrapper.text()).toContain('monitorCommon.maintenancePaused')
    await vi.advanceTimersByTimeAsync(30000)
    expect(mocks.statuses).toHaveBeenCalledTimes(3)
    wrapper.unmount(); wrapper = undefined
    await vi.advanceTimersByTimeAsync(60000)
    expect(mocks.statuses).toHaveBeenCalledTimes(3)
  })

  it('未发布任何渠道时不占位', async () => {
    mocks.statuses.mockResolvedValue({ items: [], has_more: false })
    wrapper = render(); await flushPromises()
    expect(wrapper.find('section').exists()).toBe(false)
  })
})
