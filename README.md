# 消防 IoT 平台

面向消防物联网场景的一体化管理平台。项目由 Go API、Vue 3 Web、EMQX、PostgreSQL、Redis、ClickHouse、Redpanda、MinIO 以及可选 AI 运行时组成，覆盖设备接入、协议解析、状态管理、告警、视频流预览、原始报文证据链、回放、AI 研判、MCP 和备份恢复。

项目既可以使用内存适配器快速开发，也可以通过 Docker Compose 启动完整基础设施。

## 快速开始

### Docker Compose

环境要求：

- Docker Desktop 或 Docker Engine + Compose Plugin
- 首次构建 DeepSeek Harness 时需要访问 GitHub 和 npm registry

启动基础平台：

```bash
docker compose up -d --build
docker compose ps
```

浏览器打开：

```text
http://localhost:8080
```

本地开发默认账号：

```text
租户：tenant_001
用户名：admin
密码：admin123
```

查看日志：

```bash
docker compose logs -f platform-api platform-web
```

停止服务但保留数据：

```bash
docker compose down
```

> `docker compose down -v` 会删除数据库、对象存储等持久化卷。除非确认数据可以丢弃，否则不要执行。

### 本地源码启动

后端要求 Go 1.25.5 或更高版本。前端要求 Node.js `^20.19.0` 或 `>=22.12.0`，推荐 Node.js 22 LTS。

macOS / Linux：

```bash
IOT_HTTP_ADDR=:8081 \
IOT_CORS_ALLOWED_ORIGINS=http://localhost:5173 \
go run ./cmd/iot-platform
```

另开终端启动前端：

```bash
cd frontend
npm ci
npm run dev
```

访问 `http://localhost:5173`。Vite 会把 API 和 MCP 请求代理到 `http://localhost:8081`。

PowerShell：

```powershell
$env:IOT_HTTP_ADDR=':8081'
$env:IOT_CORS_ALLOWED_ORIGINS='http://localhost:5173'
go run ./cmd/iot-platform

# 另开终端
cd frontend
npm.cmd ci
npm.cmd run dev
```

未配置外部存储时，后端使用内存业务仓储和 `./data/objects` 本地归档，适合开发与自动化测试，不用于生产。

## 界面功能

| 页面 | 功能 |
|---|---|
| 运行总览 | 设备、在线率、告警趋势和重点态势 |
| 设备管理 | 注册、启停、凭证轮换、连接指南和实时状态 |
| 产品管理 | 产品模型、物模型属性和协议包绑定 |
| 协议开发 | 协议包版本、解析器配置和样本调试 |
| 数据接入 | HTTP/MQTT 接入参数和在线联调 |
| 摄像头映射 | 摄像机与设备/空间关联、视频流预览 |
| 告警中心 | 告警查询、确认、抑制、恢复和关闭 |
| 原始报文 | 证据链检索、下载、审计和回放 |
| 告警规则 | JSON/Gengine 规则、冲突检测和 AI 草稿 |
| 知识库管理 | 上传消防规范、设备手册和处置 SOP，查看索引状态 |
| AI 工作流 | DeepSeek Harness 插件、Provider 测试、流式对话和工具轨迹 |

“AI 告警研判”是业务工作流，不是独立菜单。进入“AI 工作流”后，在左侧“运行工作流”卡片的“工作流插件”下拉框中选择：

- AI 告警研判
- AI 运维助手

## 核心能力

| 领域 | 实现 |
|---|---|
| 设备与产品 | 产品、协议包、设备注册、设备/网关凭证、发现设备转注册 |
| 数据接入 | 管理端 REST、设备凭证 HTTP、MQTT 原始 Topic、设备在线调试 |
| 原始证据链 | gzip JSONL 微批归档、SHA-256、幂等索引、单条精确恢复 |
| 协议解析 | JSON、消防烟感 Hex、Modbus 示例解析器，失败 Topic 与 DLQ |
| 分层存储 | MinIO、Redpanda、ClickHouse、Redis、PostgreSQL、可选 Weaviate |
| 状态管理 | ONLINE、OFFLINE、SUSPECTED_OFFLINE 与多维离线判定 |
| 规则与告警 | JSON/Gengine、物模型校验、冲突检测、告警生命周期和审计 |
| 实时推送 | EMQX JWT、租户 Topic ACL、WebSocket 状态和告警推送 |
| 视频 | HMAC Webhook、摄像头映射、融合告警、HLS/MP4/WebM 预览 |
| AI Provider | Eino 编排、Ollama、DeepSeek、OpenAI-compatible 适配器 |
| AI 工作流 | DeepSeek Harness 源码运行时、SSE、工具卡片、轨迹和取消 |
| MCP | Streamable HTTP、租户隔离、只读查询工具和调用审计 |
| 回放 | DRY_RUN、REINGEST、DIFF、指定解析器版本和限速 |
| 备份 | PostgreSQL/WAL、ClickHouse、Redis、MinIO-DR、校验与恢复演练 |
| 运维 | 健康检查、JSON 日志、Prometheus、Grafana、Loki、K8s 和压测器 |

