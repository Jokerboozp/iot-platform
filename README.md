# 消防 IoT 平台

面向消防物联网场景的一体化管理平台。项目由 Go API、Vue 3 Web、EMQX、PostgreSQL、Redis、ClickHouse、Redpanda、MinIO 以及可选 AI 运行时组成，覆盖设备与数据接入、协议解析、状态管理、告警、视频流预览、原始报文证据链、回放、AI 研判、MCP 和备份恢复。

当前代码库支持两种运行方式：使用内存适配器快速开发/测试，或使用 Docker Compose 启动前后端与完整基础设施。文档按当前源码、Compose 配置和可重复验证的测试结果编写；生产容量、灾备、TLS/ACL 和第三方平台验收仍需在目标环境单独完成。

> 说明：JetLinks 上游协议包不属于本项目交付范围；本项目提供独立的协议注册、配置化 JSON/Hex 解析、受限 JavaScript 解析和 GB/T 26875 接入示例。

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

需要 AI 工作流时，使用 Harness profile 一起启动（需要配置 `DEEPSEEK_API_KEY`）：

```bash
docker compose --profile harness up -d --build
docker compose --profile harness ps
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

内置管理员只能为白名单中的租户签发管理员 Token。默认白名单只有 `tenant_001`；确需让内置管理员管理多个租户时，必须显式配置：

```dotenv
IOT_ADMIN_TENANTS=tenant_001,tenant_002
```

登录请求中的 `tenantId` 不在白名单时会被拒绝。外部目录认证使用目录返回的租户，不信任请求体中的租户字段。

查看日志：

```bash
docker compose logs -f platform-api platform-web
```

查看 AI Harness 日志和健康状态：

```bash
docker compose --profile harness logs -f deepseek-harness platform-api
curl http://127.0.0.1:8091/health
```

### 一键离线部署

打包分为两个阶段：在有网机器上准备完整离线包，再把离线包传到无网服务器启动。打包机需要已安装并启动 Docker Engine/Desktop；服务器端不会执行镜像构建、依赖安装或镜像拉取。

打包机按操作系统执行：

```powershell
# Windows
powershell -ExecutionPolicy Bypass -File .\scripts\package-offline-windows.ps1
```

```bash
# macOS
bash ./scripts/package-offline-macos.sh

# Linux
bash ./scripts/package-offline-linux.sh
```

可选组件参数按系统分别为：Windows 使用 `-IncludeAi`、`-IncludeHarness`、`-IncludeThingsPanel`、`-IncludeGb26875`、`-Full`；macOS/Linux 使用对应的长参数 `--include-ai`、`--include-harness`、`--include-thingspanel`、`--include-gb26875`、`--full`。例如：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\package-offline-windows.ps1 `
  -IncludeAi -OllamaModel qwen3:8b
```

```bash
# macOS
bash ./scripts/package-offline-macos.sh --include-ai --ollama-model qwen3:8b

# Linux
bash ./scripts/package-offline-linux.sh --include-ai --ollama-model qwen3:8b
```

脚本会构建/拉取镜像、导出 `images.tar`、生成 SHA-256、复制 Compose 和部署文件，并生成离线配置。将输出的 `iot-platform-offline-*` 目录整体传到无网服务器后，按服务器操作系统执行：

```powershell
# Windows
powershell -ExecutionPolicy Bypass -File .\scripts\deploy-offline-windows.ps1
```

```bash
# macOS
bash ./scripts/deploy-offline-macos.sh

# Linux
bash ./scripts/deploy-offline-linux.sh
```

服务器端使用 `--no-build --pull never`，不会安装 Go/Node 依赖或访问镜像仓库。离线包包含镜像归档、校验文件、Compose 文件、环境配置和三种系统的部署脚本。打包机与服务器应使用兼容的 Docker CPU 架构；例如 Apple Silicon macOS 打包给 `linux/amd64` 服务器时，应使用 `linux/amd64` 架构的有网打包机。

正式密钥、Ollama 模型、Harness、ThingsPanel、GB/T 26875 以及故障排查说明见 [一键离线部署](docs/OFFLINE_DEPLOYMENT.md)。

停止服务但保留数据：

```bash
docker compose down
```

只启动国标消防终端网关：

```bash
docker compose --profile gb26875 up -d --build
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
cd iot_front
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
cd iot_front
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
| 协议开发 | 协议包版本、JSON/Hex/JavaScript 解析器配置和样本调试 |
| 协议接入助手 | 上传协议/点表，AI 生成受限 JavaScript，表单修改字段，样本预览并发布 |
| 接入指南 | HTTP/MQTT 上报参数、设备凭证、Topic 和数据联调；不是重复的设备注册入口 |
| 摄像头映射 | 摄像机与设备/空间关联、视频流预览 |
| 告警中心 | 告警查询、确认、抑制、恢复和关闭 |
| 智能巡检 | 点击“立即巡检”后检查设备业务状态、数据新鲜度、活动告警和 AI 处置建议 |
| 原始报文 | 证据链检索、下载、审计和回放 |
| 告警规则 | JSON/Gengine 规则、冲突检测和 AI 草稿 |
| 知识库管理 | 上传消防规范、设备手册和处置 SOP，查看索引状态 |
| AI 工作流 | DeepSeek Harness 插件、Provider 测试、流式对话和工具轨迹 |

