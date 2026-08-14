# ThingsPanel 二开集成边界

本仓库已在 `../upstream` 固定下载 ThingsPanel 官方后端和前端。平台扩展不修改上游核心，便于后续升级：

- ThingsPanel 继续提供用户、角色、设备、产品、分组、基础规则、可视化与菜单。
- `iot-platform` 提供原始报文归档、独立解析、分层存储、状态、消防告警、视频、AI、回放和聚合 API。
- 前端可把本平台页面通过反向代理挂到 ThingsPanel 菜单，也可独立运行。
- ThingsPanel 设备 ID 与本平台 `deviceId` 保持一致；不能一致时，由同步任务维护外部 ID 映射。
- 平台本地账号登录失败时，会调用 ThingsPanel `/api/v1/login` 验证账号，并把上游租户/角色转换为本平台短期 JWT。
- 服务账号按 `IOT_THINGSPANEL_SYNC_INTERVAL` 周期读取 ThingsPanel 设备目录，幂等同步产品和设备；已有平台设备凭证不会被覆盖。
- 管理员可调用 `POST /api/v1/integrations/thingspanel/sync` 立即同步，结果含设备/产品数量并进入审计日志。
- `thingspanel` profile 中后端连接 EMQX 时由容器启动脚本签发限于内部服务的 JWT，不启用匿名 MQTT。

## 启动

```powershell
$env:IOT_THINGSPANEL_URL='http://backend:9999'
$env:IOT_THINGSPANEL_USER='<ThingsPanel 服务账号>'
$env:IOT_THINGSPANEL_PASSWORD='<服务账号密码>'
docker compose --profile thingspanel up -d --build
```

入口：ThingsPanel Web `http://localhost:8088`，ThingsPanel API `http://localhost:9999`，消防扩展平台 Web `http://localhost:8080`，消防扩展 API `http://localhost:8081`。首次初始化 ThingsPanel 后填写服务账号并重启 `platform-api`，或直接调用手动同步接口。

目录同步映射：

| ThingsPanel | 本平台 |
|---|---|
| tenant / authority | JWT `tenantId` / `role` |
| device_config_id | Product metadata `thingsPanelDeviceConfigId` |
| device id/number/name | ManagedDevice ID/code/name |
| device type | 产品 category 与设备 role |

同步仅处理目录主数据；消防原始证据链、解析、告警、视频、AI 和回放仍由本平台负责。

推荐反向代理：

```text
/api/v1/raw-messages  -> platform-api:8080
/api/v1/replays       -> platform-api:8080
/api/v1/ai            -> platform-api:8080
/api/v1/integrations  -> platform-api:8080
/mcp                  -> platform-api:8080
其余 /api             -> ThingsPanel backend
```

上游版本由 `../upstream` 工作树的 commit 锁定。升级时先跑本项目测试、Compose profile 构建和登录/目录同步契约测试，不直接覆盖二开代码。
