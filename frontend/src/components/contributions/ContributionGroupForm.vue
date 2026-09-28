<template>
  <fieldset :disabled="readonly" class="group-config space-y-4">
    <legend class="mb-3 text-sm font-semibold">{{ t('contributions.groupConfig') }}</legend>
    <div><label :for="id('description')" class="input-label">{{ t('admin.groups.form.description') }}</label><textarea :id="id('description')" :value="config.description" class="input" rows="2" maxlength="4096" @input="patch({ description: value($event) })" /></div>
    <div class="grid gap-4 sm:grid-cols-2">
      <label>{{ t('admin.groups.form.rateMultiplierLabel') }}<input :id="id('rate_multiplier')" :value="config.rate_multiplier" class="input mt-1" type="number" min="0.00000001" step="any" required @input="number('rate_multiplier', $event, 1)"></label>
      <label>{{ t('admin.groups.form.rpmLimit') }}<input :id="id('rpm_limit')" :value="config.rpm_limit" class="input mt-1" type="number" min="0" step="1" @input="number('rpm_limit', $event, 0)"></label>
      <label>{{ t('admin.groups.subscription.type') }}<select :value="config.subscription_type" class="input mt-1" @change="subscription(value($event))"><option value="standard">{{ t('admin.groups.subscription.standard') }}</option><option value="subscription">{{ t('admin.groups.subscription.subscription') }}</option></select></label>
      <label class="flex items-center gap-2"><input type="checkbox" :checked="config.is_exclusive" @change="toggle('is_exclusive', $event)">{{ t('admin.groups.exclusiveFilter') }}</label>
    </div>

    <details v-if="config.subscription_type === 'subscription'" open>
      <summary>{{ t('admin.groups.subscription.title') }}</summary>
      <div class="grid gap-4 sm:grid-cols-3">
        <label v-for="field in subscriptionFields" :key="field.key">{{ t(field.label) }}<input :value="config[field.key]" class="input mt-1" type="number" min="0" step="any" :placeholder="t('admin.groups.subscription.noLimit')" @input="number(field.key, $event)"></label>
      </div>
      <label class="check"><input type="checkbox" :checked="config.peak_rate_enabled" @change="toggle('peak_rate_enabled', $event)">{{ t('admin.groups.peakRate.enable') }}</label>
      <div v-if="config.peak_rate_enabled" class="grid gap-4 sm:grid-cols-3">
        <label>{{ t('admin.groups.peakRate.peakStart') }}<input :value="config.peak_start" class="input mt-1" type="time" required @input="patch({ peak_start: value($event) })"></label>
        <label>{{ t('admin.groups.peakRate.peakEnd') }}<input :value="config.peak_end" class="input mt-1" type="time" required @input="patch({ peak_end: value($event) })"></label>
        <label>{{ t('admin.groups.peakRate.peakMultiplier') }}<input :value="config.peak_rate_multiplier ?? 1" class="input mt-1" type="number" min="0" step="any" @input="number('peak_rate_multiplier', $event, 1)"></label>
      </div>
    </details>

    <details>
      <summary>{{ t('admin.groups.modelPricing.title') }}</summary>
      <label class="check"><input type="checkbox" :checked="config.long_context_pricing_enabled" @change="toggle('long_context_pricing_enabled', $event)">{{ t('admin.groups.modelPricing.longContext') }}</label>
      <PricingEntryCard v-for="(entry, index) in pricing" :key="index" :entry="entry" :platform="platform" hide-token-intervals @update="updatePrice(index, $event)" @remove="removePrice(index)" />
      <button type="button" class="btn btn-secondary" @click="addPrice">{{ t('admin.groups.modelPricing.add') }}</button>
      <label class="check"><input type="checkbox" :checked="config.profit_control_enabled" @change="toggle('profit_control_enabled', $event)">{{ t('admin.groups.profitControl.enable') }}</label>
      <div v-if="config.profit_control_enabled" class="grid gap-4 sm:grid-cols-2">
        <label v-for="field in profitFields" :key="field.key">{{ t(field.label) }}<input :value="Math.round(Number(config[field.key] || 0) * 10000) / 100" class="input mt-1" type="number" min="0" max="99.99" step="0.01" @input="patch({ [field.key]: Number(value($event)) / 100 })"></label>
      </div>
    </details>

    <details>
      <summary>{{ t('admin.groups.modelAllowlist.title') }}</summary>
      <label class="check"><input type="checkbox" :checked="config.model_allowlist?.enabled" @change="patch({ model_allowlist: { enabled: checked($event), models: config.model_allowlist?.models || [] } })">{{ t('common.enabled') }}</label>
      <ModelWhitelistSelector v-if="config.model_allowlist?.enabled" :platform="platform" :model-value="config.model_allowlist.models" @update:model-value="patch({ model_allowlist: { enabled: true, models: $event } })" />
      <ReasoningEffortPolicyFields v-if="platform !== 'gemini'" ref="reasoning" :id-prefix="prefix" :platform="platform" :max-effort="config.max_reasoning_effort || ''" :over-limit="config.max_reasoning_effort_over_limit || 'downgrade'" :mappings="reasoningEffortMappingsToRows(config.reasoning_effort_mappings)" @update:max-effort="patch({ max_reasoning_effort: $event })" @update:over-limit="patch({ max_reasoning_effort_over_limit: $event })" @update:mappings="patch({ reasoning_effort_mappings: reasoningEffortMappingsToAPI($event) })" />
    </details>

    <details v-if="platform !== 'anthropic'">
      <summary>{{ t('admin.groups.imagePricing.title') }}</summary>
      <label class="check"><input type="checkbox" :checked="config.allow_image_generation" @change="toggle('allow_image_generation', $event)">{{ t('admin.groups.imagePricing.allowImageGeneration') }}</label>
      <template v-if="config.allow_image_generation">
        <label class="check"><input type="checkbox" :checked="config.image_rate_independent" @change="toggle('image_rate_independent', $event)">{{ t('admin.groups.imagePricing.independentMultiplier') }}</label>
        <label v-if="config.image_rate_independent">{{ t('admin.groups.imagePricing.imageMultiplier') }}<input :value="config.image_rate_multiplier ?? 1" class="input mt-1" type="number" min="0" step="any" @input="number('image_rate_multiplier', $event, 1)"></label>
        <div class="grid gap-4 sm:grid-cols-3"><label v-for="size in ['1k', '2k', '4k'] as const" :key="size">{{ size.toUpperCase() }} (USD)<input :value="config[`image_price_${size}`]" class="input mt-1" type="number" min="0" step="any" @input="number(`image_price_${size}`, $event)"></label></div>
        <template v-if="platform === 'gemini'">
          <label class="check"><input type="checkbox" :checked="config.allow_batch_image_generation" @change="toggle('allow_batch_image_generation', $event)">{{ t('admin.groups.imagePricing.allowBatchImageGeneration') }}</label>
          <div v-if="config.allow_batch_image_generation" class="grid gap-4 sm:grid-cols-2"><label v-for="field in batchFields" :key="field.key">{{ t(field.label) }}<input :value="config[field.key] ?? field.default" class="input mt-1" type="number" min="0" step="any" @input="number(field.key, $event, field.default)"></label></div>
        </template>
      </template>
    </details>

    <details>
      <summary>{{ t('contributions.accessPolicy') }}</summary>
      <label v-for="field in accessFields" :key="field.key" class="check"><input type="checkbox" :checked="Boolean(config[field.key])" @change="toggle(field.key, $event)">{{ t(field.label) }}</label>
      <template v-if="platform === 'openai'">
        <label class="check"><input type="checkbox" :checked="config.allow_messages_dispatch" @change="toggle('allow_messages_dispatch', $event)">{{ t('contributions.messagesDispatch') }}</label>
        <template v-if="config.allow_messages_dispatch">
          <label>{{ t('contributions.defaultModel') }}<input :value="config.default_mapped_model" class="input mt-1" @input="patch({ default_mapped_model: value($event) })"></label>
          <div class="grid gap-4 sm:grid-cols-3"><label v-for="family in ['opus', 'sonnet', 'haiku'] as const" :key="family">{{ family }}<input :value="config.messages_dispatch_model_config?.[`${family}_mapped_model`]" class="input mt-1" @input="patch({ messages_dispatch_model_config: { ...config.messages_dispatch_model_config, [`${family}_mapped_model`]: value($event) } })"></label></div>
          <div v-for="(row, i) in dispatchMappings" :key="i" class="flex gap-2">
            <input :value="row.claude_model" class="input" :aria-label="t('contributions.sourceModel')" @input="mapping(i, 'claude_model', value($event))"><input :value="row.target_model" class="input" :aria-label="t('contributions.targetModel')" @input="mapping(i, 'target_model', value($event))"><button type="button" class="btn btn-secondary" :aria-label="t('common.delete')" @click="removeMapping(i)">×</button>
          </div>
          <button type="button" class="btn btn-secondary" @click="addMapping">{{ t('contributions.addMapping') }}</button>
        </template>
        <label>{{ t('admin.groups.webSearchPricing.pricePerCall') }}<input :value="config.web_search_price_per_call" class="input mt-1" type="number" min="0" step="any" @input="number('web_search_price_per_call', $event)"></label>
      </template>
      <template v-if="platform === 'anthropic'">
        <label class="check"><input type="checkbox" :checked="config.claude_code_only" @change="toggle('claude_code_only', $event)">Claude Code Only</label>
        <label class="check"><input type="checkbox" :checked="config.mcp_xml_inject !== false" @change="toggle('mcp_xml_inject', $event)">{{ t('admin.groups.mcpXml.title') }}</label>
        <label v-if="config.claude_code_only">{{ t('contributions.fallbackGroup') }}<select :value="config.fallback_group_id ?? ''" class="input mt-1" @change="patch({ fallback_group_id: value($event) ? Number(value($event)) : null })"><option value="">—</option><option v-for="group in samePlatformGroups" :key="group.id" :value="group.id">{{ group.name }}</option></select></label>
      </template>
      <label v-if="platform === 'anthropic' && config.subscription_type !== 'subscription'">{{ t('contributions.invalidRequestGroup') }}<select :value="config.fallback_group_id_on_invalid_request ?? ''" class="input mt-1" @change="patch({ fallback_group_id_on_invalid_request: value($event) ? Number(value($event)) : null })"><option value="">—</option><option v-for="group in samePlatformGroups" :key="group.id" :value="group.id">{{ group.name }}</option></select></label>
    </details>

    <details>
      <summary>{{ t('contributions.accountsAndRouting') }}</summary>
      <label>{{ t('contributions.copyAccounts') }}<select :value="config.copy_accounts_from_group_ids || []" multiple class="input mt-1" @change="patch({ copy_accounts_from_group_ids: selected($event) })"><option v-for="group in samePlatformGroups" :key="group.id" :value="group.id">{{ group.name }}</option></select></label>
      <label v-if="platform === 'anthropic'" class="check"><input type="checkbox" :checked="config.model_routing_enabled" @change="toggle('model_routing_enabled', $event)">{{ t('contributions.modelRouting') }}</label>
      <template v-if="platform === 'anthropic' && config.model_routing_enabled">
        <div v-for="(row, index) in routing" :key="index" class="grid grid-cols-[1fr_1fr_auto] items-start gap-2">
          <input v-model="row.model" class="input" :aria-label="t('contributions.sourceModel')" @input="saveRouting"><select v-model="row.accounts" multiple class="input" :aria-label="t('contributions.account')" @change="saveRouting"><option v-for="account in accountOptions" :key="account.id" :value="account.id">{{ account.name }}</option></select><button type="button" class="btn btn-secondary" :aria-label="t('common.delete')" @click="routing.splice(index, 1); saveRouting()">×</button>
        </div>
        <button type="button" class="btn btn-secondary" @click="routing.push({ model: '', accounts: [] })">{{ t('contributions.addRouting') }}</button>
      </template>
    </details>
  </fieldset>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CreateGroupRequest } from '@/types'
