<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex items-center justify-between gap-4">
        <div>
          <h1 class="text-2xl font-bold text-gray-900 dark:text-white">{{ t('nav.leaderboard') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('leaderboard.allTime') }} · Top 20</p>
        </div>
        <button class="btn btn-secondary" :disabled="loading" @click="load">{{ t('common.refresh') }}</button>
      </div>
      <div class="card overflow-hidden">
        <div v-if="loading" class="flex justify-center py-16"><LoadingSpinner /></div>
        <div v-else-if="failed" role="alert" class="py-16 text-center text-sm text-red-600 dark:text-red-400">
          {{ t('leaderboard.loadFailed') }}
        </div>
        <div v-else class="overflow-x-auto">
          <table class="w-full min-w-[480px] text-sm">
            <thead class="bg-gray-50 text-gray-500 dark:bg-dark-800 dark:text-gray-400">
              <tr>
                <th scope="col" class="w-20 px-5 py-4 text-left font-medium">{{ t('leaderboard.rank') }}</th>
                <th scope="col" class="px-5 py-4 text-left font-medium">{{ t('leaderboard.nickname') }}</th>
                <th scope="col" class="px-5 py-4 text-right font-medium">{{ t('leaderboard.amount') }} (USD)</th>
                <th scope="col" class="px-5 py-4 text-right font-medium">{{ t('leaderboard.tokens') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="(item, index) in items" :key="index" class="hover:bg-gray-50 dark:hover:bg-dark-800/50">
                <td class="px-5 py-4 tabular-nums">
                  <span class="inline-flex h-7 w-7 items-center justify-center rounded-full font-semibold"
                    :class="index < 3 ? 'bg-primary-50 text-primary-600 dark:bg-primary-500/10 dark:text-primary-400' : 'text-gray-400'">
                    {{ index + 1 }}
                  </span>
                </td>
                <td class="max-w-64 truncate px-5 py-4 font-medium text-gray-900 dark:text-gray-100">{{ item.username || t('leaderboard.anonymous') }}</td>
                <td class="px-5 py-4 text-right font-medium tabular-nums text-gray-900 dark:text-gray-100">${{ formatCostFixed(item.actual_cost, 4) }}</td>
                <td class="px-5 py-4 text-right tabular-nums text-gray-600 dark:text-gray-300">{{ item.tokens.toLocaleString(locale) }}</td>
              </tr>
              <tr v-if="items.length === 0"><td colspan="4" class="py-16 text-center text-gray-400">{{ t('common.noData') }}</td></tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { getUsageLeaderboard, type UsageLeaderboardItem } from '@/api/leaderboard'
import { formatCostFixed } from '@/utils/format'

const { t, locale } = useI18n()
const items = ref<UsageLeaderboardItem[]>([])
const loading = ref(false)
const failed = ref(false)

async function load() {
  if (loading.value) return
  loading.value = true
  failed.value = false
  try {
    items.value = await getUsageLeaderboard()
  } catch {
    failed.value = true
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>
