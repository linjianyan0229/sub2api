import type { ContributionQuota } from '@/api/channelContributions'
type Translate = (key: string) => string
export const providerName = (platform: string) => ({ openai: 'OpenAI', anthropic: 'Claude', gemini: 'Gemini' })[platform] || platform
export const money = (amount: number) => `$${amount.toLocaleString('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 4 })}`
export function statusLabel(status: string, t: Translate) {
  return t(`contributions.states.${['pending', 'rejected', 'active', 'disabled', 'inactive', 'quota_exhausted', 'limited', 'error', 'removed'].includes(status) ? status : 'limited'}`)
}
export function quotaLabel(item: ContributionQuota, t: Translate) {
  if (!item.independent) return t('common.sharedQuota')
  return [item.daily_limit ? `${t('contributions.daily')}: ${money(item.daily_limit)}` : '', item.total_limit ? `${t('contributions.total')}: ${money(item.total_limit)}` : ''].filter(Boolean).join(' · ')
}
