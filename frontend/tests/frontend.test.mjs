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

test('video preview and AI provider playground remain available', async () => {
  const source = await sourceText()
  for (const label of ['视频流预览', '浏览器可直接预览 HLS', 'Provider 临时测试', '连接并测试插件', 'AI 运维助手', '运行轨迹', '工具调用']) {
    assert.match(source, new RegExp(label), `missing feature label: ${label}`)
  }
  const packageJson = JSON.parse(await readFile(new URL('package.json', root), 'utf8'))
  assert.ok(packageJson.dependencies?.['hls.js'], 'hls.js is required for browser HLS playback')
})

test('AI workbench uses cancellable SSE workflows and stable message keys', async () => {
  const aiView = await readFile(new URL('src/views/AiView.vue', root), 'utf8')
  const apiSource = await readFile(new URL('src/api.js', root), 'utf8')
  const sseSource = await readFile(new URL('src/sse.js', root), 'utf8')

  assert.match(aiView, /api\('\/api\/v1\/ai\/workflows'\)/)
  assert.match(aiView, /apiStream\('\/api\/v1\/ai\/chat\/stream'/)
  assert.match(aiView, /new AbortController\(\)/)
  assert.match(aiView, /:key="message\.id"/)
  for (const field of ['question','conversationId','workflowId','model','maxTokens']) assert.match(aiView, new RegExp(`\\b${field}\\b`), `missing AI request field: ${field}`)
  for (const eventType of ['run.started','text.delta','tool.started','tool.completed','run.completed','run.failed']) assert.ok(aiView.includes(`'${eventType}'`), `missing stream event: ${eventType}`)
  assert.doesNotMatch(aiView, /event\.reasoning/)
  assert.doesNotMatch(aiView, /conversationId\.value\s*=\s*event\.conversationId/)
  assert.doesNotMatch(aiView, /model:[^\n]*runtime\.value\.active/)
  assert.match(aiView, /conversationId\.value\s*=\s*makeId\('conversation'\)/)
  assert.match(apiSource, /export async function apiStream/)
  assert.match(apiSource, /text\/event-stream/)
  assert.match(sseSource, /getReader\(\)/)
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

test('frontend rejects Node versions unsupported by the build toolchain', async () => {
  const packageJson = JSON.parse(await readFile(new URL('package.json', root), 'utf8'))
  const packageLock = JSON.parse(await readFile(new URL('package-lock.json', root), 'utf8'))
  const npmrc = await readFile(new URL('.npmrc', root), 'utf8')

  const supportedNodeVersions = '^20.19.0 || >=22.12.0'
  assert.equal(packageJson.engines?.node, supportedNodeVersions)
  assert.equal(packageLock.packages?.['']?.engines?.node, supportedNodeVersions)
  assert.match(npmrc, /^engine-strict=true\s*$/m)
})
