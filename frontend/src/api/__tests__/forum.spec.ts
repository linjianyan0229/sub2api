import { beforeEach, describe, expect, it, vi } from 'vitest'
import { forumAPI } from '../forum'

const mocks = vi.hoisted(() => ({ delete: vi.fn() }))
vi.mock('../client', () => ({ apiClient: mocks }))

beforeEach(() => { mocks.delete.mockReset().mockResolvedValue({ data: { ok: true } }) })

describe('forum recall API', () => {
  it('uses the authenticated endpoint without client-supplied permissions', async () => {
    await forumAPI.recall(42)
    expect(mocks.delete).toHaveBeenCalledOnce()
    expect(mocks.delete).toHaveBeenCalledWith('/forum/messages/42')
  })

  it('propagates server permission and expiry errors', async () => {
    const error = new Error('FORUM_RECALL_EXPIRED')
    mocks.delete.mockRejectedValueOnce(error)
    await expect(forumAPI.recall(42)).rejects.toBe(error)
  })
})
