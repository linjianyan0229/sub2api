import { apiClient } from './client'

export interface UsageLeaderboardItem {
  username: string
  actual_cost: number
  tokens: number
}

export async function getUsageLeaderboard(): Promise<UsageLeaderboardItem[]> {
  const { data } = await apiClient.get<{ ranking: UsageLeaderboardItem[] }>('/usage/leaderboard')
  return data.ranking
}
