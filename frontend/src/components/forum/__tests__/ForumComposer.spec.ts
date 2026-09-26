import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import ForumComposer from '../ForumComposer.vue'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: ref('zh-CN') }) }))
beforeEach(() => { vi.useFakeTimers() })
afterEach(() => { vi.useRealTimers(); document.body.innerHTML = '' })
const props = { modelValue: '', disabled: false, sending: false, mutedUntil: null }

describe('底部悬浮输入框', () => {
  it('默认收起，悬停展开，离开且没有草稿时收起', async () => {
    const wrapper = mount(ForumComposer, { props })
    expect(wrapper.find('textarea').exists()).toBe(false)
    await wrapper.get('[data-composer-trigger]').trigger('mouseenter')
    expect(wrapper.find('textarea').exists()).toBe(true)
    await wrapper.get('.composer-dock').trigger('mouseleave')
    await vi.advanceTimersByTimeAsync(200)
    expect(wrapper.find('textarea').exists()).toBe(false)
    wrapper.unmount()
  })
  it('已有草稿时不会因鼠标离开而收起，Esc 收起但保留草稿', async () => {
    const wrapper = mount(ForumComposer, { props: { ...props, modelValue: '未发送的草稿' } })
    await wrapper.get('[data-composer-trigger]').trigger('mouseenter')
    await wrapper.get('.composer-dock').trigger('mouseleave')
    await vi.advanceTimersByTimeAsync(200)
    expect(wrapper.find('textarea').exists()).toBe(true)
    await wrapper.get('textarea').trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('textarea').exists()).toBe(false)
    await wrapper.get('[data-composer-trigger]').trigger('click'); await flushPromises()
    expect((wrapper.get('textarea').element as HTMLTextAreaElement).value).toBe('未发送的草稿')
    wrapper.unmount()
  })
  it('手机点击与键盘聚焦均可展开，输入期间不自动收起', async () => {
    const wrapper = mount(ForumComposer, { props, attachTo: document.body })
    await wrapper.get('[data-composer-trigger]').trigger('focus')
    expect(wrapper.find('textarea').exists()).toBe(true)
    await wrapper.get('[data-composer-trigger]').trigger('click'); await flushPromises()
    expect(document.activeElement).toBe(wrapper.get('textarea').element)
    await wrapper.get('.composer-dock').trigger('mouseleave')
    await vi.advanceTimersByTimeAsync(200)
    expect(wrapper.find('textarea').exists()).toBe(true)
    wrapper.unmount()
  })
})
