<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { api, apiStream, session } from '../api'
import HarnessTraceDrawer from '../components/HarnessTraceDrawer.vue'
import ToolCallCard from '../components/ToolCallCard.vue'

let sequence = 0
let abortController = null
let scrollFrame = 0
const makeId = prefix => `${prefix}_${globalThis.crypto?.randomUUID?.() || `${Date.now()}_${++sequence}`}`
const welcomeMessage = () => ({ id:'welcome', role:'assistant', status:'succeeded', text:'你好，我是消防物联网 AI 运维助手。选择一个工作流后，可以直接查询设备、告警和趋势；所有工具执行都会显示在运行轨迹中。', tools:[] })

const messages = ref([welcomeMessage()])
const runs = ref([])
const question = ref('')
const sending = ref(false)
const log = ref()
const conversationId = ref('')
const selectedRunKey = ref('')
const traceVisible = ref(false)

const runtimeLoading = ref(false)
const runtimeError = ref('')
const workflowError = ref('')
const runtime = ref({ items:[], active:{ id:'disabled', name:'未启用', enabled:false }, healthy:false, healthMessage:'正在读取 Provider 状态' })
const workflows = ref({ items:[], healthy:false, healthMessage:'正在读取工作流状态' })
const selectedWorkflowId = ref('')

const testing = ref(false)
const testResult = ref(null)
const sandboxOpen = ref([])
const sandbox = reactive({ provider:'deepseek', baseUrl:'https://api.deepseek.com', model:'deepseek-v4-flash', apiKey:'', question:'请用一句话说明你已经连接到消防物联网 AI 测试台。' })
const runConfig = reactive({ model:'', maxTokens:1200 })
const quickQuestions = ['当前有哪些高等级活动告警？', 'device_001 最近温度趋势如何？', '给出今日消防巡检重点']

const providerItems = computed(() => (runtime.value.items || []).filter(item => item.id !== 'disabled' && item.enabled !== false))
const workflowItems = computed(() => (workflows.value.items || []).filter(item => item.enabled !== false))
const selectedPlugin = computed(() => (runtime.value.items || []).find(item => item.id === sandbox.provider))
const selectedWorkflow = computed(() => workflowItems.value.find(item => workflowKey(item) === selectedWorkflowId.value))
const selectedRun = computed(() => runs.value.find(run => run.id === selectedRunKey.value) || null)
const activeHealthy = computed(() => Boolean(workflows.value.healthy))
const activeTone = computed(() => !workflowItems.value.length ? 'info' : activeHealthy.value ? 'success' : 'danger')
const healthMessage = computed(() => workflows.value.healthMessage || 'Harness 工作流状态未知')
const isAdmin = computed(() => session.role === 'admin')
const selectedCapabilities = computed(() => {
  const value = selectedWorkflow.value?.capabilities || selectedWorkflow.value?.tools || []
  return Array.isArray(value) ? value : []
})

function workflowKey(item) { return item?.id || item?.workflowId || '' }
function workflowName(item) { return item?.name || item?.label || workflowKey(item) || '未命名工作流' }
function capabilityLabel(item) { return typeof item === 'string' ? item : item?.name || item?.id || '工具' }

function timestamp(value) {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string' && /^\d+$/.test(value)) return Number(value)
  const parsed = value ? Date.parse(value) : Number.NaN
  return Number.isNaN(parsed) ? Date.now() : parsed
}

function safeText(value, fallback = '') {
  if (value == null) return fallback
  const text = typeof value === 'string' ? value : JSON.stringify(value)
  return text.length > 1200 ? `${text.slice(0, 1200)}…` : text
}

function normalizeError(value, fallback = 'AI 运行失败') {
  const error = typeof value === 'string' ? { message:value } : value || {}
  return {
    message:safeText(error.message || error.detail || error.text, fallback),
    code:safeText(error.code),
    stage:safeText(error.stage),
    traceId:safeText(error.traceId),
    retryable:Boolean(error.retryable)
  }
}

