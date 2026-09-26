<template>
  <span class="forum-avatar" aria-hidden="true">
    <img v-if="safeUrl && !failed" :src="safeUrl" alt="" referrerpolicy="no-referrer" @error="failed = true">
    <span v-else>{{ Array.from(author.username || '?')[0] }}</span>
  </span>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { ForumAuthor } from '@/api/forum'
const props = defineProps<{ author: ForumAuthor }>()
const failed = ref(false)
const safeUrl = computed(() => {
  const url = props.author.avatar_url?.trim() || ''
  return /^(https?:\/\/|data:image\/(png|jpe?g|gif|webp);base64,|\/(?!\/))/i.test(url) ? url : ''
})
watch(safeUrl, () => { failed.value = false })
</script>
<style scoped>
.forum-avatar { display: inline-flex; flex: none; width: 2.75rem; height: 2.75rem; align-items: center; justify-content: center; overflow: hidden; border-radius: 50%; background: #dbeafe; color: #1d4ed8; font-weight: 700; }
.forum-avatar img { width: 100%; height: 100%; object-fit: cover; }
</style>
