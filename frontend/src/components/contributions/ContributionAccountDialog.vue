<template>
  <BaseDialog :show="id !== null" :title="t('contributions.editAccount')" width="wide" :close-on-click-outside="!saving" :close-on-escape="!saving" @close="close">
    <p v-if="loading" class="py-10 text-center text-gray-500">{{ t('common.loading') }}</p>
    <p v-else-if="failed" role="alert" class="py-5 text-red-600">{{ t('contributions.loadFailed') }}</p>
    <form v-else id="contribution-account-form" class="space-y-5" @submit.prevent="save">
      <div><label for="account-contribution-site" class="input-label">{{ t('contributions.site') }}</label><input id="account-contribution-site" v-model="form.site_name" class="input" required maxlength="100" autocomplete="off"></div>
      <div><label for="account-contribution-url" class="input-label">{{ t('contributions.url') }}</label><input id="account-contribution-url" v-model="form.base_url" class="input" type="url" required maxlength="2048" autocomplete="off"></div>
      <div><label for="account-contribution-key" class="input-label">{{ t('contributions.key') }}</label><input id="account-contribution-key" v-model="form.api_key" class="input" type="password" maxlength="4096" autocomplete="new-password" spellcheck="false" :placeholder="t('contributions.keepKey')"></div>
      <ContributionLimits :model-value="form" prefix="account-contribution" @update:model-value="Object.assign(form, $event)" />
      <div><label class="input-label">{{ t('contributions.models') }}</label><ModelWhitelistSelector :model-value="form.models || []" :platform="platform" @update:model-value="form.models = $event" /></div>
      <label class="flex items-center gap-3 text-sm"><input id="account-contribution-status" v-model="form.show_in_status" type="checkbox" class="checkbox">{{ t('contributions.showInStatus') }}</label>
      <div v-if="form.show_in_status" class="grid gap-4 sm:grid-cols-2">
        <div><label for="account-contribution-monitor" class="input-label">{{ t('contributions.monitorModel') }}</label><input id="account-contribution-monitor" v-model="form.monitor_model" class="input" list="contribution-monitor-models" required maxlength="200"><datalist id="contribution-monitor-models"><option v-for="model in form.models" :key="model" :value="model" /></datalist></div>
        <div v-if="platform === 'openai'"><label for="account-contribution-protocol" class="input-label">{{ t('contributions.monitorProtocol') }}</label><select id="account-contribution-protocol" v-model="form.monitor_api_mode" class="input"><option value="chat_completions">Chat Completions</option><option value="responses">Responses</option></select></div>
      </div>
    </form>
    <template #footer><button class="btn btn-secondary" :disabled="saving" @click="close">{{ t('common.cancel') }}</button><button form="contribution-account-form" class="btn btn-primary" :disabled="saving || loading || failed">{{ saving ? t('common.loading') : t('common.save') }}</button></template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import { channelContributionsAPI as api, type ContributionAccountInput, type ContributionPlatform } from '@/api/channelContributions'
import { extractApiErrorMessage } from '@/utils/apiError'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import ContributionLimits from './ContributionLimits.vue'

const props = defineProps<{ id: number | null }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n(), app = useAppStore()
const form = ref<ContributionAccountInput>({ site_name: '', base_url: '', api_key: '' })
const platform = ref<ContributionPlatform>('openai')
const loading = ref(false), failed = ref(false), saving = ref(false)
let request: AbortController | undefined
watch(() => props.id, async id => {
  request?.abort()
  form.value.api_key = ''
  if (id === null) return
  const current = new AbortController(); request = current
  loading.value = true; failed.value = false
  try {
    const data = await api.account(id, current.signal)
    if (current.signal.aborted) return
    platform.value = data.platform
    form.value = { site_name: data.site_name, base_url: data.base_url, api_key: '', rpm: data.rpm, concurrency: data.concurrency, daily_limit: data.daily_limit, total_limit: data.total_limit, models: data.models || [], show_in_status: data.show_in_status || false, monitor_model: data.monitor_model || data.models?.[0] || '', monitor_api_mode: data.monitor_api_mode || 'chat_completions' }
  } catch { if (!current.signal.aborted) failed.value = true }
  finally { if (!current.signal.aborted) loading.value = false }
}, { immediate: true })
function close() { if (!saving.value) { form.value.api_key = ''; request?.abort(); emit('close') } }
watch(() => [form.value.show_in_status, form.value.models] as const, () => {
  if (form.value.show_in_status && !form.value.monitor_model) form.value.monitor_model = form.value.models?.[0] || ''
})
async function save() {
  if (props.id === null || saving.value || loading.value || failed.value) return
  saving.value = true
  try {
    await api.updateAccount(props.id, { ...form.value })
    form.value.api_key = ''
    app.showSuccess(t('contributions.accountSaved'))
    emit('saved'); emit('close')
    window.dispatchEvent(new Event('contribution-updated'))
  } catch (err) { app.showError(extractApiErrorMessage(err, t('contributions.failed'))) }
  finally { saving.value = false }
}
onBeforeUnmount(() => { request?.abort(); form.value.api_key = '' })
</script>
