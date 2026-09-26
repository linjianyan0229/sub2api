<template>
  <Teleport to="body">
    <div class="forum-celebration" role="status" aria-live="polite">
      <div ref="stage" class="celebration-stage" aria-hidden="true" />
      <div class="celebration-horizon" aria-hidden="true" />
      <button class="celebration-skip" @click="emit('close')">{{ t('forum.skip') }} ×</button>
      <div class="celebration-content">
        <div class="celebration-rank"><span />{{ t('forum.rank', { rank: arrival.rank }) }}<span /></div>
        <div class="celebration-portrait"><ForumAvatar :author="arrival.author" class="celebration-avatar" /></div>
        <h2>{{ arrival.author.username || t('leaderboard.anonymous') }}</h2>
        <p class="celebration-arrived"><i />{{ t('forum.arrived') }}</p>
        <p class="celebration-amount">${{ formatCostFixed(arrival.author.actual_cost, 4) }} <span>USD</span></p>
      </div>
    </div>
  </Teleport>
</template>
<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ForumArrival } from '@/api/forum'
import { formatCostFixed } from '@/utils/format'
import ForumAvatar from './ForumAvatar.vue'

defineProps<{ arrival: ForumArrival }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const stage = ref<HTMLElement>()
let dispose: (() => void) | undefined
let stopped = false
let finish: ReturnType<typeof setTimeout>
const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') emit('close') }

