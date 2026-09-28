<template>
  <AppLayout>
    <div class="contribution-page">
      <header class="contribution-heading">
        <div class="flex items-center gap-3"><span class="contribution-mark"><Icon name="globe" size="lg" /></span><h1>{{ t('contributions.title') }}</h1></div>
        <button class="btn btn-primary" @click="openSubmission"><Icon name="plus" size="sm" class="mr-2" />{{ t('contributions.provide') }}</button>
      </header>
      <div class="contribution-toolbar">
        <div v-if="auth.isAdmin" class="contribution-tabs" role="tablist" :aria-label="t('contributions.title')">
          <button role="tab" :aria-selected="reviewMode" @click="changeMode(true)">{{ t('contributions.review') }}</button>
          <button role="tab" :aria-selected="!reviewMode" @click="changeMode(false)">{{ t('contributions.mine') }}</button>
        </div>
        <h2 v-else class="text-sm font-medium">{{ t('contributions.mine') }}</h2>
        <div class="flex items-center gap-2">
          <select v-model="status" class="input !w-auto !py-2 text-sm" :aria-label="t('contributions.all')" @change="resetList">
            <option value="">{{ t('contributions.all') }}</option><option v-for="value in statuses" :key="value" :value="value">{{ t(`contributions.${value}`) }}</option>
          </select>
          <button class="btn btn-secondary !p-2.5" :disabled="loading" :aria-label="t('common.refresh')" @click="load"><Icon name="refresh" size="sm" /></button>
        </div>
      </div>
      <p v-if="failed" role="alert" class="py-5 text-sm text-red-600">{{ t('contributions.loadFailed') }}</p>
      <p v-else-if="loading" class="contribution-empty">{{ t('common.loading') }}</p>
      <p v-else-if="!items.length" class="contribution-empty">{{ t('contributions.empty') }}</p>
      <div v-else class="contribution-list">
        <article v-for="item in items" :key="item.id" class="contribution-row" :data-contribution-id="item.id">
          <div class="contribution-source">
            <div class="flex flex-wrap items-center gap-2"><h2>{{ item.site_name }}</h2><span class="contribution-platform">{{ platformName(item.platform) }}</span></div>
            <p class="contribution-url">{{ item.base_url }}</p>
            <div class="contribution-meta"><span>{{ date(item.created_at) }}</span><span v-if="reviewMode">{{ t('contributions.contributor') }}: {{ item.contributor_name || t('leaderboard.anonymous') }}</span></div>
          </div>
          <div class="contribution-destination">
            <span class="contribution-field-label">{{ t('contributions.group') }}</span>
            <p>{{ item.new_group_name || item.requested_group_name || '—' }}</p>
            <span v-if="item.new_group_name" class="text-xs text-teal-700 dark:text-teal-400">{{ t('contributions.newGroup') }}</span>
            <p v-if="item.account_id" class="mt-1 text-xs text-gray-500">{{ t('contributions.account') }} #{{ item.account_id }} · {{ item.site_name }}</p>
          </div>
          <div class="contribution-result">
            <span class="contribution-status" :class="`status-${item.status}`"><i />{{ t(`contributions.${item.status}`) }}</span>
            <button v-if="reviewMode && item.status === 'pending'" class="contribution-review-button" @click="openReview(item)">{{ t('contributions.reviewTitle') }}<Icon name="chevronRight" size="sm" /></button>
          </div>
          <p v-if="item.review_note" class="contribution-note">{{ item.review_note }}</p>
        </article>
      </div>
      <div v-if="before || hasMore" class="flex justify-center gap-6 py-6 text-sm">
        <button v-if="before" :disabled="loading" @click="resetList">{{ t('contributions.latest') }}</button>
        <button v-if="hasMore" :disabled="loading" @click="older">{{ t('contributions.older') }}</button>
      </div>

      <BaseDialog :show="submittingOpen" :title="t('contributions.provide')" width="normal" :close-on-click-outside="!saving" :close-on-escape="!saving" @close="closeSubmission">
        <form id="contribution-form" class="space-y-5" @submit.prevent="submit">
          <div><label for="contribution-site" class="input-label">{{ t('contributions.site') }}</label><input id="contribution-site" v-model="form.site_name" class="input" required maxlength="100" autocomplete="off"></div>
          <div><label for="contribution-url" class="input-label">{{ t('contributions.url') }}</label><input id="contribution-url" v-model="form.base_url" class="input" type="url" required maxlength="2048" placeholder="https://api.example.com" autocomplete="off"></div>
          <div><label for="contribution-key" class="input-label">{{ t('contributions.key') }}</label><input id="contribution-key" v-model="form.api_key" class="input" type="password" required maxlength="4096" autocomplete="new-password" spellcheck="false"></div>
          <div class="grid gap-5 sm:grid-cols-2">
            <div><label for="contribution-platform" class="input-label">{{ t('contributions.platform') }}</label><select id="contribution-platform" v-model="form.platform" class="input" @change="groupChoice = ''"><option value="openai">OpenAI</option><option value="anthropic">Claude</option><option value="gemini">Gemini</option></select></div>
            <div><label for="contribution-group" class="input-label">{{ t('contributions.group') }}</label><select id="contribution-group" v-model="groupChoice" class="input" required :disabled="groupsLoading"><option value="" disabled>{{ t('contributions.selectGroup') }}</option><option value="new">＋ {{ t('contributions.newGroup') }}</option><option v-for="group in filteredGroups" :key="group.id" :value="String(group.id)">{{ group.name }}</option></select></div>
          </div>
          <div v-if="groupChoice === 'new'"><label for="contribution-new-group" class="input-label">{{ t('contributions.newGroupName') }}</label><input id="contribution-new-group" v-model="form.new_group_name" class="input" required maxlength="100"></div>
        </form>
        <template #footer><button class="btn btn-secondary" :disabled="saving" @click="closeSubmission">{{ t('common.cancel') }}</button><button form="contribution-form" class="btn btn-primary" :disabled="saving || groupsLoading">{{ saving ? t('common.loading') : t('contributions.submit') }}</button></template>
      </BaseDialog>

      <BaseDialog :show="!!reviewing" :title="t('contributions.reviewTitle')" :close-on-click-outside="!saving" :close-on-escape="!saving" @close="closeReview">
        <div v-if="reviewing" class="space-y-5">
          <div><h3 class="font-semibold">{{ reviewing.site_name }}</h3><p class="mt-1 break-all text-sm text-gray-500">{{ reviewing.base_url }}</p></div>
          <dl class="grid grid-cols-2 gap-4 text-sm"><div><dt class="text-gray-500">{{ t('contributions.platform') }}</dt><dd class="mt-1">{{ platformName(reviewing.platform) }}</dd></div><div><dt class="text-gray-500">{{ t('contributions.group') }}</dt><dd class="mt-1">{{ reviewing.new_group_name || reviewing.requested_group_name }}</dd></div><div class="col-span-2"><dt class="text-gray-500">{{ t('contributions.contributor') }}</dt><dd class="mt-1">{{ reviewing.contributor_name || t('leaderboard.anonymous') }}</dd></div></dl>
          <div><label for="contribution-note" class="input-label">{{ t('contributions.note') }}</label><textarea id="contribution-note" v-model="reviewNote" class="input" rows="3" maxlength="500" /></div>
        </div>
        <template #footer><button class="btn btn-secondary text-red-600" :disabled="saving" @click="review('rejected')">{{ t('contributions.reject') }}</button><button class="btn btn-primary" :disabled="saving" @click="review('approved')">{{ t('contributions.approve') }}</button></template>
      </BaseDialog>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { channelContributionsAPI as api, type ChannelContribution, type ContributionGroup, type ContributionInput, type ContributionPlatform, type ContributionStatus } from '@/api/channelContributions'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t, locale } = useI18n()