import type { ContributionGroup, ContributionPlatform } from '@/api/channelContributions'
import ModelWhitelistSelector from '@/components/account/ModelWhitelistSelector.vue'
import PricingEntryCard from '@/components/admin/channel/PricingEntryCard.vue'
import type { PricingFormEntry } from '@/components/admin/channel/types'
import { emptyGroupPricing, groupPricingFromAPI, groupPricingToAPI } from '@/components/admin/group/groupPricing'
import ReasoningEffortPolicyFields from '@/components/admin/group/ReasoningEffortPolicyFields.vue'
import { reasoningEffortMappingsToRows, reasoningEffortMappingsToAPI } from '@/views/admin/groupsReasoningEffort'

const props = withDefaults(defineProps<{ modelValue: CreateGroupRequest; platform: ContributionPlatform; groups: ContributionGroup[]; readonly?: boolean; prefix?: string }>(), { readonly: false, prefix: 'contribution-group' })
const emit = defineEmits<{ 'update:modelValue': [value: CreateGroupRequest] }>()
const { t } = useI18n()
const config = computed(() => props.modelValue)
const patch = (change: Partial<CreateGroupRequest>) => { if (!props.readonly) emit('update:modelValue', { ...config.value, ...change }) }
const id = (key: string) => `${props.prefix}-${key}`
const value = (event: Event) => (event.target as HTMLInputElement).value
const checked = (event: Event) => (event.target as HTMLInputElement).checked
const number = (key: keyof CreateGroupRequest, event: Event, fallback: number | null = null) => patch({ [key]: value(event) === '' ? fallback : Number(value(event)) })
const toggle = (key: keyof CreateGroupRequest, event: Event) => patch({ [key]: checked(event) })
const selected = (event: Event) => Array.from((event.target as HTMLSelectElement).selectedOptions, option => Number(option.value))
const samePlatformGroups = computed(() => props.groups.filter(group => group.platform === props.platform))
const accountOptions = computed(() => [...new Map(samePlatformGroups.value.flatMap(group => group.accounts || []).map(account => [account.id, account])).values()])
const subscriptionFields = [
  { key: 'daily_limit_usd', label: 'admin.groups.subscription.dailyLimit' },
  { key: 'weekly_limit_usd', label: 'admin.groups.subscription.weeklyLimit' },
  { key: 'monthly_limit_usd', label: 'admin.groups.subscription.monthlyLimit' },
] as const
const profitFields = [{ key: 'profit_min_margin', label: 'admin.groups.profitControl.minMargin' }, { key: 'profit_safety_buffer', label: 'admin.groups.profitControl.safetyBuffer' }] as const
const batchFields = [{ key: 'batch_image_discount_multiplier', label: 'admin.groups.imagePricing.batchDiscountMultiplier', default: 0.5 }, { key: 'batch_image_hold_multiplier', label: 'admin.groups.imagePricing.batchHoldMultiplier', default: 0.6 }] as const
const accessFields = computed<{ key: keyof CreateGroupRequest; label: string }[]>(() => [
  { key: 'require_oauth_only', label: 'admin.groups.accountFilters.oauthOnly' },
  { key: 'require_privacy_set', label: 'admin.groups.accountFilters.privacySetOnly' },
  ...(props.platform === 'openai' ? [
    { key: 'allow_live' as const, label: 'contributions.allowLive' },
    { key: 'force_openai_fast' as const, label: 'contributions.forceFast' },
    { key: 'free_openai_fast' as const, label: 'contributions.freeFast' },
  ] : []),
])
function subscription(type: string) {
  patch(type === 'subscription' ? { subscription_type: 'subscription', is_exclusive: true, fallback_group_id_on_invalid_request: null } : { subscription_type: 'standard', peak_rate_enabled: false, peak_start: '', peak_end: '', peak_rate_multiplier: 1 })
}
const pricing = ref(groupPricingFromAPI(config.value.model_pricing))
function savePricing() { patch({ model_pricing: groupPricingToAPI(pricing.value, props.platform) }) }
function updatePrice(index: number, entry: PricingFormEntry) { pricing.value[index] = entry; savePricing() }
function addPrice() { pricing.value.push(emptyGroupPricing()) }
function removePrice(index: number) { pricing.value.splice(index, 1); savePricing() }
const reasoning = ref<InstanceType<typeof ReasoningEffortPolicyFields>>()
const routing = ref(Object.entries(config.value.model_routing || {}).map(([model, accounts]) => ({ model, accounts: [...accounts] })))
function saveRouting() { patch({ model_routing: Object.fromEntries(routing.value.filter(row => row.model.trim()).map(row => [row.model.trim(), [...row.accounts]])) }) }
const dispatchMappings = ref(Object.entries(config.value.messages_dispatch_model_config?.exact_model_mappings || {}).map(([claude_model, target_model]) => ({ claude_model, target_model })))
function saveMappings() { patch({ messages_dispatch_model_config: { ...config.value.messages_dispatch_model_config, exact_model_mappings: Object.fromEntries(dispatchMappings.value.filter(row => row.claude_model.trim()).map(row => [row.claude_model.trim(), row.target_model.trim()])) } }) }
function addMapping() { dispatchMappings.value.push({ claude_model: '', target_model: '' }) }
function mapping(index: number, key: 'claude_model' | 'target_model', text: string) { dispatchMappings.value[index][key] = text; saveMappings() }
function removeMapping(index: number) { dispatchMappings.value.splice(index, 1); saveMappings() }
defineExpose({ validate: () => reasoning.value?.validate() ?? true })
</script>

<style scoped>
.group-config { border-top: 1px solid #94a3b833; padding-top: 18px; font-size: 13px; }
details { border: 1px solid #94a3b833; border-radius: 12px; padding: 14px; }
summary { cursor: pointer; font-weight: 600; }
details[open] > :not(summary) { margin-top: 16px; }
.check { display: flex; align-items: center; gap: 8px; }
input[type='checkbox'] { accent-color: #0d9488; }
.group-config:disabled :deep(.cursor-pointer) { pointer-events: none; }
</style>
