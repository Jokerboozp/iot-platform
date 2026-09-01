# 一键离线部署

本项目的离线部署采用“两阶段”方式：

1. 在有网且已启动 Docker Engine 的打包机执行对应操作系统的打包脚本。
2. 把生成的 `iot-platform-offline-*` 目录整体传到服务器。
3. 服务器执行对应操作系统的部署脚本。

服务器端不会执行 `go mod download`、`npm ci`、`pnpm install`，也不会拉取镜像。部署脚本固定使用 `--no-build --pull never`。

## 打包机

Windows：

```powershell
cd D:\iot\platform
powershell -ExecutionPolicy Bypass -File .\scripts\package-offline-windows.ps1
```

macOS：

```bash
cd /path/to/iot/platform
bash ./scripts/package-offline-macos.sh
```

Linux：

```bash
cd /path/to/iot/platform
bash ./scripts/package-offline-linux.sh
```

已有正式密钥和配置时，三种系统都可以增加对应的 `--env-file` 或 `-EnvFile` 参数：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\package-offline-windows.ps1 `
  -EnvFile .\.env.production
```

```bash
bash ./scripts/package-offline-linux.sh --env-file ./.env.production
```

加入本地 Ollama + Weaviate，并把指定模型一起打包：

Weaviate 使用 `nomic-embed-text` 建立本地向量索引；离线包需要同时带上该嵌入模型和对话模型。

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\package-offline-windows.ps1 `
  -IncludeAi `
  -OllamaModel qwen3:8b `
  -OllamaEmbeddingModel nomic-embed-text
```

macOS：

```bash
bash ./scripts/package-offline-macos.sh --include-ai --ollama-model qwen3:8b --ollama-embedding-model nomic-embed-text
```

Linux：

```bash
bash ./scripts/package-offline-linux.sh --include-ai --ollama-model qwen3:8b --ollama-embedding-model nomic-embed-text
```

加入 Harness 或 ThingsPanel：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\package-offline-windows.ps1 `
  -IncludeHarness `
  -IncludeThingsPanel
```

macOS：

```bash
bash ./scripts/package-offline-macos.sh --include-harness --include-thingspanel
```

Linux：

```bash
bash ./scripts/package-offline-linux.sh --include-harness --include-thingspanel
```

如果需要同时启动 GB/T 26875 网关，增加 `-IncludeGb26875`：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\package-offline-windows.ps1 -IncludeGb26875
```

macOS：

```bash
bash ./scripts/package-offline-macos.sh --include-gb26875
```

Linux：

```bash
bash ./scripts/package-offline-linux.sh --include-gb26875
```

所有可选组件：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\package-offline-windows.ps1 -Full
```

macOS：

```bash
bash ./scripts/package-offline-macos.sh --full
```

Linux：

```bash
bash ./scripts/package-offline-linux.sh --full
```

`-Full` 只表示镜像和文件全部打包。DeepSeek Harness 运行时仍需要可访问的模型服务；真正完全无外网时，应使用 `-IncludeAi` 的本地 Ollama，并且提前打包模型卷。

如果不传 `-EnvFile`，脚本会生成随机数据库密码、JWT 密钥、管理员密码、EMQX/Grafana 密码和备份 Token，写入 `.env.offline`，并把查看凭据写入 `OFFLINE-CREDENTIALS.txt`。离线包包含密钥，必须通过受控介质传输并限制文件权限。平台不负责直播流获取；外部视频平台仍需独立配置并向平台发送视频告警。

脚本会为运行镜像固定以下离线标签：

```text
iot-platform-api:offline
iot-platform-web:offline
iot-platform-backup:offline
iot-deepseek-harness:offline
iot-thingspanel-backend:offline
iot-thingspanel-web:offline
```

## 服务器

Windows：

```powershell
cd D:\path\to\iot-platform-offline-YYYYMMDD-HHMMSS
powershell -ExecutionPolicy Bypass -File .\scripts\deploy-offline-windows.ps1
```

macOS：

```bash
cd /opt/iot-platform-offline-YYYYMMDD-HHMMSS
chmod +x scripts/deploy-offline-macos.sh
./scripts/deploy-offline-macos.sh
```

Linux：

```bash
cd /opt/iot-platform-offline-YYYYMMDD-HHMMSS
chmod +x scripts/deploy-offline-linux.sh
./scripts/deploy-offline-linux.sh
```

部署脚本会依次完成：

- 校验 `images.tar` 的 SHA-256；
- `docker load` 导入全部镜像；
- 如果存在 `ollama-data.tgz`，在目标机新建空的 `iot-platform_ollama-data` 卷并恢复模型；
- 校验 Compose 配置；
- 使用 `--no-build --pull never` 启动服务；
- 等待 `/health/live` 返回成功。

如果目标机已经存在同名 Ollama 数据卷，脚本会跳过模型恢复，不覆盖现有数据。

离线部署会随主系统一起启动 `backup-service`，默认每天按 `IOT_BACKUP_TIME` 和 `IOT_BACKUP_TIMEZONE` 备份前一天的原始日志。若需要单独重新拉起备份服务，在离线包目录执行：

```powershell
docker compose --env-file .\.env.offline -f .\compose.yaml -f .\compose.offline.yaml up -d --no-build --pull never backup-service
```

停止备份服务：

```powershell
docker compose --env-file .\.env.offline -f .\compose.yaml -f .\compose.offline.yaml stop backup-service
```

启用或停止服务不会自动删除 `backup-staging` 数据卷；如需回收历史备份空间，请先确认数据保留要求后单独处理。

## 数据迁移和安全边界

- 全新部署会创建新的数据库、对象存储和消息队列卷。
- 已有系统迁移时，应先使用项目备份能力或数据库/对象存储逻辑备份，不要直接复制 Docker/WSL 虚拟磁盘。
- 生产环境不要使用 Compose 默认密码；使用 `-EnvFile` 时，打包脚本会拒绝包含示例密码的配置。
- 设备、视频平台、ThingsPanel 或 DeepSeek 的运行时地址仍必须在目标内网可达；“镜像离线”不等于所有外部业务自动变成本地服务。
