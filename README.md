# 消防 IoT 平台

这是一个模块化 Go 平台，负责产品、协议包、设备注册与凭证、HTTP/MQTT 数据接入、原始报文证据链、独立解析、分层存储、在线状态、消防告警、视频接入、回放、AI/RAG、MCP 工具和统一 API。本项目可以独立完成设备管理与数据接入。

## 已实现能力

| 领域 | 实现 |
|---|---|
| 平台底座 | 下载并锁定 ThingsPanel 官方前后端；二开扩展与上游隔离 |
| 设备管理 | 产品与协议包绑定、设备注册/启停、独立设备凭证、凭证轮换、接入指南和在线数据调试 |
| 原始数据 | MQTT `/external/raw/#`、设备凭证 HTTP 或管理端 REST 接入；先归档、再索引、后入内部消息总线；SHA-256 证据摘要和幂等键 |
| 解析标准化 | 协议包版本与发布状态、受控插件注册表、在线解析调试；JSON、消防烟感 Hex、Modbus 解析器；标准消息类型；解析失败 Topic/DLQ |
| 分层存储 | MinIO L0、Redpanda/Kafka L1、ClickHouse L2、Redis L3、PostgreSQL L4-L6、Weaviate/MinIO L7-L9；本地适配器用于零依赖开发 |
| 状态 | ONLINE/OFFLINE/SUSPECTED_OFFLINE；连接、数据、业务三状态；理论离线时间和扫描发现时间分离 |
| 告警 | JSON 条件与受控 Gengine 表达式、物模型字段校验、规则冲突检测、持续/恢复、活动告警 Redis 去重、确认/关闭/抑制、审计 |
| 实时推送 | EMQX JWT 认证与租户 Topic ACL；状态保留消息；六级告警 Topic；内置 MQTT over WebSocket 实时页面 |
| 视频 | HMAC-SHA256 Webhook、摄像机空间映射、幂等、置信度、传感器/视频融合、媒体白名单异步转存和失败恢复 |
| AI | Eino 工作流编排、Ollama 模型适配器、异步告警诊断、受控工具审计、Office/PDF RAG、规则草稿校验且默认禁用 |
| MCP | `mcp-go` Streamable HTTP `/mcp`；设备、告警、趋势、相似告警、知识库、规则草稿工具；JWT 租户隔离 |
| 回放 | 租户/产品/设备/时间筛选；DRY_RUN、REINGEST、真实 DIFF；指定 Parser 版本、限速、差异摘要和审计 |
| 备份 | PostgreSQL 全量/WAL、ClickHouse Native、Redis RDB/AOF、Redpanda/EMQX/配置、MinIO 版本控制与独立 DR、Weaviate 快照；校验和恢复演练 |
| 运维 | JSON 日志、健康检查、Prometheus 指标/告警、Grafana/Loki、Compose、K8s/HPA/NetworkPolicy、压测器 |
| API | 独立 Gin 服务；JWT/RBAC、路径参数、安全响应头、访问日志、CORS 白名单和 MCP，不包含前端静态资源 |
| Web | 独立 Vue 3 + Vite + Element Plus 单页应用；由 Nginx 托管并反向代理 API/MCP；按需加载与响应式布局 |

## 目录

```text
cmd/iot-platform       主服务
cmd/backup-service     备份服务
cmd/loadgen            稳定/突发流量压测器
internal/core          归档后处理、状态、规则、告警、视频、AI、回放
internal/adapters      PostgreSQL/Redis/MinIO/Kafka/MQTT/ClickHouse/AI/RAG 适配器
internal/httpapi       Gin REST API、JWT/RBAC、Webhook 与 MCP
internal/mcpserver     受控 MCP 工具
frontend               Vue 3 + Element Plus 前端源码、Vite 和 Nginx 配置
deploy/k8s             生产基线清单
ops                    Prometheus/Grafana/Loki/EMQX 安全基线
```

本地前后端分别启动。后端要求 Go 1.25.5 或更高版本；前端要求 Node.js `^20.19.0` 或 `>=22.12.0`（推荐 Node.js 22 LTS），npm 会在版本不兼容时终止安装：

