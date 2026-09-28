<template>
  <fieldset :disabled="readonly" class="grid gap-4 sm:grid-cols-2">
    <label v-for="field in fields" :key="field.key" :for="`${prefix}-${field.key}`" class="text-sm">
      {{ t(`contributions.${field.key}`) }}
      <input :id="`${prefix}-${field.key}`" :value="modelValue[field.key] || (field.key === 'concurrency' ? 5 : '')" type="number" :min="field.min" :max="field.max" :step="field.step" class="input mt-1" :placeholder="t(field.key.endsWith('limit') && !(Number(modelValue.daily_limit) > 0 || Number(modelValue.total_limit) > 0) ? 'common.sharedQuota' : 'admin.groups.subscription.noLimit')" @input="update(field.key, $event)">
    </label>
  </fieldset>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { ContributionSettings } from '@/api/channelContributions'
const props = withDefaults(defineProps<{ modelValue: ContributionSettings; readonly?: boolean; prefix?: string }>(), { readonly: false, prefix: 'contribution' })
const emit = defineEmits<{ 'update:modelValue': [value: ContributionSettings] }>()
const { t } = useI18n()
const fields = [
  { key: 'rpm', min: 0, max: 100000, step: 1 },
  { key: 'concurrency', min: 1, max: 1000, step: 1 },
  { key: 'daily_limit', min: 0, max: 1000000000, step: 'any' },
  { key: 'total_limit', min: 0, max: 1000000000, step: 'any' },
] as const
function update(key: typeof fields[number]['key'], event: Event) {
  const value = (event.target as HTMLInputElement).value
  emit('update:modelValue', { ...props.modelValue, [key]: value === '' ? (key === 'concurrency' ? 5 : 0) : Number(value) })
}
</script>
