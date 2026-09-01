<script setup>
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import {
  Bell,
  Boxes,
  ChartNoAxesCombined,
  Cpu,
  Database,
  FileText,
  FlaskConical,
  LayoutDashboard,
  Library,
  LogOut,
  MessageCircle,
  Network,
  PanelLeftClose,
  PanelLeftOpen,
  Settings2,
  Upload,
  Video
} from '@lucide/vue'
import Avatar from './components/ui/Avatar.vue'
import Button from './components/ui/Button.vue'
import Input from './components/ui/Input.vue'
import Label from './components/ui/Label.vue'
import { api, notifyError, session } from './api'
import { alarmType } from './labels'
import { startRealtime, stopRealtime } from './realtime'

const DashboardView = defineAsyncComponent(() => import('./views/DashboardView.vue'))
const DevicesView = defineAsyncComponent(() => import('./views/DevicesView.vue'))
const ProductsView = defineAsyncComponent(() => import('./views/ProductsView.vue'))
const ProtocolsView = defineAsyncComponent(() => import('./views/ProtocolsView.vue'))
const ProtocolAssistantView = defineAsyncComponent(() => import('./views/ProtocolAssistantView.vue'))
const IntegrationView = defineAsyncComponent(() => import('./views/IntegrationView.vue'))
const TestDeviceView = defineAsyncComponent(() => import('./views/TestDeviceView.vue'))
const CameraMappingsView = defineAsyncComponent(() => import('./views/CameraMappingsView.vue'))
const AlarmsView = defineAsyncComponent(() => import('./views/AlarmsView.vue'))
const HealthInspectionView = defineAsyncComponent(() => import('./views/HealthInspectionView.vue'))
const RawView = defineAsyncComponent(() => import('./views/RawView.vue'))
const RulesView = defineAsyncComponent(() => import('./views/RulesView.vue'))
const KnowledgeView = defineAsyncComponent(() => import('./views/KnowledgeView.vue'))
const AiView = defineAsyncComponent(() => import('./views/AiView.vue'))
const BackupsView = defineAsyncComponent(() => import('./views/BackupsView.vue'))

const authenticated = ref(Boolean(session.token))
const active = ref('dashboard')
const collapsed = ref(false)
const pageKey = ref(0)
const loginLoading = ref(false)
const loginForm = ref({ tenantId: 'tenant_001', username: 'admin', password: 'admin123' })
const identity = ref({ tenant: session.tenant, user: session.user, role: session.role })
const currentTenant = computed(() => identity.value.tenant || loginForm.value.tenantId || '—')
const currentUser = computed(() => identity.value.user || loginForm.value.username || '账户')
const currentRole = computed(() => ({ admin: '管理员', operator: '运维人员', viewer: '访客' }[identity.value.role] || identity.value.role || '平台用户'))

const pages = {
  dashboard: { title: '运行总览', sub: '城市级消防感知与告警态势', icon: LayoutDashboard, component: DashboardView },
  devices: { title: '设备管理', sub: '注册、启停、凭证和实时状态统一管理', icon: Cpu, component: DevicesView },
  products: { title: '产品管理', sub: '产品模型与协议包绑定', icon: Boxes, component: ProductsView },
  protocols: { title: '协议开发', sub: '协议包版本、解析器配置与样本调试', icon: Network, component: ProtocolsView },
  protocolAssistant: { title:'协议接入助手', sub: '上传点表，生成并确认 Go 协议映射', icon: Network, component: ProtocolAssistantView },
  integration: { title:'接入指南', sub: '真实设备 HTTP / MQTT 参数与数据联调', icon: Upload, component: IntegrationView },
  testDevice: { title:'测试设备', sub: '模板化发送数据、事件、报警和恢复报文', icon: FlaskConical, component: TestDeviceView },
  cameras: { title: '摄像头映射', sub: '视频平台摄像头、空间位置与物联设备关联', icon: Video, component: CameraMappingsView },
  alarms: { title: '告警中心', sub: '告警确认、恢复与闭环处置', icon: Bell, component: AlarmsView },
  inspection: { title:'智能巡检', sub: '设备健康、数据新鲜度与活动告警分析', icon: ChartNoAxesCombined, component: HealthInspectionView },
  raw: { title: '原始报文', sub: '证据链检索、审计与回放', icon: FileText, component: RawView },
  rules: { title: '告警规则', sub: '可审计的动态规则与 AI 草稿', icon: Settings2, component: RulesView },
  knowledge: { title: 'Agent 知识库', sub: '文档直接归属 Agent，并使用持久化向量索引', icon: Library, component: KnowledgeView },
  ai: { title: 'AI 工作流', sub: 'DeepSeek Harness 插件、受控工具与知识库问答', icon: MessageCircle, component: AiView },
  backups: { title: '备份中心', sub: '备份记录、文件校验与恢复演练', icon: Database, component: BackupsView }
}
const current = computed(() => pages[active.value])
const menuGroups = [
  { label: '控制中心', items: ['dashboard'] },
  { label:'设备与数据', items: ['devices', 'products', 'protocols', 'protocolAssistant', 'integration', 'testDevice', 'cameras'] },
  { label: '运行中心', items: ['alarms', 'inspection', 'raw', 'rules', 'knowledge', 'ai', 'backups'] }
]

async function login() {
  loginLoading.value = true
  try {
    const data = await api('/api/v1/auth/login', { method: 'POST', body: JSON.stringify(loginForm.value) })
    session.save(data, loginForm.value.username)
    identity.value = { tenant: data.tenantId || '', user: loginForm.value.username, role: data.role || '' }
    authenticated.value = true
    active.value = 'dashboard'
    connect()
  } catch (error) {
    notifyError(error)
  } finally {
    loginLoading.value = false
  }
}

