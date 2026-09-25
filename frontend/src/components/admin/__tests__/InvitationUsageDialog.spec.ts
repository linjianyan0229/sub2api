import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import InvitationUsageDialog from '../InvitationUsageDialog.vue'
import type { RedeemCode } from '@/types'

const { load, save, success, error } = vi.hoisted(() => ({ load: vi.fn(), save: vi.fn(), success: vi.fn(), error: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { redeem: { listInvitationUsages: load, updateInvitationLimit: save } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: success, showError: error }) }))
vi.mock('vue-i18n', async (original) => ({ ...await original<typeof import('vue-i18n')>(), useI18n: () => ({ t: (key: string) => key }) }))

const code = { id: 7, type: 'invitation', max_uses: 5, used_count: 2 } as RedeemCode
function render() {
  return mount(InvitationUsageDialog, {
    props: { code },
    global: { stubs: {
      BaseDialog: { template: '<div><slot /></div>' },
      DataTable: { props: ['data'], template: '<div><div v-for="row in data" :key="row.id"><slot name="cell-username" :row="row" /><slot name="cell-ip" :value="row.ip" /></div></div>' },
      Pagination: { emits: ['update:page'], template: '<button data-next @click="$emit(\'update:page\', 2)">Next</button>' }
    } }
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  load.mockResolvedValue({ items: [{ id: 1, user_id: 2, username: '昵称', email: 'user@example.com', ip: '192.0.2.1' }, { id: 2, user_id: 3, username: '旧用户', email: '', ip: null }], total: 2 })
  save.mockResolvedValue({ ...code, max_uses: 10 })
})

describe('邀请码配置和使用记录', () => {
  it('显示用户和 IP，旧记录明确显示未知，分页传递正确参数', async () => {
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('昵称')
    expect(wrapper.text()).toContain('user@example.com')
    expect(wrapper.text()).toContain('192.0.2.1')
    expect(wrapper.text()).toContain('admin.redeem.unknownIP')
    await wrapper.get('[data-next]').trigger('click')
    await flushPromises()
    expect(load).toHaveBeenLastCalledWith(7, 2, 10)
    wrapper.unmount()
  })

  it('保存人数上限后通知列表刷新', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('input').setValue(10)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(save).toHaveBeenCalledWith(7, 10)
    expect(wrapper.emitted('updated')).toHaveLength(1)
    expect(success).toHaveBeenCalled()
    wrapper.unmount()
  })

  it('拒绝小于已使用人数的上限，并显示后端保存失败', async () => {
    const wrapper = render()
    await flushPromises()
    await wrapper.get('input').setValue(1)
    await wrapper.get('form').trigger('submit')
    expect(save).not.toHaveBeenCalled()
    expect(error).toHaveBeenCalledWith('admin.redeem.invalidMaxUses')
    save.mockRejectedValueOnce(new Error('conflict'))
    await wrapper.get('input').setValue(3)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(error).toHaveBeenCalledWith('admin.redeem.invitationSaveFailed')
    expect(wrapper.emitted('updated')).toBeUndefined()
    wrapper.unmount()
  })
})