function normalizeEvent(raw) {
  const nested = raw?.data && typeof raw.data === 'object' && !Array.isArray(raw.data) ? raw.data : {}
  return { ...raw, ...nested, type:raw?.type || nested.type || 'message' }
}

function scheduleScroll() {
  if (scrollFrame) cancelAnimationFrame(scrollFrame)
  nextTick(() => {
    scrollFrame = requestAnimationFrame(() => {
      log.value?.scrollTo({ top:log.value.scrollHeight, behavior:'smooth' })
      scrollFrame = 0
    })
  })
}

async function loadRuntime() {
  runtimeLoading.value = true
  runtimeError.value = ''
  workflowError.value = ''
  const [providerResult, workflowResult] = await Promise.allSettled([
    api('/api/v1/ai/providers'),
    api('/api/v1/ai/workflows')
  ])
  if (providerResult.status === 'fulfilled') {
    runtime.value = providerResult.value
    if (!providerItems.value.some(item => item.id === sandbox.provider) && providerItems.value[0]) sandbox.provider = providerItems.value[0].id
  } else {
    runtimeError.value = providerResult.reason?.message || 'Provider 状态读取失败'
  }
  if (workflowResult.status === 'fulfilled') {
    workflows.value = {
      items:Array.isArray(workflowResult.value?.items) ? workflowResult.value.items : [],
      healthy:Boolean(workflowResult.value?.healthy),
      healthMessage:workflowResult.value?.healthMessage || ''
    }
    if (!workflowItems.value.some(item => workflowKey(item) === selectedWorkflowId.value)) selectedWorkflowId.value = workflowKey(workflowItems.value[0])
    applyWorkflowDefaults()
  } else {
    workflowError.value = workflowResult.reason?.message || '工作流列表读取失败'
  }
  runtimeLoading.value = false
}

watch(() => sandbox.provider, () => {
  const plugin = selectedPlugin.value
  if (!plugin) return
  sandbox.baseUrl = plugin.defaultBaseUrl || ''
  sandbox.model = plugin.defaultModel || ''
  sandbox.apiKey = ''
  testResult.value = null
})

watch(selectedWorkflowId, applyWorkflowDefaults)

function applyWorkflowDefaults() {
  const workflow = selectedWorkflow.value
  runConfig.model = workflow?.defaultModel || workflow?.model || ''
  if (Number.isSafeInteger(workflow?.maxTokens)) runConfig.maxTokens = Math.max(128, Math.min(8192, workflow.maxTokens))
}

async function testPlugin() {
  if (!sandbox.provider || testing.value || !isAdmin.value) return
  testing.value = true
  testResult.value = null
  try {
    testResult.value = await api('/api/v1/ai/providers/test', { method:'POST', body:JSON.stringify(sandbox) })
  } catch (error) {
    testResult.value = { success:false, error:error.message, traceId:error.traceId }
  } finally {
    sandbox.apiKey = ''
    testing.value = false
  }
}

function addRunEvent(run, event, label, status = 'info', detail = '') {
  run.events.push({ id:event.eventId || makeId('event'), type:event.type, label, status, detail:safeText(detail), createdAt:timestamp(event.createdAt || event.timestamp) })
}

function toolCallKey(event) { return event.toolCallId || event.callId || event.tool?.toolCallId || event.tool?.id || event.id || '' }

function findOrCreateTool(run, assistant, event) {
  const callId = toolCallKey(event)
  let tool = run.tools.find(item => item.toolCallId === callId)
  if (!tool) {
    const toolName = event.toolName || event.name || (typeof event.tool === 'string' ? event.tool : event.tool?.name) || '未命名工具'
    run.tools.push({ id:makeId('tool'), toolCallId:callId || makeId('call'), name:toolName, status:'running', inputSummary:event.inputSummary || event.input?.summary || '', outputSummary:'', error:'', startedAt:timestamp(event.startedAt || event.createdAt), durationMs:null })
    tool = run.tools[run.tools.length - 1]
    assistant.tools = run.tools
  }
  return tool
}

