<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api, formatTime, notifyError, pretty } from '../api'

const protocols = ref([])
const profiles = ref([])
const loading = ref(false)
const importing = ref(false)
const uploading = ref(false)
const testingId = ref('')
const result = ref(null)
const pointFile = ref(null)
const packageFile = ref(null)

const quick = reactive({
  protocolId:'', version:'1.0.0', name:'', vendor:'', productId:'', productName:'',
  deviceId:'', deviceName:'', host:'', port:502, unitId:1, pollIntervalSec:10,
  timeoutMs:3000, retries:1, collectorId:'central', enabled:true
})
const custom = reactive({ protocolId:'', productId:'', publish:true })
const releaseCount = computed(() => protocols.value.reduce((total, item) => total + (item.releases?.length || 0), 0))

async function load() {
  loading.value = true
  try {
    const [catalog, access] = await Promise.all([api('/api/v2/protocols'), api('/api/v2/device-access-profiles')])
    protocols.value = catalog.items || []
    profiles.value = access.items || []
  } catch (error) { notifyError(error) } finally { loading.value = false }
}

function choosePointFile(event) { pointFile.value = event.target.files?.[0] || null }
function choosePackageFile(event) { packageFile.value = event.target.files?.[0] || null }
function downloadPointTemplate() {
  const csv = '\ufeffidentifier,name,functionCode,address,addressNotation,dataType,registerCount,byteOrder,wordOrder,bit,scale,offset,unit,access,pollIntervalSec,deadband,alarmMapping,description\n' +
    'temperature,温度,03,40001,4xxxx,int16,1,big,ABCD,,0.1,0,℃,read,10,0.5,,温度测点\n' +
    'pump_running,消防泵运行,01,0,zero_based,bool,1,big,ABCD,,1,0,,read,5,0,1=PUMP_RUNNING,运行线圈\n'
  const url = URL.createObjectURL(new Blob([csv], { type:'text/csv;charset=utf-8' }))
  const anchor = document.createElement('a'); anchor.href = url; anchor.download = 'modbus-points-template.csv'; anchor.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

async function importPointTable() {
  if (!pointFile.value) return ElMessage.warning('请选择 Excel 或 CSV 点表')
  if (!quick.protocolId || !quick.version || !quick.name) return ElMessage.warning('请填写协议标识、版本和名称')
  if ((quick.productId || quick.deviceId || quick.host) && (!quick.productId || !quick.deviceId || !quick.host)) return ElMessage.warning('需要直连设备时，请同时填写产品、设备和 IP/域名')
  importing.value = true
  try {
    const body = new FormData()
    body.append('file', pointFile.value)
    for (const [key, value] of Object.entries(quick)) body.append(key, String(value ?? ''))
    result.value = await api('/api/v2/modbus-tcp/import', { method:'POST', body })
    ElMessage.success(quick.deviceId ? '点表已发布，设备采集任务已启动' : '点表与协议版本已发布')
    await load()
  } catch (error) { notifyError(error) } finally { importing.value = false }
}

async function uploadPackage() {
  if (!packageFile.value || !custom.protocolId) return ElMessage.warning('请选择协议 ZIP 并填写协议标识')
  uploading.value = true
  try {
    const body = new FormData()
    body.append('package', packageFile.value)
    body.append('publish', String(custom.publish))
    body.append('productId', custom.productId)
    result.value = await api(`/api/v2/protocols/${encodeURIComponent(custom.protocolId)}/package-releases`, { method:'POST', body })
    ElMessage.success(custom.publish ? '协议包已校验并发布，无需重启平台' : '协议包已校验并保存')
    await load()
  } catch (error) { notifyError(error) } finally { uploading.value = false }
}

async function testProfile(profile) {
  testingId.value = profile.id
  try {
    result.value = await api(`/api/v2/device-access-profiles/${encodeURIComponent(profile.id)}/test`, { method:'POST', body:'{}' })
    ElMessage.success('连接与单次采集测试通过')
    await load()
  } catch (error) { notifyError(error) } finally { testingId.value = '' }
}

function newestRelease(item) { return item.releases?.[0] || {} }
function statusText(value) { return ({ DRAFT:'草稿', VALIDATED:'已校验', PUBLISHED:'已发布', DEPRECATED:'已弃用', REVOKED:'已撤销', PENDING:'待采集', ONLINE:'在线采集', ERROR:'采集异常' })[value] || value || '—' }
function statusType(value) { return ({ PUBLISHED:'success', ONLINE:'success', ERROR:'danger', REVOKED:'danger', VALIDATED:'warning', PENDING:'info' })[value] || 'info' }

onMounted(load)
</script>

<template>
  <div class="page-toolbar">
    <el-button :loading="loading" @click="load">刷新</el-button>
    <el-tag type="success" round>协议运行时 v2</el-tag>
    <span>{{ protocols.length }} 个协议，{{ releaseCount }} 个不可变版本，{{ profiles.length }} 个设备接入实例</span>
  </div>

  <el-tabs type="border-card">
    <el-tab-pane label="点表快速接入">
      <el-alert title="上传点表即可连接 Modbus TCP 设备" description="平台内置 FC01/02/03/04 采集与解析能力。先校验地址、类型和字节序，再生成不可变协议版本、产品绑定与轮询任务。" type="info" :closable="false" show-icon />
      <el-form :model="quick" label-position="top" class="top-gap">
        <div class="form-grid">
          <el-form-item label="协议标识"><el-input v-model="quick.protocolId" placeholder="例如 building-pump-modbus" /></el-form-item>
          <el-form-item label="版本"><el-input v-model="quick.version" /></el-form-item>
          <el-form-item label="协议名称"><el-input v-model="quick.name" placeholder="例如 消防泵控制柜 Modbus" /></el-form-item>
          <el-form-item label="厂商"><el-input v-model="quick.vendor" /></el-form-item>
        </div>
        <el-form-item label="Excel / CSV 点表">
          <input type="file" accept=".xlsx,.csv" @change="choosePointFile" />
          <el-button plain class="left-gap" @click="downloadPointTemplate">下载标准模板</el-button>
          <small class="subline">至少包含名称、功能码、地址和数据类型。地址统一转换为从 0 开始的 Modbus PDU 地址。</small>
        </el-form-item>
        <el-divider content-position="left">可选：同时创建设备并立即采集</el-divider>
        <div class="form-grid">
          <el-form-item label="产品 ID"><el-input v-model="quick.productId" /></el-form-item>
          <el-form-item label="产品名称"><el-input v-model="quick.productName" /></el-form-item>
          <el-form-item label="设备 ID"><el-input v-model="quick.deviceId" /></el-form-item>
          <el-form-item label="设备名称"><el-input v-model="quick.deviceName" /></el-form-item>
          <el-form-item label="设备 IP / 域名"><el-input v-model="quick.host" placeholder="192.168.1.20" /></el-form-item>
          <el-form-item label="端口"><el-input-number v-model="quick.port" :min="1" :max="65535" /></el-form-item>
          <el-form-item label="Unit ID"><el-input-number v-model="quick.unitId" :min="0" :max="255" /></el-form-item>
          <el-form-item label="轮询周期（秒）"><el-input-number v-model="quick.pollIntervalSec" :min="1" /></el-form-item>
          <el-form-item label="超时（毫秒）"><el-input-number v-model="quick.timeoutMs" :min="100" :max="30000" :step="100" /></el-form-item>
          <el-form-item label="重试次数"><el-input-number v-model="quick.retries" :min="0" :max="5" /></el-form-item>
        </div>
        <div class="dialog-actions"><el-button type="primary" :loading="importing" @click="importPointTable">校验、发布并启用</el-button></div>
      </el-form>
    </el-tab-pane>

    <el-tab-pane label="上传自定义协议包">
      <el-alert title="版本化协议包热加载" description="上传包含 manifest.yaml、测试资料和当前平台 Worker 的 ZIP。发布版本不可覆盖；校验通过后热加载，不重启 API。" type="info" :closable="false" show-icon />
      <el-form :model="custom" label-position="top" class="top-gap">
        <div class="form-grid">
          <el-form-item label="协议标识"><el-input v-model="custom.protocolId" placeholder="必须与 manifest.yaml 的 id 一致" /></el-form-item>
          <el-form-item label="绑定已有产品（可选）"><el-input v-model="custom.productId" placeholder="发布成功后立即切换该产品" /></el-form-item>
          <el-form-item label="发布方式"><el-switch v-model="custom.publish" active-text="校验后立即发布" inactive-text="仅保存已校验版本" /></el-form-item>
        </div>
        <el-form-item label="协议包 ZIP"><input type="file" accept=".zip,application/zip" @change="choosePackageFile" /><small class="subline">立即发布时必须包含 samples/cases.json，平台会真实运行样例，全部通过后才生效。</small></el-form-item>
        <pre>manifest.yaml
schemaVersion: 1
id: vendor-fire-v2
name: 厂商消防协议
version: 1.0.0
transport: TCP
payloadFormat: hex
runtime: go-json-lines-v1
entrypoints:
  linux-amd64: bin/linux-amd64/protocol-worker
capabilities: [decode]</pre>
        <div class="dialog-actions"><el-button type="primary" :loading="uploading" @click="uploadPackage">上传并校验</el-button></div>
      </el-form>
    </el-tab-pane>

    <el-tab-pane label="协议与版本">
      <el-table v-loading="loading" :data="protocols" stripe>
        <el-table-column label="协议" min-width="230"><template #default="{ row }"><b>{{ row.definition.name }}</b><small class="subline">{{ row.definition.id }} · {{ row.definition.vendor || '通用' }}</small></template></el-table-column>
        <el-table-column label="最新版本" width="130"><template #default="{ row }">{{ newestRelease(row).version || '—' }}</template></el-table-column>
        <el-table-column label="运行方式" min-width="180"><template #default="{ row }">{{ newestRelease(row).transport || '—' }} · {{ newestRelease(row).parserType || '—' }}</template></el-table-column>
        <el-table-column label="状态" width="110"><template #default="{ row }"><el-tag :type="statusType(newestRelease(row).status)" round>{{ statusText(newestRelease(row).status) }}</el-tag></template></el-table-column>
        <el-table-column label="版本历史" min-width="240"><template #default="{ row }"><el-tag v-for="release in row.releases" :key="release.version" class="right-gap" :type="statusType(release.status)" effect="plain">{{ release.version }} · {{ statusText(release.status) }}</el-tag></template></el-table-column>
      </el-table>
    </el-tab-pane>

    <el-tab-pane label="设备采集实例">
      <el-table v-loading="loading" :data="profiles" stripe>
        <el-table-column label="设备" min-width="190"><template #default="{ row }"><b>{{ row.deviceId }}</b><small class="subline">{{ row.productId }}</small></template></el-table-column>
        <el-table-column label="协议版本" min-width="190"><template #default="{ row }">{{ row.protocolId }}@{{ row.protocolVersion }}</template></el-table-column>
        <el-table-column label="连接" min-width="170"><template #default="{ row }">{{ row.host }}:{{ row.port }} · Unit {{ row.unitId }}</template></el-table-column>
        <el-table-column label="状态" width="120"><template #default="{ row }"><el-tag :type="statusType(row.runtimeStatus)" round>{{ statusText(row.runtimeStatus) }}</el-tag></template></el-table-column>
        <el-table-column label="最近成功" min-width="170"><template #default="{ row }">{{ formatTime(row.lastSuccessAt) }}</template></el-table-column>
        <el-table-column label="最近错误" min-width="220" show-overflow-tooltip><template #default="{ row }">{{ row.lastError || '—' }}</template></el-table-column>
        <el-table-column label="操作" width="120" fixed="right"><template #default="{ row }"><el-button plain type="primary" :loading="testingId===row.id" @click="testProfile(row)">连接测试</el-button></template></el-table-column>
      </el-table>
    </el-tab-pane>
  </el-tabs>

  <el-card v-if="result" shadow="never" class="surface-card top-gap">
    <template #header><strong>最近一次操作结果</strong></template>
    <pre>{{ pretty(result) }}</pre>
  </el-card>
</template>
