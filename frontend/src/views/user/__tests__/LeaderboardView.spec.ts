import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import LeaderboardView from '../LeaderboardView.vue'

const { load } = vi.hoisted(() => ({ load: vi.fn() }))
vi.mock('@/api/leaderboard', () => ({ getUsageLeaderboard: load }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key, locale: 'en' })
}))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<div><slot /></div>' } }))

beforeEach(() => { load.mockReset() })

describe('累计排行榜', () => {
  it('按接口顺序展示昵称、金额和完整 Token 数量', async () => {
    load.mockResolvedValue([
      { username: '测试用户', actual_cost: 12.5, tokens: 1234567 },
      { username: '', actual_cost: 2, tokens: 500 }
    ])
    const wrapper = mount(LeaderboardView)
    await flushPromises()
    const rows = wrapper.findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows[0]!.text()).toContain('测试用户')
    expect(rows[0]!.text()).toContain('$12.5000')
    expect(rows[0]!.text()).toContain('1,234,567')
    expect(rows[1]!.text()).toContain('leaderboard.anonymous')
    expect(wrapper.text()).toContain('Top 20')
    wrapper.unmount()
  })

  it('区分加载失败和空榜，并可刷新重试', async () => {
    load.mockRejectedValueOnce(new Error('unavailable')).mockResolvedValueOnce([])
    const wrapper = mount(LeaderboardView)
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('leaderboard.loadFailed')
    expect(wrapper.text()).not.toContain('common.noData')
    await wrapper.get('button').trigger('click')
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('common.noData')
    expect(load).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
})