“AI 告警研判”是业务工作流，不是独立菜单。进入“AI 工作流”后，在左侧“运行工作流”卡片的“工作流插件”下拉框中选择：

- AI 告警研判
- AI 运维助手
- AI 系统状态助手
- AI 设备健康巡检
- AI 协议接入助手

“协议接入助手”支持 PDF、Word、Excel/CSV 点表和直接粘贴文本。AI 只负责提出字段映射草稿；发布前必须由操作员在字段表单中确认字段名、解析表达式、消息类型和样本结果。发布内容是 `javascript_sandbox_parser` 协议包，代码在 Goja 受限沙箱中运行（无网络、文件、环境变量和平台 API），因此新增协议不需要重新编译或部署 API 镜像。发布后到“产品管理”把协议包绑定到产品，设备原始报文即可走同一解析链路。

对于不属于常见 JSON 或固定字段十六进制格式的新设备，不需要重新部署平台：

1. 在“协议开发”中使用配置化 JSON/Hex 映射，适合字段偏移、类型、缩放和枚举规则稳定的设备。
2. 在“协议接入助手”上传协议文件或点表，让 AI 生成解析 JavaScript 草稿。
3. 在字段表单中修改字段名、类型、表达式和消息类型，使用样本报文预览标准消息。
4. 确认发布后，解析代码以协议包配置保存并由受限 JavaScript 沙箱执行；再到产品管理绑定协议包。

如需在其他地方编写代码，也可以直接提交符合沙箱约束的 `javascript_sandbox_parser` 协议包。解析脚本不能访问网络、文件、环境变量、平台 API 或设备控制能力。

### 智能巡检操作约定

进入“智能巡检”页面只展示说明和空状态，不会自动触发请求；只有点击“立即巡检”才开始。本次巡检先由平台计算设备数量、状态、最近上报时间和活动告警，再让 AI 生成文字建议；即使 AI 不可用，确定性巡检结果仍然可查看。巡检按钮带有加载状态，重复点击不会并发发起同一页面的巡检。

“告警规则”支持人工新建、编辑、启停和删除，也支持 AI 生成默认禁用的规则草稿。规则可以在条件满足时产生告警、打开摄像头或通过实时 Topic 打开允许的前端页面；AI 不会直接启用规则，必须由用户在规则页检查后确认。

“原始报文”同时展示原始内容、解析状态、解析器和标准消息结果。协议包发布并绑定到产品后，新报文会沿同一 Parser Registry 链路解析；解析失败会保留原始证据并进入失败处理链路。

“接入指南”用于查看真实设备上报所需的 HTTP/MQTT 地址、认证和 Topic，并提供数据联调入口；设备是否已经注册、启用和绑定协议，以“设备管理”和“产品管理”中的状态为准。

## 核心能力

