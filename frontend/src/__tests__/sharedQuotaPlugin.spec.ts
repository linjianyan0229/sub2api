import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

const source = readFileSync(resolve(process.cwd(), '../plugins/shared-daily-quota/ui/app.js'), 'utf8')
const html = readFileSync(resolve(process.cwd(), '../plugins/shared-daily-quota/ui/index.html'), 'utf8')

afterEach(() => {
  window.dispatchEvent(new Event('pagehide'))
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  document.body.innerHTML = ''
})

describe('共享额度插件配置', () => {
  it('加载并保存下次刷新生效方式，允许切换为立即生效', async () => {
    window.location.hash = 'bridge_token=test-session'
    document.body.innerHTML = html
    const send = vi.spyOn(window, 'postMessage').mockImplementation(() => {})
    window.eval(source)
    const groupsRequest = send.mock.calls.map(([data]) => data).find(data => data.type === 'groups.list')
    window.dispatchEvent(new MessageEvent('message', { source: window, data: { source: 'sub2api-plugin-host', bridge_token: 'test-session', request_id: groupsRequest.request_id, ok: true, groups: [] } }))
    const load = send.mock.calls.map(([data]) => data).find(data => data.type === 'config.load')
    const response = { source: 'sub2api-plugin-host', bridge_token: 'test-session', request_id: load.request_id, ok: true, config: { daily_quota: 100, reset_time: '04:30', apply_mode: 'next_reset' } }
    window.dispatchEvent(new MessageEvent('message', { source: window, data: response }))
    await flushPromises()
    const mode = document.querySelector<HTMLSelectElement>('#apply-mode')!
    const saveButton = document.querySelector<HTMLButtonElement>('#save')!
    expect(mode.value).toBe('next_reset')
    expect(mode.disabled).toBe(false)
    saveButton.click()
    const save = send.mock.calls.map(([data]) => data).find(data => data.type === 'config.save')
    expect(save.config.apply_mode).toBe('next_reset')
    window.dispatchEvent(new MessageEvent('message', { source: window, data: { ...response, request_id: save.request_id } }))
    await flushPromises()
    mode.value = 'immediate'
    saveButton.click()
    const saves = send.mock.calls.map(([data]) => data).filter(data => data.type === 'config.save')
    expect(saves[1].config.apply_mode).toBe('immediate')
    window.dispatchEvent(new MessageEvent('message', { source: window, data: { ...response, request_id: saves[1].request_id } }))
    await flushPromises()
    expect(document.querySelector('#status')!.textContent).toBe('已保存')
  })

  it.each([true, false])('仅通过当前会话加载和保存配置，randomUUID 可用：%s', async (randomUUIDAvailable) => {
    if (!randomUUIDAvailable) {
      vi.stubGlobal('crypto', { getRandomValues: crypto.getRandomValues.bind(crypto) })
    }
    window.location.hash = 'bridge_token=test-session'
    document.body.innerHTML = html
    const send = vi.spyOn(window, 'postMessage').mockImplementation(() => {})
    window.eval(source)
    const groupsRequest = send.mock.calls.map(([data]) => data).find(data => data.type === 'groups.list')
    window.dispatchEvent(new MessageEvent('message', { source: window, data: { source: 'sub2api-plugin-host', bridge_token: 'test-session', request_id: groupsRequest.request_id, ok: true, groups: [] } }))
    const load = send.mock.calls.map(([data]) => data).find(data => data.type === 'config.load')
    expect(load).toBeDefined()
    const response = { source: 'sub2api-plugin-host', bridge_token: 'test-session', request_id: load.request_id, ok: true, config: { daily_quota: 123.45, reset_time: '03:30' } }
    window.dispatchEvent(new MessageEvent('message', { source: window, data: { ...response, bridge_token: 'wrong-session' } }))
    await flushPromises()
    const quota = document.querySelector<HTMLInputElement>('#daily-quota')!
    expect(quota.disabled).toBe(true)
    window.dispatchEvent(new MessageEvent('message', { source: window, data: response }))
    await flushPromises()
    expect(quota.value).toBe('123.45')
    expect(document.querySelector<HTMLInputElement>('#reset-time')!.value).toBe('03:30')
    quota.value = '20'
    const submit = vi.fn()
    document.querySelector('form')!.addEventListener('submit', submit)
    const saveButton = document.querySelector<HTMLButtonElement>('#save')!
    saveButton.click()
    // 沙箱不允许原生表单提交，保存必须由按钮点击直接触发。
    expect(submit).not.toHaveBeenCalled()
    expect(saveButton.disabled).toBe(true)
    const save = send.mock.calls.map(([data]) => data).find(data => data.type === 'config.save')
    expect(save.config).toEqual({ daily_quota: 20, reset_time: '03:30', apply_mode: 'immediate' })
    window.dispatchEvent(new MessageEvent('message', { source: window, data: { ...response, request_id: save.request_id } }))
    await flushPromises()
    expect(document.querySelector('#status')!.textContent).toBe('已保存')
    expect(saveButton.disabled).toBe(false)
  })

  it('无效金额不保存，保存失败显示错误且允许再次点击', async () => {
    window.location.hash = 'bridge_token=test-session'
    document.body.innerHTML = html
    const send = vi.spyOn(window, 'postMessage').mockImplementation(() => {})
    window.eval(source)
    const groupsRequest = send.mock.calls.map(([data]) => data).find(data => data.type === 'groups.list')
    window.dispatchEvent(new MessageEvent('message', { source: window, data: { source: 'sub2api-plugin-host', bridge_token: 'test-session', request_id: groupsRequest.request_id, ok: true, groups: [] } }))
    const load = send.mock.calls.map(([data]) => data).find(data => data.type === 'config.load')
    const response = { source: 'sub2api-plugin-host', bridge_token: 'test-session', request_id: load.request_id, ok: true, config: {} }
    window.dispatchEvent(new MessageEvent('message', { source: window, data: response }))
    await flushPromises()
    const quota = document.querySelector<HTMLInputElement>('#daily-quota')!
    const saveButton = document.querySelector<HTMLButtonElement>('#save')!
    quota.value = '-1'
    saveButton.click()
    expect(send.mock.calls.some(([data]) => data.type === 'config.save')).toBe(false)
    expect(saveButton.disabled).toBe(false)

    quota.value = '20'
    saveButton.click()
    const save = send.mock.calls.map(([data]) => data).find(data => data.type === 'config.save')
    expect(save.request_id).not.toBe(load.request_id)
    expect(save).toBeDefined()
    window.dispatchEvent(new MessageEvent('message', { source: window, data: { ...response, request_id: save.request_id, ok: false, error: '保存失败，请重试' } }))
    await flushPromises()
    expect(document.querySelector('#status')!.textContent).toBe('保存失败，请重试')
    expect(saveButton.disabled).toBe(false)
    saveButton.click()
    const saves = send.mock.calls.map(([data]) => data).filter(data => data.type === 'config.save')
    expect(saves).toHaveLength(2)
    expect(saves[1].request_id).not.toBe(saves[0].request_id)
    window.dispatchEvent(new MessageEvent('message', { source: window, data: { ...response, request_id: saves[1].request_id } }))
    await flushPromises()
    expect(document.querySelector('#status')!.textContent).toBe('已保存')
  })
})

