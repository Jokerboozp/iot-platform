import mqtt from 'mqtt'
import { api } from './api'

let client
let refreshTimer

export function stopRealtime() {
  clearTimeout(refreshTimer)
  client?.end(true)
  client = undefined
}

export async function startRealtime(onMessage) {
  stopRealtime()
  try {
    const auth = await api('/api/v1/mqtt/token', { method: 'POST' })
    client = mqtt.connect(auth.websocketUrl, {
      username: auth.username,
      password: auth.token,
      clientId: `iot-web-${crypto.randomUUID()}`,
      clean: true,
      reconnectPeriod: 3000,
      connectTimeout: 10000
    })
    client.on('connect', () => client.subscribe(auth.subscriptions || [], { qos: 1 }))
    client.on('message', (topic, payload) => onMessage?.(topic, payload.toString()))
    refreshTimer = setTimeout(() => startRealtime(onMessage), Math.max(60000, ((auth.expiresIn || 900) - 90) * 1000))
  } catch {
    refreshTimer = setTimeout(() => startRealtime(onMessage), 10000)
  }
}
