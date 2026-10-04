<template><ForumCelebration v-if="current" :key="current.id" :arrival="current" @close="next" /></template>
<script setup lang="ts">
import { defineAsyncComponent, onBeforeUnmount, onMounted, ref } from 'vue'
import { forumAPI, type ForumArrival } from '@/api/forum'
import { useForumStore } from '@/stores/forum'
const ForumCelebration = defineAsyncComponent(() => import('./ForumCelebration.vue'))
const forum = useForumStore()
const current = ref<ForumArrival | null>(null)
const queue: ForumArrival[] = []
let cursor: number | undefined
let registered = false
let timer: ReturnType<typeof setTimeout>
let stopped = false
let controller: AbortController | undefined

function next() {
  current.value = null
  while (queue.length) {
    const arrival = queue.shift()!
    if (Date.now() - Date.parse(arrival.created_at) < 30000) { current.value = arrival; break }
  }
}
async function poll() {
  if (stopped) return
  if (document.hidden) { timer = setTimeout(poll, 5000); return }
  controller = new AbortController()
  try {
    // 先建立游标，再登记上线，自己也能看到本次通报；刷新不回放旧事件。
    if (cursor === undefined) {
      const initial = await forumAPI.events(undefined, controller.signal)
      if (stopped) return
      cursor = initial.cursor
      forum.setPinned(initial.pinned ?? [])
    }
    if (!registered) {
      await forumAPI.arrive(controller.signal)
      if (stopped) return
      registered = true
    }
    const result = await forumAPI.events(cursor, controller.signal)
    if (stopped) return
    forum.setPinned(result.pinned ?? [])
    // 游标只前进，重复响应和网络重试不能让同一事件再次入队。
    const previous = cursor
    const latest = Math.max(cursor, result.cursor)
    cursor = latest
    if (document.hidden) return
    const batch = new Set<number>()
    for (const event of result.arrivals) {
      if (event.id > previous && event.id <= latest && !batch.has(event.id)) {
        batch.add(event.id)
        queue.push(event)
      }
    }
    if (queue.length > 8) queue.splice(0, queue.length - 8)
    if (!current.value) next()
  } catch { /* 网络恢复后重试；上线登记由服务端按登录会话去重。 */ }
  finally { if (!stopped) timer = setTimeout(poll, 5000) }
}
function visibility() { if (document.hidden) { current.value = null; queue.length = 0 } }
onMounted(() => { document.addEventListener('visibilitychange', visibility); void poll() })
onBeforeUnmount(() => { stopped = true; clearTimeout(timer); controller?.abort(); document.removeEventListener('visibilitychange', visibility) })
</script>
