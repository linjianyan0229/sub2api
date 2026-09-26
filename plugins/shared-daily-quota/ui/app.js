(() => {
  'use strict'
  const token = new URLSearchParams(location.hash.slice(1)).get('bridge_token')
  const pending = new Map()
  const form = document.getElementById('config-form')
  const quota = document.getElementById('daily-quota')
  const reset = document.getElementById('reset-time')
  const mode = document.getElementById('apply-mode')
  const save = document.getElementById('save')
  const status = document.getElementById('status')
  const notify = (message, error = false) => { status.textContent = message; status.dataset.error = String(error) }
  const send = (type, fields = {}) => parent.postMessage({ source: 'sub2api-plugin-ui', bridge_token: token, type, ...fields }, '*')
  function request(type, fields = {}) {
    return new Promise((resolve, reject) => {
      // getRandomValues 在 HTTP 页面和沙箱内也可用；此 ID 仅关联请求与响应。
      const request_id = Array.from(crypto.getRandomValues(new Uint8Array(16)), byte => byte.toString(16).padStart(2, '0')).join('')
      const timeout = setTimeout(() => { pending.delete(request_id); reject(new Error('操作超时，请重试')) }, 30000)
      pending.set(request_id, { resolve, reject, timeout })
      send(type, { request_id, ...fields })
    })
  }
  function onMessage(event) {
    const data = event.data
    if (event.source !== parent || !data || data.source !== 'sub2api-plugin-host' || data.bridge_token !== token) return
    const item = pending.get(data.request_id)
    if (!item) return
    clearTimeout(item.timeout); pending.delete(data.request_id)
    if (data.ok) item.resolve(data)
    else item.reject(new Error(data.error || data.message || '操作失败'))
  }
  window.addEventListener('message', onMessage)
  window.addEventListener('pagehide', () => {
    window.removeEventListener('message', onMessage)
    for (const item of pending.values()) { clearTimeout(item.timeout); item.reject(new Error('页面已关闭')) }
    pending.clear()
  }, { once: true })
  // 宿主沙箱不允许表单提交，直接通过 Bridge 保存并保留字段校验。
  save.addEventListener('click', async () => {
    if (!form.reportValidity()) return
    save.disabled = true; notify('')
    try {
      await request('config.save', { config: { daily_quota: Number(quota.value), reset_time: reset.value, apply_mode: mode.value } })
      notify('已保存')
    } catch (error) { notify(error.message, true) }
    finally { save.disabled = false }
  })
  if (!token) { notify('配置会话无效', true); return }
  send('sub2api.plugin.ready')
  send('ui.resize', { height: 450 })
  request('config.load').then(data => {
    quota.value = data.config?.daily_quota ?? ''
    reset.value = data.config?.reset_time ?? '00:00'
    mode.value = data.config?.apply_mode ?? 'immediate'
    quota.disabled = reset.disabled = mode.disabled = save.disabled = false
  }).catch(error => notify(error.message, true))
})()
