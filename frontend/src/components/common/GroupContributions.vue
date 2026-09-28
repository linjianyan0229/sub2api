<template>
  <div v-if="items?.length" class="mt-2 space-y-1 text-xs font-normal text-gray-500 dark:text-gray-400">
    <p v-for="item in items" :key="item.account_id" class="break-words">
      {{ item.account_name }} <span class="mx-1 text-gray-300" aria-hidden="true">/</span>
      {{ t('contributions.contributor') }}: {{ item.contributor_name || t('leaderboard.anonymous') }}
    </p>
  </div>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { GroupContribution } from '@/types'
defineProps<{ items?: GroupContribution[] }>()
const { t } = useI18n()
</script>
