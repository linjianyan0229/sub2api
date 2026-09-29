<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl">
      <header class="mb-8 flex items-center justify-between gap-4"><h1 class="text-2xl font-semibold tracking-tight">{{ t('contributions.quotas') }}</h1><button class="btn btn-secondary" :disabled="loading" @click="load">{{ t('common.refresh') }}</button></header>
      <section v-if="shared" class="mb-7 flex flex-wrap items-baseline justify-between gap-4 border-y border-teal-500/20 py-5">
        <span class="text-sm text-gray-500">{{ t('common.sharedQuota') }}</span><div class="text-2xl font-semibold tabular-nums text-teal-700 dark:text-teal-400">{{ money(shared.remaining) }} <span class="text-sm font-normal text-gray-400">/ {{ money(shared.total) }}</span></div>
      </section>
      <p v-if="failed" role="alert" class="py-10 text-red-600">{{ t('contributions.quotaLoadFailed') }}</p>
      <p v-else-if="loading && !items.length" class="py-20 text-center text-gray-500">{{ t('common.loading') }}</p>
      <p v-else-if="!items.length" class="py-20 text-center text-gray-500">{{ t('contributions.quotaEmpty') }}</p>
      <div v-else class="grid gap-5 lg:grid-cols-2">
        <article v-for="item in items" :key="item.id" class="overflow-hidden rounded-2xl border border-gray-200 bg-white p-6 dark:border-dark-600 dark:bg-dark-800" :data-channel="item.id">
          <div class="flex items-start justify-between gap-4"><div><p class="text-[11px] uppercase tracking-widest text-gray-400">{{ providerName(item.platform) }}</p><h2 class="mt-1 break-all text-lg font-semibold">{{ item.site_name }}</h2></div><span class="shrink-0 rounded-full px-2.5 py-1 text-xs" :class="item.status === 'active' ? 'bg-teal-500/10 text-teal-700 dark:text-teal-300' : 'bg-amber-500/10 text-amber-700 dark:text-amber-300'">{{ statusLabel(item.status, t) }}</span></div>
          <dl class="mt-5 grid grid-cols-2 gap-x-5 gap-y-3 text-xs"><div><dt class="text-gray-400">{{ t('contributions.group') }}</dt><dd class="mt-1 break-words">{{ item.groups.map(group => group.name).join(' / ') || '—' }}</dd></div><div><dt class="text-gray-400">{{ t('contributions.contributor') }}</dt><dd class="mt-1 break-words">{{ item.contributor_name }}</dd></div><div><dt class="text-gray-400">RPM</dt><dd class="mt-1">{{ item.rpm || t('admin.groups.subscription.noLimit') }}</dd></div><div><dt class="text-gray-400">{{ t('contributions.concurrency') }}</dt><dd class="mt-1">{{ item.concurrency }}</dd></div></dl>
          <div class="mt-5 space-y-4 border-t border-gray-100 pt-5 dark:border-dark-600">
            <template v-if="item.independent"><div v-for="kind in ['daily', 'total'] as const" :key="kind"><div class="flex justify-between text-xs"><span class="text-gray-500">{{ t(`contributions.${kind}`) }}</span><span class="tabular-nums">{{ money(item[`${kind}_used`]) }} / {{ item[`${kind}_limit`] ? money(item[`${kind}_limit`]!) : t('admin.groups.subscription.noLimit') }}</span></div><div v-if="item[`${kind}_limit`]" class="mt-2 h-1 overflow-hidden rounded bg-gray-100 dark:bg-dark-600"><div class="h-full rounded bg-teal-500" :style="{ width: `${Math.min(100, item[`${kind}_used`] / item[`${kind}_limit`]! * 100)}%` }" /></div></div></template>
            <p v-else class="text-sm text-teal-700 dark:text-teal-400">{{ t('common.sharedQuota') }}<span v-if="shared" class="float-right tabular-nums">{{ money(shared.remaining) }}</span></p>
          </div>
          <details v-if="item.models?.length" class="mt-5 text-xs text-gray-500"><summary class="cursor-pointer">{{ t('contributions.models') }} · {{ item.models.length }}</summary><div class="mt-3 flex flex-wrap gap-2"><span v-for="model in item.models" :key="model" class="break-all rounded border border-gray-200 px-2 py-1 dark:border-dark-600">{{ model }}</span></div></details>
        </article>
      </div>
      <div class="flex justify-center gap-6 py-7 text-sm"><button v-if="before" :disabled="loading" @click="before = 0; load()">{{ t('contributions.latest') }}</button><button v-if="hasMore" :disabled="loading" @click="older">{{ t('contributions.older') }}</button></div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { channelContributionsAPI as api, type ContributionQuota } from '@/api/channelContributions'
import { money, providerName, statusLabel } from '@/components/contributions/quotaDisplay'
import type { SharedQuota } from '@/types'
const { t } = useI18n()
const items = ref<ContributionQuota[]>([]), shared = ref<SharedQuota | null>(null)
const loading = ref(false), failed = ref(false), before = ref(0), hasMore = ref(false)
let request: AbortController | undefined, timer: ReturnType<typeof setInterval> | undefined
async function load() {
  request?.abort()
  const current = new AbortController(); request = current
  loading.value = true; failed.value = false
  try { const data = await api.quotas(before.value, current.signal); if (current.signal.aborted) return; items.value = data.items.filter(item => item.status === 'active'); shared.value = data.shared_quota; hasMore.value = data.has_more }
  catch { if (!current.signal.aborted) failed.value = true }
  finally { if (!current.signal.aborted) loading.value = false }
}
function older() { const last = items.value.at(-1); if (last) { before.value = last.id; void load() } }
function refreshVisible() { if (!document.hidden && !loading.value) void load() }
onMounted(() => { void load(); timer = setInterval(refreshVisible, 30000); document.addEventListener('visibilitychange', refreshVisible) })
onBeforeUnmount(() => { request?.abort(); clearInterval(timer); document.removeEventListener('visibilitychange', refreshVisible) })
</script>
