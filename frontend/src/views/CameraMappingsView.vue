<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api, notifyError } from '../api'
import VideoStreamPlayer from '../components/VideoStreamPlayer.vue'

const cameras = ref([])
const loading = ref(false)
const dialogVisible = ref(false)
const editing = ref('')
const previewVisible = ref(false)
const previewLoading = ref(false)
const preview = ref(null)
const blank = () => ({
  cameraId: '', cameraName: '', videoPlatformId: '', projectId: '',
  cityCode: '', districtCode: '', building: '', floor: '', areaId: '',
  relatedDeviceIds: '', streamUrl: '', streamType: '', enabled: true
})
const camera = reactive(blank())

async function load() {
  loading.value = true
  try {
    const data = await api('/api/v1/integrations/video/cameras')
    cameras.value = data.items || []
  } catch (error) {
    notifyError(error)
  } finally {
    loading.value = false
  }
}

function open(value) {
  Object.assign(camera, blank(), value ? {
    ...value,
    relatedDeviceIds: (value.relatedDeviceIds || []).join(',')
  } : {})
  editing.value = value?.cameraId || ''
  dialogVisible.value = true
}

async function save() {
  try {
    const value = {
      ...camera,
      relatedDeviceIds: camera.relatedDeviceIds.split(',').map(item => item.trim()).filter(Boolean)
    }
    await api(
      editing.value
        ? `/api/v1/integrations/video/cameras/${encodeURIComponent(editing.value)}`
        : '/api/v1/integrations/video/cameras',
      { method: editing.value ? 'PUT' : 'POST', body: JSON.stringify(value) }
    )
    ElMessage.success('摄像头映射已保存')
    dialogVisible.value = false
    await load()
  } catch (error) {
    notifyError(error)
  }
}

async function openPreview(value) {
  previewLoading.value = true
  try {
    preview.value = await api(`/api/v1/integrations/video/cameras/${encodeURIComponent(value.cameraId)}/preview`, { method:'POST', body:'{}' })
    previewVisible.value = true
  } catch (error) {
    notifyError(error)
  } finally {
    previewLoading.value = false
  }
}

function closePreview() { preview.value = null }

async function consumeNavigationAction() {
  const raw = sessionStorage.getItem('iot:navigation-detail')
  if (!raw) return
  try {
    const detail = JSON.parse(raw)
    if (!detail.autoPreview || !detail.cameraId) return
    sessionStorage.removeItem('iot:navigation-detail')
    const target = cameras.value.find(item => item.cameraId === detail.cameraId)
    if (!target) return ElMessage.error(`未找到摄像头 ${detail.cameraId}`)
    if (!target.previewEligible) return ElMessage.error(`摄像头 ${detail.cameraId} 当前不可预览`)
    await openPreview(target)
  } catch { /* ignore invalid navigation detail */ }
}

onMounted(async()=>{await load();await consumeNavigationAction()})
</script>

