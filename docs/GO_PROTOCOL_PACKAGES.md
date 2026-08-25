# Go 协议包接入

对于变长、TLV、带状态机或需要请求/应答的设备，不需要把解析器重新编译进平台。平台支持上传一个已经编译好的 Go 协议 worker，并通过统一的 JSON Lines 契约调用它。

## 运行方式

1. 在“协议开发”中新建协议包，解析器选择 `go_protocol_parser`，保存为草稿。
2. 在“上传 Go 协议包”处上传当前部署平台对应操作系统和 CPU 架构的可执行文件。也可以调用：

   ```http
   POST /api/v1/protocol-packages/{id}/artifact
   Content-Type: multipart/form-data
   field: artifact
   ```

3. 平台把文件保存到 `IOT_DATA_DIR/protocol-packages/{tenant}/{package}/{version}/`，记录 SHA-256，不接受客户端直接提交的绝对路径。
4. 先用“解析调试”运行样本，再把协议包发布并绑定到产品。运行时每条原始报文都会启动受限时长的独立 worker 进程；修改文件、超时、输出过大或返回非法标准消息都会导致本次解析失败，不会拖死 API 进程。

Go 源码不能被平台直接当作生产解析器执行。请在受控构建环境编译后上传二进制；这样不会把 Go 编译器、任意依赖和源码执行权限放进 API 容器。当前执行器已经限制路径、文件大小（64 MiB）、单次运行时间（默认 2 秒，最多 10 秒）、标准输出大小（1 MiB），并且不向 worker 传递数据库密码、JWT 密钥等应用环境变量。生产环境仍建议把 worker 放到单独的无网络容器或沙箱中。

## Worker 契约

worker 从标准输入读取一行 JSON，内容就是平台的 `RawMessage`：

```json
{
  "messageId": "raw_1",
  "tenantId": "tenant_001",
  "productId": "product_fire",
  "deviceId": "device_1",
  "protocol": "vendor-v2",
  "transport": "TCP",
  "payloadFormat": "hex",
  "payload": "AA 01 2A",
  "receivedAt": 1710000000000
}
```

worker 输出一行标准消息（也可以包在 `standardMessage` 字段中）：

```json
{
  "messageType": "PROPERTY_REPORT",
  "timestamp": 1710000000000,
  "properties": {"temperature": 42},
  "event": {},
  "tags": {"vendor": "example"}
}
```

`messageType` 必须是平台支持的标准消息类型。租户、产品和设备 ID 由平台原始报文覆盖，worker 不能借此把数据写入其他租户。发生业务解析错误时输出 `{"error":"..."}` 并以非零状态退出。

示例 worker 位于 `examples/go-protocol-worker`，可以直接编译：

```powershell
go build -trimpath -ldflags="-s -w" -o protocol-worker.exe ./examples/go-protocol-worker
```

Docker Linux 部署要编译 Linux 二进制，例如：

```powershell
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -trimpath -ldflags="-s -w" -o protocol-worker ./examples/go-protocol-worker
```

## 版本与回滚

每个协议包版本独立保存 artifact 和摘要。不要覆盖已经发布版本；上传新版本后先用样本验证，再把产品切换到新版本。旧版本文件保留在数据卷中，可通过重新绑定产品回滚。真实设备联调仍需验证设备端重发、网络隔离、CPU/内存限制和请求/应答超时，代码测试不能替代现场验收。
