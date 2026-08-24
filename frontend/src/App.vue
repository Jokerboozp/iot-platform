<script setup>
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import { Monitor, Cpu, Box, Connection, Upload, VideoCamera, Bell, Document, Operation, Collection, ChatDotRound, Fold, Expand, SwitchButton } from '@element-plus/icons-vue'
import { api, notifyError, session } from './api'
import { alarmType } from './labels'
import { startRealtime, stopRealtime } from './realtime'
const DashboardView = defineAsyncComponent(() => import('./views/DashboardView.vue'))
const DevicesView = defineAsyncComponent(() => import('./views/DevicesView.vue'))
const ProductsView = defineAsyncComponent(() => import('./views/ProductsView.vue'))
const ProtocolsView = defineAsyncComponent(() => import('./views/ProtocolsView.vue'))
const IntegrationView = defineAsyncComponent(() => import('./views/IntegrationView.vue'))
const CameraMappingsView = defineAsyncComponent(() => import('./views/CameraMappingsView.vue'))
const AlarmsView = defineAsyncComponent(() => import('./views/AlarmsView.vue'))
const RawView = defineAsyncComponent(() => import('./views/RawView.vue'))
const RulesView = defineAsyncComponent(() => import('./views/RulesView.vue'))
const KnowledgeView = defineAsyncComponent(() => import('./views/KnowledgeView.vue'))
const AiView = defineAsyncComponent(() => import('./views/AiView.vue'))

const authenticated = ref(Boolean(session.token))
const active = ref('dashboard')
const collapsed = ref(false)
const pageKey = ref(0)
const loginLoading = ref(false)
const loginForm = ref({ tenantId: 'tenant_001', username: 'admin', password: 'admin123' })
const identity = ref({ tenant:session.tenant, user:session.user, role:session.role })
const currentTenant = computed(() => identity.value.tenant || loginForm.value.tenantId || '—')
const currentUser = computed(() => identity.value.user || loginForm.value.username || '账户')
const currentRole = computed(() => ({ admin:'管理员', operator:'运维人员', viewer:'访客' }[identity.value.role] || identity.value.role || '平台用户'))

const pages = {
  dashboard: { title:'运行总览', sub:'城市级消防感知与告警态势', icon:Monitor, component:DashboardView },
  devices: { title:'设备管理', sub:'注册、启停、凭证和实时状态统一管理', icon:Cpu, component:DevicesView },
  products: { title:'产品管理', sub:'产品模型与协议包绑定', icon:Box, component:ProductsView },
  protocols: { title:'协议开发', sub:'协议包版本、解析器配置与样本调试', icon:Connection, component:ProtocolsView },
  integration: { title:'数据接入', sub:'HTTP / MQTT 接入参数和在线联调', icon:Upload, component:IntegrationView },
  cameras: { title:'摄像头映射', sub:'视频平台摄像头、空间位置与物联设备关联', icon:VideoCamera, component:CameraMappingsView },
  alarms: { title:'告警中心', sub:'告警确认、恢复与闭环处置', icon:Bell, component:AlarmsView },
  raw: { title:'原始报文', sub:'证据链检索、审计与回放', icon:Document, component:RawView },
  rules: { title:'告警规则', sub:'可审计的动态规则与 AI 草稿', icon:Operation, component:RulesView },
  knowledge: { title:'知识库管理', sub:'消防规范、设备手册与处置 SOP 索引', icon:Collection, component:KnowledgeView },
  ai: { title:'AI 工作流', sub:'DeepSeek Harness 插件、受控工具与知识库问答', icon:ChatDotRound, component:AiView }
}
const current = computed(() => pages[active.value])
const menuGroups = [
  { label:'控制中心', items:['dashboard'] },
  { label:'设备接入', items:['devices','products','protocols','integration','cameras'] },
  { label:'运行中心', items:['alarms','raw','rules','knowledge','ai'] }
]

