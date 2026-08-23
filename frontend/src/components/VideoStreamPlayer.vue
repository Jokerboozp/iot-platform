<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = defineProps({
  src: { type: String, required: true },
  streamType: { type: String, default: 'native' },
  title: { type: String, default: '视频预览' }
})

const video = ref(null)
const state = ref('loading')
const message = ref('正在连接视频流…')
let hls
let loadID = 0

function destroy() {
  if (hls) { hls.destroy(); hls = null }
  if (video.value) {
    video.value.onloadeddata = null
    video.value.onerror = null
    video.value.pause()
    video.value.removeAttribute('src')
    video.value.load()
  }
}

async function play() {
  const currentLoad = ++loadID
  destroy()
  try {
    await nextTick()
    if (!video.value || !props.src) return
    state.value = 'loading'
    message.value = '正在连接视频流…'
    const type = props.streamType.toLowerCase()
    if (type === 'hls' && !video.value.canPlayType('application/vnd.apple.mpegurl')) {
      const { default:Hls } = await import('hls.js/light')
      if (!video.value || currentLoad !== loadID) return
      if (!Hls.isSupported()) {
        state.value = 'error'
        message.value = '当前浏览器不支持 HLS 播放'
        return
      }
      hls = new Hls({ enableWorker: true, lowLatencyMode: true, backBufferLength: 30 })
      hls.loadSource(props.src)
      hls.attachMedia(video.value)
      hls.on(Hls.Events.MANIFEST_PARSED, () => startPlayback())
      hls.on(Hls.Events.ERROR, (_, data) => {
        if (!data.fatal) return
        state.value = 'error'
        message.value = data.type === Hls.ErrorTypes.NETWORK_ERROR ? '视频流连接失败，请检查地址和跨域设置' : '视频流解析失败'
      })
      return
    }
    video.value.src = props.src
    video.value.onloadeddata = startPlayback
    video.value.onerror = () => {
      state.value = 'error'
      message.value = '视频加载失败，请确认流地址可由浏览器访问'
    }
  } catch {
    if (currentLoad !== loadID) return
    state.value = 'error'
    message.value = '播放器组件加载失败，请检查网络后重新连接'
  }
}

function startPlayback() {
  state.value = 'ready'
  message.value = ''
  video.value?.play().catch(() => { message.value = '点击播放按钮开始预览' })
}

watch(() => [props.src, props.streamType], play)
onMounted(play)
onBeforeUnmount(() => { loadID++; destroy() })
</script>

<template>
  <div class="stream-player" :class="`is-${state}`">
    <video ref="video" controls muted playsinline preload="metadata" :aria-label="title" />
    <div v-if="state !== 'ready' || message" class="stream-state">
      <span v-if="state === 'loading'" class="stream-spinner" />
      <strong>{{ state === 'error' ? '无法播放' : title }}</strong>
      <small>{{ message }}</small>
      <el-button v-if="state === 'error'" plain @click="play">重新连接</el-button>
    </div>
    <div class="stream-badge"><i />LIVE · {{ streamType.toUpperCase() }}</div>
  </div>
</template>

<style scoped>
.stream-player { aspect-ratio:16/9; min-height:320px; position:relative; overflow:hidden; background:#050b16; border-radius:4px; }
video { width:100%; height:100%; display:block; object-fit:contain; background:#050b16; }
.stream-state { position:absolute; inset:0; display:grid; place-content:center; justify-items:center; gap:10px; color:#fff; background:radial-gradient(circle at 50% 45%,rgba(22,119,255,.16),transparent 38%); text-align:center; pointer-events:none; }
.stream-state small { color:#91a5c0; font-size:12px; }
.stream-state .el-button { margin-top:6px; pointer-events:auto; }
.stream-spinner { width:28px; height:28px; border:2px solid #39506f; border-top-color:#4096ff; border-radius:50%; animation:spin .8s linear infinite; }
.stream-badge { position:absolute; left:14px; top:14px; display:flex; align-items:center; gap:7px; padding:5px 8px; color:#dbe9ff; background:rgba(3,12,27,.74); border:1px solid rgba(255,255,255,.12); border-radius:3px; font-size:10px; letter-spacing:.08em; backdrop-filter:blur(8px); }
.stream-badge i { width:6px; height:6px; background:#ff4d4f; border-radius:50%; box-shadow:0 0 0 3px rgba(255,77,79,.18); }
@keyframes spin { to { transform:rotate(360deg); } }
@media (max-width:640px) { .stream-player { min-height:210px; } }
</style>
