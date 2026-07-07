## 1. 领域模型和存储

- [x] 1.1 在 `internal/domain` 增加 `ClientKind`，为 `Client` 添加 kind 字段，并让空 kind 默认按 provider 校验
- [x] 1.2 在 `internal/store` 的 proxy repository 接口增加 `ByUserID(ctx, userID string)`
- [x] 1.3 更新 SQLite clients schema 和幂等迁移，增加 `kind text not null default 'provider'`
- [x] 1.4 更新 SQLite client create/read/list/scan 路径，确保 kind 持久化和列顺序一致
- [x] 1.5 实现 SQLite proxy `ByUserID` 查询，按 user 返回 proxy 并保持稳定排序
- [x] 1.6 更新 admin service 创建 client 输入和默认值，支持创建 provider 或 consumer client
- [x] 1.7 更新 `goginx-admin create-client`，增加 consumer flag，默认继续创建 provider client
- [x] 1.8 增加 domain/store/admin CLI 相关测试，覆盖默认 provider、consumer 持久化、旧库迁移和 `ByUserID`

## 2. 控制协议

- [x] 2.1 在 `internal/control/controlpb/control.proto` 增加 `ProxyListRequest`、`ProxyListResponse` 和对应 message type 编号
- [x] 2.2 使用项目约定的 `protoc` 和 `protoc-gen-go` 重新生成 `internal/control/controlpb/control.pb.go`
- [x] 2.3 在 `internal/control/protocol.go` 增加 proxy list Go 类型、message type 映射、marshal 和 decode 分支
- [x] 2.4 复用现有 proxy proto 转换逻辑，确保 proxy list response 包含 SDK 需要的 proxy 元数据
- [x] 2.5 增加 protocol round-trip 测试，覆盖 proxy list request/response 编解码

## 3. 会话和控制通道桥接

- [x] 3.1 在 `internal/session` 增加 `StreamAcceptor` 接口，并在 session/register input 中记录 client kind
- [x] 3.2 为 QUIC stream opener 增加 `AcceptStream(ctx)`，并确认 TCP+TLS mux 已满足 `StreamAcceptor`
- [x] 3.3 为 `ClientConn` 增加 `OpenStream(ctx)`，供 SDK 通过 QUIC 或 TCP+TLS mux 主动打开数据流
- [x] 3.4 重构控制通道认证后的分支：provider 保持现有 snapshot/heartbeat 行为，consumer 使用 proxy list 和 SDK stream accept 行为
- [x] 3.5 实现 `sendProxyList`，按 consumer user 查询 proxy，过滤 enabled 状态并发送 proxy list response
- [x] 3.6 泛化控制消息读循环，支持 heartbeat 和 consumer proxy list request
- [x] 3.7 实现 `acceptSDKStreams`，仅在 opener 支持 `StreamAcceptor` 时接受 consumer 主动数据流
- [x] 3.8 实现 `handleSDKStream`，校验 proxy user、enabled 状态、provider 会话 kind，并拒绝未知、越权、禁用或 provider 离线场景
- [x] 3.9 在桥接时由服务端注入 proxy target host/port，并由 proxy type 派生 provider open-stream kind
- [x] 3.10 使用 `tunnel.CopyBidirectional` 桥接 consumer 数据流和 provider 子流，并确保关闭路径释放资源
- [x] 3.11 增加 provider open 超时常量和 limits hook 文件，首批实现超时，其他资源限制保留明确 TODO 和调用点
- [x] 3.12 增加 session/control 测试，覆盖 consumer 不替换 provider、proxy list、QUIC 桥接、TCP+TLS mux 桥接、拒绝路径和 provider open 超时

## 4. SDK 核心包

- [x] 4.1 新增 `sdk/` 包结构和包文档，声明 SDK 能力、固定 target 语义和安全边界
- [x] 4.2 实现 `sdk.Config`，包含控制地址、TLS 地址、server name、CA 文件、client ID、credential 和允许协议
- [x] 4.3 实现 SDK client 构造、connect、close 和未连接状态管理
- [x] 4.4 复用现有控制通道 dial/auth 机制完成 consumer 认证，并接收初始 proxy list response
- [x] 4.5 实现 `Proxies(ctx)`，支持返回缓存或按需发送 proxy list request 刷新列表
- [x] 4.6 实现 `Dial(ctx, proxyID)`，通过 `ClientConn.OpenStream` 发送 open-stream 请求并返回可读写连接
- [x] 4.7 实现 `DialTCP(ctx, proxyID)` 的 `net.Conn` 适配，明确 deadline/address 行为
- [x] 4.8 实现绑定 proxy ID 的 `HTTPTransport`，通过 SDK dial 路径发送 HTTP 请求
- [x] 4.9 实现 SDK 错误类型，覆盖配置错误、未连接、认证失败、proxy 访问失败和拨号失败，并避免 secret 泄露
- [x] 4.10 增加 SDK 单元测试，覆盖配置校验、未连接错误、proxy list 处理、dial open-stream 消息和错误脱敏

## 5. 本地固定目标入口

- [x] 5.1 实现 SDK 本地监听入口，按 context 生命周期启动和停止
- [x] 5.2 实现直接 TCP 转发到指定 proxy ID 的固定 target
- [x] 5.3 实现 SOCKS5 握手兼容层，但不使用请求目标覆盖远端 proxy target
- [x] 5.4 实现 HTTP CONNECT 握手兼容层，但不使用请求目标覆盖远端 proxy target
- [x] 5.5 增加本地入口测试，覆盖 context 取消、直接 TCP、SOCKS5、HTTP CONNECT 和固定 target 语义

## 6. 端到端验证和文档

- [x] 6.1 增加 e2e 测试：server、provider client、TCP proxy、echo target、consumer SDK 凭据和 SDK dial echo 往返
- [x] 6.2 增加 e2e 或集成测试，确认 consumer 连接不会替换 provider 会话
- [x] 6.3 增加本地入口集成测试，确认本地连接通过指定 proxy ID 到达固定 target
- [x] 6.4 增加 SDK 使用示例，展示 connect、proxies、dial、read/write、close 和本地入口
- [x] 6.5 更新相关文档，说明 consumer client 创建方式、SDK 安全边界、本地入口不是任意目标正向代理
- [x] 6.6 运行 `go build ./...`，确认 proto 生成和新包编译通过
- [x] 6.7 运行 `go test ./internal/domain ./internal/store/... ./internal/session ./internal/control ./sdk -count=1`
- [x] 6.8 运行 `go test ./e2e -run TestSDK -count=1` 或记录无法运行的环境原因