## 系统架构

```text
设备 / 网关 / 视频平台
  -> HTTP / EMQX MQTT / HMAC Webhook
  -> Go API
     -> 原始报文归档与幂等索引
     -> Parser Registry
     -> 状态与规则引擎
     -> 告警生命周期
     -> PostgreSQL / Redis / ClickHouse / MinIO / Redpanda
     -> AI Provider / DeepSeek Harness / MCP
  -> Vue 3 Web（Nginx 同源代理）
```

原始报文主链路：

```text
EMQX/REST
  -> MinIO 或本地 gzip JSONL
  -> PostgreSQL raw_archive_index
  -> Redpanda iot.raw.message
  -> Parser Registry
  -> 标准属性/事件/状态消息
  -> PostgreSQL + ClickHouse + Redis
  -> 规则与告警
  -> EMQX 实时 Topic + AI/RAG 旁路分析
```

## 项目目录

```text
cmd/iot-platform              平台 API 主服务
cmd/backup-service            备份与恢复演练服务
cmd/loadgen                   HTTP/MQTT 压测器
internal/core                 状态、规则、告警、视频、AI、回放
internal/adapters             数据库、消息、对象存储、AI/RAG 适配器
internal/httpapi              Gin REST API、JWT/RBAC、Webhook
internal/mcpserver            平台 MCP 与 Harness 专用只读 MCP
frontend                      Vue 3、Element Plus、HLS.js、Nginx
deploy/deepseek-harness       Harness 网关、插件清单和构建适配
deploy/k8s                    Kubernetes 基线清单
ops                           Prometheus、Grafana、Loki、EMQX 配置
scripts                       上游源码拉取等项目脚本
docs                          AI Harness、ThingsPanel 和覆盖说明
```

## Docker 服务与端口

| 服务 | 默认地址 |
|---|---|
| 平台 Web | `http://localhost:8080` |
| 平台 API | `http://localhost:8081` |
| DeepSeek Harness | `http://127.0.0.1:8091` |
| 备份服务 | `http://127.0.0.1:8090` |
| EMQX MQTT | `tcp://localhost:1883` |
| EMQX WebSocket | `ws://localhost:8083/mqtt` |
| EMQX Dashboard | `http://localhost:18083` |
| MinIO Console | `http://localhost:9001` |
| Prometheus | `http://localhost:9090` |
| Grafana | `http://localhost:3000` |

健康检查：

```bash
curl http://localhost:8081/health/live
curl http://localhost:8081/health/ready
docker compose ps
```

Compose profiles：

| Profile | 用途 | 启动方式 |
|---|---|---|
| 无 | 完整平台基础服务 | `docker compose up -d --build` |
| `harness` | DeepSeek Harness 插件工作流 | `docker compose --profile harness up -d --build` |
| `ai` | Ollama 与 Weaviate | `docker compose --profile ai up -d --build` |
| `thingspanel` | ThingsPanel 上游前后端 | `docker compose --profile thingspanel up -d --build` |

## 配置与密钥

基础平台本地开发不要求创建 `.env`，Compose 已提供本地默认值。

`.env.example` 是完整部署变量模板，不是每次启动都必须复制的文件。对已有数据卷直接复制整份模板，会同时改变 PostgreSQL、Redis、ClickHouse、MinIO、JWT、管理员和 EMQX 凭据，可能导致服务认证失败。

规则：

- 只启用某个可选组件时，只在 `.env` 中加入该组件需要的变量。
- 不要把 `.env` 提交到 Git。
- 已有持久化卷时，修改数据库密码必须同步迁移数据库内部账号。
- 生产环境应使用 Vault、External Secrets 或部署平台的 Secret 管理。
- 示例密码只适用于本地开发。

### 仅启用 DeepSeek Harness 的最小 `.env`