function applyStreamEvent(raw, assistant, run) {
  const event = normalizeEvent(raw)
  if (event.messageId) assistant.serverMessageId = event.messageId
  if (event.runId) { assistant.runId = event.runId; run.runId = event.runId }
  if (event.traceId) { assistant.traceId = event.traceId; run.traceId = event.traceId }

  switch (event.type) {
    case 'run.started':
      assistant.status = 'streaming'
      run.status = 'running'
      run.provider = event.provider || run.provider
      run.model = event.model || run.model
      run.startedAt = timestamp(event.startedAt || event.createdAt)
      addRunEvent(run, event, 'Harness 开始运行', 'running', [run.provider,run.model].filter(Boolean).join(' / '))
      break
    case 'text.delta': {
      const delta = event.delta ?? event.text ?? event.content ?? ''
      assistant.text += typeof delta === 'string' ? delta : ''
      scheduleScroll()
      break
    }
    case 'tool.started': {
      const tool = findOrCreateTool(run, assistant, event)
      tool.status = 'running'
      tool.inputSummary = event.inputSummary || event.input?.summary || tool.inputSummary
      addRunEvent(run, event, `调用工具 · ${tool.name}`, 'running', tool.inputSummary)
      break
    }
    case 'tool.completed': {
      const tool = findOrCreateTool(run, assistant, event)
      const toolError = event.error ? normalizeError(event.error, '工具调用失败') : null
      tool.status = event.success === false || ['failed','error'].includes(event.status) || toolError ? 'failed' : 'succeeded'
      tool.outputSummary = event.outputSummary || event.output?.summary || ''
      tool.error = toolError?.message || ''
      tool.durationMs = event.durationMs ?? (event.completedAt ? Math.max(0, timestamp(event.completedAt) - tool.startedAt) : null)
      addRunEvent(run, event, `工具${tool.status === 'failed' ? '失败' : '完成'} · ${tool.name}`, tool.status === 'failed' ? 'danger' : 'success', tool.error || tool.outputSummary)
      break
    }
    case 'run.completed':
      if (!assistant.text) assistant.text = safeText(event.answer || event.text, '本次运行已完成，但没有返回文本。')
      assistant.status = 'succeeded'
      assistant.usage = event.usage || null
      assistant.durationMs = event.durationMs ?? Math.max(0, Date.now() - run.startedAt)
      run.status = 'succeeded'
      run.usage = event.usage || null
      run.durationMs = assistant.durationMs
      run.finishedAt = timestamp(event.completedAt || event.createdAt)
      addRunEvent(run, event, 'Harness 运行完成', 'success', run.durationMs != null ? `${run.durationMs} ms` : '')
      break
    case 'run.failed': {
      const failure = normalizeError(event.error || event, 'AI 运行失败')
      assistant.status = 'failed'
      assistant.error = failure
      assistant.text ||= '运行未能完成。'
      run.status = 'failed'
      run.error = failure
      run.durationMs = event.durationMs ?? Math.max(0, Date.now() - run.startedAt)
      run.finishedAt = timestamp(event.failedAt || event.createdAt)
      for (const tool of run.tools.filter(item => item.status === 'running')) tool.status = 'failed'
      addRunEvent(run, event, 'Harness 运行失败', 'danger', failure.message)
      break
    }
  }
}

