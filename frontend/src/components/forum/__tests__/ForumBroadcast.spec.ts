import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ForumBroadcast from '../ForumBroadcast.vue'

const { events } = vi.hoisted(() => ({ events: vi.fn() }))
vi.mock('@/api/forum', () => ({ forumAPI: { events } }))
vi.mock('../ForumCelebration.vue', () => ({ __esModule: true, default: { props: ['message'], emits: ['close'], template: '<button data-broadcast @click="$emit(\'close\')">{{ message.content }}</button>' } }))
beforeEach(() => { vi.useFakeTimers(); events.mockReset(); Object.defineProperty(document, 'hidden', { configurable: true, value: false }) })
afterEach(() => { vi.useRealTimers() })

describe('全站通报', () => {
  it('初次登录只建立游标，新事件只播放一次，卸载停止请求', async () => {
    events.mockResolvedValueOnce({ cursor: 10, messages: [] }).mockResolvedValueOnce({ cursor: 11, messages: [{ id: 11, content: '榜上玩家发言', created_at: new Date().toISOString() }] }).mockResolvedValue({ cursor: 11, messages: [] })
    const wrapper = mount(ForumBroadcast)
    await flushPromises()
    expect(events.mock.calls[0]?.[0]).toBeUndefined()
    expect(wrapper.find('[data-broadcast]').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(5000); await flushPromises()
    expect(events.mock.calls[1]?.[0]).toBe(10)
    expect(wrapper.text()).toBe('榜上玩家发言')
    await wrapper.get('button').trigger('click')
    await vi.advanceTimersByTimeAsync(5000); await flushPromises()
    expect(wrapper.find('[data-broadcast]').exists()).toBe(false)
    wrapper.unmount()
    const calls = events.mock.calls.length
    await vi.advanceTimersByTimeAsync(10000)
    expect(events).toHaveBeenCalledTimes(calls)
  })
  it('不播放过期事件，后台标签页暂停轮询', async () => {
    events.mockResolvedValue({ cursor: 2, messages: [{ id: 2, content: '过时消息', created_at: new Date(Date.now() - 60000).toISOString() }] })
    const wrapper = mount(ForumBroadcast); await flushPromises()
    expect(wrapper.find('[data-broadcast]').exists()).toBe(false)
    Object.defineProperty(document, 'hidden', { configurable: true, value: true })
    document.dispatchEvent(new Event('visibilitychange'))
    await vi.advanceTimersByTimeAsync(10000)
    expect(events).toHaveBeenCalledOnce()
    wrapper.unmount()
  })
})