```powershell
cd D:\iot\platform
$env:IOT_HTTP_ADDR=':8081'
$env:IOT_CORS_ALLOWED_ORIGINS='http://localhost:5173'
go run ./cmd/iot-platform

# 另开终端
cd D:\iot\platform\frontend
npm.cmd install
npm.cmd run dev       # http://localhost:5173，代理 API/MCP 到 localhost:8081
npm.cmd run build     # 独立构建到 frontend/dist
```

根目录 `Dockerfile` 只构建 Gin API；`frontend/Dockerfile` 只构建并托管 Vue。生产浏览器统一访问 Web，Nginx 将 `/api`、`/health` 和 `/mcp` 转发到 API。

## 1. 零依赖启动

只需要 Go 1.25.5 或更高版本即可单独启动 API：

```powershell
cd D:\iot\platform
$env:IOT_HTTP_ADDR=':8081'
go run ./cmd/iot-platform
```

API 地址为 `http://localhost:8081`；Web 请按上一节另行启动。开发默认账号：

```text
租户: tenant_001
用户名: admin
密码: admin123
```

零依赖模式使用内存业务存储和 `./data/objects` 本地归档，适合开发/测试，不用于生产。

## 2. 完整 Compose 启动

`compose.yaml` 将 API 与 Web 作为两个独立服务运行，同时内置 Prometheus、Grafana、Loki 配置；无需复制配置文件或预先创建 `.env`：

```powershell
cd D:\iot\platform
docker compose up -d --build
```

首次拉取镜像和编译完成后，可用以下命令查看状态与日志：

```powershell
docker compose ps
docker compose logs -f platform-api platform-web
```

Compose 内置的默认账号和密码只用于本地开发。正式部署前可把 `.env.example` 复制为 `.env` 并修改其中密钥，或在 shell/部署平台注入同名环境变量；这不会改变一条命令启动的方式。

主要入口：

| 服务 | 地址 |
|---|---|
| 平台 Web（含 API 反向代理） | `http://localhost:8080` |
| Gin API（直接调试） | `http://localhost:8081` |
| EMQX MQTT | `tcp://localhost:1883` |
| EMQX WebSocket | `ws://localhost:8083/mqtt` |
| EMQX Dashboard | `http://localhost:18083` |
| MinIO Console | `http://localhost:9001` |
| Prometheus | `http://localhost:9090` |
| Grafana | `http://localhost:3000` |

AI 组件使用可选 profile：

```powershell
$env:IOT_OLLAMA_URL='http://ollama:11434'
$env:IOT_WEAVIATE_URL='http://weaviate:8080'
docker compose --profile ai up -d --build
docker compose exec ollama ollama pull qwen3:8b
docker compose restart platform-api
```

也可以使用 DeepSeek/OpenAI-compatible Provider 插件。以下示例启用 DeepSeek；密钥只通过环境变量注入：

```powershell
$env:IOT_AI_PROVIDER='deepseek'
$env:IOT_AI_BASE_URL='https://api.deepseek.com'
$env:IOT_AI_MODEL='deepseek-v4-flash'
$env:DEEPSEEK_API_KEY='<your-api-key>'
docker compose up -d --build platform-api platform-web
```

进入“AI 运维助手”可以查看当前运行时状态，或在 Provider 插件沙箱中临时填写 DeepSeek、Ollama 或 OpenAI-compatible 配置并执行连接测试。沙箱中的 API Key 不会保存到数据库或审计日志。AI Provider 采用静态注册、运行时组装的插件模式，业务核心仍只依赖统一 `AIClient` 接口。

### DeepSeek Harness 源码模式

修改版“AI 运维助手”可以直接使用 DeepSeek Harness 源码侧车。源码固定到 `deploy/deepseek-harness/REVISION` 记录的提交；首次使用先拉取源码：

```bash
./scripts/fetch-deepseek-harness.sh
```

在 `.env` 中设置：

