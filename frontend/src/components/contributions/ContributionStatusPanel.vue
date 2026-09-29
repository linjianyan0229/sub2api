<template>
  <section v-if="items.length || failed || before" class="my-6" aria-labelledby="contribution-status-title">
    <header class="mb-4 flex items-center justify-between gap-4">
      <h2 id="contribution-status-title" class="text-base font-semibold">{{ t('contributions.communityChannels') }}</h2>
      <button class="btn btn-secondary btn-sm" :disabled="loading" @click="load">{{ t('common.refresh') }}</button>
    </header>
    <p v-if="failed" role="alert" class="py-4 text-sm text-red-600">{{ t('contributions.statusLoadFailed') }}</p>
    <div v-else class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
      <article v-for="item in items" :key="item.id" :data-contribution-status="item.id" class="rounded-2xl border border-gray-200 bg-white p-5 dark:border-dark-600 dark:bg-dark-800">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0"><p class="text-[11px] text-gray-400">{{ providerName(item.platform) }}</p><h3 class="mt-1 break-words font-semibold">{{ item.site_name }}</h3></div>
          <span class="inline-flex shrink-0 items-center gap-1.5 text-xs" :class="item.status === 'active' ? 'text-teal-700 dark:text-teal-300' : 'text-amber-700 dark:text-amber-300'"><i class="h-1.5 w-1.5 rounded-full bg-current" />{{ statusLabel(item.status, t) }}</span>
        </div>
        <dl class="mt-4 grid grid-cols-2 gap-3 text-xs">
          <div><dt class="text-gray-400">{{ t('contributions.group') }}</dt><dd class="mt-1 break-words">{{ item.groups.map(group => group.name).join(' / ') }}</dd></div>
          <div><dt class="text-gray-400">{{ t('contributions.contributor') }}</dt><dd class="mt-1 break-words">{{ item.contributor_name }}</dd></div>
          <div><dt class="text-gray-400">{{ t('contributions.rpm') }}</dt><dd class="mt-1">{{ item.rpm || t('admin.groups.subscription.noLimit') }}</dd></div>
          <div><dt class="text-gray-400">{{ t('contributions.concurrency') }}</dt><dd class="mt-1">{{ item.concurrency }}</dd></div>
        </dl>
        <div class="mt-4 border-t border-gray-100 pt-3 text-xs dark:border-dark-600">
          <template v-if="item.independent"><p v-for="kind in ['daily', 'total'] as const" :key="kind" class="mt-1 flex justify-between gap-2"><span class="text-gray-400">{{ t(`contributions.${kind}`) }}</span><span class="tabular-nums">{{ money(item[`${kind}_used`]) }} / {{ item[`${kind}_limit`] ? money(item[`${kind}_limit`]!) : t('admin.groups.subscription.noLimit') }}</span></p></template>
          <p v-else class="text-teal-700 dark:text-teal-300">{{ t('common.sharedQuota') }}</p>
        </div>
        <details v-if="item.models?.length" class="mt-3 text-xs text-gray-500"><summary class="cursor-pointer">{{ t('contributions.models') }} · {{ item.models.length }}</summary><p class="mt-2 break-words">{{ item.models.join(' · ') }}</p></details>
      </article>
    </div>
    <div v-if="before || hasMore" class="mt-4 flex justify-center gap-6 text-sm"><button v-if="before" :disabled="loading" @click="before = 0; load()">{{ t('contributions.latest') }}</button><button v-if="hasMore" :disabled="loading" @click="older">{{ t('contributions.older') }}</button></div>
  </section>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { channelContributionsAPI as api, type ContributionQuota } from '@/api/channelContributions'
import { money, providerName, statusLabel } from './quotaDisplay'
const { t } = useI18n()
const items = ref<ContributionQuota[]>([]), before = ref(0), hasMore = ref(false), loading = ref(false), failed = ref(false)
let request: AbortController | undefined, timer: ReturnType<typeof setInterval> | undefined
async function load() {
  request?.abort()
  const current = new AbortController(); request = current
  loading.value = true; failed.value = false
  try {
    const data = await api.statuses(before.value, current.signal)
    if (current.signal.aborted) return
    items.value = data.items; hasMore.value = data.has_more
  } catch { if (!current.signal.aborted) failed.value = true }
  finally { if (!current.signal.aborted) loading.value = false }
}
function older() { const last = items.value.at(-1); if (last) { before.value = last.id; void load() } }
function refreshVisible() { if (!document.hidden && !loading.value) void load() }
onMounted(() => { void load(); timer = setInterval(refreshVisible, 30000); document.addEventListener('visibilitychange', refreshVisible) })
onBeforeUnmount(() => { request?.abort(); clearInterval(timer); document.removeEventListener('visibilitychange', refreshVisible) })
</script>