async function send(textValue) {
  const text = (textValue || question.value).trim()
  if (!text || sending.value) return
  if (!conversationId.value) conversationId.value = makeId('conversation')
  messages.value.push({ id:makeId('message'), role:'user', status:'succeeded', text, tools:[] })
  messages.value.push({ id:makeId('message'), role:'assistant', status:'streaming', text:'', prompt:text, tools:[], error:null })
  const assistant = messages.value[messages.value.length - 1]
  runs.value.unshift({ id:makeId('run'), runId:'', traceId:'', status:'running', workflowId:selectedWorkflowId.value, workflowName:workflowName(selectedWorkflow.value), provider:'', model:runConfig.model || selectedWorkflow.value?.defaultModel || selectedWorkflow.value?.model || '', startedAt:Date.now(), finishedAt:null, durationMs:null, usage:null, events:[], tools:[], error:null })
  const run = runs.value[0]
  assistant.runKey = run.id
  assistant.tools = run.tools
  question.value = ''
  sending.value = true
  scheduleScroll()

  const controller = new AbortController()
  abortController = controller
  const maxTokens = Math.max(128, Math.min(8192, Number(runConfig.maxTokens) || 1200))
  const body = { question:text, conversationId:conversationId.value, workflowId:selectedWorkflowId.value, model:runConfig.model || selectedWorkflow.value?.defaultModel || selectedWorkflow.value?.model || '', maxTokens }

  try {
    await apiStream('/api/v1/ai/chat/stream', { method:'POST', body:JSON.stringify(body), signal:controller.signal }, event => applyStreamEvent(event, assistant, run))
    if (assistant.status === 'streaming') {
      const failure = normalizeError({ code:'AI_STREAM_INCOMPLETE', retryable:true }, 'AI 流意外结束，请重试。')
      assistant.status = 'failed'; assistant.error = failure; assistant.text ||= '响应流未完整结束。'
      run.status = 'failed'; run.error = failure; run.durationMs = Math.max(0, Date.now() - run.startedAt)
      addRunEvent(run, { type:'run.failed' }, '响应流意外结束', 'danger', failure.message)
    }
  } catch (error) {
    if (error?.name === 'AbortError') {
      assistant.status = 'canceled'; assistant.text ||= '已停止生成。'; run.status = 'canceled'; run.durationMs = Math.max(0, Date.now() - run.startedAt)
      for (const tool of run.tools.filter(item => item.status === 'running')) tool.status = 'canceled'
      addRunEvent(run, { type:'run.canceled' }, '用户停止运行', 'info')
    } else {
      const failure = normalizeError(error)
      assistant.status = 'failed'; assistant.error = failure; assistant.text ||= '运行未能完成。'
      run.status = 'failed'; run.error = failure; run.traceId ||= failure.traceId; run.durationMs = Math.max(0, Date.now() - run.startedAt)
      addRunEvent(run, { type:'run.failed' }, '请求失败', 'danger', failure.message)
    }
  } finally {
    run.finishedAt ||= Date.now()
    if (abortController === controller) { abortController = null; sending.value = false }
    scheduleScroll()
  }
}

function stop() { abortController?.abort() }
function retry(message) { if (!sending.value) send(message.prompt) }
function clearConversation() { abortController?.abort(); messages.value = [welcomeMessage()]; runs.value = []; conversationId.value = ''; selectedRunKey.value = ''; traceVisible.value = false }
function openTrace(value) { selectedRunKey.value = value?.runKey || value?.id || ''; traceVisible.value = true }

onMounted(loadRuntime)
onBeforeUnmount(() => { abortController?.abort(); if (scrollFrame) cancelAnimationFrame(scrollFrame) })
</script>

