import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ForumMessage } from '@/api/forum'

export const useForumStore = defineStore('forum', () => {
  const pinned = ref<ForumMessage[]>([])

  function setPinned(messages: ForumMessage[]) {
    pinned.value = messages
  }

  function reset() {
    pinned.value = []
  }

  return { pinned, setPinned, reset }
})
