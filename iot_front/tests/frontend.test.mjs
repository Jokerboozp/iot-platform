import assert from 'node:assert/strict'
import { readdir, readFile } from 'node:fs/promises'
import { join } from 'node:path'
import test from 'node:test'

const root = new URL('..', import.meta.url)

async function sourceText(directory = new URL('src/', root)) {
  const entries = await readdir(directory, { withFileTypes: true })
  const contents = await Promise.all(entries.map(async entry => {
    const path = new URL(entry.name + (entry.isDirectory() ? '/' : ''), directory)
    return entry.isDirectory() ? sourceText(path) : readFile(path, 'utf8')
  }))
  return contents.flat().join('\n')
}

test('management controls and Chinese labels remain available', async () => {
  const source = await sourceText()
  for (const label of ['查看详情', '批量下载', '未注册设备', '一键注册', '摄像头映射', '保存规则', '火灾风险', '紧急', '活动中', '疑似离线']) {
    assert.match(source, new RegExp(label), `missing label: ${label}`)
  }
})

test('shared controls keep file pickers and text actions visibly shaped', async () => {
  const styles = await readFile(new URL('src/styles.css', root), 'utf8')
  const assistant = await readFile(new URL('src/views/ProtocolAssistantView.vue', root), 'utf8')
  const protocols = await readFile(new URL('src/views/ProtocolsView.vue', root), 'utf8')
  assert.match(styles, /\.el-button\.is-text, \.el-button\.is-link \{[^}]*border: 1px solid var\(--border\)/)
  assert.match(styles, /input\[type="file"\]::file-selector-button/)
  assert.match(styles, /\.el-button--small \{[^}]*min-height: 28px/)
  assert.match(`${assistant}\n${protocols}`, /<input type="file"/)
})

test('video preview and AI provider playground remain available', async () => {
  const source = await sourceText()
  for (const label of ['视频流预览', '浏览器可直接预览 HLS', 'Provider 测试与配置', '添加自定义 Provider', '保存到当前租户', '连接并测试插件', 'DEEPSEEK HARNESS', 'AI 工作流', '运行轨迹', '工具调用']) {
    assert.match(source, new RegExp(label), `missing feature label: ${label}`)
  }
  const packageJson = JSON.parse(await readFile(new URL('package.json', root), 'utf8'))
  assert.ok(packageJson.dependencies?.['hls.js'], 'hls.js is required for browser HLS playback')
  assert.match(source, /\.provider-select-row\s*\{[^}]*width:100%;[^}]*min-width:0;/, 'provider selector row must fill the Element Plus form content width')
})

test('knowledge management uploads files and lists tenant documents', async () => {
  const app = await readFile(new URL('src/App.vue', root), 'utf8')
  const view = await readFile(new URL('src/views/KnowledgeView.vue', root), 'utf8')
  for (const label of ['知识库管理', '上传知识文档', '上传并建立索引', '已上传文档', '打开 AI 工作流']) {
    assert.match(`${app}\n${view}`, new RegExp(label), `missing knowledge UI label: ${label}`)
  }
  assert.match(view, /api\('\/api\/v1\/knowledge\/documents'\)/)
  assert.match(view, /method:'POST', body:form/)
  assert.match(view, /new FormData\(\)/)
  assert.match(view, /persistentIndex/)
  assert.match(view, /api\('\/api\/v1\/products'\)/)
  assert.match(view, /<el-select v-model="productId" filterable clearable/)
  for (const label of ['知识分类', '知识标签', '告警处置 SOP']) assert.match(view, new RegExp(label), `missing knowledge metadata UI: ${label}`)
})

