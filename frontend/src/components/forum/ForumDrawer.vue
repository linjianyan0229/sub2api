<template>
  <Teleport to="body">
    <dialog ref="dialog" class="forum-drawer" :aria-label="title" @cancel.prevent="emit('close')" @click.self="emit('close')">
      <div class="drawer-content">
        <header><h2>{{ title }}</h2><button type="button" :aria-label="t('common.close')" @click="emit('close')">×</button></header>
        <slot />
      </div>
    </dialog>
  </Teleport>
</template>
<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
const props = defineProps<{ show: boolean; title: string }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const dialog = ref<HTMLDialogElement>()
watch(() => props.show, show => {
  if (show && !dialog.value?.open) dialog.value?.showModal()
  else if (!show && dialog.value?.open) dialog.value?.close()
}, { flush: 'post' })
onBeforeUnmount(() => dialog.value?.close())
</script>
<style scoped>
.forum-drawer { position: fixed; inset: 0 0 0 auto; margin: 0; padding: 0; width: min(370px, 92vw); height: 100dvh; max-height: 100dvh; border: 0; border-left: 1px solid #e2e8f0; background: #f8fafc; color: #0f172a; box-shadow: -20px 0 80px #0f172a12; }
.forum-drawer::backdrop { background: #0f172a55; backdrop-filter: blur(4px); }
.drawer-content { min-height: 100%; padding: 26px; }
header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 34px; }
h2 { font-weight: 700; font-size: 18px; }
header button { width: 34px; height: 34px; border-radius: 50%; font-size: 26px; color: #64748b; }
header button:hover { background: #64748b18; }
.dark .forum-drawer { background: #101c29; color: #e2e8f0; border-color: #334155; }
</style>
