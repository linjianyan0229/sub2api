import { apiClient } from './client'

export type ContributionPlatform = 'openai' | 'anthropic' | 'gemini'
export type ContributionStatus = 'pending' | 'approved' | 'rejected'
export interface ChannelContribution {
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
}
export interface ContributionInput {
  site_name: string
  base_url: string
  api_key: string
  platform: ContributionPlatform
  group_id: number | null
  new_group_name: string
}
export interface ContributionGroup { id: number; name: string; platform: ContributionPlatform }

export const channelContributionsAPI = {
  async list(admin: boolean, status = '', before = 0, signal?: AbortSignal) {
    return (await apiClient.get<{ items: ChannelContribution[]; has_more: boolean }>(`${admin ? '/admin' : ''}/channel-contributions`, { params: { status, before }, signal })).data
  },
  async groups() { return (await apiClient.get<ContributionGroup[]>('/channel-contributions/groups')).data },
  async submit(input: ContributionInput) { return (await apiClient.post<ChannelContribution>('/channel-contributions', input)).data },
  async review(id: number, decision: 'approved' | 'rejected', note: string) {
    await apiClient.post(`/admin/channel-contributions/${id}/review`, { decision, note })
  }
}
