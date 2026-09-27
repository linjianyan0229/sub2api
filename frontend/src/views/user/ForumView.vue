<template>
  <AppLayout>
    <div class="forum-page">
      <header class="forum-heading">
        <div class="forum-title">
          <span class="forum-emblem" aria-hidden="true"><svg viewBox="0 0 28 28" fill="none" stroke="currentColor" stroke-width="1.4"><path d="M5 6h15a3 3 0 0 1 3 3v8a3 3 0 0 1-3 3h-7l-6 4v-4H5a3 3 0 0 1-3-3V9a3 3 0 0 1 3-3Z" /><path d="M8 11h9m-9 4h5" stroke-linecap="round" /></svg></span>
          <h1>{{ t('nav.forum') }}</h1>
        </div>
        <div class="forum-heading-actions">
          <button class="forum-icon-button" :disabled="loading" :aria-label="t('common.refresh')" :title="t('common.refresh')" @click="load"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><path d="M20 7v5h-5M4 17v-5h5M5.6 8A7 7 0 0 1 18 6l2 6M4 12l2 6a7 7 0 0 0 12.4-2" stroke-linecap="round" stroke-linejoin="round" /></svg></button>
          <button v-if="auth.isAdmin" class="forum-manage-button" @click="managementOpen = true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><path d="M4 7h16M4 17h16M8 4v6m8 4v6" stroke-linecap="round" /></svg>{{ t('forum.management') }}</button>
        </div>
      </header>
      <p v-if="failed" role="alert" class="forum-error">{{ t('forum.loadFailed') }}</p>
      <section class="discussion-space" :aria-label="t('nav.forum')">
        <section v-if="page?.pinned.length" class="forum-pinned" :aria-label="t('forum.pinned')">
          <span class="pin-label">{{ t('forum.pinned') }}</span>
          <div class="pinned-content">
            <article v-for="message in page.pinned" :key="message.id" class="pinned-message">
              <div class="flex items-center justify-between gap-3"><div class="flex min-w-0 flex-wrap items-center gap-2"><strong>{{ message.author.username || t('leaderboard.anonymous') }}</strong><ForumAdminBadge v-if="message.author.is_admin" /></div><button v-if="auth.isAdmin" class="forum-action" :disabled="busy" @click="act(() => forumAPI.pin(message.id, false))">{{ t('forum.unpin') }}</button></div>
              <p class="forum-body">{{ message.content }}</p>
            </article>
          </div>
        </section>
        <div ref="history" class="forum-history" tabindex="0" :aria-label="t('forum.discussions')">
          <div v-if="page?.has_more || before" class="history-navigation">
            <button v-if="page?.has_more" :disabled="loading" @click="older">{{ t('forum.older') }}</button>
            <button v-if="before" :disabled="loading" @click="latest">{{ t('forum.latest') }}</button>
          </div>
          <p v-if="!page" class="forum-empty-label">{{ t('common.loading') }}</p>
          <div v-else-if="!page.messages.length" class="forum-empty">
            <svg viewBox="0 0 180 130" fill="none" aria-hidden="true"><path d="M42 24h90a20 20 0 0 1 20 20v36a20 20 0 0 1-20 20H84l-30 18v-18H42a20 20 0 0 1-20-20V44a20 20 0 0 1 20-20Z" stroke="currentColor" stroke-width="1"/><path d="M58 14h77a28 28 0 0 1 28 28v36M44 49h82M44 65h57" stroke="currentColor" stroke-width="1" stroke-linecap="round"/><circle cx="127" cy="90" r="5" fill="currentColor" /></svg>
            <p>{{ t('forum.empty') }}</p>
          </div>
          <div v-else class="discussion-stream">
            <article v-for="message in chronologicalMessages" :key="message.id" class="forum-message" :class="{ 'own-message': message.author.user_id === auth.user?.id }" :data-message-id="message.id">
              <ForumAvatar :author="message.author" />
              <div class="message-content">
                <div class="message-author">
                  <strong>{{ message.author.username || t('leaderboard.anonymous') }}</strong>
                  <ForumAdminBadge v-if="message.author.is_admin" />
                  <span v-if="message.rank" class="forum-rank">{{ t('forum.rank', { rank: message.rank }) }}</span>
                  <span class="message-usage">${{ formatCostFixed(message.author.actual_cost, 4) }} USD</span>
                </div>
                <p class="forum-body message-text">{{ message.content }}</p>
                <div class="message-meta">
                  <time :datetime="message.created_at">{{ date(message.created_at) }}</time>
                  <template v-if="auth.isAdmin">
                    <button class="forum-action" :disabled="busy" @click="act(() => forumAPI.pin(message.id, !message.pinned))">{{ message.pinned ? t('forum.unpin') : t('forum.pin') }}</button>
                    <button v-if="message.author.user_id && message.author.user_id !== auth.user?.id" class="forum-action" @click="muteTarget = message.author">{{ t('forum.mute') }}</button>
                  </template>
                </div>
              </div>
            </article>
          </div>
        </div>
        <footer class="forum-dock-area">
          <ForumComposer v-model="draft" :disabled="!page" :sending="sending" :muted-until="muteUntil" @send="send" />
        </footer>
      </section>
      <ForumDrawer :show="managementOpen && auth.isAdmin" :title="t('forum.management')" @close="managementOpen = false">
        <div class="forum-management space-y-6">

          <div class="space-y-3">
            <label for="global-mute" class="block text-sm font-medium">{{ t('forum.globalMute') }}</label>
            <p v-if="page?.settings.muted_until && Date.parse(page.settings.muted_until) > now" class="text-xs text-amber-600">{{ t('forum.mutedUntil', { time: date(page.settings.muted_until) }) }}</p>
            <select id="global-mute" v-model.number="globalMinutes" class="input"><option :value="10">{{ t('forum.minutes', { n: 10 }) }}</option><option :value="60">{{ t('forum.hours', { n: 1 }) }}</option><option :value="1440">{{ t('forum.days', { n: 1 }) }}</option></select>
            <div class="flex gap-2"><button class="btn btn-secondary flex-1" :disabled="busy" @click="act(() => forumAPI.settings({ mute_minutes: globalMinutes }))">{{ t('forum.mute') }}</button><button class="btn btn-secondary flex-1" :disabled="busy" @click="act(() => forumAPI.settings({ mute_minutes: 0 }))">{{ t('forum.unmute') }}</button></div>
          </div>
          <form class="space-y-3 border-t border-gray-100 pt-5 dark:border-dark-700" @submit.prevent="act(() => forumAPI.settings({ retention_days: retention }))">
            <label for="retention" class="block text-sm font-medium">{{ t('forum.retention') }}</label>
            <input id="retention" v-model.number="retention" class="input" type="number" min="0" max="3650" step="1" required>
            <p class="text-xs text-gray-500">{{ t('forum.retentionHint') }}</p>
            <button class="btn btn-secondary w-full" :disabled="busy || !page">{{ t('common.save') }}</button>
          </form>
          <div class="space-y-3 border-t border-gray-100 pt-5 dark:border-dark-700">
            <button class="forum-action" @click="loadMutes">{{ t('forum.mutedUsers') }}</button>
            <div v-for="mute in mutes" :key="mute.author.user_id" class="flex items-center justify-between gap-2 text-sm">
              <div class="min-w-0"><p class="truncate">{{ mute.author.username || t('leaderboard.anonymous') }}</p><p class="text-xs text-gray-400">{{ date(mute.muted_until) }}</p></div>
              <button class="forum-action shrink-0" :disabled="busy" @click="act(() => forumAPI.mute(mute.author.user_id, 0))">{{ t('forum.unmute') }}</button>
            </div>
            <p v-if="mutesLoaded && !mutes.length" class="text-xs text-gray-400">{{ t('common.noData') }}</p>
          </div>
          <button class="w-full border-t border-gray-100 pt-5 text-left text-sm text-red-600 dark:border-dark-700 dark:text-red-400" :disabled="busy" @click="managementOpen = false; confirmClear = true">{{ t('forum.clear') }}</button>

        </div>
      </ForumDrawer>
      <BaseDialog :show="!!muteTarget" :title="t('forum.mutePerson', { name: muteTarget?.username || t('leaderboard.anonymous') })" width="narrow" @close="muteTarget = null">
        <label for="personal-mute" class="mb-2 block text-sm">{{ t('forum.duration') }}</label>
        <input id="personal-mute" v-model.number="personalMinutes" class="input" type="number" min="1" max="525600" step="1">
        <template #footer><button class="btn btn-primary" :disabled="busy || !Number.isInteger(personalMinutes) || personalMinutes < 1 || personalMinutes > 525600" @click="mutePerson">{{ t('forum.mute') }}</button></template>
      </BaseDialog>
      <ConfirmDialog :show="confirmClear" :title="t('forum.clear')" :message="t('forum.clearConfirm')" danger @cancel="confirmClear = false" @confirm="clearAll" />
    </div>
  </AppLayout>