| 领域 | 实现 |
|---|---|
| 设备与产品 | 产品、协议包、设备注册、设备/网关凭证、发现设备转注册 |
| 接入指南 | 管理端 REST、设备凭证 HTTP、MQTT 原始 Topic、设备数据联调 |
| 原始证据链 | gzip JSONL 微批归档、SHA-256、幂等索引、单条精确恢复 |
| 协议解析 | JSON/Hex 配置映射、受限 JavaScript 脚本、可上传 Go 协议 Worker、AI 协议接入助手、消防烟感/GB26875/Modbus 示例解析器，失败 Topic 与 DLQ |
| 分层存储 | MinIO、Redpanda、ClickHouse、Redis、PostgreSQL、可选 Weaviate |
| 状态管理 | ONLINE、OFFLINE、SUSPECTED_OFFLINE 与多维离线判定 |
| 规则与告警 | JSON/Gengine、物模型校验、冲突检测、告警生命周期和审计 |
| 实时推送 | EMQX JWT、租户 Topic ACL、WebSocket 状态和告警推送 |
| 视频 | HMAC Webhook、摄像头映射、融合告警、HLS/MP4/WebM 预览 |
| AI Provider | Eino 编排、Ollama、DeepSeek、OpenAI-compatible 适配器 |
| AI 工作流 | DeepSeek Harness 源码运行时、SSE、工具卡片、轨迹、告警研判、知识库绑定、Agent 管理、设备巡检和取消 |
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
cmd/gb26875-gateway           GB/T 26875 TCP/UDP 设备网关
cmd/gb26875-virtual-device    GB/T 26875 虚拟设备和联调工具
cmd/backup-service            备份与恢复演练服务
cmd/loadgen                   HTTP/MQTT 压测器
internal/core                 状态、规则、告警、视频、AI、回放
internal/adapters             数据库、消息、对象存储、AI/RAG 适配器
internal/httpapi              Gin REST API、JWT/RBAC、Webhook
internal/mcpserver            平台 MCP 与 Harness 专用只读 MCP
iot_front                    独立 Vue 3、shadcn-vue 视觉系统、Element Plus 兼容层、HLS.js、Nginx 前端项目
deploy/deepseek-harness       Harness 网关、插件清单和构建适配
deploy/k8s                    Kubernetes 基线清单
ops                           Prometheus、Grafana、Loki、EMQX 配置
scripts                       上游源码拉取等项目脚本
docs                          AI Harness、ThingsPanel 和覆盖说明
```

设备协议的选择、发布和运行时链路见 [设备协议接入流程](docs/设备协议接入流程.md)；复杂协议的 Go Worker 契约见 [Go 协议包接入](docs/GO_PROTOCOL_PACKAGES.md)。

## Docker 服务与端口

| 服务 | 默认地址 |
|---|---|
| 平台 Web | `http://localhost:8080` |
| 平台 API | `http://localhost:8081` |
| DeepSeek Harness | `http://127.0.0.1:8091` |
| GB/T 26875 网关（可选） | `tcp://localhost:26875` / `udp://localhost:26875` |
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
| `gb26875` | 国标消防终端 TCP 接入 | `docker compose --profile gb26875 up -d --build` |
| `harness` | DeepSeek Harness 插件工作流 | `docker compose --profile harness up -d --build` |
| `ai` | Ollama 与 Weaviate | `docker compose --profile ai up -d --build` |
| `thingspanel` | ThingsPanel 上游前后端 | `docker compose --profile thingspanel up -d --build` |

### GB/T 26875 设备接入示例

项目提供独立的国标消防终端 TCP/UDP 网关和虚拟设备，用于验证设备注册、凭证、原始报文、解析和标准消息链路：

```bash
docker compose --profile gb26875 up -d --build

# 本地运行虚拟设备（端口和参数以命令帮助为准）
go run ./cmd/gb26875-virtual-device --help
```

网关接收设备报文后调用平台设备接入接口；平台侧仍按“设备管理 → 产品协议绑定 → 原始报文 → 标准消息”的链路处理。协议字段和示例报文见 [GB/T 26875 对接说明](docs/GB26875_DAHUA_V103.md)。

协议开发现在按复杂度提供三条路径：JSON/固定字段十六进制直接配置；受限 JavaScript 适用于纯转换；变长、TLV、状态机或请求/应答协议可选择 `go_protocol_parser`，上传受 SHA-256、路径、超时和输出大小限制的已编译 Go Worker。Go 源码不能在 API 进程内直接编译执行，完整契约见 [Go 协议包接入](docs/GO_PROTOCOL_PACKAGES.md)。

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
| `device-health-inspector` | AI 设备健康巡检 | 设备状态、数据新鲜度、活动告警和维修优先级建议 |
| `protocol-assistant` | AI 协议接入助手 | 协议文档、点表和样本报文的字段映射草稿 |

这些是 Harness 的内置插件。动态 Agent 会保存在 Harness 数据卷中，数量和名称以“AI 工作流 → 管理中心 → Agent 管理”的实时清单为准。

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

管理员进入“管理中心 → Agent 管理”后，还可以查看启用和禁用的完整清单。动态 Agent 支持 JSON 编辑、启用/禁用和删除；内置插件只读。保存或状态变更后不需要重新部署容器。

### 工作流知识库绑定

展开“AI 工作流 → 工作流知识库绑定”，可以按租户为当前插件配置：

- 绑定一个或多个产品；留空表示当前租户的全部产品。
- 限定知识分类和必须包含的标签。
- 选择按需检索、每次强制检索或完全禁用知识库。
- 配置 TopK、最低相似度以及无证据时是否阻止回答。

