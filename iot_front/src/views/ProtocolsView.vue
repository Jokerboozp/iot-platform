<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api, notifyError, parseJSON, pretty } from '../api'
import { enabledStatuses, label, parsers, tagType } from '../labels'

const items = ref([])
const testing = ref(false)
const loading = ref(false)
const dialog = ref(false)
const readonly = ref(false)
const result = ref('选择或保存一个协议包后可进行调试。')
const blank = () => ({ id:'', name:'', version:'1.0.0', protocol:'json', transport:'MQTT', payloadFormat:'json', parserType:'custom_json_parser', status:'DRAFT', config:'{}', description:'' })
const form = reactive(blank())
const sample = ref('{"data":{"temp":805,"smoke":true},"kind":"smoke","occurredAt":1710000000}')
const defaultScript = () => 'function parse(raw) {\n  return {\n    properties: raw.payload.properties || {},\n    tags: raw.payload.tags || {},\n    messageType: "PROPERTY_REPORT"\n  }\n}'
const scriptSource = ref(defaultScript())

async function load() {
  loading.value = true
  try {
    const data = await api('/api/v1/protocol-packages')
    items.value = data.items || []
  } catch (error) {
    notifyError(error)
  } finally {
    loading.value = false
  }
}

function reset() {
  Object.assign(form, blank())
  scriptSource.value = defaultScript()
  result.value = '选择或保存一个协议包后可进行调试。'
}

function openCreate() {
  reset()
  readonly.value = false
  dialog.value = true
}

function view(item) {
  Object.assign(form, { ...item, config:pretty(item.config || {}) })
  scriptSource.value = item.config?.source || defaultScript()
  readonly.value = true
  dialog.value = true
}

function edit(item) {
  Object.assign(form, { ...item, config:pretty(item.config || {}) })
  scriptSource.value = item.config?.source || defaultScript()
  readonly.value = false
  dialog.value = true
}

function debug(item) {
  edit(item)
}

function startEdit() {
  readonly.value = false
}

function parserChanged(value) {
  if (value === 'configurable_json_parser') {
    form.protocol = 'json'
    form.payloadFormat = 'json'
    form.config = pretty({ properties:{ temperature:{ path:'$.data.temp', type:'number', scale:0.1 }, smoke:'$.data.smoke' }, tags:{ deviceType:'$.kind' }, timestampPath:'$.occurredAt', timestampUnit:'s' })
    sample.value = '{"data":{"temp":805,"smoke":true},"kind":"smoke","occurredAt":1710000000}'
  } else if (value === 'configurable_hex_parser') {
    form.protocol = 'config-hex'
    form.payloadFormat = 'hex'
    form.config = pretty({ startHex:'AA', endHex:'55', checksum:'sum8', checksumStartOffset:1, fields:[{ name:'temperature', offset:1, length:2, type:'int16', endian:'little', scale:0.1 }] })
    sample.value = 'AA 20 03 00 23 55'
  } else if (value === 'javascript_sandbox_parser') {
    form.protocol = 'javascript'
    form.config = ''
    sample.value = '{"properties":{"temperature":88.5,"smoke":true},"tags":{"deviceType":"smoke"}}'
  } else if (value === 'go_protocol_parser') {
    form.protocol = 'external-go'
    form.payloadFormat = 'hex'
    form.config = pretty({ timeoutMs: 2000 })
    sample.value = 'AA 01 2A'
  }
}

async function loadScriptFile(event) {
  const file = event.target.files?.[0]
  if (!file) return
  if (file.size > 65536) {
    ElMessage.error('解析脚本不能超过 64 KiB')
    event.target.value = ''
    return
  }
  try {
    scriptSource.value = await file.text()
    ElMessage.success('解析脚本已载入')
  } catch (error) {
    notifyError(error)
  } finally {
    event.target.value = ''
  }
}

async function save() {
  try {
    const config = form.parserType === 'javascript_sandbox_parser' ? { source:scriptSource.value } : parseJSON(form.config, '配置 JSON')
    const value = { ...form, config }
    const editing = !!form.id
    const data = await api(editing ? `/api/v1/protocol-packages/${encodeURIComponent(form.id)}` : '/api/v1/protocol-packages', {
      method: editing ? 'PUT' : 'POST',
      body: JSON.stringify(value)
    })
    ElMessage.success('协议包已保存')
    await load()
    edit(items.value.find(item => item.id === data.id) || data)
  } catch (error) {
    notifyError(error)
  }
}

async function uploadArtifact(event) {
  const file = event.target.files?.[0]
  event.target.value = ''
  if (!file) return
  if (!form.id) {
    ElMessage.warning('请先保存协议包，再上传 Go 协议 Worker')
    return
  }
  if (file.size <= 0 || file.size > 64 * 1024 * 1024) {
    ElMessage.error('Go 协议 Worker 必须大于 0 且不超过 64 MiB')
    return
  }
  try {
    const body = new FormData()
    body.append('artifact', file)
    const data = await api(`/api/v1/protocol-packages/${encodeURIComponent(form.id)}/artifact`, { method:'POST', body })
    Object.assign(form, { ...data, config:pretty(data.config || {}) })
    ElMessage.success('Go 协议 Worker 已上传并校验摘要')
    await load()
  } catch (error) {
    notifyError(error)
  }
}

