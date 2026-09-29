<template>
  <section v-if="items.length" class="mx-1 mt-5 border-t border-gray-200 pt-4 dark:border-dark-700">
    <router-link to="/channel-quotas" class="mb-3 flex items-center justify-between px-2 text-xs font-semibold" @click="$emit('navigate')">{{ t('contributions.quotas') }}<span aria-hidden="true">↗</span></router-link>
    <router-link v-for="item in items.slice(0, 3)" :key="item.id" to="/channel-quotas" class="mb-3 block rounded-lg px-2 py-2 text-xs hover:bg-gray-100 dark:hover:bg-dark-700" @click="$emit('navigate')">
      <div class="flex items-center justify-between gap-2"><span class="truncate font-medium">{{ item.site_name }}</span><span class="shrink-0 text-[10px]" :class="item.status === 'active' ? 'text-teal-600 dark:text-teal-400' : 'text-amber-600 dark:text-amber-400'">{{ statusLabel(item.status, t) }}</span></div>
      <div class="mt-1 truncate text-[10px] text-gray-500">{{ providerName(item.platform) }} · {{ item.groups.map(group => group.name).join(' / ') || '—' }}</div>
      <div class="mt-1 text-[10px] tabular-nums text-gray-500">{{ quotaLabel(item, t) }}</div>
    </router-link>
  </section>
</template>

<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { channelContributionsAPI, type ContributionQuota } from '@/api/channelContributions'
import { providerName, quotaLabel, statusLabel } from './quotaDisplay'
defineEmits<{ navigate: [] }>()
const { t } = useI18n()
const items = ref<ContributionQuota[]>([])
let request: AbortController | undefined, timer: ReturnType<typeof setInterval> | undefined
async function load() {
  if (document.hidden || request) return
  const current = new AbortController(); request = current
  try { const data = await channelContributionsAPI.quotas(0, current.signal); if (!current.signal.aborted) items.value = data.items.filter(item => item.status === 'active') }
  catch { if (!current.signal.aborted) items.value = [] }
  finally { if (request === current) request = undefined }
}
onMounted(() => { void load(); timer = setInterval(load, 30000); document.addEventListener('visibilitychange', load); window.addEventListener('contribution-updated', load) })
onBeforeUnmount(() => { request?.abort(); clearInterval(timer); document.removeEventListener('visibilitychange', load); window.removeEventListener('contribution-updated', load) })
</script>
