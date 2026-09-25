<template>
  <BaseDialog :show="!!code" :title="t('admin.redeem.invitationUsage')" width="wide" @close="emit('close')">
    <form class="mb-6 flex items-end gap-3" @submit.prevent="saveLimit">
      <div class="flex-1">
        <label for="invitation-max-uses" class="input-label">{{ t('admin.redeem.maxUses') }}</label>
        <input id="invitation-max-uses" v-model.number="maxUses" type="number" :min="Math.max(1, usedCount)" :max="2147483647" step="1" required class="input" />
      </div>
      <span class="pb-2 text-sm text-gray-500">{{ t('admin.redeem.invitationUsedCount', { count: usedCount }) }}</span>
      <button type="submit" class="btn btn-primary" :disabled="saving || loading">{{ t('common.save') }}</button>
    </form>
    <DataTable :columns="columns" :data="items" :loading="loading">
      <template #cell-username="{ row }">
        <div>{{ row.username || t('admin.redeem.userPrefix', { id: row.user_id }) }}</div>
        <div class="text-xs text-gray-500">{{ row.email }}</div>
      </template>
      <template #cell-ip="{ value }">{{ value || t('admin.redeem.unknownIP') }}</template>
      <template #cell-created_at="{ value }">{{ formatDateTime(value) }}</template>
    </DataTable>
    <Pagination :page="page" :page-size="pageSize" :total="total" @update:page="changePage" @update:page-size="changePageSize" />
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { RedeemCode, InvitationUsage } from '@/types'
import { useAppStore } from '@/stores/app'
import { formatDateTime } from '@/utils/format'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'

const props = defineProps<{ code: RedeemCode | null }>()
const emit = defineEmits<{ close: []; updated: [] }>()
const { t } = useI18n()
const appStore = useAppStore()
const maxUses = ref(1)
const usedCount = ref(0)
const items = ref<InvitationUsage[]>([])
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
const loading = ref(false)
const saving = ref(false)
let requestVersion = 0
const columns = computed(() => [
  { key: 'username', label: t('admin.redeem.columns.usedBy') },
  { key: 'ip', label: t('admin.redeem.registrationIP') },
  { key: 'created_at', label: t('admin.redeem.columns.usedAt') }
])

async function loadUsages() {
  const code = props.code
  if (!code) return
  const version = ++requestVersion
  loading.value = true
  items.value = []
  try {
    const result = await adminAPI.redeem.listInvitationUsages(code.id, page.value, pageSize.value)
    if (version !== requestVersion) return
    items.value = result.items
    total.value = result.total
    usedCount.value = Math.max(usedCount.value, result.total)
  } catch {
    if (version === requestVersion) appStore.showError(t('admin.redeem.failedToLoad'))
  } finally {
    if (version === requestVersion) loading.value = false
  }
}

watch(() => props.code, (code) => {
  ++requestVersion
  if (!code) return
  maxUses.value = code.max_uses ?? 1
  usedCount.value = code.used_count ?? 0
  page.value = 1
  total.value = 0
  void loadUsages()
}, { immediate: true })

function changePage(value: number) { page.value = value; void loadUsages() }
function changePageSize(value: number) { pageSize.value = value; page.value = 1; void loadUsages() }

async function saveLimit() {
  const code = props.code
  if (!code || saving.value) return
  if (!Number.isInteger(maxUses.value) || maxUses.value < Math.max(1, usedCount.value) || maxUses.value > 2147483647) {
    appStore.showError(t('admin.redeem.invalidMaxUses'))
    return
  }
  saving.value = true
  try {
    const updated = await adminAPI.redeem.updateInvitationLimit(code.id, maxUses.value)
    if (props.code?.id === code.id) {
      maxUses.value = updated.max_uses ?? 1
      usedCount.value = updated.used_count ?? 0
    }
    appStore.showSuccess(t('admin.redeem.invitationSaved'))
    emit('updated')
  } catch {
    appStore.showError(t('admin.redeem.invitationSaveFailed'))
  } finally {
    saving.value = false
  }
}
</script>
