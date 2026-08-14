import { ElMessage } from 'element-plus'

export const session = {
  get token() { return localStorage.getItem('iot_token') || '' },
  get tenant() { return localStorage.getItem('iot_tenant') || '' },
  save(data) {
    localStorage.setItem('iot_token', data.accessToken)
    localStorage.setItem('iot_tenant', data.tenantId || '')
  },
  clear() { localStorage.removeItem('iot_token'); localStorage.removeItem('iot_tenant') }
}

export async function api(path, options = {}) {
  const headers = { ...(options.body instanceof FormData ? {} : { 'Content-Type': 'application/json' }), ...(options.headers || {}) }
  if (session.token) headers.Authorization = `Bearer ${session.token}`
  const response = await fetch(path, { ...options, headers })
  const data = await response.json().catch(() => ({}))
  if (!response.ok) {
    if (response.status === 401 && path !== '/api/v1/auth/login') window.dispatchEvent(new Event('iot:unauthorized'))
    throw new Error(data.detail || `HTTP ${response.status}`)
  }
  return data
}

export async function download(path, filename, options = {}) {
  const headers = { ...(options.headers || {}) }
  if (session.token) headers.Authorization = `Bearer ${session.token}`
  const response = await fetch(path, { ...options, headers })
  if (!response.ok) {
    const data = await response.json().catch(() => ({}))
    throw new Error(data.detail || `HTTP ${response.status}`)
  }
  const url = URL.createObjectURL(await response.blob())
  const anchor = document.createElement('a')
  anchor.href = url; anchor.download = filename; anchor.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

export function notifyError(error) { ElMessage.error(error?.message || String(error)) }
export const formatTime = value => value ? new Date(Number(value)).toLocaleString() : '—'
export const pretty = value => JSON.stringify(value, null, 2)
export function parseJSON(value, label = 'JSON') {
  try { return JSON.parse(value || '{}') } catch { throw new Error(`${label} 格式不正确`) }
}
