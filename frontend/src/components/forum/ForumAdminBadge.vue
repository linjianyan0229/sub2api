<template>
  <span class="inline-flex shrink-0 items-center gap-1 rounded-md border border-teal-600/20 bg-teal-500/10 px-1.5 py-0.5 text-[10px] font-semibold text-teal-700 dark:text-teal-300">
    <svg class="h-3 w-3" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" aria-hidden="true"><path d="M8 1.5 13 3.5v4c0 3-3 5.5-5 7-2-1.5-5-4-5-7v-4L8 1.5Z"/><path d="m5.5 7.5 1.7 1.7 3.3-3.4" stroke-linecap="round" stroke-linejoin="round"/></svg>
    {{ t('admin.users.roles.admin') }}
  </span>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
const { t } = useI18n()
</script>
