<template>
  <Teleport to="body">
    <div class="forum-celebration" role="status" aria-live="polite">
      <div ref="stage" class="celebration-stage" aria-hidden="true" />
      <button ref="skip" class="celebration-skip" @click="emit('close')">{{ t('forum.skip') }} ×</button>
      <div class="celebration-content">
        <div class="celebration-rank">{{ t('forum.rank', { rank: message.rank }) }}</div>
        <ForumAvatar :author="message.author" class="celebration-avatar" />
        <h2>{{ message.author.username || t('leaderboard.anonymous') }}</h2>
        <p class="celebration-amount">${{ formatCostFixed(message.author.actual_cost, 4) }} <span>USD</span></p>
        <p class="celebration-message">{{ message.content }}</p>
      </div>
    </div>
  </Teleport>
</template>
<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ForumMessage } from '@/api/forum'
import { formatCostFixed } from '@/utils/format'
import ForumAvatar from './ForumAvatar.vue'

defineProps<{ message: ForumMessage }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const stage = ref<HTMLElement>()
const skip = ref<HTMLButtonElement>()
let dispose: (() => void) | undefined
let stopped = false
let finish: ReturnType<typeof setTimeout>
const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') emit('close') }

onMounted(async () => {
  document.addEventListener('keydown', escape)
  finish = setTimeout(() => emit('close'), 4200)
  // 减少动态效果偏好和无 WebGL 设备保留静态通报，不影响其他页面。
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
  try {
    const THREE = await import('three')
    if (stopped || !stage.value) return
    const renderer = new THREE.WebGLRenderer({ alpha: true, antialias: true, powerPreference: 'low-power' })
    const releases: Array<() => void> = []
    dispose = () => {
      renderer.setAnimationLoop(null)
      releases.splice(0).forEach(release => release())
      renderer.dispose(); renderer.forceContextLoss(); renderer.domElement.remove()
    }
    renderer.setPixelRatio(Math.min(window.devicePixelRatio, 1.5))
    stage.value.appendChild(renderer.domElement)
    const scene = new THREE.Scene()
    const camera = new THREE.PerspectiveCamera(55, window.innerWidth / window.innerHeight, 0.1, 100)
    camera.position.z = 13
    const group = new THREE.Group()
    scene.add(group)
    const positions = new Float32Array(900 * 3)
    for (let i = 0; i < 900; i++) {
      const angle = Math.random() * Math.PI * 2
      const radius = 4 + Math.random() * 10
      positions[i * 3] = Math.cos(angle) * radius
      positions[i * 3 + 1] = Math.sin(angle) * radius
      positions[i * 3 + 2] = (Math.random() - 0.5) * 18
    }
    const stars = new THREE.BufferGeometry()
    releases.push(() => stars.dispose())
    stars.setAttribute('position', new THREE.BufferAttribute(positions, 3))
    const starMaterial = new THREE.PointsMaterial({ color: '#7dd3fc', size: 0.035, transparent: true, opacity: 0.85, blending: THREE.AdditiveBlending, depthWrite: false })
    releases.push(() => starMaterial.dispose())
    group.add(new THREE.Points(stars, starMaterial))
    const ringGeometry = new THREE.TorusGeometry(4.4, 0.018, 8, 160)
    releases.push(() => ringGeometry.dispose())
    const ringMaterial = new THREE.MeshBasicMaterial({ color: '#f6cc77', transparent: true, opacity: 0.8 })
    releases.push(() => ringMaterial.dispose())
    const rings = [0, 1, 2].map(index => {
      const ring = new THREE.Mesh(ringGeometry, ringMaterial)
      ring.rotation.x = 0.55 + index * 0.6
      ring.rotation.y = index * 0.6
      group.add(ring)
      return ring
    })
    const resize = () => {
      camera.aspect = window.innerWidth / window.innerHeight
      camera.updateProjectionMatrix()
      renderer.setSize(window.innerWidth, window.innerHeight)
    }
    window.addEventListener('resize', resize)
    releases.push(() => window.removeEventListener('resize', resize))
    resize()
    const started = performance.now()
    renderer.setAnimationLoop(() => {
      const elapsed = (performance.now() - started) / 1000
      group.rotation.z = elapsed * 0.045
      rings.forEach((ring, index) => { ring.rotation.y = elapsed * (0.18 + index * 0.1) })
      camera.position.z = 13 - Math.min(elapsed, 3) * 0.3
      renderer.render(scene, camera)
    })
  } catch {
    // WebGL 不可用时仍展示昵称、名次与发言。
    dispose?.()
  }
})
onBeforeUnmount(() => {
  stopped = true
  clearTimeout(finish)
  document.removeEventListener('keydown', escape)
  dispose?.()
})
</script>
<style scoped>
.forum-celebration { position: fixed; inset: 0; z-index: 10000; display: grid; place-items: center; color: #f8fafc; background: radial-gradient(ellipse at center, #163763e8, #070f24f5 72%); pointer-events: none; }
.celebration-stage { position: absolute; inset: 0; overflow: hidden; }
.celebration-skip { pointer-events: auto; position: absolute; top: max(24px, env(safe-area-inset-top)); right: 24px; z-index: 2; border: 1px solid #ffffff55; border-radius: 24px; padding: 8px 18px; background: #0e2144; color: #f8fafc; }
.celebration-skip:focus-visible { outline: 2px solid #7dd3fc; outline-offset: 4px; }
.celebration-content { position: relative; width: min(620px, 86vw); text-align: center; animation: celebration-enter .5s ease-out both; }
.celebration-rank { margin-bottom: 24px; color: #f6cc77; font-size: clamp(24px, 5vw, 42px); font-weight: 800; }
.celebration-avatar { width: 80px; height: 80px; font-size: 32px; border: 2px solid #f6cc77; box-shadow: 0 0 42px #f6cc7733; }
h2 { margin: 18px 0 8px; font-size: clamp(28px, 5vw, 48px); font-weight: 700; overflow-wrap: anywhere; }
.celebration-amount { color: #f6cc77; font-size: 20px; font-variant-numeric: tabular-nums; }
.celebration-amount span { font-size: 12px; }
.celebration-message { display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; margin: 26px auto 0; max-width: 34em; font-size: 18px; line-height: 1.7; overflow-wrap: anywhere; white-space: pre-wrap; }
@keyframes celebration-enter { from { opacity: 0; transform: scale(.94) } to { opacity: 1; transform: scale(1) } }
@media (prefers-reduced-motion: reduce) { .celebration-content { animation: none; } }
</style>