const auth = useAuthStore(), app = useAppStore()
const reviewMode = ref(auth.isAdmin)
const status = ref(auth.isAdmin ? 'pending' : '')
const statuses: ContributionStatus[] = ['pending', 'approved', 'rejected']
const items = ref<ChannelContribution[]>([]), groups = ref<ContributionGroup[]>([])
const loading = ref(false), failed = ref(false), saving = ref(false), groupsLoading = ref(false)
const submittingOpen = ref(false), reviewing = ref<ChannelContribution | null>(null), reviewNote = ref('')
const before = ref(0), hasMore = ref(false), groupChoice = ref('')
const form = reactive<ContributionInput>({ site_name: '', base_url: '', api_key: '', platform: 'openai', group_id: null, new_group_name: '' })
const filteredGroups = computed(() => groups.value.filter(group => group.platform === form.platform))
const platformName = (value: ContributionPlatform) => ({ openai: 'OpenAI', anthropic: 'Claude', gemini: 'Gemini' })[value]
const date = (value: string) => new Date(value).toLocaleString(locale.value)
let request: AbortController | undefined
function error(err: unknown) { app.showError(extractApiErrorMessage(err, t('contributions.failed'))) }
async function load() {
  request?.abort()
  const current = new AbortController(); request = current
  loading.value = true; failed.value = false
  try {
    const result = await api.list(reviewMode.value, status.value, before.value, current.signal)
    if (current.signal.aborted) return
    items.value = result.items; hasMore.value = result.has_more
  } catch { if (!current.signal.aborted) failed.value = true }
  finally { if (!current.signal.aborted) loading.value = false }
}
function resetList() { before.value = 0; void load() }
function changeMode(value: boolean) { reviewMode.value = value; status.value = value ? 'pending' : ''; resetList() }
function older() { const last = items.value.at(-1); if (last) { before.value = last.id; void load() } }
async function openSubmission() {
  submittingOpen.value = true; groupsLoading.value = true
  try { groups.value = await api.groups() } catch (err) { error(err); groups.value = [] }
  finally { groupsLoading.value = false }
}
function closeSubmission() { if (!saving.value) { submittingOpen.value = false; form.api_key = '' } }
async function submit() {
  if (saving.value) return
  if (!groupChoice.value || (groupChoice.value === 'new' && !form.new_group_name.trim())) { app.showError(t('contributions.groupRequired')); return }
  saving.value = true
  try {
    await api.submit({ ...form, group_id: groupChoice.value === 'new' ? null : Number(groupChoice.value), new_group_name: groupChoice.value === 'new' ? form.new_group_name : '' })
    form.api_key = ''; submittingOpen.value = false
    app.showSuccess(t('contributions.submitted')); changeMode(false)
  } catch (err) { error(err) }
  finally { saving.value = false }
}
function openReview(item: ChannelContribution) { reviewing.value = item; reviewNote.value = '' }
function closeReview() { if (!saving.value) reviewing.value = null }
async function review(decision: 'approved' | 'rejected') {
  if (!reviewing.value || saving.value) return
  saving.value = true
  try { await api.review(reviewing.value.id, decision, reviewNote.value); reviewing.value = null; app.showSuccess(t('contributions.reviewed')); await load() }
  catch (err) { error(err) }
  finally { saving.value = false }
}
onMounted(load)
onBeforeUnmount(() => { request?.abort(); form.api_key = '' })
</script>