function logout() {
  stopRealtime()
  session.clear()
  identity.value = { tenant: '', user: '', role: '' }
  authenticated.value = false
}

function openPage(name, detail) {
  active.value = name
  pageKey.value++
  if (detail) sessionStorage.setItem('iot:navigation-detail', JSON.stringify(detail))
}

function handleUIAction(payload) {
  try {
    const event = JSON.parse(payload)
    const action = event?.action || {}
    if (action.type === 'OPEN_CAMERA' && typeof action.cameraId === 'string' && action.cameraId) {
      openPage('cameras', { cameraId: action.cameraId, actionId: event.id })
      ElMessage.info(`规则联动：已定位摄像头信息 ${action.cameraId}`)
      return
    }
    const allowedPages = new Set(['dashboard', 'devices', 'products', 'protocols', 'protocolAssistant', 'integration', 'testDevice', 'cameras', 'alarms', 'inspection', 'raw', 'rules', 'knowledge', 'ai', 'backups'])
    if (action.type === 'OPEN_PAGE' && allowedPages.has(action.page)) {
      openPage(action.page)
      ElMessage.warning('规则联动：已打开相关业务页面')
    }
  } catch {
    // Ignore malformed or unsupported UI actions.
  }
}

function connect() {
  startRealtime((topic, payload) => {
    if (topic.includes('/alarm/')) {
      try {
        const alarm = JSON.parse(payload)
        ElMessage.warning(`新告警：${alarmType(alarm.alarmType)} · ${alarm.deviceName || alarm.deviceId || ''}`)
      } catch {
        // Ignore malformed notification payloads.
      }
    }
    if (topic.includes('/ui-action/')) handleUIAction(payload)
    window.dispatchEvent(new CustomEvent('iot:realtime', { detail: { topic, payload } }))
  })
}

function unauthorized() {
  logout()
  ElMessage.error('登录已过期，请重新登录')
}

onMounted(() => {
  window.addEventListener('iot:unauthorized', unauthorized)
  if (authenticated.value) connect()
})

onBeforeUnmount(() => {
  window.removeEventListener('iot:unauthorized', unauthorized)
  stopRealtime()
})
</script>

<template>
  <el-config-provider :locale="zhCn" size="small">
    <div v-if="!authenticated" class="login-page">
      <section class="login-hero">
        <div class="hero-brand"><span>IoT</span><strong>消防智联平台</strong></div>
        <div>
          <span class="eyebrow">FIRE SAFETY · IOT PLATFORM</span>
          <h1>连接设备，洞察现场，<br />驱动消防业务。</h1>
          <p>统一管理产品、设备、规则与视频资源，让城市消防物联数据在一个平台内完成接入、监控和智能研判。</p>
        </div>
        <div class="hero-tags"><span>统一设备模型</span><span>实时规则引擎</span><span>AI 辅助研判</span></div>
      </section>

      <section class="login-panel">
        <form class="login-form" @submit.prevent="login">
          <span class="form-kicker">欢迎回来</span>
          <h2>登录管理平台</h2>
          <p>使用您的平台账号继续</p>
          <div class="login-fields">
            <div class="login-field">
              <Label for="tenant-id">租户</Label>
              <Input id="tenant-id" v-model="loginForm.tenantId" autocomplete="organization" />
            </div>
            <div class="login-field">
              <Label for="username">用户名</Label>
              <Input id="username" v-model="loginForm.username" autocomplete="username" />
            </div>
            <div class="login-field">
              <Label for="password">密码</Label>
              <Input id="password" v-model="loginForm.password" type="password" autocomplete="current-password" />
            </div>
          </div>
          <Button type="submit" class="login-submit" :loading="loginLoading">进入平台</Button>
        </form>
      </section>
    </div>

    <div v-else class="app-shell">
      <aside class="app-aside" :class="{ 'is-collapsed': collapsed }">
        <div class="brand"><span>IoT</span><div v-show="!collapsed"><strong>消防智联</strong><small>物联网管理平台</small></div></div>
        <div class="menu-scroll">
          <div class="menu-scroll-inner">
            <template v-for="group in menuGroups" :key="group.label">
              <div v-show="!collapsed" class="menu-group">{{ group.label }}</div>
              <button v-for="name in group.items" :key="name" class="menu-item" :class="{ active: active === name }" :aria-current="active === name ? 'page' : undefined" @click="openPage(name)">
                <component :is="pages[name].icon" />
                <span v-show="!collapsed">{{ pages[name].title }}</span>
              </button>
            </template>
          </div>
        </div>
        <button class="logout-button" @click="logout"><LogOut /><span v-show="!collapsed">退出登录</span></button>
      </aside>

      <main class="app-main">
        <header class="topbar">
          <div class="title-area">
            <button class="collapse-button" aria-label="折叠菜单" @click="collapsed = !collapsed"><component :is="collapsed ? PanelLeftOpen : PanelLeftClose" /></button>
            <div><span>首页 / {{ current.title }}</span><h2>{{ current.title }}</h2><p>{{ current.sub }}</p></div>
          </div>
          <div class="top-actions">
            <div class="health"><i />服务正常</div>
            <span class="tenant-pill">{{ currentTenant }}</span>
            <div class="account"><Avatar>{{ currentUser.slice(0, 1) }}</Avatar><span>{{ currentUser }} · {{ currentRole }}</span></div>
          </div>
        </header>
        <section class="main-content"><component :is="current.component" :key="`${active}-${pageKey}`" @navigate="openPage" /></section>
      </main>
    </div>
  </el-config-provider>
</template>
