<template>
  <div ref="dock" class="composer-dock" :class="{ expanded: opened }" @mouseleave="scheduleClose" @mouseenter="cancelClose" @keydown.esc.stop="close">
    <button ref="trigger" class="composer-trigger" type="button" data-composer-trigger :aria-expanded="opened" aria-controls="forum-compose-panel" @mouseenter="open" @focus="openOnFocus" @blur="dismissed = false" @click="focusInput">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true"><path d="m15 5 4 4M4 20l4-1 12-12a2.8 2.8 0 0 0-4-4L4 15v5Z" stroke-linejoin="round" /><path d="M12 20h8" stroke-linecap="round" /></svg>
      <span>{{ t('forum.write') }}</span>
      <span class="trigger-chevron" :class="{ rotated: opened }">⌃</span>
    </button>
    <div id="forum-compose-panel" class="composer-panel" :inert="!opened || undefined">
      <form v-if="opened" class="composer-form" @submit.prevent="emit('send')" @focusout="scheduleClose">
        <div class="composer-topline">
          <span>{{ t('forum.write') }}</span>
          <button type="button" class="composer-close" :aria-label="t('forum.collapse')" @click="close">×</button>
        </div>
        <label for="forum-content" class="sr-only">{{ t('forum.compose') }}</label>
        <textarea id="forum-content" ref="input" :value="modelValue" rows="3" maxlength="2000" :placeholder="t('forum.compose')" :disabled="disabled || sending || !!mutedUntil" @input="emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)" />
        <div class="composer-footer">
          <span>{{ mutedUntil ? t('forum.mutedUntil', { time: new Date(mutedUntil).toLocaleString(locale) }) : `${Array.from(modelValue).length} / 2000` }}</span>
          <button type="submit" class="composer-send" :disabled="disabled || sending || !!mutedUntil || !modelValue.trim()">
            {{ sending ? t('common.submitting') : t('forum.send') }}
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" aria-hidden="true"><path d="m4 12 16-8-5 16-3-7-8-1Zm8 1 8-9" stroke-linejoin="round" /></svg>
          </button>
        </div>
      </form>
    </div>
  </div>
</template>
<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
const props = defineProps<{ modelValue: string; disabled: boolean; sending: boolean; mutedUntil: string | null }>()
const emit = defineEmits<{ 'update:modelValue': [value: string]; send: [] }>()
const { t, locale } = useI18n()
const opened = ref(false)
const dock = ref<HTMLElement>()
const input = ref<HTMLTextAreaElement>()
const trigger = ref<HTMLButtonElement>()
let closeTimer: ReturnType<typeof setTimeout> | undefined
let dismissed = false
function cancelClose() { clearTimeout(closeTimer) }
function open() { cancelClose(); dismissed = false; opened.value = true }
function openOnFocus() { if (!dismissed) open() }
async function focusInput() { open(); await nextTick(); input.value?.focus() }
function close() { cancelClose(); dismissed = true; opened.value = false; trigger.value?.focus() }
function scheduleClose() {
  cancelClose()
  closeTimer = setTimeout(() => {
    if (!props.modelValue && !props.sending && !dock.value?.contains(document.activeElement) && !dock.value?.matches(':hover')) opened.value = false
  }, 180)
}
onBeforeUnmount(cancelClose)
</script>
<style scoped>
.composer-dock { position: relative; display: flex; flex-direction: column; align-items: center; gap: 0; width: min(720px, 100%); margin: 0 auto; }
.composer-trigger { order: 2; display: flex; align-items: center; justify-content: center; gap: 13px; height: 52px; min-width: 164px; padding: 0 24px; border: 1px solid #14b8a640; border-radius: 30px; color: #0f766e; background: #f0fdfaf5; box-shadow: 0 8px 30px #0f766e14, inset 0 1px 0 #fff; transition: box-shadow .25s, transform .25s, background .25s; font-weight: 600; }
.composer-trigger:hover, .composer-trigger:focus-visible { outline: none; box-shadow: 0 0 0 5px #14b8a614, 0 10px 34px #0f766e25; transform: translateY(-2px); }
.composer-trigger svg { width: 19px; height: 19px; }
.trigger-chevron { margin-left: 9px; font-size: 19px; opacity: .65; transition: transform .25s; }
.trigger-chevron.rotated { transform: rotate(180deg); }
.composer-panel { order: 1; width: 100%; max-height: 0; opacity: 0; overflow: hidden; transform: translateY(16px) scale(.98); transition: max-height .28s ease, opacity .2s, transform .28s, margin-bottom .28s; }
.expanded .composer-panel { max-height: 280px; opacity: 1; transform: none; margin-bottom: 14px; }
.composer-form { border: 1px solid #14b8a63d; border-radius: 22px; background: #fffffff5; padding: 12px 18px 16px; box-shadow: 0 6px 24px #0f766e0d; }
.composer-topline { display: flex; align-items: center; justify-content: space-between; color: #64748b; font-size: 11px; }
.composer-close { width: 28px; height: 28px; font-size: 22px; border-radius: 50%; }
.composer-close:hover { background: #64748b15; }
textarea { display: block; width: 100%; min-height: 78px; max-height: 130px; resize: none; padding: 8px 0; border: 0; outline: 0; box-shadow: none; background: transparent; color: #0f172a; font-size: 14px; line-height: 1.7; }
textarea::placeholder { color: #94a3b8; }
.composer-footer { display: flex; align-items: center; justify-content: space-between; gap: 10px; color: #94a3b8; font-size: 11px; }
.composer-send { display: inline-flex; flex: none; gap: 10px; align-items: center; padding: 9px 16px; background: #0f766e; color: white; border-radius: 13px; font-size: 13px; }
.composer-send svg { width: 16px; height: 16px; }
.composer-send:disabled { opacity: .35; cursor: not-allowed; }
button:focus-visible { outline: 2px solid #14b8a6; outline-offset: 3px; }
.dark .composer-trigger { color: #99f6e4; background: #102a30ed; border-color: #5eead447; box-shadow: 0 8px 40px #0003, inset 0 1px 0 #5eead414; }
.dark .composer-trigger:hover, .dark .composer-trigger:focus-visible { box-shadow: 0 0 0 5px #2dd4bf0d, 0 0 40px #14b8a622; }
.dark .composer-form { background: #14232df7; border-color: #5eead438; box-shadow: 0 12px 32px #0002; }
.dark textarea { color: #e2e8f0; }
.dark .composer-topline { color: #94a3b8; }
.dark .composer-send { color: #083344; background: #5eead4; }
@media (prefers-reduced-motion: reduce) { .composer-panel, .composer-trigger, .trigger-chevron { transition: none; } }
</style>
