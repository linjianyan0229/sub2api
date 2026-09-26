<template><ForumCelebration v-if="current" :key="current.id" :message="current" @close="next" /></template>
<script setup lang="ts">
import { defineAsyncComponent, onBeforeUnmount, onMounted, ref } from 'vue'
import { forumAPI, type ForumMessage } from '@/api/forum'
const ForumCelebration = defineAsyncComponent(() => import('./ForumCelebration.vue'))
const current = ref<ForumMessage | null>(null)
const queue: ForumMessage[] = []
let cursor: number | undefined
let timer: ReturnType<typeof setTimeout>
let stopped = false
let controller: AbortController | undefined

function next() {
  current.value = null
  while (queue.length) {
    const message = queue.shift()!
    if (Date.now() - Date.parse(message.created_at) < 30000) { current.value = message; break }
  }
}
async function poll() {
  if (stopped) return
  if (document.hidden) { timer = setTimeout(poll, 5000); return }
  controller = new AbortController()
  try {
    const result = await forumAPI.events(cursor, controller.signal)
    if (stopped) return
    cursor = result.cursor
    queue.push(...result.messages)
    // 突发发言不积压过时动画；未读发言始终可在论坛查看。
    if (queue.length > 8) queue.splice(0, queue.length - 8)
    if (!current.value) next()
  } catch { /* 网络恢复后从已有游标继续，接口只返回最近 30 秒的发言。 */ }
  finally { if (!stopped) timer = setTimeout(poll, 5000) }
}
function visibility() { if (document.hidden) { current.value = null; queue.length = 0 } }
onMounted(() => { document.addEventListener('visibilitychange', visibility); void poll() })
onBeforeUnmount(() => { stopped = true; clearTimeout(timer); controller?.abort(); document.removeEventListener('visibilitychange', visibility) })
</script>