```dotenv
DEEPSEEK_API_KEY=<your-deepseek-api-key>
IOT_AI_HARNESS_URL=http://deepseek-harness:8091
IOT_AI_HARNESS_TOKEN=<至少32个ASCII字符的随机内部密钥>
IOT_AI_HARNESS_MCP_URL=http://platform-api:8080/mcp/harness
IOT_AI_HARNESS_MODEL=deepseek-v4-flash
IOT_AI_HARNESS_TIMEOUT=90s
```

生成内部密钥：

```bash
openssl rand -hex 32
```

`IOT_AI_HARNESS_TOKEN` 只用于 Go API 与 Harness 侧车之间的内部认证，不是 DeepSeek API Key。

## DeepSeek Harness 插件工作流

平台把模型 Provider 与业务 AI 插件分成两层：

```text
AI 工作台
  -> Go API（登录、租户、SSE、审计）
  -> DeepSeek Harness 源码网关
  -> 业务插件 manifest
  -> Harness Agent Runtime
  -> 平台只读 MCP 工具
```

当前插件：

| ID | 界面名称 | 用途 |
|---|---|---|
| `ops-assistant` | AI 运维助手 | 设备状态、活动告警、趋势、知识库辅助排障 |
| `alarm-handler` | AI 告警研判 | 告警核验、影响判断、相似告警和处置建议 |
| `system-observer` | AI 系统状态助手 | 系统状态、产品与设备数量、在线离线、告警与资源统计 |

### 启动 Harness

拉取锁定版本的官方源码：

```bash
./scripts/fetch-deepseek-harness.sh
```

源码会进入被 Git 忽略的 `upstream/deepseek-harness`。上游提交由 `deploy/deepseek-harness/REVISION` 固定。

创建前一节所示的最小 `.env`，然后运行：

```bash
docker compose --profile harness up -d --build --force-recreate \
  platform-api deepseek-harness platform-web
```

验证：

```bash
docker compose --profile harness ps
curl http://127.0.0.1:8091/health
docker compose --profile harness logs -f deepseek-harness platform-api
```

登录后进入“AI 工作流”页面，在“运行工作流 → 工作流插件”中选择工作流。前端支持：

- SSE 流式输出
- 工具调用状态卡片
- 运行轨迹和 Trace ID
- 取消运行与重新执行
- 模型和最大输出长度配置
- 为每个工作流单独配置知识库绑定

### 工作流知识库绑定

展开“AI 工作流 → 工作流知识库绑定”，可以按租户为当前插件配置：

- 绑定一个或多个产品；留空表示当前租户的全部产品。
- 限定知识分类和必须包含的标签。
- 选择按需检索、每次强制检索或完全禁用知识库。
- 配置 TopK、最低相似度以及无证据时是否阻止回答。

绑定不是只写入提示词。平台会将约束写入每次运行的短期 Harness Token，MCP 服务端会覆盖模型提交的产品、分类、标签、TopK 和阈值，防止模型扩大检索范围。“强制检索”会在进入 Harness 前预召回知识；“必须有证据”在无匹配结果时会停止运行。

这部分与 Dify 的知识库工作流理念相似，但实现仍属于本项目：DeepSeek Harness 负责插件运行，Go API 负责租户、绑定策略、检索授权和审计，不需要额外部署 Dify。

### 新增业务插件

管理员可以直接在“AI 工作流 → 通过 JSON 创建 Agent”粘贴 Manifest 并提交。平台会执行两层校验：Go API 检查字段和只读工具白名单，Harness 再验证完整 Manifest。成功后 Agent 立即出现在工作流下拉框，不需要重建容器。

动态 Agent 保存在 `deepseek-harness-data` 卷的 `/data/plugins`，容器重启后仍然存在。内置 `alarm-handler`、`ops-assistant` 和 `system-observer` 禁止覆盖；自定义 Agent 使用相同 ID 再提交会更新。

可用的只读工具为：

```text
mcp__iot__query_system_overview
mcp__iot__query_device_latest
mcp__iot__query_alarm_list
mcp__iot__query_property_history
mcp__iot__query_similar_alarms
mcp__iot__query_knowledge_base
```

也可以继续通过代码维护内置插件。插件清单位于 `deploy/deepseek-harness/plugins/*.json`。复制现有清单后修改：

```json
{
  "schemaVersion": 1,
  "id": "my-workflow",
  "name": "我的 AI 插件",
  "description": "插件用途",
  "version": "1.0.0",
  "enabled": true,
  "persona": "受控角色提示和禁止事项",
  "defaultModel": "deepseek-v4-flash",
  "maxTokens": 4096,
  "allowedTools": ["mcp__iot__query_device_latest"]
}
```

