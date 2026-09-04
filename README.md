# 消防 IoT 平台

面向消防物联网场景的一体化管理平台。项目由 Go API、独立 Vue 3 Web、EMQX、PostgreSQL、Redis、ClickHouse、Redpanda、MinIO 以及可选 AI 运行时组成，覆盖设备与数据接入、协议解析、Kafka/MQTT 双通道转发、状态管理、告警、视频告警与摄像头元数据、原始报文证据链、回放、AI 研判、MCP 和备份恢复。

当前代码库支持三种常用运行方式：使用 Docker Compose 启动前后端与基础设施；分开启动 Go API 和 Vue 前端进行本地开发；或只构建前端静态资源并交给现有 Nginx/Ingress 部署。文档按当前源码和部署配置编写；生产容量、灾备、TLS/ACL 和第三方平台验收仍需在目标环境单独完成。

> 说明：设备协议已经重构为独立的 Protocol v2 子系统，不依赖 JetLinks。简单设备上传 CSV/XLSX 点表后由内置 Modbus TCP 运行时主动采集；复杂设备上传带 Manifest、目标平台 Worker 和样例测试的版本化 ZIP，校验发布后无需重启 API。

## 文档导航

建议按以下顺序阅读：

1. [快速选择运行方式](#1-快速选择运行方式)
2. [离线部署](#2-离线部署)
3. [本地开发](#3-本地开发)
4. [前端部署](#4-前端部署)
5. [功能与使用说明](#5-功能与使用说明)
6. [协议与设备接入](#协议与设备接入)
7. [配置、AI 与运维](#10-配置与密钥)
8. [测试、排障与生产边界](#19-测试)

根 README 只保留启动、部署和边界说明；协议、离线包、Harness 和 ThingsPanel 的细节放在[相关文档](#24-相关文档)中。

## 1. 快速选择运行方式

### Docker Compose（推荐）

环境要求：

- Docker Desktop 或 Docker Engine + Compose Plugin
- 首次构建 DeepSeek Harness 时需要访问 GitHub 和 npm registry

启动基础平台：

Compose 以生产安全模式启动，首次运行前必须通过 shell 环境变量或 `.env` 显式设置
`IOT_JWT_SECRET`（至少 32 个字符）和 `IOT_ADMIN_PASSWORD`（至少 12 个字符）。固定示例值和占位符会被拒绝。

首次部署或修改了后端/前端代码、Dockerfile 时，只构建应用服务：

```powershell
docker compose up -d --build platform-api platform-web
docker compose ps
```

日常启动不需要重新构建：

```powershell
docker compose up -d
docker compose ps
```

当前默认 Compose 配置中的构建服务是 `platform-api` 和 `platform-web`。不要每次都执行不带服务名的
`--build`；按服务构建可以避免无关组件产生新的构建缓存。Compose 会自动启动目标服务依赖的基础设施。

需要 AI 工作流时，使用 Harness profile 一起启动（需要配置 `DEEPSEEK_API_KEY`）：

```powershell
docker compose --profile harness up -d --build platform-api deepseek-harness platform-web
docker compose --profile harness ps
```

浏览器打开：

```text
http://localhost:8080
```

内置管理员账号：

```text
租户：tenant_001
用户名：admin
密码：启动前设置的 IOT_ADMIN_PASSWORD
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

#### Docker 构建缓存与磁盘空间

`--build` 不会每次完整重建，但源码变更会让 `COPY . .` 和编译步骤产生新的 BuildKit 缓存记录；旧缓存不会因为
容器重建自动删除。建议按以下方式控制空间：

- 没有代码或 Dockerfile 变更时，只执行 `docker compose up -d`。
- 只修改后端时执行 `docker compose up -d --build platform-api`；只修改前端时执行
  `docker compose up -d --build platform-web`。
- DeepSeek Harness 和 ThingsPanel 仅在需要时启用对应 profile；`backup-service`、Ollama 与 Weaviate 随基础平台启动，
  用于原始日志定时备份和本地持久化 Agent 知识库。备份配置见[备份与恢复](#18-备份与恢复)。
- 不要日常使用 `--no-cache` 或 `--pull`；只有需要强制刷新基础镜像时再使用，否则会增加下载和构建量。

查看占用并清理 7 天未使用的构建缓存：

```powershell
docker system df
docker builder prune -f --filter "until=168h" --keep-storage 8GB
docker image prune -f --filter "until=168h"
```

`docker builder prune` 会删除可重新生成的未使用构建缓存，可能使下一次构建变慢；`docker image prune` 不带 `-a`
时只清理悬空镜像。不要在未确认数据可丢弃前执行 `docker compose down -v`、`docker system prune --volumes`
或 `docker image prune -a`，这些命令可能删除数据库卷、对象存储卷或仍需使用的镜像。

清理 Docker 内部缓存后，WSL 的 `docker_data.vhdx` 文件在宿主机上可能仍保持原大小。需要回收宿主机空间时，先完全
退出 Docker Desktop，再以管理员身份打开 PowerShell 执行：

```powershell
wsl --shutdown

$dockerVhdx = (Get-ChildItem -LiteralPath (Join-Path $env:LOCALAPPDATA 'Docker\wsl') `
  -Filter docker_data.vhdx -Recurse -File | Select-Object -First 1).FullName
if (-not $dockerVhdx) { throw '找不到 Docker Desktop 的 docker_data.vhdx' }

@"
select vdisk file="$dockerVhdx"
compact vdisk
exit
"@ | diskpart
```

压缩前必须确认 Docker Desktop 已退出；`wsl --shutdown` 会停止所有 WSL2 实例，但不会注销它们。压缩失败并提示文件
正在使用时，不要强制删除或卸载虚拟磁盘，先关闭仍在运行的 Docker/WSL 进程。

## 2. 离线部署

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

## 3. 本地开发

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

## 4. 前端部署

前端位于 `iot_front`，是独立的 Vue 3 + Vite 项目。浏览器请求默认使用同源路径 `/api`、`/health` 和 `/mcp`；生产环境应通过 Nginx 或 Ingress 把这些路径转发到 Go API。`VITE_API_PROXY_TARGET` 只用于 Vite 开发服务器代理，不是生产 API 地址配置。

### 4.1 本地开发前端

先启动后端 API。PowerShell：

```powershell
$env:IOT_HTTP_ADDR=':8081'
$env:IOT_CORS_ALLOWED_ORIGINS='http://localhost:5173'
go run ./cmd/iot-platform
```

另开终端启动前端：

```powershell
cd iot_front
npm.cmd ci
npm.cmd run dev
```

默认访问 `http://localhost:5173`。Vite 会将 `/api`、`/health` 和 `/mcp` 代理到 `http://localhost:8081`；后端端口改变时设置 `VITE_API_PROXY_TARGET`：

```powershell
$env:VITE_API_PROXY_TARGET='http://localhost:18081'
npm.cmd run dev
```

macOS/Linux 使用同样的 npm 命令，只需把 PowerShell 环境变量写法改为 `VITE_API_PROXY_TARGET=http://localhost:18081 npm run dev`。

### 4.2 构建前端

```bash
cd iot_front
npm ci
npm test
npm run build
```

构建产物在 `iot_front/dist`。`npm run preview` 只用于本地预览，不是生产服务。Node.js 要求为 `^20.19.0` 或 `>=22.12.0`，推荐 Node.js 22 LTS；项目启用了 `engine-strict`，版本不符合时安装会直接失败。

### 4.3 Docker Compose 中部署前端（推荐）

根目录 `compose.yaml` 的 `platform-web` 以 `iot_front` 为构建上下文，使用 `iot_front/Dockerfile` 构建 Nginx 镜像。容器内通过 Docker DNS 访问 `platform-api:8080`，并由 Nginx 代理 API、健康检查、MCP 以及 SSE 流式接口。

```bash
docker compose build platform-api platform-web
docker compose up -d platform-api platform-web
docker compose ps
```

访问 `http://localhost:8080`。如果只修改了前端，重新构建并重建 `platform-web`：

```bash
docker compose up -d --build --force-recreate platform-web
```

生产环境使用自定义端口时设置 `IOT_WEB_PORT`；API 对外端口由 `IOT_API_PORT` 控制。容器之间仍然使用 `platform-api:8080`，不需要把 API 容器端口改成宿主机端口。

### 4.4 独立前端 Docker 镜像

`iot_front/Dockerfile` 适合把前端交给独立的镜像仓库或 Kubernetes。默认 Nginx 配置假定 API 服务名为 `platform-api`、容器端口为 `8080`，因此可以直接用于 Compose 和当前 Kubernetes 基线。

```bash
docker build -t iot-platform-web:local ./iot_front
```

如果前端容器与 API 不在同一个 Docker 网络，请先复制 `iot_front/nginx.conf` 为部署配置，再把其中 `/api/`、`/health/`、`/mcp` 和 SSE 路由的 `proxy_pass http://platform-api:8080` 改为实际 API 地址，然后挂载该配置启动：

```bash
docker run -d --name iot-platform-web --restart unless-stopped -p 8080:8080 -v "$PWD/nginx.standalone.conf:/etc/nginx/templates/default.conf.template:ro" iot-platform-web:local
```

Docker Desktop 访问宿主机 API 时可使用 `host.docker.internal`；Linux 需要增加 `--add-host=host.docker.internal:host-gateway`，或直接配置可达的 API 主机名。不要把 API 地址写成容器内的 `127.0.0.1`。

### 4.5 静态文件交给现有 Nginx/Ingress

如果环境已经有统一网关，可以只发布 `iot_front/dist`：

```bash
cd iot_front
npm ci
npm run build
# 将 dist/ 发布到现有静态文件目录
```

网关必须同时提供 SPA fallback（未知页面回退到 `index.html`）和以下同源转发：

| 路径 | 转发目标 | 说明 |
|---|---|---|
| `/api/` | Go API | REST 与文件上传/下载 |
| `/api/v1/ai/chat/stream` | Go API | 关闭代理缓冲，保留长连接 |
| `/health/` | Go API | 前端容器健康检查 |
| `/mcp` | Go API | MCP 与流式响应，关闭代理缓冲 |

可直接以 `iot_front/nginx.conf` 作为 Nginx 配置模板。当前摄像头页面只展示元数据，不需要配置视频预览域名或媒体 CSP。前端 API 调用是相对路径，不能只设置 `VITE_API_PROXY_TARGET` 就把生产前端改成跨域直连。

### 4.6 Kubernetes 前端部署

先构建并推送前端镜像，再替换 `deploy/k8s/platform.yaml` 中的占位镜像和 Secret：

```bash
docker build -t your-registry/iot-platform-web:1.0.0 ./iot_front
docker push your-registry/iot-platform-web:1.0.0
kubectl apply -f deploy/k8s/platform.yaml
```

该清单包含 API/Web Deployment、Service、健康检查和 NetworkPolicy，但不包含 Ingress；需要在集群中另行配置域名、TLS 和外部入口。Web Pod 通过 Kubernetes Service `platform-api:8080` 访问 API。

## 5. 功能与使用说明

### 协议与设备接入

| 页面 | 功能 |
|---|---|
| 运行总览 | 设备、在线率、告警趋势和重点态势 |
| 设备管理 | 注册、启停、凭证轮换、连接指南和实时状态 |
| 产品管理 | 产品模型和当前协议版本绑定 |
| 设备接入 | 点表直连、自定义 ZIP 协议包、不可变版本、设备采集实例和连接测试 |
| 接入指南 | HTTP/MQTT 上报参数、设备凭证、Topic 和数据联调；不是重复的设备注册入口 |
| 摄像头映射 | 摄像头品牌、名称、点位、楼层/建筑/房间和设备关联；直播流由外部视频平台提供 |
| 告警中心 | 告警查询、确认、抑制、恢复和关闭 |
| 智能巡检 | 点击“立即巡检”后检查设备业务状态、数据新鲜度、活动告警和 AI 处置建议 |
| 原始报文 | 证据链检索、下载、审计和回放 |
| 告警规则 | JSON/Gengine 规则、冲突检测和 AI 草稿 |
| 知识库管理 | 上传消防规范、设备手册和处置 SOP，查看索引状态 |
| AI 工作流 | DeepSeek Harness 插件、流式对话、工具轨迹和 Agent 管理 |
| 测试设备 | 自动准备绑定产品、协议和默认报文，可发送数据、事件、报警和恢复报文 |

“AI 工作流”中的聊天下拉框只展示交互式聊天助手：

- AI 运维助手
- AI 系统状态助手

“AI 告警研判”和“AI 设备健康巡检”是业务专用工作流，分别由告警中心和智能巡检页面调用，不显示在聊天工作流下拉框中。旧的 AI 协议接入助手接口暂时保留兼容，但不再出现在主菜单，新设备统一从“设备接入”进入。

设备接入只保留两条主路径：

1. **点表快速接入**：上传 CSV/XLSX，平台校验地址歧义、功能码、数据类型、字节序、缩放和重复标识，编译 FC01/02/03/04 读取块；可同时创建产品、设备和 Modbus TCP 连接实例并立即轮询。
2. **自定义协议包**：上传版本化 ZIP。根目录必须包含 `manifest.yaml`，立即发布时还必须包含 `samples/cases.json`；平台核对当前 OS/CPU Worker、路径和 SHA-256，并真实运行全部样例后发布。

协议定义、协议 release、点表 release、产品绑定和设备连接参数分别持久化。`protocolId + version` 发布后不可覆盖；切换产品时保留上一版本并支持回滚。主动 Modbus 响应仍先写入 RawMessage，再进入 Parser Registry、StandardMessage、Kafka/MQTT、规则与告警链路。

消息类型在界面显示中文名称，接口和 Go Worker 使用稳定代码：

| 中文名称 | 稳定代码 | 用途 |
|---|---|---|
| 属性上报 | `PROPERTY_REPORT` | 测点、开关量和当前状态值 |
| 事件上报 | `EVENT_REPORT` | 一次性发生的复位、心跳或测试事件 |
| 告警上报 | `ALARM_REPORT` | 设备明确上报的告警事件；无需告警规则即可生成平台告警，规则可额外提供分类、等级或联动动作 |
| 状态变化 | `STATE_CHANGE` | 在线、离线或业务状态变化 |
| 指令应答 | `COMMAND_REPLY` | 设备对平台指令的响应 |
| 日志上报 | `LOG_REPORT` | 运行日志或诊断信息 |

完整架构、点表列定义、ZIP 目录和验收边界见 [设备协议包与点表接入方案](docs/DEVICE_PROTOCOL_ACCESS_DESIGN.md)。示例文件位于 `examples/protocol-v2`。

### 智能巡检操作约定

进入“智能巡检”页面只展示说明和空状态，不会自动触发请求；只有点击“立即巡检”才开始。本次巡检先由平台计算设备数量、状态、最近上报时间和活动告警，再让 AI 生成文字建议；即使 AI 不可用，确定性巡检结果仍然可查看。巡检按钮带有加载状态，重复点击不会并发发起同一页面的巡检；完成后可点击“下载 PDF”导出同一份已核实快照和建议。

“告警规则”支持人工新建、编辑、启停和删除，也支持 AI 生成默认禁用的规则草稿。规则可以在条件满足时产生告警、打开摄像头或通过实时 Topic 打开允许的前端页面；AI 不会直接启用规则，必须由用户在规则页检查后确认。草稿返回纯 JSON、字段含义说明和等价的 Gengine 表达式；Gengine 默认只以注释占位符展示，人工填入后才参与运行。

“原始报文”同时展示原始内容、解析状态、解析器和标准消息结果。协议包发布并绑定到产品后，新报文会沿同一 Parser Registry 链路解析；解析失败会保留原始证据并进入失败处理链路。

“接入指南”用于查看真实设备上报所需的 HTTP/MQTT 地址、认证和 Topic，并提供数据联调入口；设备是否已经注册、启用和绑定协议，以“设备管理”和“产品管理”中的状态为准。

## 6. 核心能力

| 领域 | 实现 |
|---|---|
| 设备与产品 | 产品、协议包、设备注册、设备/网关凭证、发现设备转注册 |
| 接入指南 | 管理端 REST、设备凭证 HTTP、MQTT 原始 Topic、设备数据联调 |
| 原始证据链 | 按设备频率分层写入 PostgreSQL/ClickHouse、SHA-256、幂等索引、每日 gzip JSONL MinIO 备份、单条精确恢复 |
| 协议解析 | JSON/Hex 配置映射、受限 JavaScript 脚本、可上传 Go 协议 Worker、AI 协议接入助手、消防烟感/GB26875/Modbus 示例解析器，失败重试与 DLQ |
| 分层存储 | MinIO、Redpanda、ClickHouse、Redis、PostgreSQL、持久化 Weaviate |
| 状态管理 | ONLINE、OFFLINE、SUSPECTED_OFFLINE 与多维离线判定 |
| 规则与告警 | JSON/Gengine、物模型校验、冲突检测、告警生命周期和审计 |
| 实时推送 | EMQX JWT、租户 Topic ACL、WebSocket 状态和告警推送 |
| 视频 | HMAC Webhook、摄像头元数据、摄像头到设备的一对多反向查询、设备告警带出摄像头信息、跨源融合 |
| AI Provider | Eino 编排、Ollama、DeepSeek、OpenAI-compatible 适配器 |
| AI 工作流 | DeepSeek Harness 源码运行时、SSE、工具卡片、轨迹、告警研判、知识库绑定、Agent 管理、设备巡检和取消、健康巡检 PDF |
| MCP | Streamable HTTP、租户隔离、只读查询工具和调用审计 |
| 回放 | DRY_RUN、REINGEST、DIFF、指定解析器版本和限速 |
| 备份 | 原始日志每日 MinIO 备份、PostgreSQL/WAL、ClickHouse、Redis、MinIO-DR、校验与恢复演练 |
| 运维 | 健康检查、JSON 日志、Prometheus、Grafana、Loki、K8s 和压测器 |

## 7. 系统架构

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
  -> 原始日志分层存储：低频 -> PostgreSQL raw_message_log；高频 -> ClickHouse iot_raw_message
  -> PostgreSQL raw_archive_index（幂等索引与发布状态）
  -> Redpanda iot.raw.message（内部解析队列）
  -> Parser Registry
  -> 解析失败：仅保留归档，可回放；不发布解析结果
  -> 解析成功的标准消息
     -> Kafka：iot.property.report / iot.event.report / iot.parsed.message
     -> MQTT：/iot/parsed/{tenant}/{product}/{device}/{messageType}
     -> PostgreSQL + ClickHouse + Redis、规则与告警
  -> EMQX 告警/状态/AI/UI 实时 Topic + AI/RAG 旁路分析

每天由 `backup-service` 在配置的时间（默认 `00:05`，`Asia/Shanghai`）读取前一天两个数据库中的全部原始日志，合并为一个 gzip JSONL 制品并上传 MinIO；设备每次上报不会再单独写 MinIO。JSONL 每行包含 `storage`（`postgres`/`clickhouse`）和完整 `message`，方便审计和恢复。
```

## 8. 项目目录

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
iot_front                    独立 Vue 3、shadcn-vue 视觉系统、Element Plus 兼容层、Nginx 前端项目
deploy/deepseek-harness       Harness 网关、插件清单和构建适配
deploy/k8s                    Kubernetes 基线清单
ops                           Prometheus、Grafana、Loki、EMQX 配置
scripts                       上游源码拉取等项目脚本
docs                          AI Harness、ThingsPanel 和覆盖说明
```

设备协议的选择、发布和运行时链路见 [设备协议接入流程](docs/设备协议接入流程.md)；复杂协议的 Go Worker 契约见 [Go 协议包接入](docs/GO_PROTOCOL_PACKAGES.md)。

## 9. 服务、端口与健康检查

| 服务 | 默认地址 |
|---|---|
| 平台 Web | `http://localhost:8080` |
| 平台 API | `http://localhost:8081` |
| DeepSeek Harness | `http://127.0.0.1:8091` |
| GB/T 26875 网关（可选） | `tcp://localhost:26875` / `udp://localhost:26875` |
| 备份服务（可选，默认不启动） | `http://127.0.0.1:8092` |
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
| 无 | 完整平台基础服务（包含 backup-service、Ollama、Weaviate） | `docker compose up -d --build` |
| `gb26875` | 国标消防终端 TCP 接入 | `docker compose --profile gb26875 up -d --build` |
| `harness` | DeepSeek Harness 插件工作流 | `docker compose --profile harness up -d --build` |
| `thingspanel` | ThingsPanel 上游前后端 | `docker compose --profile thingspanel up -d --build` |

### GB/T 26875 设备接入示例

项目提供独立的国标消防终端 TCP/UDP 网关和虚拟设备，用于验证设备注册、凭证、原始报文、解析和标准消息链路：

```bash
docker compose --profile gb26875 up -d --build

# 本地运行虚拟设备（端口和参数以命令帮助为准）
go run ./cmd/gb26875-virtual-device --help
```

网关接收设备报文后调用平台设备接入接口；平台侧仍按“设备管理 → 产品协议绑定 → 原始报文 → 标准消息”的链路处理。协议字段和示例报文见 [GB/T 26875 对接说明](docs/GB26875_DAHUA_V103.md)。

协议开发现在按复杂度提供四条路径：JSON/固定字段十六进制直接配置；Excel/点表通过协议接入助手生成 Go Modbus 映射；受限 JavaScript 仅适用于管理员维护的纯转换；变长、TLV、状态机或请求/应答协议选择 `go_protocol_parser` 并上传受 SHA-256、路径、超时和输出大小限制的已编译 Go Worker。Go 源码不能在 API 进程内直接编译执行，完整契约见 [配置驱动协议](docs/CONFIGURABLE_PROTOCOLS.md) 和 [Go 协议包接入](docs/GO_PROTOCOL_PACKAGES.md)。

## 10. 配置与密钥

直接运行 Go API 且保持 `IOT_DEV_MODE=true` 时不要求创建 `.env`。Docker Compose 明确使用
`IOT_DEV_MODE=false`，因此必须设置 `IOT_JWT_SECRET` 和 `IOT_ADMIN_PASSWORD`，缺失、长度不足或仍为占位符时 API 会拒绝启动。

`.env.example` 是完整部署变量模板，其中的值仍是待替换占位符，不能原样用于生产。对已有数据卷直接复制整份模板，会同时改变 PostgreSQL、Redis、ClickHouse、MinIO、JWT、管理员和 EMQX 凭据，可能导致服务认证失败。已有环境只需在现有 `.env` 或部署平台 Secret 中补齐上述两个认证变量，不要覆盖其他持久化服务密码。

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

## 11. DeepSeek Harness 工作流

平台把模型 Provider 与业务 AI 插件分成两层：

```text
AI 工作台
  -> Go API（登录、租户、SSE、审计）
  -> DeepSeek Harness 源码网关
  -> 业务插件 manifest
  -> Harness Agent Runtime
  -> 平台只读 MCP 工具
```

当前 Harness 插件完整清单（其中业务专用工作流不属于聊天插件管理清单）：

| ID | 界面名称 | 用途 |
|---|---|---|
| `ops-assistant` | AI 运维助手 | 设备状态、活动告警、趋势、知识库辅助排障 |
| `alarm-handler` | AI 告警研判 | 告警核验、影响判断、相似告警和处置建议 |
| `system-observer` | AI 系统状态助手 | 系统状态、产品与设备数量、在线离线、告警与资源统计 |
| `device-health-inspector` | AI 设备健康巡检 | 设备状态、数据新鲜度、活动告警和维修优先级建议 |
| `protocol-assistant` | AI 协议接入助手 | 协议文档、点表和样本报文的字段映射草稿 |

这些是 Harness 的内置插件。动态 Agent 会保存在 Harness 数据卷中，数量和名称以“AI 工作流 → Agent 管理”的实时清单为准。

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

管理员进入“AI 工作流 → Agent 管理”后，还可以查看启用和禁用的完整清单。动态 Agent 支持 JSON 编辑、启用/禁用和删除；内置插件只读。点击“新建 Agent”会在弹窗中提交 Manifest，保存或状态变更后不需要重新部署容器。

### Agent 知识库

知识库统一从左侧“Agent 知识库”进入；文档管理、Agent 归属和检索策略集中在同一页面，避免在 AI 工作流中重复配置。管理员或运维人员可以按租户为当前 Agent 配置：

- 查看当前 Agent 已直接绑定的文档数量。
- 选择按需检索、每次强制检索或完全禁用知识库。
- 配置 TopK、最低相似度以及无证据时是否阻止回答。

上传文档时必须填写 `workflowId`，文档索引也保存同一 Agent ID。平台会将该 ID 写入每次运行的短期 Harness Token，
MCP 服务端只允许检索该 Agent 的文档；模型无法通过修改工具参数扩大范围。“强制检索”会在进入 Harness 前预召回知识；
“必须有证据”在无匹配结果时会停止运行。

这部分与 Dify 的知识库工作流理念相似，但实现仍属于本项目：DeepSeek Harness 负责插件运行，Go API 负责租户、绑定策略、检索授权和审计，不需要额外部署 Dify。

### 新增业务插件

管理员可以直接在“AI 工作流 → Agent 管理”查看完整插件清单。动态 Agent 支持编辑 Manifest、启用/禁用和删除；“新建 Agent”在独立弹窗中通过 JSON 粘贴 Manifest。平台会执行两层校验：Go API 检查字段和只读工具白名单，Harness 再验证完整 Manifest。保存后 Agent 立即出现在工作流下拉框，不需要重建容器。

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

## 12. 其他 AI Provider

不使用 Harness 时，也可以启用 Eino Provider 链路，用于告警分析、规则草稿和报表等后端能力。

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
docker compose up -d --build platform-api ollama weaviate

# Weaviate 的本地向量化模型（首次部署必须拉取）
docker compose exec ollama ollama pull nomic-embed-text
# 可选：Ollama 作为对话 Provider 时再拉取对话模型
docker compose exec ollama ollama pull qwen3:8b
docker compose restart platform-api
```

Compose 默认已经启动 Ollama 和 Weaviate；上面的命令只用于显式重建或补拉模型。Weaviate 数据保存在
`weaviate-data` 命名卷，Ollama 模型保存在 `ollama-data` 命名卷。

AI 工作流页面只展示当前模型服务状态，不再提供 Provider 在线测试或自定义 Provider 配置入口。告警分析、规则草稿等后端能力仍按服务端配置选择 Eino Provider；Provider 凭据和地址不由前端保存或修改。

## 13. Agent 知识库

进入左侧“Agent 知识库”页面可以：

- 拖放或选择单个知识文档。
- 必须直接选择一个 Agent；文档不会再通过产品、分类和标签叠加筛选。
- 设置知识分类与标签作为文档元数据，方便查看。
- 上传原文件并自动提取、分片和建立索引。
- 查看当前租户的文档、索引状态、分片数、大小和上传时间。
- 点击“详情”可以查看实际切片：切片序号、Unicode 字符范围、重叠字符数、切片正文和是否已向量化；详情接口为 `GET /api/v1/knowledge/documents/{id}`。
- 在本页面的“Agent 知识库策略”区域调整该 Agent 的检索模式、TopK、最低相似度和无匹配知识时的处理方式。

支持 PDF、DOCX、PPTX、XLSX、ODT、ODP、ODS、HTML/XML 和 UTF-8 文本，单文件最大 32 MiB。扫描版 PDF 需要先完成 OCR。

当前分片规则是：先提取并清洗文件文本，再按固定窗口切片，每段最多 1200 个 Unicode 字符，默认重叠 200 个字符。
字符范围使用 `[StartChar, EndChar)` 约定（起点包含、终点不包含），所以详情页可以直接核对每段在清洗后全文中的位置。
Weaviate 持久化保存切片正文和这些元数据，并通过 `text2vec-ollama` 使用 `nomic-embed-text` 向量化；页面展示向量化状态和模型信息，不直接展开高维向量数组。

默认 Compose 使用持久化 Weaviate；页面会显示 `WEAVIATE 持久索引`。如果手动运行 API 且未配置
`IOT_WEAVIATE_URL`，才会退回本地内存索引：原文件和文档记录会持久保存，但 API 重启后检索索引需要重新建立。

## 14. 摄像头映射与视频告警

摄像头页面只维护外部视频平台的基础信息：品牌、摄像头名称、摄像头点位、建筑、楼层、房间和关联设备。
一个设备可以关联多个摄像头，但一个摄像头最多关联一个设备。设备告警和视频告警会带出安全的摄像头元数据，供前端或外部视频平台定位直播流。

平台不会解析、采集、代理或预览视频流，不启动 ZLMediaKit，也不调用海康/大华取流 SDK；摄像头表不保存
直播地址和厂商凭据。外部视频平台负责根据 `cameraId` 或摄像头点位提供直播流，平台只负责告警关联和元数据定位。

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

## 15. API 快速验证

登录：

```bash
curl -sS http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"$IOT_ADMIN_PASSWORD\",\"tenantId\":\"tenant_001\"}"
```

PowerShell：

```powershell
$loginBody = @{ username = 'admin'; password = $env:IOT_ADMIN_PASSWORD; tenantId = 'tenant_001' } | ConvertTo-Json
$login = Invoke-RestMethod -Method Post http://localhost:8080/api/v1/auth/login `
  -ContentType application/json `
  -Body $loginBody
$headers = @{ Authorization = "Bearer $($login.accessToken)" }
```

主要接口：

```text
POST         /api/v1/auth/login
GET/POST/PUT /api/v1/products[/{id}]
GET/POST/PUT /api/v1/protocol-packages[/{id}]
POST         /api/v1/protocol-packages/{id}/artifact
POST         /api/v1/protocol-packages/{id}/test
GET/POST     /api/v2/protocols
GET/POST     /api/v2/protocols/{id}/releases
POST         /api/v2/protocols/{id}/package-releases
POST         /api/v2/protocols/{id}/releases/{version}/publish
POST         /api/v2/products/{id}/protocol-binding
POST         /api/v2/products/{id}/protocol-binding/rollback
POST         /api/v2/modbus-tcp/import
GET/POST/PUT /api/v2/device-access-profiles[/{id}]
POST         /api/v2/device-access-profiles/{id}/test
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
POST         /api/v1/ai/health-inspection/pdf
POST         /api/v1/ai/protocol-assistant/generate
POST         /api/v1/ai/protocol-assistant/preview
POST         /api/v1/ai/protocol-assistant/publish
POST         /api/v1/ai/rule-draft
POST         /api/v1/ai/reports
GET/POST     /api/v1/knowledge/documents
GET          /api/v1/knowledge/documents/{id}
GET/POST/PUT /api/v1/integrations/video/cameras[/{id}]
GET          /api/v1/integrations/video/relations?relationType=device&targetId={id}
POST         /api/v1/integrations/video/alarm
POST         /api/v1/mqtt/token
POST         /api/v1/device-mqtt/token
POST         /api/v1/integrations/thingspanel/sync
GET/POST/DELETE /mcp
POST         /mcp/harness
```

## 16. 消息主题

Redpanda/Kafka：

```text
iot.raw.message             iot.parsed.message
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
/iot/parsed/{tenantId}/{productId}/{deviceId}/{messageType}
/iot/device/state/{tenantId}/{productId}/{deviceId}
/iot/alarm/{eventType}/{cityCode}/{districtCode}/{buildingId}/{deviceType}/{deviceId}
```

EMQX 使用 HS256 JWT、内嵌 ACL 和默认拒绝策略，匿名连接会被拒绝。生产安全基线见 [EMQX 生产安全配置](ops/emqx/PRODUCTION_SECURITY.md)。

## 17. MCP

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

## 18. 备份与恢复

`backup-service` 现在随主 Compose 默认启动，并使用 `restart: unless-stopped`，主系统启动或容器异常退出后会自动拉起。启动后默认每天 `00:05`（`Asia/Shanghai`）备份前一天的全部原始日志到 MinIO。全量/增量基础设施备份仍保留为手动能力，不再由默认调度器周期执行。

启动或停止备份服务：

```powershell
docker compose up -d --build
docker compose stop backup-service
```

完整启动主系统时直接执行 `docker compose up -d --build` 即可；如需临时停止备份服务，执行 `docker compose stop backup-service`，后续再次执行 `docker compose up -d` 会恢复它。`backup-staging` 数据卷不会被自动删除，历史备份数据仍会保留。

登录前端后进入“运行中心 → 备份中心”，可以查看全量、增量、原始日志和恢复演练记录，打开任务详情查看 manifest 中的制品清单与 SHA-256。管理员可以在页面触发全量/增量/原始日志备份、下载制品和执行非破坏性恢复演练；普通查看账号只能查看记录和文件元数据。页面通过 `platform-api` 代理访问 `backup-service`，浏览器不会接触备份管理令牌。

Compose 默认使用 `IOT_BACKUP_URL=http://backup-service:8090`，并将同一个 `IOT_BACKUP_ADMIN_TOKEN` 注入 API 和备份服务。若拆分部署，请让 Go API 能访问 `IOT_BACKUP_URL`，并保证两个服务的令牌一致；备份文件以 MinIO 中的 manifest 为准，不依赖浏览器访问 backup-service 的 8090 端口。

手动备份与非破坏性恢复演练：

```powershell
$backupHeaders = @{Authorization='Bearer change-me-backup-admin-token'}
Invoke-RestMethod -Method Post 'http://localhost:8092/backup?type=FULL' -Headers $backupHeaders
Invoke-RestMethod -Method Post 'http://localhost:8092/backup?type=RAW_LOGS' -Headers $backupHeaders
Invoke-RestMethod -Method Post 'http://localhost:8092/restore/drill?backupId=latest' -Headers $backupHeaders
```

调度时间可通过 `IOT_BACKUP_TIME` 和 `IOT_BACKUP_TIMEZONE` 配置；`RAW_LOGS` 手动触发默认也备份前一天。设备原始日志的分层阈值通过 `IOT_RAW_HIGH_FREQUENCY_INTERVAL_SEC` 配置，默认 60 秒：设备的 `reportIntervalSec` 或实际消息间隔不超过阈值时写入 ClickHouse，否则写入 PostgreSQL。单数据库部署会自动回退到可用数据库。

恢复演练只读取制品、校验哈希并记录结果，不覆盖当前数据。生产恢复必须先在隔离环境验证。

## 19. 测试

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

## 20. 压测

```powershell
go run ./cmd/loadgen -url http://localhost:8080 -token $login.accessToken `
  -transport http -profile steady -rate 10000 -duration 2h `
  -devices 300000 -workers 128 -report .\steady-10k.json

go run ./cmd/loadgen -url http://localhost:8080 -token $login.accessToken `
  -transport mqtt -mqtt-broker tcp://localhost:1883 -profile burst `
  -rate 10000 -burst-rate 50000 -burst-duration 15m -duration 30m
```

单机开发烟测不能替代生产容量验收。30k/50k 吞吐、长时间稳定性、HA 切换与 RPO/RTO 必须在目标集群验证。

## 21. 常见问题

### 看不到“AI 运维助手”或工作流下拉框

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

5. 退出旧会话，强制刷新并重新登录。进入“AI 工作流 → 运行工作流 → 工作流插件”；告警研判、设备巡检和协议接入助手请从各自业务页面进入。

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

## 22. ThingsPanel

```bash
docker compose --profile thingspanel up -d --build
```

ThingsPanel Web 默认为 `http://localhost:8088`，API 默认为 `http://localhost:9999`。本项目不修改上游工作树，消防原始证据链、解析、告警、视频、AI 和回放继续由本平台负责。

详细边界和目录同步说明见 [ThingsPanel 二开集成](docs/THINGSPANEL_INTEGRATION.md)。

## 23. 生产部署检查

- 替换所有示例密码、JWT 密钥、Harness Token 和视频平台 Secret。
- 使用 HTTPS、受限管理网络、TLS/mTLS 和企业身份源。
- 为 PostgreSQL、Redis、MinIO、Redpanda、ClickHouse 配置高可用与独立磁盘。
- 将密钥放入 Vault、External Secrets 或云 Secret Manager。
- 配置媒体 Origin 白名单和 Nginx CSP。
- 对原始报文下载、回放、告警操作和 AI 工具调用设置审计留存。
- 执行容量测试、故障注入、备份恢复演练和数据保留合规评审。
- Kubernetes 清单中的 Secret 仅为占位符，不可直接用于生产。

Kubernetes 基线位于 `deploy/k8s`。

## 24. 相关文档

- [DeepSeek Harness 业务插件运行时](docs/AI_PLUGIN_HARNESS.md)
- [ThingsPanel 二开集成边界](docs/THINGSPANEL_INTEGRATION.md)
- [设计文档实现覆盖表](docs/IMPLEMENTATION_COVERAGE.md)
- [EMQX 生产安全配置](ops/emqx/PRODUCTION_SECURITY.md)