<template>
  <div class="page-toolbar">
    <el-button type="primary" @click="open()">新增映射</el-button>
    <el-button @click="load">刷新</el-button>
    <span>维护摄像头与城市、建筑、区域及物联设备的关联关系</span>
  </div>

  <el-card shadow="never" class="surface-card table-card">
    <el-table v-loading="loading" :data="cameras" stripe>
      <el-table-column label="摄像头" min-width="180">
        <template #default="{ row }"><b>{{ row.cameraName }}</b><small class="subline">{{ row.cameraId }}</small></template>
      </el-table-column>
      <el-table-column label="平台 / 项目" min-width="160">
        <template #default="{ row }">{{ row.videoPlatformId || '—' }}<small class="subline">{{ row.projectId || '—' }}</small></template>
      </el-table-column>
      <el-table-column label="位置" min-width="220">
        <template #default="{ row }">{{ [row.cityCode, row.districtCode, row.building, row.floor, row.areaId].filter(Boolean).join(' / ') || '—' }}</template>
      </el-table-column>
      <el-table-column label="关联设备" min-width="160">
        <template #default="{ row }">{{ (row.relatedDeviceIds || []).join(', ') || '—' }}</template>
      </el-table-column>
      <el-table-column label="视频流" width="110" align="center">
        <template #default="{ row }"><el-tag v-if="row.streamConfigured || row.streamUrl" type="primary" effect="plain">{{ (row.streamType || 'AUTO').toUpperCase() }}</el-tag><span v-else>—</span></template>
      </el-table-column>
      <el-table-column label="状态" width="100" align="center">
        <template #default="{ row }"><el-tag :type="row.enabled ? 'success' : 'info'" round>{{ row.enabled ? '已启用' : '已停用' }}</el-tag></template>
      </el-table-column>
      <el-table-column label="操作" width="154" align="center" fixed="right">
        <template #default="{ row }"><div class="table-actions"><el-button type="primary" link :disabled="!row.previewEligible" :loading="previewLoading" @click="openPreview(row)">预览</el-button><el-button type="primary" link @click="open(row)">编辑</el-button></div></template>
      </el-table-column>
      <template #empty><el-empty description="暂无摄像头映射" /></template>
    </el-table>
  </el-card>

  <el-dialog v-model="dialogVisible" :title="editing ? '编辑摄像头映射' : '新增摄像头映射'" width="min(720px, 94vw)">
    <el-form :model="camera" label-position="top">
      <div class="form-grid">
        <el-form-item label="摄像头 ID"><el-input v-model="camera.cameraId" :disabled="!!editing" /></el-form-item>
        <el-form-item label="摄像头名称"><el-input v-model="camera.cameraName" /></el-form-item>
        <el-form-item label="视频平台 ID"><el-input v-model="camera.videoPlatformId" /></el-form-item>
        <el-form-item label="项目 ID"><el-input v-model="camera.projectId" /></el-form-item>
        <el-form-item label="城市编码"><el-input v-model="camera.cityCode" /></el-form-item>
        <el-form-item label="区县编码"><el-input v-model="camera.districtCode" /></el-form-item>
        <el-form-item label="建筑"><el-input v-model="camera.building" /></el-form-item>
        <el-form-item label="楼层"><el-input v-model="camera.floor" /></el-form-item>
      </div>
      <el-form-item label="区域 ID"><el-input v-model="camera.areaId" /></el-form-item>
      <el-form-item label="关联设备 ID（逗号分隔）"><el-input v-model="camera.relatedDeviceIds" /></el-form-item>
      <div class="form-grid">
        <el-form-item label="视频流地址"><el-input v-model="camera.streamUrl" placeholder="https://media.example/live/camera.m3u8" /></el-form-item>
        <el-form-item label="流类型"><el-select v-model="camera.streamType" placeholder="自动识别" clearable><el-option label="HLS (.m3u8)" value="hls" /><el-option label="MP4" value="mp4" /><el-option label="WebM" value="webm" /><el-option label="浏览器原生" value="native" /><el-option label="RTSP（需网关转换）" value="rtsp" /><el-option label="RTMP（需网关转换）" value="rtmp" /></el-select></el-form-item>
      </div>
      <el-alert title="浏览器可直接预览 HLS、MP4 和 WebM；流媒体 Origin 需由管理员加入服务端白名单，RTSP/RTMP 请先通过网关转换。" type="info" :closable="false" show-icon />
      <el-form-item><el-switch v-model="camera.enabled" active-text="启用该摄像头" /></el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="dialogVisible = false">取消</el-button>
      <el-button type="primary" @click="save">保存映射</el-button>
    </template>
  </el-dialog>

  <el-dialog v-model="previewVisible" :title="preview?.cameraName || '视频流预览'" width="min(960px, 94vw)" destroy-on-close @closed="closePreview">
    <VideoStreamPlayer v-if="preview" :src="preview.playbackUrl" :stream-type="preview.streamType" :title="preview.cameraName" />
    <template #footer><span class="preview-meta">{{ preview?.cameraId }} · {{ preview?.streamType?.toUpperCase() }}</span><el-button @click="previewVisible=false">关闭预览</el-button></template>
  </el-dialog>
</template>

<style scoped>
.preview-meta { margin-right:auto; color:var(--muted); font-size:12px; }
:deep(.el-dialog__footer) { display:flex; align-items:center; }
:deep(.el-alert) { margin:-4px 0 18px; }
</style>
