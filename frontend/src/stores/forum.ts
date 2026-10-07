import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ForumMessage } from '@/api/forum'

export const useForumStore = defineStore('forum', () => {
  const pinned = ref<ForumMessage[]>([])
  const recalledIds = ref(new Set<number>())

  function setPinned(messages: ForumMessage[]) {
    pinned.value = messages.filter(message => !recalledIds.value.has(message.id))
  }

  function removeMessage(id: number) {
    // Ignore stale polling snapshots that were started before the recall completed.
    recalledIds.value.add(id)
    pinned.value = pinned.value.filter(message => message.id !== id)
  }

  function reset() {
    pinned.value = []
    recalledIds.value.clear()
  }

  return { pinned, recalledIds, setPinned, removeMessage, reset }
})
