<template>
  <div
    v-if="current"
    ref="root"
    class="forum-pins-shell"
    :class="{ 'pins-compact': compact, 'pins-expanded': expanded }"
    @keydown="onKeydown"
  >
    <section class="forum-pins" :aria-label="label || t('forum.pinned')">
      <button
        class="pin-capsule pin-open"
        ref="trigger"
        :aria-label="expanded ? t('forum.collapsePinned') : t('forum.readAllPinned', { count: messages.length })"
        :aria-controls="detailsId"
        :aria-expanded="expanded"
        @click="expanded = !expanded"
      >
        <span class="pins-label">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true">
            <path d="m9 3 6 0-1 6 4 4v2H6v-2l4-4-1-6Zm3 12v6" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          <span v-if="!compact">{{ label || t('forum.pinned') }}</span>
        </span>
        <span class="pins-viewport" aria-hidden="true">
          <Transition name="pin-slide">
            <span :key="current.id" class="pin-item" :data-pin-id="current.id">
              <strong v-if="!compact" class="pin-author">{{ current.author.username || t('leaderboard.anonymous') }}</strong>
              <ForumAdminBadge v-if="!compact && current.author.is_admin" />
              <span class="pin-content">{{ current.content }}</span>
            </span>
          </Transition>
        </span>
        <span class="pins-count" aria-hidden="true">{{ index + 1 }}/{{ messages.length }}</span>
        <Icon :name="expanded ? 'chevronUp' : 'chevronDown'" size="xs" class="pin-expand" aria-hidden="true" />
      </button>
    </section>
    <div v-if="expanded" :id="detailsId" class="pin-details" role="region" tabindex="0" :aria-label="t('forum.pinned')" data-pin-detail>
      <article v-for="message in messages" :key="message.id" class="pin-detail" :data-pin-detail-id="message.id">
        <div class="pin-detail-heading">
          <div class="pin-detail-author">
            <strong>{{ message.author.username || t('leaderboard.anonymous') }}</strong>
            <ForumAdminBadge v-if="message.author.is_admin" />
          </div>
          <button v-if="manageable" class="pin-unpin" :disabled="busy" @click="emit('unpin', message.id)">
            {{ t('forum.unpin') }}
          </button>
        </div>
        <p class="pin-detail-content">{{ message.content }}</p>
      </article>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, getCurrentInstance, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useDocumentVisibility } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import type { ForumMessage } from '@/api/forum'
import Icon from '@/components/icons/Icon.vue'
import ForumAdminBadge from './ForumAdminBadge.vue'

const props = defineProps<{
  messages: ForumMessage[]
  label?: string
  manageable?: boolean
  busy?: boolean
  compact?: boolean
}>()
const emit = defineEmits<{ unpin: [id: number] }>()
const { t } = useI18n()
const visibility = useDocumentVisibility()
const root = ref<HTMLElement>()
const trigger = ref<HTMLButtonElement>()
const detailsId = `forum-pin-details-${getCurrentInstance()!.uid}`
const index = ref(0)
const expanded = ref(false)
const current = computed(() => props.messages[index.value % props.messages.length])
const autoplay = computed(() => props.messages.length > 1 && !expanded.value && visibility.value === 'visible')
let timer: ReturnType<typeof setInterval> | undefined

function stopTimer() {
  clearInterval(timer)
  timer = undefined
}
function onOutsidePointer(event: PointerEvent) {
  if (expanded.value && event.target instanceof Node && !root.value?.contains(event.target)) {
    expanded.value = false
  }
}
function onKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape' || !expanded.value) return
  expanded.value = false
  event.stopPropagation()
  trigger.value?.focus()
}
watch(() => props.messages.map(message => message.id).join(','), () => {
  index.value = 0
  if (!props.messages.length) expanded.value = false
})
watch(autoplay, (enabled) => {
  stopTimer()
  if (enabled) timer = setInterval(() => { index.value = (index.value + 1) % props.messages.length }, 5000)
}, { immediate: true })
onMounted(() => document.addEventListener('pointerdown', onOutsidePointer))
onBeforeUnmount(() => {
  stopTimer()
  document.removeEventListener('pointerdown', onOutsidePointer)
})
</script>