<template>
  <div class="ai-runtime" v-loading="runtimeLoading">
    <div><span class="section-kicker">AI OPERATIONS</span><strong>AI 运维助手</strong><small>工作流、Provider 与工具解耦；每次运行都有可审计的 Harness 轨迹。</small></div>
    <div class="runtime-actions"><div class="runtime-status"><el-tag :type="activeTone" effect="light">{{ selectedWorkflow ? 'Harness 工作流' : '未配置' }}</el-tag><span>{{ runConfig.model || '无活动模型' }}</span><i :class="{ online:activeHealthy }" />{{ healthMessage }}</div><el-button size="small" :loading="runtimeLoading" @click="loadRuntime">刷新状态</el-button></div>
  </div>

  <div class="ai-workbench">
    <el-card shadow="never" class="surface-card control-card">
      <template #header><div class="card-header"><div><strong>运行工作流</strong><small>选择业务插件并查看可用工具</small></div><el-tag effect="plain">PLUGIN</el-tag></div></template>
      <div class="control-scroll">
        <el-alert v-if="workflowError" :title="workflowError" type="error" :closable="false" show-icon><el-button text size="small" @click="loadRuntime">重新加载</el-button></el-alert>
        <el-form label-position="top"><el-form-item label="工作流插件"><el-select v-model="selectedWorkflowId" placeholder="选择 AI 工作流" :disabled="sending || !workflowItems.length"><el-option v-for="item in workflowItems" :key="workflowKey(item)" :label="workflowName(item)" :value="workflowKey(item)" /></el-select></el-form-item></el-form>
        <el-empty v-if="!runtimeLoading && !workflowItems.length" description="暂无可用工作流" :image-size="62" />
        <div v-if="selectedWorkflow" class="workflow-description"><div><span class="workflow-icon">WF</span><div><strong>{{ workflowName(selectedWorkflow) }}</strong><small>{{ selectedWorkflow.version ? `v${selectedWorkflow.version}` : '服务端托管' }}</small></div></div><p>{{ selectedWorkflow.description || '该工作流会按服务端策略调用受控工具。' }}</p><div v-if="selectedCapabilities.length" class="capability-list"><el-tag v-for="item in selectedCapabilities" :key="capabilityLabel(item)" size="small" effect="plain">{{ capabilityLabel(item) }}</el-tag></div></div>
        <el-form class="run-config" label-position="top" :model="runConfig" :disabled="sending"><div><el-form-item label="运行模型"><el-input v-model="runConfig.model" placeholder="使用活动模型" /></el-form-item><el-form-item label="最大输出"><el-input-number v-model="runConfig.maxTokens" :min="128" :max="8192" :step="128" controls-position="right" /></el-form-item></div></el-form>
        <div class="provider-summary"><span>活动 Provider</span><strong>{{ runtime.active?.name || '未配置' }}</strong><small>{{ runtime.active?.model || '模型由服务端选择' }}</small><el-alert v-if="runtimeError" :title="runtimeError" type="warning" :closable="false" show-icon /></div>

        <el-collapse v-model="sandboxOpen" class="sandbox-collapse">
          <el-collapse-item name="provider-test">
            <template #title><div class="collapse-title"><div><strong>Provider 临时测试</strong><small>插件沙箱 · 配置不会保存</small></div><el-tag size="small" effect="plain">ADMIN</el-tag></div></template>
            <el-alert v-if="!isAdmin" class="admin-notice" title="Provider 连接测试仅限管理员；你仍可使用已配置的运维 AI。" type="warning" :closable="false" show-icon />
            <el-form label-position="top" :model="sandbox" :disabled="!isAdmin">
              <el-form-item label="模型插件"><el-select v-model="sandbox.provider"><el-option v-for="item in providerItems" :key="item.id" :label="item.name" :value="item.id" /></el-select></el-form-item>
              <div v-if="selectedPlugin" class="plugin-description"><strong>{{ selectedPlugin.name }}</strong><span>{{ selectedPlugin.description }}</span></div>
              <el-form-item label="API Base URL"><el-input v-model="sandbox.baseUrl" placeholder="https://api.deepseek.com" /></el-form-item><el-form-item label="模型"><el-input v-model="sandbox.model" placeholder="deepseek-chat" /></el-form-item>
              <el-form-item v-if="selectedPlugin?.requiresApiKey || sandbox.provider === 'openai-compatible'" label="API Key"><el-input v-model="sandbox.apiKey" type="password" show-password autocomplete="off" placeholder="仅用于本次连接测试" /></el-form-item>
              <el-form-item label="测试问题"><el-input v-model="sandbox.question" type="textarea" :rows="3" maxlength="2000" show-word-limit /></el-form-item><el-alert title="API Key 不会保存，也不会写入 trace 或审计日志。" type="info" :closable="false" show-icon /><el-button class="test-button" type="primary" :loading="testing" @click="testPlugin">连接并测试插件</el-button>
            </el-form>
            <div v-if="testResult" class="test-result" :class="{ success:testResult.success, failed:!testResult.success }"><div><strong>{{ testResult.success ? '连接成功' : '连接失败' }}</strong><el-tag :type="testResult.success ? 'success' : 'danger'" effect="dark">{{ testResult.success ? `${testResult.latencyMs} ms` : 'ERROR' }}</el-tag></div><p>{{ testResult.answer || testResult.error }}</p><small v-if="testResult.traceId">Trace · {{ testResult.traceId }}</small></div>
          </el-collapse-item>
        </el-collapse>
      </div>
    </el-card>

    <el-card shadow="never" class="surface-card chat-card ai-chat-card">
      <template #header><div class="card-header chat-header"><div><strong>{{ workflowName(selectedWorkflow) }}</strong><small>{{ conversationId ? `会话 · ${conversationId}` : '新会话 · 服务端受控租户上下文' }}</small></div><div><el-button text :disabled="!runs.length" @click="openTrace(runs[0])">运行轨迹</el-button><el-button text type="primary" @click="clearConversation">清空对话</el-button></div></div></template>
      <div class="quick-prompts"><button v-for="item in quickQuestions" :key="item" :disabled="sending || !workflowItems.length" @click="send(item)">{{ item }}</button></div>
      <div ref="log" class="chat-log" aria-live="polite">
        <div v-for="message in messages" :key="message.id" class="message-row" :class="message.role">
          <span class="message-avatar">{{ message.role === 'assistant' ? 'AI' : '我' }}</span><div class="message-content"><div class="chat-message" :class="[message.role,`is-${message.status}`]"><p v-if="message.text">{{ message.text }}</p><div v-else-if="message.status === 'streaming'" class="typing"><i/><i/><i/><span>正在运行工作流</span></div><ToolCallCard v-for="tool in message.tools" :key="tool.id || tool.toolCallId" :tool="tool" /><div v-if="message.error" class="message-error"><strong>{{ message.error.message }}</strong><small v-if="message.error.code || message.error.stage">{{ [message.error.code,message.error.stage].filter(Boolean).join(' · ') }}</small><small v-if="message.traceId || message.error.traceId">Trace · {{ message.traceId || message.error.traceId }}</small><el-button v-if="message.prompt" text size="small" :disabled="sending" @click="retry(message)">重新运行</el-button></div></div><div v-if="message.role === 'assistant' && message.runKey" class="message-meta"><span v-if="message.status === 'streaming'">运行中</span><span v-else>{{ message.status === 'succeeded' ? '已完成' : message.status === 'canceled' ? '已停止' : '运行失败' }}</span><span v-if="message.durationMs != null">{{ message.durationMs }} ms</span><span v-if="message.usage?.totalTokens != null">{{ message.usage.totalTokens }} tokens</span><button @click="openTrace(message)">查看轨迹</button></div></div>
        </div>
      </div>
      <div class="chat-compose"><el-input v-model="question" type="textarea" :autosize="{ minRows:1,maxRows:4 }" maxlength="4000" resize="none" placeholder="询问设备、告警、趋势或处置知识；Shift + Enter 换行" :disabled="sending || !workflowItems.length" @keydown.enter.exact.prevent="send()" /><el-button v-if="sending" type="danger" plain @click="stop">停止</el-button><el-button v-else type="primary" :disabled="!question.trim() || !workflowItems.length" @click="send()">发送</el-button></div><small class="chat-notice">AI 输出仅供辅助判断，不会自动执行设备控制或启用规则。</small>
    </el-card>
  </div>
  <HarnessTraceDrawer v-model="traceVisible" :run="selectedRun" />
