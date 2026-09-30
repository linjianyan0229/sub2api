<template>
  <section v-if="items.length || failed || before" class="my-6" aria-labelledby="contribution-status-title">
    <header class="mb-4 flex items-center justify-between gap-4">
      <h2 id="contribution-status-title" class="text-base font-semibold">{{ t('contributions.communityChannels') }}</h2>
      <button class="btn btn-secondary btn-sm" :disabled="loading" @click="load">{{ t('common.refresh') }}</button>
    </header>
    <p v-if="failed" role="alert" class="py-4 text-sm text-red-600">{{ t('contributions.statusLoadFailed') }}</p>
    <div v-else class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
      <article v-for="item in items" :key="item.id" :data-contribution-status="item.id">
        <MonitorCard
          :item="item.monitor"
          window="7d"
          :availability-value="item.monitor.timeline.length ? item.monitor.availability_7d : null"
          :countdown-seconds="item.probing ? countdown : undefined"
          @click="selected = item"
        />
        <p class="mt-2 flex justify-between gap-3 px-2 text-xs text-gray-500 dark:text-gray-400">
          <span>{{ t('contributions.contributor') }} · {{ item.contributor_name }}</span>
          <span v-if="!item.probing && item.monitor.timeline.length">{{ t('monitorCommon.maintenancePaused') }}</span>
        </p>
      </article>
    </div>
    <div v-if="before || hasMore" class="mt-4 flex justify-center gap-6 text-sm"><button v-if="before" :disabled="loading" @click="before = 0; load()">{{ t('contributions.latest') }}</button><button v-if="hasMore" :disabled="loading" @click="older">{{ t('contributions.older') }}</button></div>
    <MonitorDetailDialog :show="selected !== null" :monitor-id="selected?.id ?? null" :title="selected?.monitor.name || ''" :load-detail="api.monitorStatus" @close="selected = null" />
  </section>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { channelContributionsAPI as api, type ContributionMonitor } from '@/api/channelContributions'
import MonitorCard from '@/components/user/monitor/MonitorCard.vue'
import MonitorDetailDialog from '@/components/user/MonitorDetailDialog.vue'
const { t } = useI18n()
const items = ref<ContributionMonitor[]>([]), before = ref(0), hasMore = ref(false), loading = ref(false), failed = ref(false)
const selected = ref<ContributionMonitor | null>(null), countdown = ref(30)
let request: AbortController | undefined, timer: ReturnType<typeof setInterval> | undefined
async function load() {
  request?.abort()
  const current = new AbortController(); request = current
  loading.value = true; failed.value = false
  try {
    const data = await api.statuses(before.value, current.signal)
    if (current.signal.aborted) return
    items.value = data.items; hasMore.value = data.has_more
    if (selected.value && !data.items.some(item => item.id === selected.value?.id)) selected.value = null
  } catch { if (!current.signal.aborted) failed.value = true }
  finally { if (!current.signal.aborted) { loading.value = false; countdown.value = 30 } }
}
function older() { const last = items.value.at(-1); if (last) { before.value = last.id; void load() } }
function refreshVisible() { if (!document.hidden && !loading.value) void load() }
function tick() { if (!document.hidden && !loading.value && --countdown.value <= 0) refreshVisible() }
onMounted(() => { void load(); timer = setInterval(tick, 1000); document.addEventListener('visibilitychange', refreshVisible); window.addEventListener('contribution-updated', refreshVisible) })
onBeforeUnmount(() => { request?.abort(); clearInterval(timer); document.removeEventListener('visibilitychange', refreshVisible); window.removeEventListener('contribution-updated', refreshVisible) })
</script>