<style scoped>
.forum-pins-shell { position: relative; min-width: 0; }
.forum-pins { --pin-ink: #0f766e; --pin-muted: #64748b; --pin-surface: #f0fdfa; --pin-border: #99f6e480; min-width: 0; }
.pin-capsule { display: flex; align-items: center; gap: 10px; width: 100%; height: 40px; padding: 0 14px; overflow: hidden; border: 1px solid var(--pin-border); border-radius: 999px; background: var(--pin-surface); text-align: left; }
.pin-capsule:hover { border-color: #14b8a670; }
.pins-label { display: flex; flex: none; align-items: center; gap: 6px; color: var(--pin-ink); font-size: 11px; font-weight: 600; white-space: nowrap; }
.pins-label svg { width: 16px; height: 16px; }
.pins-count { flex: none; color: var(--pin-muted); font-size: 10px; font-variant-numeric: tabular-nums; }
.pins-viewport { position: relative; flex: 1; min-width: 0; height: 100%; overflow: hidden; }
.pin-item { position: absolute; inset: 0; display: flex; align-items: center; gap: 8px; min-width: 0; }
.pin-author { flex: none; max-width: 112px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--pin-ink); font-size: 11px; font-weight: 600; }
.pin-content { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: #334155; font-size: 12px; }
.pin-expand { flex: none; color: var(--pin-ink); }
button:focus-visible { outline: 2px solid #14b8a6; outline-offset: -3px; }
.pins-expanded .pin-capsule { border-color: #14b8a670; }
.pin-details { margin-top: 8px; padding: 16px; max-height: min(42dvh, 360px); overflow-y: auto; overscroll-behavior: contain; border: 1px solid #99f6e480; border-radius: 18px; background: #f0fdfa; color: #334155; scrollbar-width: thin; }
.pins-compact .pin-details { position: absolute; top: 100%; left: 0; z-index: 40; width: 100%; min-width: min(320px, calc(100vw - 80px)); max-height: min(65dvh, 480px); box-shadow: 0 12px 32px #0f766e18; }
.pin-detail + .pin-detail { margin-top: 20px; padding-top: 20px; border-top: 1px solid #94a3b830; }
.pin-detail-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.pin-detail-author { display: flex; min-width: 0; flex-wrap: wrap; align-items: center; gap: 8px; overflow-wrap: anywhere; font-size: 13px; }
.pin-detail-content { margin-top: 10px; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 14px; line-height: 1.85; }
.pin-unpin { flex: none; padding: 4px 8px; border-radius: 8px; color: #0f766e; font-size: 12px; }
.pin-unpin:hover { background: #14b8a615; }
.pin-unpin:disabled { opacity: .4; }
.pin-slide-enter-active, .pin-slide-leave-active { transition: transform .4s ease, opacity .4s ease; }
.pin-slide-enter-from { opacity: 0; transform: translateY(100%); }
.pin-slide-leave-to { opacity: 0; transform: translateY(-100%); }
.dark .forum-pins { --pin-ink: #5eead4; --pin-muted: #94a3b8; --pin-surface: #102b2c; --pin-border: #2dd4bf26; }
.dark .pin-content { color: #e2e8f0; }
.dark .pin-unpin { color: #5eead4; }
.dark .pin-details { background: #102b2c; color: #e2e8f0; border-color: #2dd4bf26; }
.pins-compact .pin-capsule { height: 36px; gap: 8px; padding-inline: 10px; }
@media (max-width: 639px) { .pin-capsule { gap: 6px; padding-inline: 10px; } .pins-label > span, .pin-author { display: none; } .pins-compact .pin-capsule { gap: 5px; padding-inline: 8px; } }
@media (prefers-reduced-motion: reduce) { .pin-slide-enter-active, .pin-slide-leave-active { transition: none; } }
</style>
