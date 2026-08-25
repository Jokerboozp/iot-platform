# 配置驱动协议

协议开发页面现在提供三种不需要新增 Go 代码的解析方式：

## JSON 路径映射

解析器选择 `configurable_json_parser`，协议标识使用 `json`，载荷格式使用 `json`。配置示例：

```json
{
  "properties": {
    "temperature": {"path": "$.data.temp", "type": "number", "scale": 0.1},
    "smoke": "$.data.smoke"
  },
  "tags": {
    "deviceType": "$.kind"
  },
  "timestampPath": "$.occurredAt",
  "timestampUnit": "s",
  "messageType": "PROPERTY_REPORT"
}
```

字段支持 `number`、`integer`、`boolean`、`string` 和 `json`，未找到的路径可用 `default` 提供默认值。路径是安全的 JSONPath-lite，只读对象字段和数组下标，不执行脚本。

## 固定字段十六进制映射

解析器选择 `configurable_hex_parser`，协议标识使用 `config-hex`，载荷格式使用 `hex`：

```json
{
  "startHex": "AA",
  "endHex": "55",
  "checksum": "sum8",
  "checksumStartOffset": 1,
  "fields": [
    {"name": "temperature", "offset": 1, "length": 2, "type": "int16", "endian": "little", "scale": 0.1}
  ]
}
```

字段偏移从完整报文第 0 字节开始，支持 `uint8`、`int8`、`uint16`、`int16`、`uint32`、`int32`、`float32`、`ascii` 和 `hex`。这适用于固定长度传感器报文；变长、TLV、多信息体和复杂会话协议继续使用受控内置解析器，例如 `gb26875_dahua_parser`。

发布前请在“解析调试”中输入样本并确认标准消息结果，再将协议包发布并绑定到产品。

## 编译后的 Go 协议包

如果协议是变长、TLV、需要状态机或有请求/应答，选择 `go_protocol_parser` 并上传已经编译的 Go worker。平台不在 API 容器里编译或直接执行 `.go` 源码，而是通过独立进程的 JSON Lines 契约调用上传文件；路径、SHA-256、超时和输出上限由平台校验。完整契约、编译示例和生产隔离边界见 [`GO_PROTOCOL_PACKAGES.md`](GO_PROTOCOL_PACKAGES.md)。

## AI 协议接入助手

“协议接入助手”把协议资料整理成同一条人工确认链路：

1. 在页面上传 PDF、DOCX、XLSX/CSV 点表或直接粘贴点表文本，并填写一条真实样本报文。
2. 平台把文件提取为文本，调用已配置的 AI Provider 生成结构化字段草稿和受限 JavaScript 源码。
3. 操作员在“字段映射与代码”表单中修改字段名、解析表达式、消息类型，必要时直接编辑源码。
4. 点击“运行解析预览”，平台在与生产相同的 Goja 沙箱中执行脚本，展示标准消息、属性、事件和标签。
5. 点击“确认并发布协议”，平台再次校验样本并保存 `javascript_sandbox_parser` 协议包。到产品管理绑定该包后，设备原始报文会在运行时使用这段脚本，不需要重新编译或部署服务。

对应 API 为：

- `POST /api/v1/ai/protocol-assistant/generate`（`multipart/form-data`，字段 `file`、`pointTable`、`samplePayload` 等）
- `POST /api/v1/ai/protocol-assistant/preview`
- `POST /api/v1/ai/protocol-assistant/publish`

AI 草稿和用户提交的源码都只能在解析沙箱内运行。脚本无网络、文件、环境变量、数据库和平台 API 权限；源代码大小、执行时间和输出大小也有上限。发布仍需要 `operator` 权限和样本校验，生成失败或样本不匹配时不会自动发布。

## 受限 JavaScript 解析器

对于无法用 JSON 路径或固定字节字段描述的设备，选择
`javascript_sandbox_parser`，在协议开发页面直接编写或载入 `.js` 文件。脚本只
允许定义一个纯函数 `parse(raw)`，不能访问文件、网络、环境变量、数据库或平台
服务；单个脚本最大 64 KiB，执行超时会被终止。保存并发布协议包后，脚本内容随
协议包配置存储，设备报文会在运行中的解析器中立即使用，不需要重新构建或部署
API 镜像。

```javascript
function parse(raw) {
  const bytes = hexToBytes(raw.payload)
  const smoke = (bytes[0] & 1) === 1
  return {
    messageType: smoke ? 'ALARM_REPORT' : 'PROPERTY_REPORT',
    properties: {
      smoke,
      temperature: bytes[1] / 10,
      battery: bytes[2]
    },
    tags: { deviceType: 'smoke' }
  }
}
```

解析函数接收完整原始报文对象，返回 `messageType`、`properties`、`event`、
`tags`、`timestamp` 中需要的字段。可以使用内置的 `hexToBytes(text)` 和
`toInt(text)` 辅助函数。这个扩展点适合平台管理员维护的纯转换脚本；如果要让
不受信任的租户上传代码，建议把脚本执行器拆成独立、无网络、限资源的 worker。