</template>

<style scoped>
.ai-runtime { min-height:74px; margin-bottom:16px; padding:15px 18px; display:flex; align-items:center; justify-content:space-between; gap:20px; background:#fff; border:1px solid #e8e8e8; border-left:3px solid #1677ff; border-radius:4px; }.ai-runtime>div:first-child { display:grid; gap:3px; }.section-kicker { color:#1677ff; font-size:9px; font-weight:700; letter-spacing:.14em; }.ai-runtime strong { font-size:16px; }.ai-runtime small { color:var(--muted); }.runtime-actions { display:flex; align-items:center; gap:12px; }.runtime-status { display:flex; align-items:center; gap:8px; color:#646c73; font-size:12px; white-space:nowrap; }.runtime-status i { width:7px; height:7px; background:#ff4d4f; border-radius:50%; }.runtime-status i.online { background:#52c41a; }
.ai-workbench { display:grid; grid-template-columns:minmax(280px,320px) minmax(0,1fr); gap:16px; align-items:stretch; }.control-card,.ai-chat-card { height:clamp(580px,calc(100vh - 190px),780px); min-height:0; }.card-header>div { display:grid; gap:3px; }.card-header small { display:block; }.control-card :deep(.el-card__body) { height:calc(100% - 57px); padding:0; }.control-scroll { height:100%; padding:16px; overflow:auto; }.control-scroll>.el-alert { margin-bottom:14px; }.workflow-description { margin:-3px 0 14px; padding:12px; background:#f5f9ff; border:1px solid #d6e8ff; border-radius:4px; }.workflow-description>div:first-child { display:flex; align-items:center; gap:9px; }.workflow-description>div:first-child>div { display:grid; gap:2px; }.workflow-description strong { color:#1554ad; font-size:12px; }.workflow-description small { color:#8c8c8c; font-size:10px; }.workflow-description p { margin:9px 0; color:#646c73; font-size:11px; line-height:1.6; }.workflow-icon { width:30px; height:30px; display:grid; place-items:center; color:#fff; background:#1677ff; border-radius:4px; font-size:9px; font-weight:800; }.capability-list { display:flex; flex-wrap:wrap; gap:5px; }.run-config { margin-bottom:2px; }.run-config>div { display:grid; grid-template-columns:minmax(0,1fr) 112px; gap:9px; }.run-config :deep(.el-input-number) { width:100%; }.provider-summary { margin:0 0 14px; padding:11px; display:grid; gap:4px; background:#fafafa; border:1px solid #ededed; border-radius:4px; }.provider-summary>span { color:#8c8c8c; font-size:10px; }.provider-summary strong { font-size:12px; }.provider-summary small { color:#646c73; }.provider-summary .el-alert { margin-top:7px; }.sandbox-collapse { border-top:1px solid #ededed; }.collapse-title { width:100%; padding-right:8px; display:flex; align-items:center; justify-content:space-between; }.collapse-title>div { display:grid; gap:2px; }.collapse-title strong { font-size:12px; }.collapse-title small { color:#8c8c8c; font-size:10px; }.plugin-description { margin:-4px 0 15px; display:grid; gap:4px; }.plugin-description strong { color:#1554ad; font-size:11px; }.plugin-description span { color:#646c73; font-size:10px; line-height:1.5; }.admin-notice { margin-bottom:14px; }.test-button { width:100%; margin-top:12px; }.test-result { margin-top:14px; padding:11px; border:1px solid; border-radius:4px; }.test-result.success { background:#f6ffed; border-color:#b7eb8f; }.test-result.failed { background:#fff2f0; border-color:#ffccc7; }.test-result>div { display:flex; justify-content:space-between; align-items:center; }.test-result p { margin:8px 0; color:#3d3d3d; font-size:11px; line-height:1.6; white-space:pre-wrap; }.test-result small { color:#8c8c8c; word-break:break-all; }
.ai-chat-card :deep(.el-card__body) { height:calc(100% - 57px); display:flex; flex-direction:column; }.chat-header>div:last-child { display:flex; align-items:center; }.quick-prompts { flex:none; display:flex; flex-wrap:wrap; gap:7px; padding-bottom:12px; border-bottom:1px solid #f0f0f0; }.quick-prompts button { padding:6px 9px; color:#1554ad; background:#f5f9ff; border:1px solid #d6e8ff; border-radius:3px; font-size:11px; cursor:pointer; }.quick-prompts button:hover:not(:disabled) { color:#fff; background:#1677ff; border-color:#1677ff; }.quick-prompts button:disabled { opacity:.5; cursor:not-allowed; }.chat-log { min-height:0; flex:1; padding:15px 3px 6px; overflow:auto; }.message-row { display:flex; align-items:flex-start; gap:9px; }.message-row.user { flex-direction:row-reverse; }.message-avatar { width:28px; height:28px; flex:0 0 28px; display:grid; place-items:center; color:#fff; background:#3f4960; border-radius:4px; font-size:10px; font-weight:700; }.message-row.user .message-avatar { background:#1677ff; }.message-content { min-width:0; max-width:min(82%,760px); margin-bottom:14px; }.message-row.user .message-content { display:flex; flex-direction:column; align-items:flex-end; }.chat-message { margin:0; padding:10px 12px; color:#3d3d3d; background:#f5f5f5; border-radius:4px; font-size:12px; line-height:1.75; word-break:break-word; }.chat-message.user { color:#fff; background:#1677ff; }.chat-message.is-failed { background:#fff7f6; border:1px solid #ffccc7; }.chat-message.is-canceled { color:#646c73; background:#fafafa; border:1px dashed #d9d9d9; }.chat-message p { margin:0; white-space:pre-wrap; }.typing { min-width:150px; display:flex; align-items:center; gap:5px; color:#8c8c8c; }.typing i { width:5px; height:5px; background:#8c8c8c; border-radius:50%; animation:pulse 1s infinite; }.typing i:nth-child(2){animation-delay:.16s}.typing i:nth-child(3){animation-delay:.32s}.typing span { margin-left:4px; font-size:10px; }.message-error { margin-top:9px; padding-top:9px; display:grid; gap:3px; border-top:1px solid #ffccc7; }.message-error strong { color:#cf1322; font-size:11px; }.message-error small { color:#8c8c8c; font-size:9px; word-break:break-all; }.message-error .el-button { width:max-content; height:auto; margin-top:3px; padding:0; }.message-meta { margin-top:5px; display:flex; align-items:center; gap:8px; color:#8c8c8c; font-size:9px; }.message-meta button { padding:0; color:#1677ff; background:none; border:0; font-size:9px; cursor:pointer; }.chat-compose { flex:none; display:flex; align-items:flex-end; gap:9px; padding-top:11px; border-top:1px solid #f0f0f0; }.chat-compose .el-button { min-width:72px; }.chat-notice { margin-top:8px; color:#8c8c8c; text-align:center; }
@keyframes pulse { 50% { opacity:.28; transform:translateY(-2px); } }
@media (max-width:1120px) { .ai-workbench { grid-template-columns:1fr; }.control-card { height:auto; min-height:0; }.control-card :deep(.el-card__body) { height:auto; }.control-scroll { max-height:560px; }.ai-chat-card { height:650px; } }
@media (max-width:640px) { .ai-runtime { align-items:flex-start; flex-direction:column; }.runtime-actions,.runtime-status { width:100%; white-space:normal; flex-wrap:wrap; }.runtime-actions { align-items:flex-start; }.runtime-actions .el-button { margin-left:auto; }.ai-chat-card { height:580px; }.chat-header { align-items:flex-start; gap:8px; }.chat-header>div:last-child { flex-wrap:wrap; justify-content:flex-end; }.message-content { max-width:88%; }.chat-compose .el-button { min-width:58px; } }
</style>
