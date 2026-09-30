import { apiClient } from './client'
import type { CreateGroupRequest, SharedQuota } from '@/types'
import type { UserMonitorDetail, UserMonitorView } from './channelMonitor'

export type ContributionPlatform = 'openai' | 'anthropic' | 'gemini'
export type ContributionStatus = 'pending' | 'approved' | 'rejected'
export interface ContributionSettings {
  rpm?: number
  concurrency?: number
  daily_limit?: number
  total_limit?: number
  models?: string[]
  new_group_config?: CreateGroupRequest
  show_in_status?: boolean
  monitor_model?: string
  monitor_api_mode?: 'chat_completions' | 'responses'
}
export interface ChannelContribution extends ContributionSettings {
  id: number
  contributor_name: string
  site_name: string
  base_url: string
  platform: ContributionPlatform
  requested_group_id: number | null
  requested_group_name: string
  new_group_name: string
  status: ContributionStatus
  review_note: string
  reviewed_at: string | null
  group_id: number | null
  account_id: number | null
  created_at: string
  can_edit?: boolean
}
export interface ContributionAccountInput extends Omit<ContributionSettings, 'new_group_config'> {
  site_name: string
  base_url: string
  api_key?: string
}
export interface ContributionAccountView extends ContributionAccountInput { platform: ContributionPlatform }
export interface ContributionInput extends ContributionSettings {
  site_name: string
  base_url: string
  api_key: string
  platform: ContributionPlatform
  group_id: number | null
  new_group_name: string
}
export interface ContributionGroup { id: number; name: string; platform: ContributionPlatform; accounts?: { id: number; name: string }[] | null }
export interface ContributionQuota extends ContributionSettings {
  id: number
  site_name: string
  contributor_name: string
  platform: ContributionPlatform
  status: string
  groups: { id: number; name: string }[]
  independent: boolean
  daily_used: number
  total_used: number
}
export interface ContributionQuotaPage { items: ContributionQuota[]; shared_quota: SharedQuota | null; has_more: boolean }
export interface ContributionMonitor { id: number; contributor_name: string; monitor: UserMonitorView; probing: boolean }
export interface ContributionMonitorPage { items: ContributionMonitor[]; has_more: boolean }

export const channelContributionsAPI = {
  async list(admin: boolean, status = '', before = 0, signal?: AbortSignal) {
    return (await apiClient.get<{ items: ChannelContribution[]; has_more: boolean }>(`${admin ? '/admin' : ''}/channel-contributions`, { params: { status, before }, signal })).data
  },
  async groups() { return (await apiClient.get<ContributionGroup[]>('/channel-contributions/groups')).data },
  async account(id: number, signal?: AbortSignal) { return (await apiClient.get<ContributionAccountView>(`/channel-contributions/${id}/account`, { signal })).data },
  async updateAccount(id: number, input: ContributionAccountInput) { await apiClient.put(`/channel-contributions/${id}/account`, input) },
  async statuses(before = 0, signal?: AbortSignal) { return (await apiClient.get<ContributionMonitorPage>('/channel-contributions/statuses', { params: { before }, signal })).data },
  async monitorStatus(id: number) { return (await apiClient.get<UserMonitorDetail>(`/channel-contributions/${id}/status`)).data },
  async submit(input: ContributionInput) { return (await apiClient.post<ChannelContribution>('/channel-contributions', input)).data },
  async review(id: number, decision: 'approved' | 'rejected', note: string, models?: string[]) {
    await apiClient.post(`/admin/channel-contributions/${id}/review`, { decision, note, models })
  },
  async models(id: number) { return (await apiClient.post<{ models: string[] }>(`/admin/channel-contributions/${id}/models`)).data.models },
  async quotas(before = 0, signal?: AbortSignal) {
    return (await apiClient.get<ContributionQuotaPage>('/channel-contributions/quotas', { params: { before }, signal })).data
  }
}
