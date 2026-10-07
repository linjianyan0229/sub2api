import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import type { ForumMessage } from '@/api/forum'
import ForumPinnedBar from '../ForumPinnedBar.vue'

const visibility = ref('visible')
vi.mock('@vueuse/core', () => ({ useDocumentVisibility: () => visibility }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const message = (id: number): ForumMessage => ({
  id, author: { user_id: id, username: `Author ${id}`, avatar_url: '', actual_cost: 1 },
  content: `Pinned message ${id}`, rank: 0, pinned: true, created_at: '2026-10-03T00:00:00Z',
})
const three = [message(3), message(2), message(1)]
const wrappers: ReturnType<typeof mount>[] = []
function render(messages = three, compact = false) {
  const wrapper = mount(ForumPinnedBar, {
    props: { messages, compact },
    global: { stubs: { Transition: { template: '<span><slot /></span>' } } },
  })
  wrappers.push(wrapper)
  return wrapper
}
const ids = (wrapper: ReturnType<typeof render>) => wrapper.findAll('[data-pin-id]').map(item => item.attributes('data-pin-id'))
beforeEach(() => { vi.useFakeTimers(); visibility.value = 'visible' })
afterEach(() => { wrappers.splice(0).forEach(wrapper => wrapper.unmount()); vi.useRealTimers() })

describe('Pinned message capsule', () => {
  it.each([false, true])('rotates text inside one persistent capsule (compact=%s)', async compact => {
    const wrapper = render(three, compact)
    const capsule = wrapper.get('.pin-capsule').element
    expect(ids(wrapper)).toEqual(['3'])
    expect(wrapper.findAll('button')).toHaveLength(1)
    expect(wrapper.find('[data-pin-pause]').exists()).toBe(false)
    expect(wrapper.find('[data-pin-next]').exists()).toBe(false)
    for (const id of ['2', '1', '3']) {
      await vi.advanceTimersByTimeAsync(5000)
      expect(ids(wrapper)).toEqual([id])
      expect(wrapper.findAll('.pin-capsule')).toHaveLength(1)
      expect(wrapper.get('.pin-capsule').element).toBe(capsule)
    }
  })
  it('rotates two pins so neither is hidden permanently', async () => {
    const wrapper = render(three.slice(0, 2))
    await vi.advanceTimersByTimeAsync(5000)
    expect(ids(wrapper)).toEqual(['2'])
    await vi.advanceTimersByTimeAsync(5000)
    expect(ids(wrapper)).toEqual(['3'])
  })
  it.each([0, 1])('does not schedule rotation for %i pins', async count => {
    const wrapper = render(three.slice(0, count))
    expect(vi.getTimerCount()).toBe(0)
    expect(wrapper.findAll('.pin-capsule')).toHaveLength(count)
    await vi.advanceTimersByTimeAsync(15000)
    expect(ids(wrapper)).toEqual(count ? ['3'] : [])
  })
  it('does not reset rotation on unchanged polling snapshots or hover', async () => {
    const wrapper = render()
    await vi.advanceTimersByTimeAsync(5000)
    await wrapper.setProps({ messages: three.map(item => ({ ...item })) })
    expect(ids(wrapper)).toEqual(['2'])
    await wrapper.get('.pin-open').trigger('mouseenter')
    await vi.advanceTimersByTimeAsync(5000)
    expect(ids(wrapper)).toEqual(['1'])
  })
  it('suspends work in background tabs and resumes automatically', async () => {
    const wrapper = render()
    visibility.value = 'hidden'
    await vi.advanceTimersByTimeAsync(10000)
    expect(ids(wrapper)).toEqual(['3'])
    visibility.value = 'visible'
    await vi.advanceTimersByTimeAsync(5000)
    expect(ids(wrapper)).toEqual(['2'])
  })
  it.each([false, true])('clicking opens all full messages as plain text (compact=%s)', async compact => {
    const content = '<script>alert(1)</script>\n' + 'Long message '.repeat(100)
    const wrapper = render([{ ...three[0], content }, ...three.slice(1)], compact)
    await wrapper.get('.pin-open').trigger('click')
    expect(wrapper.get('[data-pin-detail]').text()).toContain(content)
    expect(wrapper.findAll('[data-pin-detail-id]')).toHaveLength(3)
    for (const item of three) expect(wrapper.get('[data-pin-detail]').text()).toContain(item.author.username)
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    expect(wrapper.find('.modal-overlay').exists()).toBe(false)
    expect(wrapper.get('[data-pin-detail]').element.parentElement).toBe(wrapper.element)
    expect(wrapper.get('.pin-open').attributes('aria-controls')).toBe(wrapper.get('[data-pin-detail]').attributes('id'))
    expect(wrapper.get('.pin-open').attributes('aria-expanded')).toBe('true')
    await vi.advanceTimersByTimeAsync(10000)
    expect(ids(wrapper)).toEqual(['3'])
    await wrapper.get('.pin-open').trigger('click')
    expect(wrapper.find('[data-pin-detail]').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(5000)
    expect(ids(wrapper)).toEqual(['2'])
  })
  it('collapses on Escape or an outside click without blocking the page', async () => {
    const wrapper = render()
    await wrapper.get('.pin-open').trigger('click')
    await wrapper.get('[data-pin-detail]').trigger('pointerdown')
    expect(wrapper.find('[data-pin-detail]').exists()).toBe(true)
    await wrapper.get('[data-pin-detail]').trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('[data-pin-detail]').exists()).toBe(false)
    await wrapper.get('.pin-open').trigger('click')
    document.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true }))
    await wrapper.vm.$nextTick()
    expect(wrapper.find('[data-pin-detail]').exists()).toBe(false)
  })
  it('updates the expanded list on unpin and closes it when all pins are removed', async () => {
    const wrapper = render()
    await wrapper.get('.pin-open').trigger('click')
    await wrapper.setProps({ messages: [three[1]] })
    expect(ids(wrapper)).toEqual(['2'])
    expect(wrapper.findAll('[data-pin-detail-id]')).toHaveLength(1)
    await wrapper.setProps({ messages: [] })
    expect(wrapper.find('.forum-pins').exists()).toBe(false)
    await wrapper.setProps({ messages: three })
    expect(wrapper.find('[data-pin-detail]').exists()).toBe(false)
  })
  it('shows management actions only in the expanded list and cleans up timers', async () => {
    const wrapper = render()
    await wrapper.setProps({ manageable: true })
    expect(wrapper.find('.pin-unpin').exists()).toBe(false)
    await wrapper.get('.pin-open').trigger('click')
    expect(wrapper.findAll('.pin-unpin')).toHaveLength(3)
    expect(wrapper.findAll('[data-pin-recall]')).toHaveLength(3)
    await wrapper.findAll('.pin-unpin')[1].trigger('click')
    expect(wrapper.emitted('unpin')).toEqual([[2]])
    await wrapper.findAll('[data-pin-recall]')[1].trigger('click')
    expect(wrapper.emitted('recall')).toEqual([[2]])
    await wrapper.setProps({ busy: true })
    expect(wrapper.get('.pin-unpin').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-pin-recall]').attributes('disabled')).toBeDefined()
    await wrapper.setProps({ manageable: false })
    expect(wrapper.find('.pin-unpin').exists()).toBe(false)
    expect(wrapper.find('[data-pin-recall]').exists()).toBe(false)
    wrapper.unmount()
    wrappers.pop()
    expect(vi.getTimerCount()).toBe(0)
  })
})
