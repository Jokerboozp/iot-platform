# EMQX 生产安全基线

开发 Compose 已启用与平台同密钥的 HS256 JWT 认证、JWT 内嵌 ACL、文件授权兜底拒绝和匿名连接拒绝。浏览器令牌 15 分钟自动续期，设备用独立凭证换取 24 小时限域令牌，平台服务令牌通过 CredentialsProvider 每次重连刷新。

生产部署还必须：

- 通过 Secret 管理系统注入随机 `IOT_JWT_SECRET`，平台和 EMQX 必须一致；禁止使用 Compose 默认值。
- 设置随机 `EMQX_NODE__COOKIE`，同一 EMQX 集群节点保持一致。
- TCP/WebSocket 监听器启用 TLS；公网设备优先 mTLS，Dashboard 和 API 仅开放到运维网。
- 若接入企业 LDAP/OIDC/JWKS，替换本地 HMAC 验证并保留同等 Topic ACL。
- 定期轮换设备凭证和平台 JWT 密钥，轮换期间使用双密钥/JWKS 避免全量断连。

最小权限原则：

- 采集侧只允许发布 `/jetlinks/raw/{tenantId}/{productId}/{deviceId}`。
- 平台接入服务只允许订阅 `/jetlinks/raw/#`。
- 视频平台只允许发布 `/external/video/alarm/{tenantId}/{cameraId}`。
- Web 用户只允许订阅其租户范围内的 `/iot/alarm/{tenantId}/#` 与 `/iot/device/state/{tenantId}/#`。
- 设备只允许发布 `/external/raw/{tenantId}/{productId}/{deviceId}` 并订阅自己的命令 Topic。
- 禁止外部账号订阅 `/jetlinks/raw/#`，生产监听器启用 TLS/mTLS。
- 管理 API 与 Dashboard 只开放在运维网，默认密码必须修改。
