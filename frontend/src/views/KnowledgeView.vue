<script setup>
import { computed, onMounted, ref } from 'vue'
import { Collection, UploadFilled } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'

import { api, formatTime, notifyError, session } from '../api'

const emit = defineEmits(['navigate'])
const uploadRef = ref(null)
const documents = ref([])
const products = ref([])
const loading = ref(false)
const uploading = ref(false)
const uploadDialog = ref(false)
const detailDialog = ref(false)
const selectedDocument = ref(null)
const selectedFile = ref(null)
const productId = ref('')
const category = ref('manual')
const tags = ref([])
const runtime = ref({ indexMode:'', persistentIndex:false })

const canUpload = computed(() => session.role === 'admin' || session.role === 'operator')
const indexedCount = computed(() => documents.value.filter(item => item.status === 'INDEXED').length)
const totalChunks = computed(() => documents.value.reduce((sum, item) => sum + Number(item.metadata?.chunks || 0), 0))
const totalSize = computed(() => documents.value.reduce((sum, item) => sum + Number(item.metadata?.size || 0), 0))

function formatBytes(value) {
  const size = Number(value || 0)
  if (size < 1024) return `${size} B`
  if (size < 1024 ** 2) return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 ** 2).toFixed(1)} MB`
}

async function load() {
  loading.value = true
  try {
    const [data, productData] = await Promise.all([api('/api/v1/knowledge/documents'), api('/api/v1/products')])
    documents.value = Array.isArray(data.items) ? data.items : []
    products.value = Array.isArray(productData.items) ? productData.items : []
    runtime.value = { indexMode:data.indexMode || '', persistentIndex:Boolean(data.persistentIndex) }
  } catch (error) {
    notifyError(error)
  } finally {
    loading.value = false
  }
}

function openUpload() {
  if (!canUpload.value) return
  uploadDialog.value = true
}

function chooseFile(file) {
  if (Number(file.size || file.raw?.size || 0) > 32 * 1024 * 1024) {
    selectedFile.value = null
    uploadRef.value?.clearFiles()
    ElMessage.error('知识库文件不能超过 32 MiB')
    return
  }
  selectedFile.value = file.raw || null
}

function removeFile() {
  selectedFile.value = null
}

function rejectExtra() {
  ElMessage.warning('每次只能上传一个知识库文件')
}

function showDocument(document) {
  selectedDocument.value = document
  detailDialog.value = true
}

async function upload() {
  if (!selectedFile.value) {
    ElMessage.warning('请先选择知识库文件')
    return
  }
  uploading.value = true
  try {
    const form = new FormData()
    form.append('file', selectedFile.value)
    if (productId.value.trim()) form.append('productId', productId.value.trim())
    if (category.value.trim()) form.append('category', category.value.trim())
    if (tags.value.length) form.append('tags', tags.value.join(','))
    const created = await api('/api/v1/knowledge/documents', { method:'POST', body:form })
    ElMessage.success(`知识库已索引：${created.filename}`)
    selectedFile.value = null
    productId.value = ''
    category.value = 'manual'
    tags.value = []
    uploadRef.value?.clearFiles()
    uploadDialog.value = false
    await load()
  } catch (error) {
    notifyError(error)
  } finally {
    uploading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="knowledge-hero">
    <div>
      <span>RAG KNOWLEDGE</span>
      <h3>知识库管理</h3>
      <p>上传消防规范、设备手册和处置 SOP，供 DeepSeek Harness 的受控知识检索工具使用。</p>
    </div>
    <div class="hero-actions">
      <el-tag :type="runtime.persistentIndex ? 'success' : 'warning'" effect="dark">{{ runtime.persistentIndex ? 'WEAVIATE 持久索引' : '本地内存索引' }}</el-tag>
      <el-button type="primary" plain @click="emit('navigate','ai')">打开 AI 工作流</el-button>
    </div>
  </section>

  <el-alert v-if="!runtime.persistentIndex" title="当前使用本地内存索引：文档原文和记录会保存，但 API 重启后检索索引需要重新建立。生产环境请启用 Weaviate。" type="warning" :closable="false" show-icon />

  <div class="knowledge-stats">
    <el-card shadow="never" class="surface-card"><span>知识文档</span><strong>{{ documents.length }}</strong><small>当前租户</small></el-card>
    <el-card shadow="never" class="surface-card"><span>已完成索引</span><strong>{{ indexedCount }}</strong><small>可供 AI 检索</small></el-card>
    <el-card shadow="never" class="surface-card"><span>内容分片</span><strong>{{ totalChunks }}</strong><small>{{ formatBytes(totalSize) }}</small></el-card>
  </div>

  <div class="page-toolbar knowledge-toolbar">
    <el-button type="primary" :disabled="!canUpload" @click="openUpload">上传知识文档</el-button>
    <el-button :loading="loading" @click="load">刷新</el-button>
    <span>共 {{ documents.length }} 份文档，详情操作位于列表右侧。</span>
  </div>

  <el-card shadow="never" class="surface-card table-card documents-card">
    <template #header><div class="card-header"><div><strong>已上传文档</strong><small>仅显示当前租户的知识库记录</small></div><el-tag effect="plain">{{ runtime.indexMode || 'INDEX' }}</el-tag></div></template>
    <el-table v-loading="loading" :data="documents" stripe>
      <el-table-column label="文档" min-width="240">
        <template #default="{ row }"><div class="document-name"><el-icon><Collection /></el-icon><span><b>{{ row.filename }}</b><small>{{ row.id }}</small></span></div></template>
      </el-table-column>
      <el-table-column label="范围" min-width="180"><template #default="{ row }"><b>{{ row.productId || '通用知识' }}</b><small class="subline">{{ row.category || '未分类' }}</small><div class="tag-line"><el-tag v-for="tag in row.tags || []" :key="tag" size="small" effect="plain">{{ tag }}</el-tag></div></template></el-table-column>
      <el-table-column label="索引" width="105" align="center"><template #default="{ row }"><el-tag :type="row.status === 'INDEXED' ? 'success' : 'warning'">{{ row.status }}</el-tag></template></el-table-column>
      <el-table-column label="分片 / 大小" width="120" align="right"><template #default="{ row }">{{ row.metadata?.chunks || 0 }}<small class="subline">{{ formatBytes(row.metadata?.size) }}</small></template></el-table-column>
      <el-table-column label="上传时间" min-width="160"><template #default="{ row }">{{ formatTime(row.createdAt) }}</template></el-table-column>
      <el-table-column label="操作" width="100" fixed="right" align="center"><template #default="{ row }"><div class="table-actions"><el-button link type="primary" @click="showDocument(row)">详情</el-button></div></template></el-table-column>
      <template #empty><el-empty description="暂无知识库文档" /></template>
    </el-table>
  </el-card>

  <el-dialog v-model="uploadDialog" title="上传知识文档" width="min(650px, 94vw)">
    <el-alert v-if="!canUpload" title="当前账号只有查看权限，上传需要管理员或运维人员角色。" type="info" :closable="false" show-icon />
    <el-upload ref="uploadRef" drag :auto-upload="false" :disabled="!canUpload || uploading" :limit="1" accept=".pdf,.docx,.pptx,.xlsx,.odt,.odp,.ods,.txt,.md,.csv,.json,.html,.htm,.xml" :on-change="chooseFile" :on-remove="removeFile" :on-exceed="rejectExtra">
      <el-icon class="upload-icon"><UploadFilled /></el-icon>
      <div class="el-upload__text">拖放文件到这里，或<em>点击选择</em></div>
      <template #tip><div class="el-upload__tip">支持 PDF、Office、OpenDocument、HTML/XML 和 UTF-8 文本；扫描版 PDF 需要先完成 OCR。</div></template>
    </el-upload>
    <el-form label-position="top" class="top-gap">
      <el-form-item label="关联产品（可选）"><el-select v-model="productId" filterable clearable :disabled="!canUpload || uploading" placeholder="不选择表示通用知识"><el-option v-for="product in products" :key="product.id" :label="`${product.name || product.id} · ${product.id}`" :value="product.id" /></el-select><small class="field-tip">产品列表来自当前租户；不选择时，文档可作为通用知识被工作流检索。</small></el-form-item>
      <div class="metadata-grid"><el-form-item label="知识分类"><el-select v-model="category" :disabled="!canUpload || uploading"><el-option label="设备手册" value="manual" /><el-option label="告警处置 SOP" value="alarm-sop" /><el-option label="运维维修" value="maintenance" /><el-option label="消防规范" value="regulation" /><el-option label="常见问题" value="faq" /></el-select></el-form-item><el-form-item label="知识标签"><el-select v-model="tags" multiple filterable allow-create default-first-option :disabled="!canUpload || uploading" placeholder="输入标签后回车" /></el-form-item></div>
    </el-form>
    <template #footer><el-button @click="uploadDialog=false">取消</el-button><el-button type="primary" :loading="uploading" :disabled="!canUpload || !selectedFile" @click="upload">上传并建立索引</el-button></template>
  </el-dialog>

  <el-dialog v-model="detailDialog" title="知识文档详情" width="min(620px, 94vw)">
    <el-descriptions v-if="selectedDocument" :column="1" border>
      <el-descriptions-item label="文件名">{{ selectedDocument.filename }}</el-descriptions-item>
      <el-descriptions-item label="文档 ID">{{ selectedDocument.id }}</el-descriptions-item>
      <el-descriptions-item label="关联产品">{{ selectedDocument.productId || '通用知识' }}</el-descriptions-item>
      <el-descriptions-item label="知识分类">{{ selectedDocument.category || '未分类' }}</el-descriptions-item>
      <el-descriptions-item label="索引状态">{{ selectedDocument.status }}</el-descriptions-item>
      <el-descriptions-item label="内容统计">{{ selectedDocument.metadata?.chunks || 0 }} 个分片 / {{ formatBytes(selectedDocument.metadata?.size) }}</el-descriptions-item>
      <el-descriptions-item label="标签">{{ (selectedDocument.tags || []).join('、') || '无' }}</el-descriptions-item>
      <el-descriptions-item label="上传时间">{{ formatTime(selectedDocument.createdAt) }}</el-descriptions-item>
    </el-descriptions>
    <template #footer><el-button @click="detailDialog=false">关闭</el-button></template>
  </el-dialog>
</template>

<style scoped>
.knowledge-hero { min-height:112px; margin-bottom:14px; padding:20px 22px; display:flex; align-items:center; justify-content:space-between; gap:20px; color:#fff; background:linear-gradient(125deg,#0d2850,#1554ad 68%,#1677ff); border-radius:6px; overflow:hidden; position:relative; }
.knowledge-hero::after { content:""; width:220px; height:220px; position:absolute; right:-70px; top:-115px; border:1px solid rgba(255,255,255,.16); border-radius:50%; box-shadow:0 0 0 42px rgba(255,255,255,.035),0 0 0 84px rgba(255,255,255,.025); }
.knowledge-hero>div { position:relative; z-index:1; }.knowledge-hero span { color:rgba(255,255,255,.68); font-size:9px; font-weight:700; letter-spacing:.16em; }.knowledge-hero h3 { margin:5px 0; font-size:22px; }.knowledge-hero p { margin:0; color:rgba(255,255,255,.72); font-size:12px; }.hero-actions { display:flex; align-items:center; gap:9px; }.hero-actions .el-button { color:#fff; background:rgba(255,255,255,.08); border-color:rgba(255,255,255,.28); }
.knowledge-stats { margin:14px 0; display:grid; grid-template-columns:repeat(3,1fr); gap:12px; }.knowledge-stats .el-card :deep(.el-card__body) { min-height:92px; display:grid; grid-template-columns:1fr auto; align-items:center; gap:2px 14px; }.knowledge-stats span,.knowledge-stats small { color:var(--muted); font-size:11px; }.knowledge-stats strong { grid-row:1/3; grid-column:2; color:#1554ad; font-size:28px; }.knowledge-stats small { grid-column:1; }
.knowledge-toolbar { margin-top:2px; }.upload-icon { color:#1677ff; font-size:38px; }.field-tip { display:block; margin-top:5px; color:var(--muted); font-size:10px; line-height:1.5; }.metadata-grid { display:grid; grid-template-columns:.8fr 1.2fr; gap:10px; }.tag-line { display:flex; flex-wrap:wrap; gap:3px; margin-top:4px; }.document-name { display:flex; align-items:center; gap:9px; }.document-name>.el-icon { width:30px; height:30px; flex:none; color:#1677ff; background:#eaf3ff; border-radius:4px; }.document-name span { min-width:0; }.document-name b { display:block; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }.document-name small { display:block; margin-top:3px; color:var(--muted); font-size:9px; word-break:break-all; }
@media (max-width:640px) { .knowledge-hero { align-items:flex-start; flex-direction:column; }.knowledge-hero p { line-height:1.6; }.hero-actions { width:100%; justify-content:space-between; }.knowledge-stats { grid-template-columns:1fr; }.documents-card { overflow:hidden; }.metadata-grid { grid-template-columns:1fr; } }
</style>