修改后重新构建或重启 Harness。完整说明见 [DeepSeek Harness 业务插件运行时](docs/AI_PLUGIN_HARNESS.md)。

### Harness 安全边界

- 只开放设备状态、告警、属性历史、相似告警和知识库查询。
- 不加载 shell、文件系统、Skills、Jobs、子 Agent 或设备控制工具。
- 浏览器拿不到内部 MCP JWT。
- 每次运行使用短期、绑定租户/用户/Run ID 的 MCP Token。
- reasoning、完整工具参数和原始工具结果不会返回前端。
- 所有 MCP 工具调用进入平台审计。

## 其他 AI Provider

不使用 Harness 时，也可以启用 Eino Provider 链路，用于告警分析、规则草稿、报表和 Provider 在线测试。

DeepSeek 示例：

```dotenv
IOT_AI_PROVIDER=deepseek
IOT_AI_BASE_URL=https://api.deepseek.com
IOT_AI_MODEL=deepseek-v4-flash
DEEPSEEK_API_KEY=<your-api-key>
```

```bash
docker compose up -d --build --force-recreate platform-api platform-web
```

Ollama + Weaviate：

```bash
IOT_OLLAMA_URL=http://ollama:11434 \
IOT_WEAVIATE_URL=http://weaviate:8080 \
docker compose --profile ai up -d --build

docker compose exec ollama ollama pull qwen3:8b
docker compose restart platform-api
```

AI 页面中的 Provider 沙箱支持 DeepSeek、Ollama 和 OpenAI-compatible 临时连接测试。沙箱 API Key 不写入数据库或审计日志。

## 知识库管理

进入左侧“知识库管理”页面可以：

- 拖放或选择单个知识文档。
- 可选关联产品 ID。
- 设置知识分类与标签，供工作流绑定精确过滤。
- 上传原文件并自动提取、分片和建立索引。
- 查看当前租户的文档、索引状态、分片数、大小和上传时间。
- 直接跳转到“AI 工作流”验证知识检索。

支持 PDF、DOCX、PPTX、XLSX、ODT、ODP、ODS、HTML/XML 和 UTF-8 文本，单文件最大 32 MiB。扫描版 PDF 需要先完成 OCR。

未配置 `IOT_WEAVIATE_URL` 时使用本地内存索引：原文件和文档记录会持久保存，但 API 重启后检索索引需要重新建立。页面会显示相应警告；生产环境应启用 Weaviate 持久索引。

## 摄像头映射与视频流预览

摄像头映射用于把视频平台的 `cameraId` 与平台设备、建筑、区域等空间信息关联。视频告警到达后，平台可以结合附近传感器状态、设备属性和当前告警完成跨源研判。

浏览器预览支持：

- HLS：`.m3u8`
- MP4
- WebM

浏览器不能直接播放 RTSP/RTMP。需要先通过 MediaMTX、ZLMediaKit 等媒体网关转换为 HLS、WebRTC 或浏览器支持的文件流。

允许预览的媒体 Origin：

```dotenv
IOT_VIDEO_PREVIEW_ALLOWED_ORIGINS=https://video.example.internal,http://localhost:8888
IOT_VIDEO_PREVIEW_CSP_SOURCES=https://video.example.internal http://localhost:8888
```

`IOT_VIDEO_PREVIEW_ALLOWED_ORIGINS` 使用逗号分隔的精确 Origin；`IOT_VIDEO_PREVIEW_CSP_SOURCES` 使用空格分隔。HLS 主播放列表、子播放列表和分片使用到的 Origin 都必须包含。

未配置白名单时可以保存摄像头映射，但 API 会拒绝创建浏览器预览，防止利用媒体 URL 探测内网。

视频 Webhook 签名：

```text
X-Video-Platform-ID: video-platform-1
X-Timestamp: Unix 秒
X-Signature: hex(HMAC-SHA256(secret, timestamp + rawBody))
```

时间戳允许误差 5 分钟，`eventId` 用于幂等。

## API 快速验证

登录：

```bash
curl -sS http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123","tenantId":"tenant_001"}'
```

PowerShell：

```powershell
$login = Invoke-RestMethod -Method Post http://localhost:8080/api/v1/auth/login `
  -ContentType application/json `
  -Body '{"username":"admin","password":"admin123","tenantId":"tenant_001"}'
$headers = @{ Authorization = "Bearer $($login.accessToken)" }
```

