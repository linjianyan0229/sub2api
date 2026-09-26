import { apiClient } from './client'

export interface ForumAuthor {
  user_id: number
  username: string
  avatar_url: string
  actual_cost: number
}
export interface ForumMessage {
  id: number
  author: ForumAuthor
  content: string
  rank: number
  pinned: boolean
  created_at: string
}
export interface ForumArrival {
  id: number
  author: ForumAuthor
  rank: number
  created_at: string
}
export interface ForumPage {
  messages: ForumMessage[]
  pinned: ForumMessage[]
  settings: { muted_until: string | null; retention_days: number }
  muted_until: string | null
  has_more: boolean
}
export interface ForumMute { author: ForumAuthor; muted_until: string }

export const forumAPI = {
  async page(before = 0, signal?: AbortSignal) {
    return (await apiClient.get<ForumPage>('/forum', { params: { before }, signal })).data
  },
  async send(content: string) {
    return (await apiClient.post<ForumMessage>('/forum/messages', { content })).data
  },
  async events(after?: number, signal?: AbortSignal) {
    return (await apiClient.get<{ cursor: number; arrivals: ForumArrival[] }>('/forum/events', { params: { after }, signal })).data
  },
  async arrive(signal?: AbortSignal) {
    await apiClient.post('/forum/arrival', undefined, { signal })
  },
  async settings(settings: { mute_minutes?: number; retention_days?: number }) {
    await apiClient.patch('/admin/forum/settings', settings)
  },
  async mutes() { return (await apiClient.get<ForumMute[]>('/admin/forum/mutes')).data },
  async mute(user: number, minutes: number) { await apiClient.put(`/admin/forum/users/${user}/mute`, { minutes }) },
  async pin(id: number, pinned: boolean) { await apiClient.put(`/admin/forum/messages/${id}/pin`, { pinned }) },
  async clear() { await apiClient.delete('/admin/forum/messages', { data: { confirm: true } }) }
}
