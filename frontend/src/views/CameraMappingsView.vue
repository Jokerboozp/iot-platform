<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api, notifyError } from '../api'

const cameras = ref([])
const loading = ref(false)
const dialogVisible = ref(false)
const editing = ref('')
const blank = () => ({
  cameraId: '', cameraName: '', videoPlatformId: '', projectId: '',
  cityCode: '', districtCode: '', building: '', floor: '', areaId: '',
  relatedDeviceIds: '', streamUrl: '', enabled: true
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

onMounted(load)
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
      <el-table-column label="关联设备" min-width="180">
        <template #default="{ row }">{{ (row.relatedDeviceIds || []).join(', ') || '—' }}</template>
      </el-table-column>
      <el-table-column label="状态" width="100" align="center">
        <template #default="{ row }"><el-tag :type="row.enabled ? 'success' : 'info'" round>{{ row.enabled ? '已启用' : '已停用' }}</el-tag></template>
      </el-table-column>
      <el-table-column label="操作" width="92" align="center" fixed="right">
        <template #default="{ row }"><div class="table-actions"><el-button type="primary" plain @click="open(row)">编辑</el-button></div></template>
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
      <el-form-item label="视频流地址"><el-input v-model="camera.streamUrl" /></el-form-item>
      <el-form-item><el-switch v-model="camera.enabled" active-text="启用该摄像头" /></el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="dialogVisible = false">取消</el-button>
      <el-button type="primary" @click="save">保存映射</el-button>
    </template>
  </el-dialog>
</template>