```text
DEEPSEEK_API_KEY=<your-api-key>
IOT_AI_HARNESS_URL=http://deepseek-harness:8091
IOT_AI_HARNESS_TOKEN=<至少 32 字节随机内部密钥>
IOT_AI_HARNESS_MCP_URL=http://platform-api:8080/mcp/harness
IOT_AI_HARNESS_MODEL=deepseek-v4-flash
IOT_AI_HARNESS_TIMEOUT=90s
```

然后从本地源码构建并启动：

```bash
docker compose --profile harness up -d --build platform-api deepseek-harness platform-web
docker compose logs -f deepseek-harness platform-api
```

Harness 侧车只加载 IoT 运维 Workflow 和平台提供的受控 MCP 只读工具，不加载 shell、文件写入、子 Agent 或设备控制能力。浏览器不会直接拿到 MCP 凭据；平台为每次运行签发短期、绑定租户和 Run ID 的内部令牌。业务插件清单位于 `deploy/deepseek-harness/plugins/`，新增或调整 Workflow 不需要修改模型 Provider。

上述环境变量均可写入可选的 `.env`。模型未启用时，主链路照常运行并保存“待人工研判”的降级 AI 结果。

## 3. 核心数据流程

```text
EMQX/REST 原始报文
  -> MinIO/本地 gzip JSONL 归档
  -> PostgreSQL raw_archive_index 幂等索引
  -> Kafka/Redpanda iot.raw.message
  -> Parser Registry
  -> 标准消息 Topic
  -> PostgreSQL + ClickHouse + Redis
  -> 动态规则和告警生命周期
  -> EMQX 实时 Topic + Kafka 告警 Topic
  -> 异步 AI/RAG 诊断
```

MinIO 归档器以 20ms/500 条为上限生成微批量 `jsonl.gz`，每条消息仍有独立 PostgreSQL 索引、SHA-256、行偏移和幂等键。写入顺序仍是“批对象成功 → 逐条索引 → Kafka”，读取和回放会按索引偏移从批对象精确恢复单条报文。

## 4. API 快速验证

登录：

```powershell
$login = Invoke-RestMethod -Method Post http://localhost:8080/api/v1/auth/login `
  -ContentType application/json `
  -Body '{"username":"admin","password":"admin123","tenantId":"tenant_001"}'
$headers = @{ Authorization = "Bearer $($login.accessToken)" }
```

创建规则：

```powershell
$rule = @{
  id='rule_temperature_smoke'; name='高温烟雾'; alarmType='FIRE_RISK';
  level='HIGH'; match='all'; enabled=$true;
  conditions=@(
    @{field='temperature';operator='>';value=80},
    @{field='smoke';operator='eq';value=$true}
  );
  recovery=@(@{field='temperature';operator='<=';value=70})
} | ConvertTo-Json -Depth 8
Invoke-RestMethod -Method Post http://localhost:8080/api/v1/rules -Headers $headers -ContentType application/json -Body $rule
```

测试原始报文：

```powershell
$raw = @{
  messageId='raw_demo_001'; tenantId='tenant_001'; productId='fire_smoke_json';
  deviceId='device_001'; protocol='json'; payloadFormat='json';
  payload=@{
    properties=@{temperature=88.5;smoke=$true;battery=91};
    tags=@{cityCode='city_001';districtCode='district_01';buildingId='A';deviceType='smoke'}
  }
} | ConvertTo-Json -Depth 10
Invoke-RestMethod -Method Post http://localhost:8080/api/v1/raw-messages -Headers $headers -ContentType application/json -Body $raw
```

常用接口：