<style scoped>
.contribution-page { max-width: 1140px; margin: 0 auto; }
.contribution-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 6px 0 30px; }
.contribution-heading h1 { font-size: 26px; font-weight: 650; letter-spacing: -.04em; }
.contribution-mark { display: grid; place-items: center; width: 44px; height: 44px; color: #0f766e; border: 1px solid #14b8a638; border-radius: 50%; }
.contribution-toolbar { display: flex; align-items: center; justify-content: space-between; gap: 16px; border-bottom: 1px solid #94a3b830; padding-bottom: 16px; }
.contribution-tabs { display: flex; gap: 22px; }
.contribution-tabs button { padding: 9px 0; font-size: 14px; color: #64748b; border-bottom: 2px solid transparent; }
.contribution-tabs button[aria-selected='true'] { color: #0f766e; border-color: #0d9488; }
.contribution-row { display: grid; grid-template-columns: minmax(0, 1.7fr) minmax(0, 1fr) auto; gap: 24px; align-items: center; padding: 25px 4px; border-bottom: 1px solid #94a3b824; }
.contribution-source h2 { font-size: 16px; font-weight: 600; overflow-wrap: anywhere; }
.contribution-platform { font-size: 10px; color: #64748b; border: 1px solid #94a3b833; padding: 1px 6px; border-radius: 4px; }
.contribution-url { color: #64748b; font-size: 12px; margin-top: 5px; overflow-wrap: anywhere; }
.contribution-meta { display: flex; flex-wrap: wrap; gap: 12px; font-size: 11px; color: #64748b; margin-top: 12px; }
.contribution-destination { font-size: 13px; overflow-wrap: anywhere; }
.contribution-field-label { display: block; font-size: 10px; color: #64748b; margin-bottom: 5px; }
.contribution-result { display: flex; flex-direction: column; align-items: flex-end; gap: 12px; }
.contribution-status { display: inline-flex; gap: 6px; align-items: center; font-size: 11px; white-space: nowrap; }
.contribution-status i { width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
.status-pending { color: #b45309; } .status-approved { color: #0f766e; } .status-rejected { color: #b91c1c; }
.contribution-review-button { display: inline-flex; align-items: center; gap: 4px; color: #0f766e; font-size: 12px; padding: 5px 0; }
.contribution-note { grid-column: 1/-1; padding: 10px 14px; background: #94a3b80d; border-left: 2px solid #94a3b84d; font-size: 12px; white-space: pre-wrap; overflow-wrap: anywhere; }
.contribution-empty { padding: 90px 0; text-align: center; font-size: 14px; color: #64748b; }
:global(.dark) .contribution-tabs button[aria-selected='true'], :global(.dark) .status-approved, :global(.dark) .contribution-review-button, :global(.dark) .contribution-mark { color: #5eead4; }
:global(.dark) .status-pending { color: #fbbf24; } :global(.dark) .status-rejected { color: #fca5a5; }
@media (max-width: 640px) { .contribution-heading { align-items: flex-start; } .contribution-heading h1 { font-size: 22px; } .contribution-mark { display: none; } .contribution-row { grid-template-columns: minmax(0, 1fr) auto; gap: 16px; } .contribution-destination { grid-column: 1; grid-row: 2; } .contribution-result { grid-column: 2; grid-row: 1/3; } .contribution-tabs { gap: 12px; } }
</style>