test('AI workbench uses cancellable SSE workflows and stable message keys', async () => {
  const aiView = await readFile(new URL('src/views/AiView.vue', root), 'utf8')
  const apiSource = await readFile(new URL('src/api.js', root), 'utf8')
  const sseSource = await readFile(new URL('src/sse.js', root), 'utf8')

  assert.match(aiView, /api\('\/api\/v1\/ai\/workflows'\)/)
  assert.match(aiView, /runtimeRequestSequence/)
  assert.match(aiView, /requestSequence !== runtimeRequestSequence/)
  assert.match(aiView, /workflowManageRequestSequence/)
  assert.match(aiView, /loadWorkflowManagement\(true\)/)
  assert.match(aiView, /apiStream\('\/api\/v1\/ai\/chat\/stream'/)
  assert.match(aiView, /new AbortController\(\)/)
  assert.match(aiView, /:key="message\.id"/)
  for (const field of ['question','conversationId','workflowId','model','maxTokens']) assert.match(aiView, new RegExp(`\\b${field}\\b`), `missing AI request field: ${field}`)
  for (const eventType of ['run.started','text.delta','tool.started','tool.completed','run.completed','run.failed']) assert.ok(aiView.includes(`'${eventType}'`), `missing stream event: ${eventType}`)
  assert.doesNotMatch(aiView, /event\.reasoning/)
  assert.doesNotMatch(aiView, /conversationId\.value\s*=\s*event\.conversationId/)
  assert.doesNotMatch(aiView, /model:[^\n]*runtime\.value\.active/)
  assert.match(aiView, /conversationId\.value\s*=\s*makeId\('conversation'\)/)
  for (const label of ['工作流知识库绑定', '强制检索', '最低相似度', '无匹配知识时', '保存知识库绑定']) assert.match(aiView, new RegExp(label), `missing workflow knowledge binding UI: ${label}`)
  for (const label of ['通过 JSON 创建 Agent', 'Agent Manifest JSON', '校验并创建 Agent', '保存后立即进入工作流列表']) assert.match(aiView, new RegExp(label), `missing dynamic Agent UI: ${label}`)
  for (const field of ['schemaVersion','id','name','description','version','enabled','persona','defaultModel','maxTokens','capabilities','allowedTools']) assert.match(aiView, new RegExp(`name:'${field}'`), `missing Agent field documentation: ${field}`)
  assert.match(aiView, /JSON 标准不支持注释/)
  assert.match(aiView, /allowedTools 可用工具/)
  for (const label of ['本次运行', '选择工作流', '运行参数', '运行环境', '管理中心', 'AI 工作流管理']) assert.match(aiView, new RegExp(label), `missing workflow hierarchy label: ${label}`)
  assert.match(aiView, /<el-drawer v-model="managementVisible"/)
  assert.match(aiView, /<el-menu :default-active="managementTab"/)
  for (const menuClass of ['menu-agent','menu-knowledge','menu-provider']) assert.match(aiView, new RegExp(menuClass), `missing colored management menu: ${menuClass}`)
  assert.doesNotMatch(aiView, /<el-collapse/)
  assert.match(aiView, /knowledge-binding/)
  assert.match(aiView, /method:'PUT'/)
  assert.match(aiView, /api\('\/api\/v1\/ai\/workflows', \{ method:'POST'/)
  for (const marker of ['/api/v1/ai/workflows/admin', "method:'DELETE'", '工作流插件管理', '已配置的工作流插件', '内置只读', '启用', '禁用', '删除']) {
    assert.match(aiView, new RegExp(marker.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')), `missing workflow management marker: ${marker}`)
  }
  assert.match(aiView, /method:'PUT'/)
  assert.match(apiSource, /export async function apiStream/)
  assert.match(apiSource, /request\.cache = 'no-store'/)
  assert.match(aiView, /providerProfileStorageKey/)
  assert.match(aiView, /provider:custom \? 'openai-compatible' : sandbox\.provider/)
  assert.match(apiSource, /text\/event-stream/)
  assert.match(sseSource, /getReader\(\)/)
})

test('protocol mappings, parsed raw results and device connection state are visible', async () => {
  const protocols = await readFile(new URL('src/views/ProtocolsView.vue', root), 'utf8')
  const raw = await readFile(new URL('src/views/RawView.vue', root), 'utf8')
  const devices = await readFile(new URL('src/views/DevicesView.vue', root), 'utf8')
  const app = await readFile(new URL('src/App.vue', root), 'utf8')
  const integration = await readFile(new URL('src/views/IntegrationView.vue', root), 'utf8')
  const labels = await readFile(new URL('src/labels.js', root), 'utf8')
  for (const label of ['可配置解析', '配置 JSON', '解析调试', '标准解析结果', '解析脚本']) assert.match(`${protocols}\n${raw}`, new RegExp(label), `missing label: ${label}`)
  assert.match(protocols, /configurable_json_parser/)
  assert.match(protocols, /configurable_hex_parser/)
  assert.match(protocols, /javascript_sandbox_parser/)
  assert.match(protocols, /go_protocol_parser/)
  assert.match(protocols, /uploadArtifact/)
  assert.match(labels, /受限 JavaScript 解析器/)
  assert.match(app, /label:'设备与数据'/)
  assert.match(app, /title:'接入指南'/)
  assert.match(integration, /设备连接指南/)
  assert.match(raw, /standardMessage/)
    assert.match(devices, /hasReported\(row\)/)
  assert.match(devices, /查看数据/)
  assert.match(devices, /配置接入/)
})

test('test device workbench provisions a fixture and sends editable data and alarm templates', async () => {
  const app = await readFile(new URL('src/App.vue', root), 'utf8')
  const view = await readFile(new URL('src/views/TestDeviceView.vue', root), 'utf8')
  for (const label of ['测试设备', '发送正常数据', '发送报警数据', '发送恢复数据', '报文模板', '建议测试顺序']) {
    assert.match(`${app}\n${view}`, new RegExp(label), `missing test device label: ${label}`)
  }
  assert.match(app, /testDevice/)
  assert.match(view, /api\('\/api\/v1\/test-devices\/provision'/)
  assert.match(view, /device-registry\/\$\{encodeURIComponent\(device\.value\.id\)\}\/debug/)
  assert.match(view, /messageId.*<unique>/)
  assert.match(view, /alarm.*true/)
  assert.match(view, /localStorage/)
  assert.match(view, /不会自动创建告警规则/)
  assert.doesNotMatch(view, /系统生成的高温烟雾规则/)
  assert.match(app, /action\.type === 'OPEN_PAGE'/)
  assert.match(app, /openPage\(action\.page\)/)
  assert.match(app, /'devices'/)
})

test('alarm acknowledgement action is unavailable after the alarm is acknowledged', async () => {
  const actions = await import('../src/alarmActions.js')
  const alarms = await readFile(new URL('src/views/AlarmsView.vue', root), 'utf8')
  assert.equal(actions.canAcknowledgeAlarm('ACTIVE'), true)
  assert.equal(actions.canAcknowledgeAlarm('ACKED'), false)
  assert.equal(actions.canAcknowledgeAlarm('CLOSED'), false)
  assert.equal(actions.canCloseAlarm('ACKED'), true)
  assert.match(alarms, /canAcknowledgeAlarm\(row\.status\)/)
  assert.match(alarms, /canCloseAlarm\(row\.status\)/)
  assert.match(alarms, /actionPending\[row\.alarmId\]/)
  assert.match(alarms, /await load\(\)/)
})

test('backup center exposes history, artifact downloads and restore drills', async () => {
  const app = await readFile(new URL('src/App.vue', root), 'utf8')
  const view = await readFile(new URL('src/views/BackupsView.vue', root), 'utf8')
  const labels = await readFile(new URL('src/labels.js', root), 'utf8')
  for (const marker of ['备份中心', '立即全量备份', '立即增量备份', '详情 / 文件', '恢复演练', '下载 manifest.json', '备份文件']) {
    assert.match(`${app}\n${view}`, new RegExp(marker), `missing backup center marker: ${marker}`)
  }
  assert.match(labels, /backupTypes/)
  assert.match(labels, /backupStatuses/)
  for (const route of ['/api/v1/backups', '/restore-drill', '/files']) assert.match(view, new RegExp(route.replaceAll('/', '\\/')))
  assert.match(view, /download\(/)
  assert.match(app, /backups/)
})

test('protocol assistant and health inspection pages expose the review workflow', async () => {
  const assistant = await readFile(new URL('src/views/ProtocolAssistantView.vue', root), 'utf8')
  const inspection = await readFile(new URL('src/views/HealthInspectionView.vue', root), 'utf8')
  const app = await readFile(new URL('src/App.vue', root), 'utf8')
  for (const label of ['协议接入助手', '协议文件', '点表 / 协议片段', '生成 Go 协议映射', '字段映射', '线圈地址', '运行解析预览', '保存并发布 Go 协议包']) {
    assert.match(assistant, new RegExp(label), `missing protocol assistant label: ${label}`)
  }
  assert.doesNotMatch(assistant, /解析 JavaScript|解析表达式|function parse/)
  for (const route of ['/api/v1/ai/protocol-assistant/generate', '/api/v1/ai/protocol-assistant/preview', '/api/v1/ai/protocol-assistant/publish']) {
    assert.match(assistant, new RegExp(route.replaceAll('/', '\\/')))
  }
  for (const label of ['设备健康巡检', '立即巡检', '状态正常', '活动告警', 'AI 巡检建议']) {
    assert.match(inspection, new RegExp(label), `missing inspection label: ${label}`)
  }
  assert.match(inspection, /api\('\/api\/v1\/ai\/health-inspection'/)
  assert.doesNotMatch(inspection, /onMounted\(run\)/)
  assert.match(inspection, /点击“立即巡检”开始检查/)
  assert.match(app, /title:'协议接入助手'/)
  assert.match(app, /title:'智能巡检'/)
  assert.match(app, /protocolAssistant/)
  assert.match(app, /inspection/)
})

test('AI rule drafts keep failures visible in the page', async () => {
  const rulesView = await readFile(new URL('src/views/RulesView.vue', root), 'utf8')
  assert.match(rulesView, /draftError\.value=e\?\.message/)
  assert.match(rulesView, /v-if="draftError"/)
  assert.match(rulesView, /规则草稿生成失败/)
})

test('Agent automation drafts and allowlisted UI actions remain wired end to end', async () => {
  const aiView = await readFile(new URL('src/views/AiView.vue', root), 'utf8')
  const rulesView = await readFile(new URL('src/views/RulesView.vue', root), 'utf8')
  const app = await readFile(new URL('src/App.vue', root), 'utf8')
  const cameras = await readFile(new URL('src/views/CameraMappingsView.vue', root), 'utf8')
  assert.match(aiView, /RULE_DRAFT_READY/)
  assert.match(aiView, /clientAction\.persisted/)
  assert.match(aiView, /AI_HISTORY_STORAGE_PREFIX/)
  assert.match(aiView, /restoreConversation\(\)/)
  assert.match(aiView, /persistConversation\(\)/)
  assert.match(aiView, /mcp__iot__create_rule_draft/)
  assert.match(rulesView, /联动动作 JSON/)
  assert.match(rulesView, /detail\.ruleDraft/)
  assert.match(rulesView, /detail\.persisted/)
  assert.match(app, /\/ui-action\//)
  assert.match(app, /action\.type === 'OPEN_CAMERA'/)
  assert.match(app, /allowedPages/)
  assert.match(cameras, /detail\.autoPreview/)
  assert.match(cameras, /await openPreview\(target\)/)
})

test('AI conversation history survives view recreation and stays tenant scoped', async () => {
  const { AI_HISTORY_STORAGE_PREFIX, loadAIHistory, saveAIHistory } = await import('../src/aiHistory.js')
  const values = new Map()
  const storage = { getItem:key => values.get(key) ?? null, setItem:(key,value) => values.set(key,value), removeItem:key => values.delete(key) }
  const session = { tenant:'tenant-a', user:'alice' }
  assert.equal(saveAIHistory(storage, session, { conversationId:'conversation-1', selectedWorkflowId:'ops-assistant', messages:[{ id:'m1', role:'user', status:'succeeded', text:'温度超过 80' }, { id:'m2', role:'assistant', status:'streaming', text:'' }], runs:[{ id:'r1', status:'running' }] }), true)
  assert.ok([...values.keys()][0].startsWith(AI_HISTORY_STORAGE_PREFIX))
  const restored = loadAIHistory(storage, session, 123456)
  assert.equal(restored.conversationId, 'conversation-1')
  assert.equal(restored.messages[0].text, '温度超过 80')
  assert.equal(restored.messages[1].status, 'canceled')
  assert.equal(restored.runs[0].status, 'canceled')
  assert.equal(restored.runs[0].finishedAt, 123456)
  assert.equal(loadAIHistory(storage, { tenant:'tenant-b', user:'alice' }), null)
})

test('AI rule draft cards reconcile persisted snapshots with current rule state', async () => {
  const { reconcileRuleDraftMessages } = await import('../src/ruleDraftStatus.js')
  const messages = [{ id:'assistant-1', ruleDraftPersisted:true, ruleDraftState:'draft', ruleDraft:{ id:'rule-1', name:'旧名称', enabled:false } }]
  assert.equal(reconcileRuleDraftMessages(messages, [{ id:'rule-1', name:'已启用规则', enabled:true, version:2 }]), 1)
  assert.equal(messages[0].ruleDraftState, 'enabled')
  assert.equal(messages[0].ruleDraft.enabled, true)
  assert.equal(messages[0].ruleDraft.name, '已启用规则')
  reconcileRuleDraftMessages(messages, [])
  assert.equal(messages[0].ruleDraftState, 'missing')
})

test('frontend builds independently and proxies backend routes', async () => {
  const vite = await readFile(new URL('vite.config.js', root), 'utf8')
  const nginx = await readFile(new URL('nginx.conf', root), 'utf8')
  assert.match(vite, /outDir:\s*'dist'/)
  assert.doesNotMatch(vite, /internal\/httpapi\/static/)
  for (const route of ['/api/', '/health/', '/mcp']) assert.ok(nginx.includes(route), `nginx is missing ${route}`)
  assert.ok(nginx.includes('platform-api:8080'))
  assert.match(nginx, /location = \/api\/v1\/ai\/chat\/stream\s*\{[\s\S]*?proxy_buffering off;[\s\S]*?proxy_cache off;[\s\S]*?gzip off;[\s\S]*?proxy_read_timeout 3600s;[\s\S]*?proxy_set_header Connection "";/)
  assert.match(nginx, /IOT_VIDEO_PREVIEW_CSP_SOURCES/)
  assert.doesNotMatch(nginx, /connect-src[^;]*\bhttp:\s+https:/)
})

test('programmatic Element Plus services include their global styles', async () => {
  const main = await readFile(new URL('src/main.js', root), 'utf8')
  assert.match(main, /element-plus\/theme-chalk\/el-message-box\.css/)
  assert.match(main, /element-plus\/theme-chalk\/el-message\.css/)
})

test('frontend rejects Node versions unsupported by the build toolchain', async () => {
  const packageJson = JSON.parse(await readFile(new URL('package.json', root), 'utf8'))
  const packageLock = JSON.parse(await readFile(new URL('package-lock.json', root), 'utf8'))
  const npmrc = await readFile(new URL('.npmrc', root), 'utf8')

  const supportedNodeVersions = '^20.19.0 || >=22.12.0'
  assert.equal(packageJson.engines?.node, supportedNodeVersions)
  assert.equal(packageLock.packages?.['']?.engines?.node, supportedNodeVersions)
  assert.match(npmrc, /^engine-strict=true\s*$/m)
})
