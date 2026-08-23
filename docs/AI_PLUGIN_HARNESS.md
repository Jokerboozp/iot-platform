# DeepSeek Harness 业务插件运行时

平台把“模型 Provider”和“业务 AI 插件”拆成两层：Provider 只负责模型调用，业务插件负责角色提示、可见工具和执行边界。因此 AI 运维助手、AI 告警研判等能力可以独立增加、停用或切换，不需要修改告警核心逻辑。

```text
Web AI 工作台
  -> Go API（登录用户、租户隔离、SSE、审计）
    -> DeepSeek Harness 源码网关
      -> 业务插件 manifest
        -> DeepSeek Harness agent runtime
          -> 租户绑定的只读 MCP 工具
```

现有的 Eino/Provider 链路继续用于告警自动分析、规则草稿和 Provider 在线测试；Harness 工作流用于可追踪、可选插件的交互式助手。两者互不耦合。

## 源码版本

Harness 源码不会复制进本仓库。`deploy/deepseek-harness/REVISION` 固定了经过适配的上游提交，脚本会将该提交拉到被 Git 忽略的 `upstream/deepseek-harness`：

```bash
./scripts/fetch-deepseek-harness.sh
```

脚本不会覆盖有本地修改的上游目录。升级时先验证新提交，再同时更新 `REVISION` 和脚本中的锁定值。

Docker 构建会按上游 `python/sdk-runtime` 的依赖清单生成独立 Node carrier，并执行真实 SDK、Cordis 与模拟 MCP 的启动握手；源码工作区的开发用软链接不会直接作为生产运行时。

## 业务插件

插件清单位于 `deploy/deepseek-harness/plugins/*.json`。网关启动时校验全部清单，并通过 `GET /v1/plugins` 提供给前端。当前包含：

- `ops-assistant`：设备、告警、属性趋势、相似告警和知识库辅助排障。
- `alarm-handler`：聚焦告警事实核验、影响判断和人工处置建议。

新增插件时复制一份清单并修改以下字段：

```json
{
  "schemaVersion": 1,
  "id": "my-workflow",
  "name": "我的 AI 插件",
  "description": "面向用户的说明",
  "version": "1.0.0",
  "enabled": true,
  "persona": "严格限定业务角色和禁止事项的系统提示",
  "defaultModel": "deepseek-v4-flash",
  "maxTokens": 4096,
  "allowedTools": ["mcp__iot__query_device_latest"]
}
```

`id` 必须唯一。`allowedTools` 只能是部署代码内定义的只读上限集合；即使清单被误改，Harness 的全局单调拒绝 guard 也会拒绝 shell、文件、子 Agent、设备控制和写操作。修改清单后重新构建或重启侧车即可，不需要修改网关代码。

## 请求与事件协议

Go API 暴露：

- `GET /api/v1/ai/workflows`：列出已安装的业务插件。
- `POST /api/v1/ai/chat`：兼容的非流式调用；未配置 Harness 时回退到原有本地助手。
- `POST /api/v1/ai/chat/stream`：SSE 流式运行插件。

浏览器只提交 `workflowId`、`conversationId`、`question` 和可选的 `maxTokens`。每次运行由 Go API 生成 Run ID，并签发有效期两分钟、绑定租户、用户、Run ID、Audience 和只读 scopes 的 MCP JWT。浏览器拿不到该令牌。

流式事件固定为：

- `run.started`
- `text.delta`
- `tool.started`
- `tool.completed`
- `run.completed`
- `run.failed`

Harness 的 reasoning 分片不会发给浏览器；工具事件只包含工具名、调用 ID、状态和安全摘要，不返回完整参数、原始结果或凭据。

## 安全边界

- Go API 与 Harness 的固定内部令牌使用 `X-IOT-Harness-Token`；短期 MCP JWT 单独使用 `Authorization: Bearer ...`，两者不混用。
- 专用 `/mcp/harness` 仅接受 POST，限制请求体大小，并校验 `tokenUse`、Audience、租户和每个工具的精确 scope。
- Harness 组合不加载 shell、文件系统、skills、jobs、goal、todo 或 subagent 工具。
- 每个浏览器会话 ID 都由后端结合租户和用户派生为内部 Session ID，防止跨租户会话碰撞。
- 工具查询有条数上限；所有工具调用写入平台审计仓储。

## Docker 启动

在 `.env` 中配置：

```text
DEEPSEEK_API_KEY=<your-api-key>
IOT_AI_HARNESS_URL=http://deepseek-harness:8091
IOT_AI_HARNESS_TOKEN=<至少 32 字节随机内部密钥>
IOT_AI_HARNESS_MCP_URL=http://platform-api:8080/mcp/harness
IOT_AI_HARNESS_MODEL=deepseek-v4-flash
IOT_AI_HARNESS_TIMEOUT=90s
```

然后运行：

```bash
./scripts/fetch-deepseek-harness.sh
docker compose --profile harness up -d --build platform-api deepseek-harness platform-web
docker compose logs -f deepseek-harness platform-api
```

没有配置 `IOT_AI_HARNESS_URL` 时，平台不会连接侧车，原有 AI Provider 和非 Harness 功能仍可使用。
