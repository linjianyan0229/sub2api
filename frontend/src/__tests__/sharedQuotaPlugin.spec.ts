import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
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
  it.each([true, false])('仅通过当前会话加载和保存配置，randomUUID 可用：%s', async (randomUUIDAvailable) => {
    if (!randomUUIDAvailable) {
      vi.stubGlobal('crypto', { getRandomValues: crypto.getRandomValues.bind(crypto) })
    }
    window.location.hash = 'bridge_token=test-session'
    document.body.innerHTML = html
    const send = vi.spyOn(window, 'postMessage').mockImplementation(() => {})
    window.eval(source)
    const load = send.mock.calls.map(([data]) => data).find(data => data.type === 'config.load')
    expect(load).toBeDefined()
    const response = { source: 'sub2api-plugin-host', bridge_token: 'test-session', request_id: load.request_id, ok: true, config: { daily_quota: 123.45, reset_time: '03:30' } }
    window.dispatchEvent(new MessageEvent('message', { source: window, data: { ...response, bridge_token: 'wrong-session' } }))
    await Promise.resolve()
    const quota = document.querySelector<HTMLInputElement>('#daily-quota')!
    expect(quota.disabled).toBe(true)
    window.dispatchEvent(new MessageEvent('message', { source: window, data: response }))
    await Promise.resolve()
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
    expect(save.config).toEqual({ daily_quota: 20, reset_time: '03:30' })
    window.dispatchEvent(new MessageEvent('message', { source: window, data: { ...response, request_id: save.request_id } }))
    await Promise.resolve()
    expect(document.querySelector('#status')!.textContent).toBe('已保存')
    expect(saveButton.disabled).toBe(false)
  })

  it('无效金额不保存，保存失败显示错误且允许再次点击', async () => {
    window.location.hash = 'bridge_token=test-session'
    document.body.innerHTML = html
    const send = vi.spyOn(window, 'postMessage').mockImplementation(() => {})
    window.eval(source)
    const load = send.mock.calls.map(([data]) => data).find(data => data.type === 'config.load')
    const response = { source: 'sub2api-plugin-host', bridge_token: 'test-session', request_id: load.request_id, ok: true, config: {} }
    window.dispatchEvent(new MessageEvent('message', { source: window, data: response }))
    await Promise.resolve()
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
    await Promise.resolve()
    expect(document.querySelector('#status')!.textContent).toBe('保存失败，请重试')
    expect(saveButton.disabled).toBe(false)
    saveButton.click()
    const saves = send.mock.calls.map(([data]) => data).filter(data => data.type === 'config.save')
    expect(saves).toHaveLength(2)
    expect(saves[1].request_id).not.toBe(saves[0].request_id)
    window.dispatchEvent(new MessageEvent('message', { source: window, data: { ...response, request_id: saves[1].request_id } }))
    await Promise.resolve()
    expect(document.querySelector('#status')!.textContent).toBe('已保存')
  })
})