</template>
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import ForumAvatar from '@/components/forum/ForumAvatar.vue'
import ForumAdminBadge from '@/components/forum/ForumAdminBadge.vue'
import ForumComposer from '@/components/forum/ForumComposer.vue'
import ForumDrawer from '@/components/forum/ForumDrawer.vue'
import { forumAPI, type ForumAuthor, type ForumMute, type ForumPage } from '@/api/forum'
import { useAuthStore, useAppStore } from '@/stores'
import { formatCostFixed } from '@/utils/format'

const { t, locale } = useI18n()
const auth = useAuthStore()
const app = useAppStore()
const page = ref<ForumPage | null>(null)
const loading = ref(false)
const failed = ref(false)
const sending = ref(false)
const busy = ref(false)
const draft = ref('')
const before = ref(0)
const now = ref(Date.now())
const retention = ref(30)
const globalMinutes = ref(10)
const personalMinutes = ref(60)
const mutes = ref<ForumMute[]>([])
const mutesLoaded = ref(false)
const muteTarget = ref<ForumAuthor | null>(null)
const confirmClear = ref(false)
const managementOpen = ref(false)
const history = ref<HTMLElement>()
const chronologicalMessages = computed(() => [...(page.value?.messages || [])].reverse())
let timer: ReturnType<typeof setInterval>
let stopped = false
let controller: AbortController | undefined
const muteUntil = computed(() => {
  if (auth.isAdmin || !page.value) return null
  return [page.value.muted_until, page.value.settings.muted_until].filter((value): value is string => !!value && Date.parse(value) > now.value).sort((a, b) => Date.parse(b) - Date.parse(a))[0] || null
})
const date = (value: string) => new Date(value).toLocaleString(locale.value)
function showError(error: unknown) { app.showError((error as { message?: string })?.message || t('forum.failed')) }
async function load() {
  if (loading.value || stopped) return
  loading.value = true
  controller = new AbortController()
  try {
    const result = await forumAPI.page(before.value, controller.signal)
    if (stopped) return
    const atBottom = !page.value || !history.value || history.value.scrollHeight - history.value.clientHeight - history.value.scrollTop < 80
    if (!page.value) retention.value = result.settings.retention_days
    page.value = result
    failed.value = false
    now.value = Date.now()
    await nextTick()
    if (atBottom && !before.value && history.value) history.value.scrollTop = history.value.scrollHeight
  } catch { if (!stopped) failed.value = true }
  finally { loading.value = false }
}
async function send() {
  if (!draft.value.trim() || sending.value || muteUntil.value) return
  sending.value = true
  try { await forumAPI.send(draft.value); draft.value = ''; before.value = 0; await load() }
  catch (error) { showError(error); await load() }
  finally { sending.value = false }
}
async function act(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true
  try { await action(); await load(); if (mutesLoaded.value) await loadMutes(); app.showSuccess(t('common.saved')) }
  catch (error) { showError(error) }
  finally { busy.value = false }
}
async function loadMutes() {
  try { mutes.value = await forumAPI.mutes(); mutesLoaded.value = true }
  catch (error) { showError(error) }
}
async function mutePerson() {
  const user = muteTarget.value?.user_id
  if (!user) return
  await act(() => forumAPI.mute(user, personalMinutes.value))
  muteTarget.value = null
}
async function clearAll() { confirmClear.value = false; await act(() => forumAPI.clear()); before.value = 0; await load() }
function latest() { before.value = 0; void load() }
function older() { const last = page.value?.messages.at(-1); if (last) { before.value = last.id; void load() } }
onMounted(() => { void load(); timer = setInterval(() => { now.value = Date.now(); if (!document.hidden) void load() }, 5000) })
onBeforeUnmount(() => { stopped = true; clearInterval(timer); controller?.abort() })
</script>
<style scoped>
.forum-page { --forum-ink: #1e293b; --forum-muted: #64748b; --forum-line: #94a3b824; --forum-accent: #0f766e; display: flex; flex-direction: column; max-width: 1150px; height: calc(100dvh - 130px); min-height: 460px; margin: 0 auto; color: var(--forum-ink); }
.forum-heading { display: flex; flex: none; align-items: center; justify-content: space-between; gap: 16px; padding: 0 0 22px; }
.forum-title { display: flex; gap: 15px; align-items: center; }
.forum-title h1 { font-size: 26px; font-weight: 700; letter-spacing: -.04em; }
.forum-emblem { display: grid; place-items: center; width: 44px; height: 44px; border-radius: 15px; background: #14b8a610; color: var(--forum-accent); transform: rotate(-6deg); }
.forum-emblem svg { width: 28px; height: 28px; }
.forum-heading-actions { display: flex; align-items: center; gap: 12px; }
.forum-heading-actions svg { width: 18px; height: 18px; }
.forum-icon-button { display: grid; place-items: center; width: 36px; height: 36px; border-radius: 50%; color: var(--forum-muted); }
.forum-icon-button:hover { background: var(--forum-line); }
.forum-manage-button { display: inline-flex; align-items: center; gap: 9px; padding: 9px 13px; border: 1px solid var(--forum-line); border-radius: 12px; font-size: 12px; color: var(--forum-muted); }
.discussion-space { display: flex; flex: 1; flex-direction: column; min-height: 0; border-top: 1px solid var(--forum-line); position: relative; }
.forum-history { flex: 1; min-height: 0; overflow-y: auto; overscroll-behavior: contain; scrollbar-width: thin; scrollbar-color: #94a3b830 transparent; padding: 12px 20px 24px; }
.forum-history:focus-visible { outline: 1px solid #14b8a640; outline-offset: -1px; }
.discussion-stream { max-width: 860px; margin: 0 auto; }
.forum-message { display: flex; gap: 16px; padding: 24px 0; }
.forum-message + .forum-message { border-top: 1px solid var(--forum-line); }
.message-content { flex: 1; min-width: 0; }
.message-author { display: flex; flex-wrap: wrap; align-items: center; gap: 7px 12px; font-size: 13px; }
.message-author strong { max-width: 100%; overflow-wrap: anywhere; font-weight: 600; }
.message-usage { color: var(--forum-muted); font-size: 10px; font-variant-numeric: tabular-nums; }
.forum-body { white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.85; }
.message-text { display: inline-block; max-width: 100%; margin-top: 9px; padding: 12px 16px; border-radius: 2px 16px 16px; background: #94a3b80e; font-size: 14px; }
.own-message .message-text { background: #14b8a60d; border-left: 2px solid #14b8a63d; }
.message-meta { display: flex; flex-wrap: wrap; align-items: center; gap: 15px; margin-top: 10px; font-size: 10px; color: var(--forum-muted); }
.forum-rank { padding: 2px 6px; border: 1px solid #d9770624; border-radius: 5px; color: #b45309; background: #f59e0b0a; font-size: 10px; font-weight: 600; }
.forum-pinned { display: flex; flex: none; gap: 20px; max-height: 180px; overflow-y: auto; padding: 16px 20px; border-bottom: 1px solid var(--forum-line); background: linear-gradient(90deg,#14b8a608,transparent); }
.pin-label { flex: none; color: var(--forum-accent); font-size: 11px; margin-top: 2px; }
.pinned-content { flex: 1; min-width: 0; }
.pinned-message { font-size: 12px; }
.pinned-message + .pinned-message { margin-top: 12px; }
.pinned-message strong { font-weight: 600; }
.pinned-message .forum-body { margin-top: 4px; }
.forum-dock-area { position: relative; flex: none; z-index: 2; padding: 16px 20px 8px; }
.forum-dock-area::before { content: ''; position: absolute; left: 15%; right: 15%; top: 0; height: 1px; background: linear-gradient(90deg,transparent,#14b8a62e,transparent); }
.forum-empty { display: flex; flex-direction: column; align-items: center; justify-content: center; min-height: 260px; height: 80%; color: #0f766e55; }
.forum-empty svg { width: 154px; margin-bottom: 22px; }
.forum-empty p, .forum-empty-label { font-size: 12px; color: var(--forum-muted); }
.forum-empty-label { padding: 70px 0; text-align: center; }
.history-navigation { display: flex; justify-content: center; gap: 24px; padding: 10px; color: var(--forum-accent); font-size: 11px; }
.forum-error { padding: 0 0 12px; color: #dc2626; font-size: 12px; }
.forum-action { color: var(--forum-accent, #0f766e); font-size: 11px; font-weight: 500; }
.forum-action:disabled { opacity: .5; }
button:focus-visible { outline: 2px solid #14b8a6; outline-offset: 4px; }
.dark .forum-page { --forum-ink: #e2e8f0; --forum-muted: #8292a8; --forum-line: #94a3b816; --forum-accent: #5eead4; }
.dark .forum-action { color: #5eead4; }
.dark .forum-empty { color: #5eead440; }
.dark .forum-rank { color: #fcd68b; border-color: #fcd68b26; background: #fcd68b08; }
.dark .message-text { background: #ffffff04; }
.dark .own-message .message-text { background: #14b8a610; }
@media (max-width: 767px) { .forum-page { height: calc(100dvh - 98px); min-height: 390px; } .forum-heading { padding-bottom: 16px; } .forum-title { gap: 10px; } .forum-title h1 { font-size: 22px; } .forum-emblem { width: 36px; height: 36px; border-radius: 12px; } .forum-heading-actions { gap: 5px; } .forum-manage-button { padding: 8px; font-size: 11px; } .forum-manage-button svg { display: none; } .forum-history { padding: 8px 0 18px; } .forum-message { gap: 10px; padding: 20px 0; } .message-text { padding: 9px 11px; } .forum-dock-area { padding: 12px 0 4px; } .forum-pinned { padding: 12px 0; gap: 12px; } }
</style>