async function login() {
  loginLoading.value = true
  try {
    const data = await api('/api/v1/auth/login', { method:'POST', body:JSON.stringify(loginForm.value) })
    session.save(data, loginForm.value.username)
    identity.value = { tenant:data.tenantId || '', user:loginForm.value.username, role:data.role || '' }
    authenticated.value = true; active.value = 'dashboard'; connect()
  } catch (error) { notifyError(error) } finally { loginLoading.value = false }
}
function logout() { stopRealtime(); session.clear(); identity.value = { tenant:'', user:'', role:'' }; authenticated.value = false }
function openPage(name, detail) { active.value = name; pageKey.value++; if (detail) sessionStorage.setItem('iot:navigation-detail', JSON.stringify(detail)) }
function connect() {
  startRealtime((topic, payload) => {
    if (topic.includes('/alarm/')) {
      try { const alarm = JSON.parse(payload); ElMessage.warning(`新告警：${alarmType(alarm.alarmType)} · ${alarm.deviceName || alarm.deviceId || ''}`) } catch { /* ignore malformed notification */ }
    }
    window.dispatchEvent(new CustomEvent('iot:realtime', { detail:{ topic, payload } }))
  })
}
function unauthorized() { logout(); ElMessage.error('登录已过期，请重新登录') }
onMounted(() => { window.addEventListener('iot:unauthorized', unauthorized); if (authenticated.value) connect() })
onBeforeUnmount(() => { window.removeEventListener('iot:unauthorized', unauthorized); stopRealtime() })
</script>

<template><el-config-provider :locale="zhCn" size="small">
  <div v-if="!authenticated" class="login-page">
    <section class="login-hero">
      <div class="hero-brand"><span>IoT</span><strong>消防智联平台</strong></div>
      <div><span class="eyebrow">FIRE SAFETY · IOT PLATFORM</span><h1>连接设备，洞察现场，<br>驱动消防业务。</h1><p>统一管理产品、设备、规则与视频资源，让城市消防物联数据在一个平台内完成接入、监控和智能研判。</p></div>
      <div class="hero-tags"><span>统一设备模型</span><span>实时规则引擎</span><span>AI 辅助研判</span></div>
    </section>
    <section class="login-panel">
      <el-form :model="loginForm" label-position="top" @submit.prevent="login">
        <span class="form-kicker">欢迎回来</span><h2>登录管理平台</h2><p>使用您的平台账号继续</p>
        <el-form-item label="租户"><el-input v-model="loginForm.tenantId" size="large" /></el-form-item>
        <el-form-item label="用户名"><el-input v-model="loginForm.username" size="large" /></el-form-item>
        <el-form-item label="密码"><el-input v-model="loginForm.password" type="password" show-password size="large" /></el-form-item>
        <el-button native-type="submit" type="primary" :loading="loginLoading">进入平台</el-button>
      </el-form>
    </section>
  </div>

  <el-container v-else class="app-shell">
    <el-aside :width="collapsed ? '52px' : '208px'" class="app-aside">
      <div class="brand"><span>IoT</span><div v-show="!collapsed"><strong>消防智联</strong><small>物联网管理平台</small></div></div>
      <el-scrollbar class="menu-scroll">
        <template v-for="group in menuGroups" :key="group.label">
          <div v-show="!collapsed" class="menu-group">{{ group.label }}</div>
          <button v-for="name in group.items" :key="name" class="menu-item" :class="{ active:active===name }" @click="openPage(name)"><el-icon><component :is="pages[name].icon" /></el-icon><span v-show="!collapsed">{{ pages[name].title }}</span></button>
        </template>
      </el-scrollbar>
      <button class="logout-button" @click="logout"><el-icon><SwitchButton /></el-icon><span v-show="!collapsed">退出登录</span></button>
    </el-aside>
    <el-container>
      <el-header class="topbar">
        <div class="title-area">
          <button class="collapse-button" aria-label="折叠菜单" @click="collapsed=!collapsed"><el-icon><component :is="collapsed ? Expand : Fold" /></el-icon></button>
          <div><span>首页 / {{ current.title }}</span><h2>{{ current.title }}</h2><p>{{ current.sub }}</p></div>
        </div>
        <div class="top-actions"><div class="health"><i />服务正常</div><span class="tenant-pill">{{ currentTenant }}</span><div class="account"><el-avatar :size="32">{{ currentUser.slice(0,1) }}</el-avatar><span>{{ currentUser }} · {{ currentRole }}</span></div></div>
      </el-header>
      <el-main class="main-content"><component :is="current.component" :key="`${active}-${pageKey}`" @navigate="openPage" /></el-main>
    </el-container>
  </el-container>
</el-config-provider></template>
