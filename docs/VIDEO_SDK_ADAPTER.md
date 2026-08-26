# 大华/海康视频 SDK 适配边界

平台预览链路分成三层：

1. 直接地址：摄像头映射保存已 allowlist 的 HLS/MP4/WebM，或保存 RTSP/RTMP
   并交给 ZLMediaKit 拉流。
2. 大华：继续通过独立适配器获取短时效直播地址。
3. 海康：Go API 进程直接调用官方 HikCentral Professional Artemis OpenAPI，
   不请求现有 Java 服务，也不保留 /hik/getUrl 兼容分支。

浏览器最终只拿到 ZLMediaKit 的固定 HLS 地址；厂商短时效地址不会写入摄像头
映射表。

## 海康官方 Artemis Go 客户端

海康官方公开资料提供的是设备网络 SDK/原生动态库以及 HikCentral OpenAPI，并未
提供可直接 go get 的厂商官方 Go module。当前 Go 实现因此直接按官方 OpenAPI
签名协议调用：

```
POST /artemis/api/video/v2/cameras/previewURLs
```

Go 代码位于 internal/adapters/video/hikvision_artemis.go，使用部署注入的：

```dotenv
IOT_VIDEO_HIKVISION_API_URL=https://hikcentral.example.internal
IOT_VIDEO_HIKVISION_APP_KEY=<Artemis AppKey>
IOT_VIDEO_HIKVISION_APP_SECRET=<Artemis AppSecret>
```

IOT_VIDEO_HIKVISION_API_URL 可以填 HikCentral 网关根地址，也可以填完整的
/artemis/api/video/v2/cameras/previewURLs 地址。平台会发送官方预览参数：

```json
{
  "cameraIndexCode": "camera-index-code",
  "streamType": 0,
  "protocol": "rtsp",
  "transmode": 1
}
```

请求使用 X-Ca-Key、X-Ca-Signature、X-Ca-Signature-Headers 以及时间戳和
nonce 完成 HMAC-SHA256 签名。响应中的 data.url、data.protocol 和
data.expireTime 被转换为平台内部的短时效 VideoStream。

sdkCameraId 对海康必须填写 cameraIndexCode；sdkCredentialRef 仅保留为
平台侧审计/租户边界字段，AppKey/AppSecret 从环境或密钥管理系统注入，不进入
摄像头映射表。

## 大华适配器契约

大华 dahua_sdk 仍使用独立适配器。平台每次执行预览时发起：

```http
POST /stream
Content-Type: application/json
Authorization: Bearer <IOT_VIDEO_DAHUA_SDK_TOKEN>

{
  "provider": "dahua_sdk",
  "cameraId": "channel-001",
  "credentialRef": "vault://video/dahua/channel-001",
  "tenantId": "tenant-001"
}
```

平台只接受 HTTP 2xx 和包含 streamUrl/url 的 JSON；适配器应使用大华
官方 SDK 登录或取流，并只返回短时效播放地址。

## ZLMediaKit 与过期地址刷新

当厂商返回 RTSP、RTMP 或 HLS 地址时，平台调用 ZLMediaKit addStreamProxy
并返回固定的 HLS 播放地址。如果下一次预览拿到的新厂商地址发生变化，平台先
调用 delStreamProxy 删除旧代理，再注册新代理，避免继续拉取已过期地址。

平台会同时校验：

- HikCentral/大华适配器地址命中 IOT_VIDEO_MEDIA_ALLOWED_HOSTS；
- 厂商返回的流源地址命中 IOT_VIDEO_MEDIA_ALLOWED_HOSTS；
- 浏览器播放 Origin 命中 IOT_VIDEO_PREVIEW_ALLOWED_ORIGINS。

## 验收边界

自动化测试覆盖 Go 签名请求、官方 API 路径、响应解析、短时效地址刷新、
allowlist 和 ZLMediaKit addStreamProxy/delStreamProxy。没有目标 HikCentral
版本、AppKey/AppSecret、摄像头账号和可达设备时，不能把这些测试等同于真实设备
验收。

现场必须补充验证：

- HikCentral OpenAPI 已启用，AppKey/AppSecret 有视频预览权限；
- cameraIndexCode 属于目标 HikCentral 租户且可预览；
- expireTime 前后刷新地址；
- RTSP/RTMP 地址能被 ZLMediaKit 拉取并生成 HLS；
- 浏览器能播放 HLS，且不同租户的固定流名不冲突；
- 日志、错误和审计中不打印 AppSecret、厂商密码或带 token 的完整 URL。

官方资料入口：

- [海康 HikCentral Professional OpenAPI](https://enpinfo.hikvision.com/hkwsen/unzip/20240201150721_74009_doc/)
- [海康开放平台](https://open.hikvision.com/osp)
- [大华 SDK 下载入口](https://previous-depp.dahuasecurity.com/integration/guide/download/sdk)
- [ZLMediaKit HTTP API](https://github.com/ZLMediaKit/ZLMediaKit/wiki/MediaServer%E6%94%AF%E6%8C%81%E7%9A%84HTTP-API)