```text
GET/POST     /api/v1/protocol-packages
PUT          /api/v1/protocol-packages/{id}
POST         /api/v1/protocol-packages/{id}/test
GET/POST     /api/v1/products
PUT          /api/v1/products/{id}
GET/POST     /api/v1/device-registry
PUT          /api/v1/device-registry/{id}
POST         /api/v1/device-registry/{id}/credentials
GET          /api/v1/device-registry/{id}/connection-guide
POST         /api/v1/device-registry/{id}/debug
POST         /api/v1/discovered-devices/{id}/register
POST         /api/v1/device-ingest/{deviceId}  # X-Device-Key + X-Device-Secret
GET/POST     /api/v1/rules
PUT/DELETE   /api/v1/rules/{id}
GET  /api/v1/devices
GET  /api/v1/devices/{deviceId}/latest
GET  /api/v1/devices/{deviceId}/properties/history?property=temperature&start=...&end=...
GET  /api/v1/alarms
POST /api/v1/alarms/{id}/actions
GET  /api/v1/raw-messages
GET  /api/v1/raw-messages/{messageId}
GET  /api/v1/raw-messages/{messageId}/download
POST /api/v1/raw-messages/download  # messageIds 批量整合为 ZIP
POST /api/v1/raw-messages/replay
GET  /api/v1/ai/alarm-analysis/{alarmId}
POST /api/v1/ai/chat
POST /api/v1/ai/rule-draft
POST /api/v1/ai/reports
POST /api/v1/knowledge/documents
POST /api/v1/integrations/video/alarm
GET/POST/PUT /api/v1/integrations/video/cameras[/{id}]
POST /api/v1/mqtt/token
POST /api/v1/device-mqtt/token             # 设备凭证换取限域 JWT
POST /api/v1/mqtt/load-token               # 仅管理员压测
POST /api/v1/integrations/thingspanel/sync
POST /mcp
```

网关设备使用自己的设备凭证上报子设备。首次出现的子设备会自动注册，并记录所属网关：

```json
{
  "messageId": "raw_child_001",
  "deviceId": "child_device_001",
  "deviceName": "A 区烟感 001",
  "productId": "child_product_id",
  "payload": {
    "properties": {"temperature": 25.5, "smoke": false}
  }
}
```

MQTT 上报可在报文中增加 `gatewayId`；生产环境必须通过 EMQX ACL 保证网关只能上报自己名下的 Topic 和子设备。

## 5. Topic

内部 Kafka/Redpanda：

```text
iot.raw.message             iot.parse.failed
iot.property.report         iot.event.report
iot.device.state            iot.video.alarm
iot.alarm.raised            iot.alarm.recovered
iot.alarm.confirmed         iot.alarm.ai-analysis
iot.replay.request          iot.dlq.{serviceName}
```

外部 EMQX：

```text
/external/raw/{tenantId}/{productId}/{deviceId}
/jetlinks/raw/{tenantId}/{productId}/{deviceId}
/external/video/alarm/{tenantId}/{cameraId}
/iot/device/state/{tenantId}/{productId}/{deviceId}
/iot/alarm/{eventType}/{cityCode}/{districtCode}/{buildingId}/{deviceType}/{deviceId}
```

Compose 已启用 HS256 JWT 认证、JWT 内嵌 ACL 和默认拒绝策略，匿名连接会被拒绝。生产仍必须按 [`ops/emqx/PRODUCTION_SECURITY.md`](ops/emqx/PRODUCTION_SECURITY.md) 配置 TLS/mTLS、随机密钥、Erlang Cookie 和受限网络入口。

## 6. 视频签名

```text
X-Video-Platform-ID: video-platform-1
X-Timestamp: Unix 秒
X-Signature: hex(HMAC-SHA256(secret, timestamp + rawBody))
```

时间戳允许误差 5 分钟；`eventId` 用于幂等。开发模式可不签名，生产模式拒绝未知平台或无效签名。

摄像头映射支持 HLS（`.m3u8`）、MP4 和 WebM 浏览器预览；RTSP/RTMP 需要先经 MediaMTX、ZLMediaKit 等网关转换。为防止预览请求探测查看者的内网，必须通过 `IOT_VIDEO_PREVIEW_ALLOWED_ORIGINS` 配置精确的流媒体 Origin（协议、主机和端口），例如 `https://video.example.internal,http://mediamtx:8888`。生产前端还要把所有 HLS 主/子播放列表及分片 Origin 以空格分隔写入 `IOT_VIDEO_PREVIEW_CSP_SOURCES`；Nginx 会据此生成 CSP 并阻止跨白名单重定向和分片请求。未配置白名单时仍可保存映射，但后端拒绝创建浏览器预览。

## 7. MCP

