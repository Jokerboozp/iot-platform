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

test('frontend builds independently and proxies backend routes', async () => {
  const vite = await readFile(new URL('vite.config.js', root), 'utf8')
  const nginx = await readFile(new URL('nginx.conf', root), 'utf8')
  assert.match(vite, /outDir:\s*'dist'/)
  assert.doesNotMatch(vite, /internal\/httpapi\/static/)
  for (const route of ['/api/', '/health/', '/mcp']) assert.ok(nginx.includes(route), `nginx is missing ${route}`)
  assert.ok(nginx.includes('platform-api:8080'))
})