onMounted(async () => {
  document.addEventListener('keydown', escape)
  finish = setTimeout(() => emit('close'), 5200)
  // 减动偏好和无 WebGL 设备使用静态通报。
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return
  try {
    const THREE = await import('three')
    if (stopped || !stage.value) return
    const renderer = new THREE.WebGLRenderer({ alpha: true, antialias: false, powerPreference: 'low-power' })
    const releases: Array<() => void> = []
    dispose = () => {
      renderer.setAnimationLoop(null)
      releases.splice(0).forEach(release => release())
      renderer.dispose(); renderer.forceContextLoss(); renderer.domElement.remove()
    }
    const pixelRatio = Math.min(window.devicePixelRatio, 1.5)
    renderer.setPixelRatio(pixelRatio)
    stage.value.appendChild(renderer.domElement)
    const scene = new THREE.Scene()
    const camera = new THREE.OrthographicCamera(-20, 20, 10, -10, 0.1, 50)
    camera.position.z = 20
    const compact = window.innerWidth < 640
    const halfWidth = () => 10 * window.innerWidth / window.innerHeight
    const starCount = compact ? 220 : 520
    const positions = new Float32Array(starCount * 3)
    const sizes = new Float32Array(starCount)
    for (let i = 0; i < starCount; i++) {
      positions[i * 3] = (Math.random() - 0.5) * 64
      positions[i * 3 + 1] = (Math.random() - 0.5) * 24
      positions[i * 3 + 2] = -5
      sizes[i] = (1 + Math.random() * 2.8) * pixelRatio
    }
    const stars = new THREE.BufferGeometry()
    releases.push(() => stars.dispose())
    stars.setAttribute('position', new THREE.BufferAttribute(positions, 3))
    stars.setAttribute('aSize', new THREE.BufferAttribute(sizes, 1))
    const starMaterial = new THREE.ShaderMaterial({
      uniforms: { uTime: { value: 0 } },
      vertexShader: `attribute float aSize; varying float vLight; uniform float uTime;
        void main() {
          vLight = .45 + .3 * sin(uTime * .8 + position.x * 5.);
          gl_PointSize = aSize;
          gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.);
        }`,
      fragmentShader: `varying float vLight;
        void main() {
          float glow = pow(max(0., 1. - length(gl_PointCoord - .5) * 2.), 1.5);
          gl_FragColor = vec4(.64, .82, 1., glow * vLight);
        }`,
      transparent: true, blending: THREE.AdditiveBlending, depthWrite: false
    })
    releases.push(() => starMaterial.dispose())
    scene.add(new THREE.Points(stars, starMaterial))

    // 共用平面和材质，用着色器绘制渐隐拖尾、柔光和星芒，避免后处理开销。
    const geometry = new THREE.PlaneGeometry(1, 1)
    releases.push(() => geometry.dispose())
    const materials = ['#9fdbff', '#c3c5ff', '#f8dca3'].map(color => {
      const material = new THREE.ShaderMaterial({
        uniforms: { uColor: { value: new THREE.Color(color) } },
        vertexShader: `varying vec2 vUv;
          void main() { vUv = uv; gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.); }`,
        fragmentShader: `varying vec2 vUv; uniform vec3 uColor;
          void main() {
            float x = vUv.x;
            float y = abs(vUv.y - .5);
            float tail = pow(1. - x, 1.8) * smoothstep(.015, .1, x);
            float core = exp(-y * y / .00065) * tail;
            float halo = exp(-y * y / .022) * tail * .28;
            vec2 head = vec2((x - .075) * 7., (vUv.y - .5) * 1.3);
            float flare = exp(-dot(head, head) * 85.);
            float spike = exp(-abs(head.x) * 130. - abs(head.y) * 9.) * .45;
            float alpha = core + halo + flare + spike;
            gl_FragColor = vec4(mix(uColor, vec3(1.), min(1., flare + core * .5)), min(alpha, 1.));
          }`,
        transparent: true, blending: THREE.AdditiveBlending, depthWrite: false
      })
      releases.push(() => material.dispose())
      return material
    })
    const meteors = Array.from({ length: compact ? 30 : 58 }, (_, index) => {
      const near = index % 5 === 0
      const mesh = new THREE.Mesh(geometry, materials[index % 7 === 0 ? 2 : index % 2]!)
      mesh.scale.set(near ? 6 + Math.random() * 3 : 2 + Math.random() * 3.2, near ? .65 : .24, 1)
      mesh.rotation.z = .61
      mesh.position.z = near ? 1 : -2
      scene.add(mesh)
      return { mesh, offset: (Math.random() - .5) * (halfWidth() * 2 + 48), phase: Math.random() * 38, speed: near ? 12 : 6 + Math.random() * 4 }
    })
    const resize = () => {
      camera.left = -halfWidth(); camera.right = halfWidth()
      camera.updateProjectionMatrix()
      renderer.setSize(window.innerWidth, window.innerHeight)
    }
    window.addEventListener('resize', resize)
    releases.push(() => window.removeEventListener('resize', resize))
    resize()
    const started = performance.now()
    renderer.setAnimationLoop(() => {
      const elapsed = (performance.now() - started) / 1000
      starMaterial.uniforms.uTime!.value = elapsed
      for (const { mesh, offset, phase, speed } of meteors) {
        const y = ((phase - elapsed * speed) % 38 + 38) % 38 - 16
        mesh.position.set(offset + y / Math.tan(.61), y, mesh.position.z)
      }
      renderer.render(scene, camera)
    })
  } catch {
    // 图形初始化失败时保留可读的静态通报。
    dispose?.()
    dispose = undefined
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
.forum-celebration { position: fixed; inset: 0; z-index: 10000; display: grid; place-items: center; overflow: hidden; color: #f4f7ff; background: radial-gradient(ellipse at 75% 0%, #193356d9, transparent 60%), radial-gradient(ellipse at 15% 100%, #292448cc, transparent 60%), #040914f5; pointer-events: none; animation: sky-reveal 5.2s ease both; }
.celebration-stage { position: absolute; inset: 0; overflow: hidden; }
.celebration-horizon { position: absolute; width: 140%; height: 50%; bottom: -47%; border-radius: 50%; box-shadow: 0 -6px 90px 20px #89b9ff35, 0 -1px 0 1px #afd7ff40; }
.celebration-skip { pointer-events: auto; position: absolute; top: max(24px, env(safe-area-inset-top)); right: 24px; z-index: 2; border: 1px solid #c6dcff30; border-radius: 24px; padding: 8px 18px; background: #09142680; color: #bacbe3; font-size: 13px; backdrop-filter: blur(12px); }
.celebration-skip:focus-visible { outline: 2px solid #9fdbff; outline-offset: 4px; }
.celebration-content { position: relative; width: min(620px, 86vw); text-align: center; text-shadow: 0 2px 22px #020511; animation: arrival-enter .9s cubic-bezier(.2,.75,.2,1) both; }
.celebration-content::before { content: ''; position: absolute; inset: -60px -20px; z-index: -1; background: radial-gradient(ellipse, #080f20d9, transparent 70%); }
.celebration-rank { display: flex; align-items: center; justify-content: center; gap: 18px; margin-bottom: 30px; color: #e6d5b0; font-size: 12px; font-weight: 600; letter-spacing: .24em; }
.celebration-rank span { height: 1px; width: 46px; background: linear-gradient(90deg, transparent, #e6d5b088); }
.celebration-rank span:last-child { transform: rotate(180deg); }
.celebration-portrait { display: inline-flex; padding: 7px; border: 1px solid #aacaf132; border-radius: 50%; background: #b7d7ff08; box-shadow: 0 0 65px #8baeff18; }
.celebration-avatar { width: 74px; height: 74px; font-size: 28px; border: 1px solid #d0e3ff70; background: #183453; color: #d6eaff; }
h2 { margin: 20px 0 10px; font-size: clamp(30px, 5vw, 58px); font-weight: 650; letter-spacing: .035em; line-height: 1.2; overflow-wrap: anywhere; max-height: 3.6em; overflow: hidden; }
.celebration-arrived { display: flex; justify-content: center; align-items: center; gap: 10px; color: #b8d5ef; font-size: 13px; letter-spacing: .22em; }
.celebration-arrived i { width: 5px; height: 5px; border-radius: 50%; background: #a2ebdf; box-shadow: 0 0 12px #a2ebdf; }
.celebration-amount { display: inline-block; margin-top: 30px; padding-top: 15px; border-top: 1px solid #bdd7ff24; color: #e6d5b0; font-size: 19px; font-weight: 400; font-variant-numeric: tabular-nums; letter-spacing: .04em; }
.celebration-amount span { margin-left: 4px; font-size: 10px; color: #a9b8cd; letter-spacing: .16em; }
@keyframes arrival-enter { from { opacity: 0; transform: translateY(18px); filter: blur(8px); } to { opacity: 1; transform: translateY(0); filter: blur(0); } }
@keyframes sky-reveal { 0%,100% { opacity: 0; } 12%,86% { opacity: 1; } }
@media (max-height: 500px) { .celebration-rank { margin-bottom: 12px; } .celebration-avatar { width: 48px; height: 48px; } h2 { margin-top: 10px; } .celebration-amount { margin-top: 14px; } }
@media (prefers-reduced-motion: reduce) { .forum-celebration, .celebration-content { animation: none; } }
</style>
