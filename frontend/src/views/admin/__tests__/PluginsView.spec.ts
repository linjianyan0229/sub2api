import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import PluginsView from '../PluginsView.vue'

const {
  listPlugins,
  uploadPlugin,
  enablePlugin,
  savePluginConfig,
  createUISession,
  listGroups,
  stepUpRun,
} = vi.hoisted(() => ({
  listPlugins: vi.fn(),
  uploadPlugin: vi.fn(),
  enablePlugin: vi.fn(),
  savePluginConfig: vi.fn(),
  createUISession: vi.fn(),
  listGroups: vi.fn(),
  stepUpRun: vi.fn((action: () => Promise<unknown>) => action()),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: { getAllIncludingInactive: listGroups },
    plugins: {
      list: listPlugins,
      upload: uploadPlugin,
      enable: enablePlugin,
      disable: vi.fn(),
      remove: vi.fn(),
      getConfig: vi.fn().mockResolvedValue({}),
      saveConfig: savePluginConfig,
      test: vi.fn().mockResolvedValue({ success: true, message: 'ok', latency_ms: 1 }),
      createUISession,
    },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
  }),
}))

vi.mock('@/composables/useStepUp', () => ({
  useStepUp: () => ({ run: stepUpRun }),
  isStepUpBlocked: () => false,
  isStepUpCancelled: () => false,
  stepUpBlockReason: () => '',
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))

const plugin = {
  id: 7,
  plugin_key: 'local.test.transport',
  name: 'Test Transport',
  version: '1.0.0',
  description: '',
  author: 'test',
  manifest: {
    schema_version: 1,
    id: 'local.test.transport',
    name: 'Test Transport',
    version: '1.0.0',
    requires: {
      sub2api: '>=0.1.0',
      plugin_protocol: 1,
      transport_api: 1,
      ui_bridge: 1,
    },
    capabilities: [],
    ui: { entrypoint: 'ui/index.html' },
  },
  binary_sha256: 'a'.repeat(64),
  signature_status: 'trusted' as const,
  state: 'disabled' as const,
  last_error: '',
  installed_at: '2026-08-22T00:00:00Z',
  updated_at: '2026-08-22T00:00:00Z',
  bindings: [
    {
      id: 1,
      plugin_id: 7,
      capability: 'openai.oauth.outbound_transport.v1',
      platform: 'openai',
      account_type: 'oauth',
      enabled: false,
      rollout_percent: 100,
    },
  ],
  compatibility: {
    compatible: true,
    tested: true,
    status: 'compatible' as const,
    message: '',
    current_sub2api_version: '0.1.0',
    required_sub2api_version: '>=0.1.0',
    recommended_sub2api_version: '0.1.0',
    plugin_protocol: 1,
    transport_api: 1,
    ui_bridge: 1,
  },
  runtime_healthy: false,
  runtime_message: '',
}

function mountView() {
  return mount(PluginsView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        BaseDialog: { template: '<div><slot /></div>' },
        Icon: true,
        TotpStepUpDialog: true,
      },
    },
  })
}

describe('管理员插件页二次验证', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    stepUpRun.mockImplementation((action: () => Promise<unknown>) => action())
    listPlugins.mockResolvedValue([plugin])
    uploadPlugin.mockResolvedValue(plugin)
    enablePlugin.mockResolvedValue(plugin)
    savePluginConfig.mockResolvedValue({ enabled: true })
    listGroups.mockResolvedValue([{ id: 1, name: '分组一', platform: 'openai', status: 'active', model_pricing: ['private'] }])
    createUISession.mockResolvedValue({
      url: '/api/v1/plugin-ui/token/index.html#bridge_token=bridge',
      bridge_token: 'bridge',
      ui_bridge_version: 1,
      expires_at: '2026-08-22T01:00:00Z',
    })
  })

  it('启用插件通过 step-up 控制器执行', async () => {
    const wrapper = mountView()
    await flushPromises()

    const button = wrapper.findAll('button').find((item) => item.text().includes('admin.plugins.enable'))
    expect(button).toBeDefined()
    await button!.trigger('click')
    await flushPromises()

    expect(stepUpRun).toHaveBeenCalledTimes(1)
    expect(enablePlugin).toHaveBeenCalledWith(7, 100, false)
  })

  it('上传插件通过 step-up 控制器执行', async () => {
    const wrapper = mountView()
    await flushPromises()
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', {
      configurable: true,
      value: [new File(['plugin'], 'transport.s2plugin', { type: 'application/zip' })],
    })

    await input.trigger('change')
    await flushPromises()

    expect(stepUpRun).toHaveBeenCalledTimes(1)
    expect(uploadPlugin).toHaveBeenCalledTimes(1)
  })

  it.each([true, false])('分组选择仅向共享额度插件返回最少字段，拥有能力：%s', async (allowed) => {
    listPlugins.mockResolvedValue([{ ...plugin, manifest: { ...plugin.manifest, capabilities: allowed ? [{ id: 'billing.shared_daily_quota.v1', platform: 'all', account_type: 'all' }] : [] } }])
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text().includes('admin.plugins.configure'))!.trigger('click')
    await flushPromises()
    const frame = wrapper.get('iframe').element as HTMLIFrameElement
    // jsdom 中 iframe 未挂到 document，提供等价的消息源以验证 bridge 边界。
    const frameWindow = { postMessage: vi.fn() }
    Object.defineProperty(frame, 'contentWindow', { configurable: true, value: frameWindow })
    const dispatch = (token: string) => window.dispatchEvent(new MessageEvent('message', { source: frameWindow as unknown as Window, origin: 'null', data: { source: 'sub2api-plugin-ui', bridge_token: token, type: 'groups.list', request_id: 'groups-request' } }))
    dispatch('wrong-token'); await flushPromises()
    expect(listGroups).not.toHaveBeenCalled()
    dispatch('bridge'); await flushPromises()
    if (allowed) {
      expect(listGroups).toHaveBeenCalledOnce()
      expect(frameWindow.postMessage).toHaveBeenCalledWith(expect.objectContaining({ ok: true, groups: [{ id: 1, name: '分组一', platform: 'openai', status: 'active' }] }), '*')
    } else {
      expect(listGroups).not.toHaveBeenCalled()
      expect(frameWindow.postMessage).toHaveBeenCalledWith(expect.objectContaining({ ok: false }), '*')
    }
    wrapper.unmount()
  })
})