async function runTest() {
  if (!form.id) return ElMessage.warning('请先选择或保存协议包')
  testing.value = true
  try {
    const payload = form.payloadFormat === 'hex' ? sample.value.trim() : parseJSON(sample.value, '样本载荷')
    result.value = pretty(await api(`/api/v1/protocol-packages/${encodeURIComponent(form.id)}/test`, { method:'POST', body:JSON.stringify({ payload }) }))
  } catch (error) {
    result.value = error.message
  } finally {
    testing.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page-toolbar">
    <el-button type="primary" @click="openCreate">新建协议包</el-button>
    <el-button :loading="loading" @click="load">刷新</el-button>
    <el-tag type="success" round>可配置解析</el-tag>
    <span>共 {{ items.length }} 个协议包，解析配置和调试入口位于列表右侧。</span>
  </div>

  <el-card shadow="never" class="surface-card table-card">
    <el-table v-loading="loading" :data="items" stripe>
      <el-table-column label="协议包" min-width="240">
        <template #default="{ row }">
          <b>{{ row.name || row.id }}</b>
          <small class="subline">{{ row.id }} · 版本 {{ row.version }}</small>
        </template>
      </el-table-column>
      <el-table-column label="传输 / 协议" min-width="150">
        <template #default="{ row }"><b>{{ row.transport || '-' }}</b><small class="subline">{{ row.protocol || '-' }} / {{ row.payloadFormat || '-' }}</small></template>
      </el-table-column>
      <el-table-column label="解析器" min-width="180"><template #default="{ row }">{{ label(parsers, row.parserType, '其他解析器') }}</template></el-table-column>
      <el-table-column label="状态" width="110" align="center"><template #default="{ row }"><el-tag :type="tagType(row.status)" round>{{ label(enabledStatuses, row.status, row.status) }}</el-tag></template></el-table-column>
      <el-table-column label="说明" min-width="180" show-overflow-tooltip><template #default="{ row }">{{ row.description || '-' }}</template></el-table-column>
      <el-table-column label="操作" width="250" fixed="right" align="center">
        <template #default="{ row }">
          <div class="table-actions">
            <el-button plain type="primary" @click="view(row)">详情</el-button>
            <el-button plain type="primary" @click="edit(row)">编辑</el-button>
            <el-button plain type="primary" @click="debug(row)">调试</el-button>
          </div>
        </template>
      </el-table-column>
      <template #empty><el-empty description="暂无协议包" /></template>
    </el-table>
  </el-card>

  <el-dialog v-model="dialog" :title="readonly ? `协议包详情 · ${form.name || form.id}` : (form.id ? `编辑协议包 · ${form.name}` : '新建协议包')" width="min(980px, 96vw)">
    <el-form :model="form" label-position="top" :disabled="readonly">
      <el-alert title="协议解析可以通过配置或受限脚本扩展" description="JSON / 固定字段十六进制可直接配置；其他设备可上传纯解析 JavaScript，无需重新部署 API。脚本不能访问文件、网络或平台服务。" type="info" :closable="false" show-icon />
      <div class="form-grid top-gap">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="版本"><el-input v-model="form.version" /></el-form-item>
        <el-form-item label="协议标识"><el-input v-model="form.protocol" /></el-form-item>
        <el-form-item label="传输方式"><el-select v-model="form.transport"><el-option v-for="x in ['MQTT','HTTP','TCP','MODBUS_TCP']" :key="x" :label="x" :value="x" /></el-select></el-form-item>
        <el-form-item label="载荷格式"><el-select v-model="form.payloadFormat"><el-option label="结构化文本" value="json" /><el-option label="十六进制" value="hex" /></el-select></el-form-item>
        <el-form-item label="解析器"><el-select v-model="form.parserType" @change="parserChanged"><el-option v-for="(text,key) in parsers" :key="key" :label="text" :value="key" /></el-select></el-form-item>
      </div>
      <el-form-item label="发布状态"><el-select v-model="form.status"><el-option label="草稿" value="DRAFT" /><el-option label="已发布" value="PUBLISHED" /><el-option label="已停用" value="DISABLED" /></el-select></el-form-item>
      <el-form-item v-if="form.parserType==='javascript_sandbox_parser'" label="解析脚本">
        <input type="file" accept=".js,text/javascript" @change="loadScriptFile" />
        <el-input v-model="scriptSource" class="top-gap" type="textarea" :rows="12" />
        <small class="subline">脚本必须定义 `function parse(raw)`，最大 64 KiB；可使用 `hexToBytes()` 和 `toInt()`。</small>
      </el-form-item>
      <el-form-item v-else-if="form.parserType==='go_protocol_parser'" label="Go 协议 Worker">
        <input type="file" :disabled="readonly" @change="uploadArtifact" />
        <small class="subline">上传已编译的当前部署平台可执行文件（最大 64 MiB）。Go 源码请先在受控环境编译；平台通过独立进程 JSON Lines 契约调用，不把源码或编译器放进 API 容器。</small>
        <el-input v-model="form.config" class="top-gap" type="textarea" :rows="4" />
        <small class="subline">上传后这里会显示 artifact 路径、SHA-256 和 timeoutMs；不要手工修改 artifact.path。</small>
      </el-form-item>
      <el-form-item v-else label="配置 JSON"><el-input v-model="form.config" type="textarea" :rows="8" /><small class="subline">JSON 映射示例：`$.data.temperature`；十六进制字段使用 offset、length、type、endian、scale。</small></el-form-item>
      <el-form-item label="说明"><el-input v-model="form.description" type="textarea" :rows="2" /></el-form-item>
    </el-form>
    <el-divider content-position="left">解析调试</el-divider>
    <el-input v-model="sample" type="textarea" :rows="5" />
    <div class="dialog-actions top-gap"><el-button :loading="testing" @click="runTest">运行解析</el-button></div>
    <pre>{{ result }}</pre>
    <template #footer>
      <el-button v-if="readonly" type="primary" @click="startEdit">编辑</el-button>
      <el-button @click="dialog=false">关闭</el-button>
      <el-button v-if="!readonly" type="primary" @click="save">保存协议包</el-button>
    </template>
  </el-dialog>
</template>