MCP 使用 Streamable HTTP，端点 `/mcp`，请求携带平台 Bearer JWT。工具：

```text
query_device_latest
query_alarm_list
query_property_history
query_similar_alarms
query_knowledge_base
create_rule_draft
```

租户来自认证上下文，调用方不能用工具参数切换租户。工具只读平台接口；规则工具只生成禁用草稿。

## 8. 备份与恢复

`backup-service` 默认每天全量、每 15 分钟增量运行：

1. 全量保存 PostgreSQL custom dump；增量归集 PostgreSQL WAL（Compose 已启用 5 分钟归档超时）。
2. 导出 ClickHouse 各表 DDL 与 Native 数据、Redis RDB，并保留服务端 AOF。
3. 保存 Redpanda Topic/分区清单、EMQX 数据配置和 Compose/Kubernetes 配置。
4. 启用所有业务 MinIO Bucket 版本控制，并增量复制到独立 `minio-dr`。
5. AI profile 启用时触发 Weaviate filesystem snapshot；知识原文同时随 MinIO 复制。
6. 对每个制品计算 SHA-256、上传后重新读取校验，写入 manifest 和 `backup_task`。

手动备份与非破坏性恢复演练：

```powershell
$backupHeaders = @{Authorization='Bearer change-me-backup-admin-token'}
Invoke-RestMethod -Method Post 'http://localhost:8090/backup?type=FULL' -Headers $backupHeaders
Invoke-RestMethod -Method Post 'http://localhost:8090/restore/drill?backupId=latest' -Headers $backupHeaders
```

恢复演练会从 MinIO 重新读取全部制品、校验哈希并记录 `RESTORE_DRILL`，不会覆盖生产数据。正式恢复仍必须把制品恢复到隔离环境，经一致性检查后再执行切换。

## 9. 测试与压测

```powershell
$env:PATH='C:\Program Files\Go\bin;' + $env:PATH
go test ./...
go test -race ./internal/core ./internal/httpapi
go vet ./...
```

压测器支持 HTTP/MQTT 两条入口、稳定、突发、断网补发三种曲线，并输出 JSON P50/P95/P99、错误率和阈值判定：

```powershell
go run ./cmd/loadgen -url http://localhost:8080 -token $login.accessToken `
  -transport http -profile steady -rate 10000 -duration 2h -devices 300000 -workers 128 `
  -report .\steady-10k.json

go run ./cmd/loadgen -url http://localhost:8080 -token $login.accessToken `
  -transport mqtt -mqtt-broker tcp://localhost:1883 -profile burst `
  -rate 10000 -burst-rate 50000 -burst-duration 15m -duration 30m

go run ./cmd/loadgen -url http://localhost:8080 -token $login.accessToken `
  -profile offline -rate 10000 -offline-duration 10m -burst-rate 50000 -duration 30m
```

按设计目标分别执行 5k/10k/30k 稳态、50k-100k 突发和断网补发场景。单机开发结果不能替代生产集群压测。

## 10. 生产前检查

- 替换所有示例密码、JWT 密钥和视频平台 Secret。
- EMQX 关闭匿名访问，启用 TLS/mTLS、JWT/ACL，限制大范围通配订阅。
- API 通过 HTTPS 暴露，管理入口限制在运维网络。
- PostgreSQL、Redis、MinIO、Redpanda、ClickHouse 使用高可用集群与独立磁盘。
- 原始报文下载、回放、告警操作和 AI 工具调用接入组织级 RBAC/审计留存。
- 做容量校准、故障注入、备份恢复演练和数据保留合规评审。
- Kubernetes Secret 示例只是占位符，生产使用 Vault、External Secrets 等密钥系统。

## ThingsPanel

`docker compose --profile thingspanel up -d --build` 可启动锁版的官方前后端。平台支持 ThingsPanel 登录回退、服务账号目录同步、手动同步 API 与周期同步；参见 [`docs/THINGSPANEL_INTEGRATION.md`](docs/THINGSPANEL_INTEGRATION.md)。本项目未修改上游工作树，避免把行业扩展和底座升级耦合。