it('分组可选择、校验、移除，并保留全站额度设置', async () => {
  window.location.hash = 'bridge_token=test-session'
  document.body.innerHTML = html
  const send = vi.spyOn(window, 'postMessage').mockImplementation(() => {})
  window.eval(source)
  const respond = (type: string, fields: Record<string, unknown>) => {
    const request = send.mock.calls.map(([data]) => data).filter(data => data.type === type).at(-1)
    window.dispatchEvent(new MessageEvent('message', { source: window, data: { source: 'sub2api-plugin-host', bridge_token: 'test-session', request_id: request.request_id, ok: true, ...fields } }))
  }
  respond('config.load', { config: { daily_quota: 100, reset_time: '03:30', apply_mode: 'next_reset', group_limits: { '1': 20 } } })
  await flushPromises()
  expect(document.querySelector<HTMLButtonElement>('#save')!.disabled).toBe(true)
  respond('groups.list', { groups: [{ id: 1, name: '分组一', platform: 'openai' }, { id: 2, name: '<img src=x onerror=alert(1)>', platform: 'anthropic' }] })
  await flushPromises()
  const select = document.querySelector<HTMLSelectElement>('#group-select')!
  const save = document.querySelector<HTMLButtonElement>('#save')!
  expect(document.querySelector<HTMLInputElement>('#group-quota-1')!.value).toBe('20')
  expect(select.querySelector('option[value="1"]')).toBeNull()
  select.value = '2'; select.dispatchEvent(new Event('change'))
  const second = document.querySelector<HTMLInputElement>('#group-quota-2')!
  expect(document.querySelector('img')).toBeNull()
  save.click()
  expect(send.mock.calls.some(([data]) => data.type === 'config.save')).toBe(false)
  second.value = '-1'; save.click()
  expect(send.mock.calls.some(([data]) => data.type === 'config.save')).toBe(false)
  second.value = '0'; save.click()
  let saved = send.mock.calls.map(([data]) => data).filter(data => data.type === 'config.save').at(-1)
  expect(saved.config).toEqual({ daily_quota: 100, reset_time: '03:30', apply_mode: 'next_reset', group_limits: { '1': 20, '2': 0 } })
  respond('config.save', {}); await flushPromises()
  document.querySelectorAll<HTMLButtonElement>('.remove-group').forEach(button => button.click())
  expect(select.querySelector('option[value="1"]')).not.toBeNull()
  save.click()
  saved = send.mock.calls.map(([data]) => data).filter(data => data.type === 'config.save').at(-1)
  expect(saved.config.group_limits).toBeUndefined()
  expect(saved.config.daily_quota).toBe(100)
  respond('config.save', {}); await flushPromises()
})

it('分组加载失败时禁止保存，避免清除已有上限', async () => {
  window.location.hash = 'bridge_token=test-session'
  document.body.innerHTML = html
  const send = vi.spyOn(window, 'postMessage').mockImplementation(() => {})
  window.eval(source)
  for (const [data] of send.mock.calls) {
    if (data.type === 'config.load' || data.type === 'groups.list') {
      window.dispatchEvent(new MessageEvent('message', { source: window, data: { source: 'sub2api-plugin-host', bridge_token: 'test-session', request_id: data.request_id, ok: data.type === 'config.load', config: { daily_quota: 100, reset_time: '00:00' }, error: '分组加载失败' } }))
    }
  }
  await flushPromises()
  expect(document.querySelector<HTMLButtonElement>('#save')!.disabled).toBe(true)
  expect(document.querySelector('#status')!.textContent).toBe('分组加载失败')
})
