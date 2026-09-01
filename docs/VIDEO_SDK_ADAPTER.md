# 视频接入边界

当前平台不负责直播流解析、采集、代理或预览。摄像头管理只保存以下元数据：

- 品牌
- 摄像头名称
- 摄像头点位
- 建筑、楼层、房间
- 关联设备

关联关系为“一摄像头最多关联一个设备、一个设备可以关联多个摄像头”。设备告警和视频告警返回安全的摄像头
元数据，前端或外部视频平台可以据此定位直播流。

## 外部视频平台职责

外部视频平台负责：

- 维护真实直播地址、厂商凭据和 SDK 会话；
- 根据 `cameraId` 或点位信息提供直播流；
- 向平台发送视频告警 Webhook 或 MQTT 消息。

平台不会调用海康 Artemis、大华取流 SDK，也不会启动 ZLMediaKit。摄像头 API 不接受或返回直播地址、SDK 地址、
厂商凭据和播放地址；直播预览接口已移除。

## 视频告警 Webhook

平台仍然支持视频告警接入和跨源融合。请求需要使用：

```text
X-Video-Platform-ID: video-platform-1
X-Timestamp: Unix 秒
X-Signature: hex(HMAC-SHA256(secret, timestamp + rawBody))
```

生产环境使用 `IOT_VIDEO_PLATFORM_SECRETS` 和 `IOT_VIDEO_PLATFORM_TENANTS` 将视频平台绑定到租户；视频告警中的
`cameraId` 必须属于该租户且处于启用状态。告警落库后，平台根据摄像头关联信息补充品牌、名称、点位、建筑、
楼层和房间，但不接触直播流本身。

旧版本的厂商取流和 ZLMediaKit 适配器代码仅作为历史兼容测试保留，不属于当前生产启动链路。