绑定不是只写入提示词。平台会将约束写入每次运行的短期 Harness Token，MCP 服务端会覆盖模型提交的产品、分类、标签、TopK 和阈值，防止模型扩大检索范围。“强制检索”会在进入 Harness 前预召回知识；“必须有证据”在无匹配结果时会停止运行。

这部分与 Dify 的知识库工作流理念相似，但实现仍属于本项目：DeepSeek Harness 负责插件运行，Go API 负责租户、绑定策略、检索授权和审计，不需要额外部署 Dify。

### 新增业务插件

管理员可以直接在“AI 工作流 → Agent 管理”查看完整插件清单。动态 Agent 支持编辑 Manifest、启用/禁用和删除；“新建 Agent”仍可通过 JSON 粘贴 Manifest。平台会执行两层校验：Go API 检查字段和只读工具白名单，Harness 再验证完整 Manifest。保存后 Agent 立即出现在工作流下拉框，不需要重建容器。

动态 Agent 保存在 `deepseek-harness-data` 卷的 `/data/plugins`，容器重启后仍然存在。内置 `alarm-handler`、`ops-assistant`、`system-observer`、`device-health-inspector` 和 `protocol-assistant` 只能查看，禁止覆盖、修改或删除；自定义 Agent 使用相同 ID 保存会更新。

可用的只读工具为：

```text
mcp__iot__query_system_overview
mcp__iot__query_device_latest
mcp__iot__query_alarm_list
mcp__iot__query_property_history
mcp__iot__query_similar_alarms
mcp__iot__query_knowledge_base
mcp__iot__create_rule_draft
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

- 只开放设备状态、告警、属性历史、相似告警、知识库查询和默认禁用的规则草稿生成。
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

AI 页面“管理中心 → Provider 测试与配置”支持 DeepSeek、Ollama 和 OpenAI-compatible 连接测试。管理员可以添加自定义 OpenAI-compatible Provider，保存名称、Provider ID、Base URL 和模型配置到当前浏览器/租户的配置中；API Key 只在本次测试使用，不写入浏览器配置、数据库或审计日志。自定义 Provider 的 Base URL 来源必须加入 `IOT_AI_PROVIDER_TEST_ALLOWED_ORIGINS` 白名单，平台不会为了测试关闭 SSRF 防护。

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

时间戳允许误差 5 分钟，`eventId` 用于幂等。生产环境还必须把平台绑定到租户，避免签名平台代发其他租户告警：

```dotenv
IOT_VIDEO_PLATFORM_SECRETS=video-platform-1:<platform-secret>
IOT_VIDEO_PLATFORM_TENANTS=video-platform-1:tenant_001
```

平台绑定不匹配的 `tenantId` 会被拒绝；生产 Webhook 的 `cameraId` 还必须属于该租户且处于启用状态。MQTT 视频和设备状态消息同样以租户范围 Topic 为准，不信任 Payload 中的租户字段。

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
POST         /api/v1/protocol-packages/{id}/artifact
POST         /api/v1/protocol-packages/{id}/test
GET/POST/PUT /api/v1/device-registry[/{id}]
POST         /api/v1/discovered-devices/{id}/register
POST         /api/v1/device-registry/{id}/credentials
GET          /api/v1/device-registry/{id}/connection-guide
POST         /api/v1/device-registry/{id}/debug
POST         /api/v1/device-ingest/{deviceId}
POST         /api/v1/device-states
GET/POST     /api/v1/raw-messages
POST         /api/v1/raw-messages/download
GET          /api/v1/raw-messages/{id}
GET          /api/v1/raw-messages/{id}/download
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
GET          /api/v1/ai/workflows/admin
PUT/DELETE   /api/v1/ai/workflows/{id}
GET/PUT      /api/v1/ai/workflows/{id}/knowledge-binding
POST         /api/v1/ai/chat
POST         /api/v1/ai/chat/stream
GET          /api/v1/ai/alarm-analysis/{alarmId}
POST         /api/v1/ai/alarm-analysis/{alarmId}/run
POST         /api/v1/ai/health-inspection
POST         /api/v1/ai/protocol-assistant/generate
POST         /api/v1/ai/protocol-assistant/preview
POST         /api/v1/ai/protocol-assistant/publish
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
cd iot_front
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

2. Harness 健康接口在全新数据卷上至少应返回 `pluginCount: 5`（内置插件数）；如果通过 Agent 管理新增并启用了动态插件，数量会相应增加：

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