主要接口：

```text
POST         /api/v1/auth/login
GET/POST/PUT /api/v1/products[/{id}]
GET/POST/PUT /api/v1/protocol-packages[/{id}]
POST         /api/v1/protocol-packages/{id}/test
GET/POST/PUT /api/v1/device-registry[/{id}]
POST         /api/v1/device-registry/{id}/credentials
GET          /api/v1/device-registry/{id}/connection-guide
POST         /api/v1/device-registry/{id}/debug
POST         /api/v1/device-ingest/{deviceId}
GET/POST     /api/v1/raw-messages
GET          /api/v1/raw-messages/{id}
POST         /api/v1/raw-messages/replay
GET          /api/v1/replays/{id}
GET          /api/v1/devices
GET          /api/v1/devices/{deviceId}/latest
GET          /api/v1/devices/{deviceId}/properties/history
GET/POST/PUT/DELETE /api/v1/rules[/{id}]
GET          /api/v1/alarms
GET          /api/v1/alarms/{id}
POST         /api/v1/alarms/{id}/actions
GET          /api/v1/ai/providers
POST         /api/v1/ai/providers/test
GET/POST     /api/v1/ai/workflows
GET/PUT      /api/v1/ai/workflows/{id}/knowledge-binding
POST         /api/v1/ai/chat
POST         /api/v1/ai/chat/stream
GET          /api/v1/ai/alarm-analysis/{alarmId}
POST         /api/v1/ai/rule-draft
POST         /api/v1/ai/reports
GET/POST     /api/v1/knowledge/documents
GET/POST/PUT /api/v1/integrations/video/cameras[/{id}]
POST         /api/v1/integrations/video/cameras/{id}/preview
POST         /api/v1/integrations/video/alarm
POST         /api/v1/mqtt/token
POST         /api/v1/device-mqtt/token
POST         /api/v1/integrations/thingspanel/sync
GET/POST/DELETE /mcp
POST         /mcp/harness
```

## Topic

Redpanda/Kafka：

```text
iot.raw.message             iot.parse.failed
iot.property.report         iot.event.report
iot.device.state            iot.video.alarm
iot.alarm.raised            iot.alarm.recovered
iot.alarm.confirmed         iot.alarm.ai-analysis
iot.replay.request          iot.dlq.{serviceName}
```

EMQX：

```text
/external/raw/{tenantId}/{productId}/{deviceId}
/jetlinks/raw/{tenantId}/{productId}/{deviceId}
/external/video/alarm/{tenantId}/{cameraId}
/iot/device/state/{tenantId}/{productId}/{deviceId}
/iot/alarm/{eventType}/{cityCode}/{districtCode}/{buildingId}/{deviceType}/{deviceId}
```

EMQX 使用 HS256 JWT、内嵌 ACL 和默认拒绝策略，匿名连接会被拒绝。生产安全基线见 [EMQX 生产安全配置](ops/emqx/PRODUCTION_SECURITY.md)。

## MCP

平台 MCP 端点为 `/mcp`，使用平台 Bearer JWT。工具包括：

```text
query_system_overview
query_device_latest
query_alarm_list
query_property_history
query_similar_alarms
query_knowledge_base
create_rule_draft
```

租户由认证上下文决定，调用方不能通过工具参数切换租户。规则工具只生成默认禁用的草稿。

Harness 使用独立的 `/mcp/harness`，仅接受平台签发的短期内部令牌和只读 scopes。

## 备份与恢复

`backup-service` 默认每天全量备份、每 15 分钟执行增量任务，覆盖 PostgreSQL/WAL、ClickHouse、Redis、MinIO、Redpanda/EMQX 配置和可选 Weaviate 快照。

手动备份与非破坏性恢复演练：

```powershell
$backupHeaders = @{Authorization='Bearer change-me-backup-admin-token'}
Invoke-RestMethod -Method Post 'http://localhost:8090/backup?type=FULL' -Headers $backupHeaders
Invoke-RestMethod -Method Post 'http://localhost:8090/restore/drill?backupId=latest' -Headers $backupHeaders
```

恢复演练只读取制品、校验哈希并记录结果，不覆盖当前数据。生产恢复必须先在隔离环境验证。

## 测试

后端：

```bash
go test -count=1 ./...
go test -race ./internal/core ./internal/httpapi
go vet ./...
```

前端：

