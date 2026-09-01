<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { AlertTriangle, BellRing, Clock3, Volume2, X } from '@lucide/vue'
import { formatTime, session } from '../api'
import { alarmSources, alarmType, label } from '../labels'
import {
  alertKeys,
  alertTagType,
  DEFAULT_ALERT_SETTINGS,
  isWithinQuietHours,
  loadAlertSettings,
  normalizeAlertSettings,
  parseRealtimeAlert,
  playAlarmTone,
  saveAlertSettings
} from '../globalAlert'

const emit = defineEmits(['navigate'])

const popupAlerts = ref([])
const settingsVisible = ref(false)
const settings = reactive({ ...DEFAULT_ALERT_SETTINGS })
const settingsDraft = reactive({ ...DEFAULT_ALERT_SETTINGS })
const storageIdentity = { tenant: session.tenant, user: session.user }
const seenAlerts = new Map()

const quietHoursLabel = computed(() => {
  if (!settings.quietStart || !settings.quietEnd || settings.quietStart === settings.quietEnd) return '未设置'
  return `${settings.quietStart} - ${settings.quietEnd}`
})

function currentQuietHours() {
  return isWithinQuietHours(new Date(), settings)
}

function clearExpiredAlertKeys() {
  const now = Date.now()
  for (const [key, expiresAt] of seenAlerts) {
    if (expiresAt <= now) seenAlerts.delete(key)
  }
}

function isDuplicate(alert) {
  clearExpiredAlertKeys()
  const keys = alertKeys(alert)
  if (keys.some(key => seenAlerts.has(key))) return true
  for (const key of keys) seenAlerts.set(key, Date.now() + 10 * 60 * 1000)
  return false
}

function handleRealtime(event) {
  const alert = parseRealtimeAlert(event?.detail?.topic, event?.detail?.payload)
  if (!alert || isDuplicate(alert)) return

  const quiet = currentQuietHours()
  if (settings.soundEnabled && !quiet) void playAlarmTone()
  if (!settings.popupEnabled || quiet) return
  popupAlerts.value = [alert, ...popupAlerts.value].slice(0, 3)
}

function dismissAlert(id) {
  popupAlerts.value = popupAlerts.value.filter(item => item.id !== id)
}

function viewAlert(alert) {
  dismissAlert(alert.id)
  if (alert.alarmId) {
    emit('navigate', 'alarms', { alarmId: alert.alarmId })
    return
  }
  if (alert.messageId) {
    emit('navigate', 'raw', { messageId: alert.messageId, deviceId: alert.deviceId })
    return
  }
  emit('navigate', 'alarms')
}

function openSettings() {
  Object.assign(settingsDraft, settings)
  settingsVisible.value = true
}

function saveSettingsForm() {
  const next = normalizeAlertSettings(settingsDraft)
  Object.assign(settings, next)
  saveAlertSettings(window.localStorage, storageIdentity, next)
  if (!next.popupEnabled || currentQuietHours()) popupAlerts.value = []
  settingsVisible.value = false
  ElMessage.success('告警提醒设置已保存')
}

async function testSound() {
  const played = await playAlarmTone()
  if (played) ElMessage.success('警报声试听已播放')
  else ElMessage.warning('当前浏览器不支持声音播放，请检查浏览器权限')
}

defineExpose({ openSettings })

onMounted(() => {
  Object.assign(settings, loadAlertSettings(window.localStorage, storageIdentity))
  window.addEventListener('iot:realtime', handleRealtime)
})

onBeforeUnmount(() => {
  window.removeEventListener('iot:realtime', handleRealtime)
})
</script>

<template>
  <TransitionGroup v-if="popupAlerts.length" name="global-alert" tag="section" class="global-alert-popups" aria-label="实时报警通知">
    <article v-for="item in popupAlerts" :key="item.id" class="global-alert-popup" :class="`is-${item.kind}`" role="alertdialog" aria-live="assertive">
      <div class="global-alert-head">
        <div class="global-alert-icon"><component :is="item.kind === 'fault' ? AlertTriangle : BellRing" /></div>
        <div class="global-alert-title">
          <span class="global-alert-kicker">{{ item.kind === 'fault' ? '设备故障' : '实时报警' }}</span>
          <h3>{{ item.alarmTypeLabel || alarmType(item.alarmType) }}</h3>
          <span>{{ item.deviceName || item.deviceId || '未知设备' }}</span>
        </div>
        <button class="global-alert-close" type="button" aria-label="关闭报警提示" @click="dismissAlert(item.id)"><X /></button>
      </div>

      <div class="global-alert-meta">
        <el-tag :type="alertTagType(item.alarmLevel)" round>{{ item.alarmLevelLabel }}</el-tag>
        <span>{{ formatTime(item.timestamp) }}</span>
        <span v-if="item.source">来源：{{ label(alarmSources, item.source, item.source) }}</span>
      </div>
      <p v-if="item.detail" class="global-alert-detail">{{ item.detail }}</p>
      <div class="global-alert-identifiers">
        <span>设备 ID <code>{{ item.deviceId || '—' }}</code></span>
        <span v-if="item.alarmId">告警编号 <code>{{ item.alarmId }}</code></span>
        <span v-else-if="item.messageId">消息 ID <code>{{ item.messageId }}</code></span>
      </div>
      <div class="global-alert-actions">
        <el-button size="small" type="danger" plain @click="viewAlert(item)">{{ item.alarmId ? '查看告警详情' : '查看原始报文' }}</el-button>
        <el-button size="small" @click="dismissAlert(item.id)">关闭提示</el-button>
      </div>
    </article>
  </TransitionGroup>

  <el-dialog v-model="settingsVisible" title="告警提醒设置" width="min(540px, calc(100vw - 24px))">
    <div class="alert-settings">
      <div class="alert-setting-row">
        <div class="alert-setting-copy">
          <strong>显示报警弹窗</strong>
          <span>关闭后不显示右下角弹窗，告警仍会保留在告警中心。</span>
        </div>
        <el-switch v-model="settingsDraft.popupEnabled" active-text="开启" inactive-text="关闭" />
      </div>
      <div class="alert-setting-row alert-setting-row-stack">
        <div class="alert-setting-copy">
          <strong>弹窗静默时段</strong>
          <span>每天该时段不显示弹窗，也不播放警报声；留空表示不设置。</span>
        </div>
        <div class="alert-quiet-times">
          <el-time-picker v-model="settingsDraft.quietStart" value-format="HH:mm" format="HH:mm" placeholder="开始时间" clearable />
          <span>至</span>
          <el-time-picker v-model="settingsDraft.quietEnd" value-format="HH:mm" format="HH:mm" placeholder="结束时间" clearable />
        </div>
      </div>
      <div class="alert-setting-row">
        <div class="alert-setting-copy">
          <strong>播放警报声</strong>
          <span>新报警或故障到达时播放提示音，静默时段除外。</span>
        </div>
        <div class="alert-sound-setting"><el-switch v-model="settingsDraft.soundEnabled" active-text="开启" inactive-text="关闭" /><el-button size="small" plain @click="testSound"><Volume2 />试听</el-button></div>
      </div>
      <el-alert type="info" :closable="false" show-icon>
        <template #title>当前静默时段：{{ quietHoursLabel }}</template>
      </el-alert>
      <div class="alert-settings-note"><Clock3 /> 设置仅保存在当前浏览器的当前租户和用户下。</div>
    </div>
    <template #footer>
      <el-button @click="settingsVisible = false">取消</el-button>
      <el-button type="primary" @click="saveSettingsForm">保存设置</el-button>
    </template>
  </el-dialog>
</template>
