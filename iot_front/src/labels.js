export const alarmTypes = { FIRE_RISK:'火灾风险', FIRE:'火灾告警', SMOKE_DETECTED:'检测到烟雾', FLAME_DETECTED:'检测到火焰', HIGH_TEMPERATURE:'温度过高', DEVICE_OFFLINE:'设备离线', WATER_PRESSURE_LOW:'水压过低', WATER_LEVEL_ABNORMAL:'水位异常', ELECTRICAL_FIRE:'电气火灾', GAS_LEAK:'可燃气体泄漏', MANUAL_ALARM:'手动报警' }
export const alarmLevels = { CRITICAL:'紧急', HIGH:'高', MEDIUM:'中', LOW:'低', INFO:'提示' }
export const alarmStatuses = { ACTIVE:'活动中', ACKED:'已确认', RECOVERED:'已恢复', CLOSED:'已关闭', SUPPRESSED:'已抑制' }
export const alarmSources = { device:'设备上报', video:'视频分析', 'managed-device':'受管设备', 'external-ingest':'外部接入', mqtt:'消息队列接入', http:'网页接口接入', manual:'人工录入' }
export const enabledStatuses = { ENABLED:'已启用', DISABLED:'已停用', DRAFT:'草稿', PUBLISHED:'已发布' }
export const deviceRoles = { DIRECT:'直接设备', GATEWAY:'网关设备', CHILD:'子设备' }
export const businessStatuses = { ONLINE:'在线', OFFLINE:'离线', SUSPECTED_OFFLINE:'疑似离线', NEVER_SEEN:'从未上线', UNKNOWN:'未知' }
export const connectionStatuses = { CONNECTED:'已连接', DISCONNECTED:'未连接', UNKNOWN:'未知' }
export const dataStatuses = { ACTIVE:'数据活跃', SILENT:'数据静默', UNKNOWN:'未知' }
export const categories = { smoke:'烟雾探测器', fire:'火灾探测器', water_pressure:'水压传感器', camera:'摄像机', gateway:'边缘网关', sensor:'通用传感器', video_ai:'视频智能分析设备', other:'其他设备' }
export const parsers = { custom_json_parser:'通用结构化数据', configurable_json_parser:'配置驱动 JSON 映射', configurable_hex_parser:'配置驱动十六进制字段', javascript_sandbox_parser:'受限 JavaScript 解析器', go_protocol_parser:'可上传 Go 协议 Worker', gb26875_dahua_parser:'国标消防终端（大华 v1.03）', fire_smoke_parser:'烟感十六进制', modbus_parser:'工业总线寄存器' }
export const label = (map, value, fallback = '未知') => map[String(value ?? '')] || fallback
export const alarmType = value => label(alarmTypes, value, '其他告警类型')
export function tagType(value) {
  if (['ONLINE','ENABLED','PUBLISHED','RECOVERED'].includes(value)) return 'success'
  if (['ACTIVE','MEDIUM','ACKED'].includes(value)) return 'warning'
  if (['HIGH','CRITICAL'].includes(value)) return 'danger'
  return 'info'
}