```bash
cd frontend
npm ci
npm test
npm run build
```

Harness 网关：

```bash
node --test deploy/deepseek-harness/gateway.test.mjs
docker build -f deploy/deepseek-harness/Dockerfile -t iot-deepseek-harness:local .
docker run --rm --entrypoint node iot-deepseek-harness:local \
  /harness/examples/iot-ops-agent/runtime-smoke.mjs
```

部署配置：

```bash
docker compose --profile harness config --quiet
go test ./internal/deploycheck
```

## 压测

```powershell
go run ./cmd/loadgen -url http://localhost:8080 -token $login.accessToken `
  -transport http -profile steady -rate 10000 -duration 2h `
  -devices 300000 -workers 128 -report .\steady-10k.json

go run ./cmd/loadgen -url http://localhost:8080 -token $login.accessToken `
  -transport mqtt -mqtt-broker tcp://localhost:1883 -profile burst `
  -rate 10000 -burst-rate 50000 -burst-duration 15m -duration 30m
```

单机开发烟测不能替代生产容量验收。30k/50k 吞吐、长时间稳定性、HA 切换与 RPO/RTO 必须在目标集群验证。

## 常见问题

### 看不到“AI 告警研判”或工作流下拉框

1. 确认使用 `--profile harness` 启动：

   ```bash
   docker compose --profile harness ps
   ```

2. Harness 健康接口应返回 `pluginCount: 3`：

   ```bash
   curl http://127.0.0.1:8091/health
   ```

3. 确认 `.env` 中包含：

   ```dotenv
   IOT_AI_HARNESS_URL=http://deepseek-harness:8091
   ```

4. 重新创建 API、Harness 和 Web：

   ```bash
   docker compose --profile harness up -d --build --force-recreate \
     platform-api deepseek-harness platform-web
   ```

5. 退出旧会话，强制刷新并重新登录。进入“AI 工作流 → 运行工作流 → 工作流插件”。

### 修改 `.env` 后数据库认证失败

这通常是因为容器读到了新密码，但持久化卷中的数据库账号仍使用旧密码。

- 不要通过删除卷解决有数据的环境。
- 恢复原来的变量值，或在数据库内部安全地迁移账号密码。
- 只启用 Harness 时使用本文提供的最小 `.env`，不要复制完整模板。

查看具体错误：

```bash
docker compose logs --tail=100 platform-api postgres clickhouse emqx
```

### 前端仍显示旧界面

```bash
docker compose --profile harness up -d --build --force-recreate platform-web
```

然后退出登录并使用 `Command/Ctrl + Shift + R` 强制刷新。

### Harness 显示 MODEL_ERROR

- 检查 `DEEPSEEK_API_KEY` 是否有效。
- 检查模型和 Base URL 是否匹配。
- `MODEL_ERROR` 与插件加载失败不同；先查看 Harness 健康接口的 `pluginCount`。

## ThingsPanel

```bash
docker compose --profile thingspanel up -d --build
```

ThingsPanel Web 默认为 `http://localhost:8088`，API 默认为 `http://localhost:9999`。本项目不修改上游工作树，消防原始证据链、解析、告警、视频、AI 和回放继续由本平台负责。

详细边界和目录同步说明见 [ThingsPanel 二开集成](docs/THINGSPANEL_INTEGRATION.md)。

## 生产部署检查

- 替换所有示例密码、JWT 密钥、Harness Token 和视频平台 Secret。
- 使用 HTTPS、受限管理网络、TLS/mTLS 和企业身份源。
- 为 PostgreSQL、Redis、MinIO、Redpanda、ClickHouse 配置高可用与独立磁盘。
- 将密钥放入 Vault、External Secrets 或云 Secret Manager。
- 配置媒体 Origin 白名单和 Nginx CSP。
- 对原始报文下载、回放、告警操作和 AI 工具调用设置审计留存。
- 执行容量测试、故障注入、备份恢复演练和数据保留合规评审。
- Kubernetes 清单中的 Secret 仅为占位符，不可直接用于生产。

Kubernetes 基线位于 `deploy/k8s`。

## 相关文档

- [DeepSeek Harness 业务插件运行时](docs/AI_PLUGIN_HARNESS.md)
- [ThingsPanel 二开集成边界](docs/THINGSPANEL_INTEGRATION.md)
- [设计文档实现覆盖表](docs/IMPLEMENTATION_COVERAGE.md)
- [EMQX 生产安全配置](ops/emqx/PRODUCTION_SECURITY.md)
