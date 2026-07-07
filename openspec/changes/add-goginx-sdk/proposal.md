## Why

第三方应用需要在不暴露额外公网端口的前提下，使用 GoGinX 已有控制通道安全访问用户名下的远端服务。现有控制通道只支持 provider 客户端接收服务端发起的代理子流，缺少 consumer 角色、按用户代理列表和 SDK 主动打开远端服务流的合同。

## What Changes

- 新增 Go library SDK，供 Go 程序、Android Go 组件或其他嵌入方使用 `client_id + credential` 连接 GoGinX 控制通道。
- 新增 consumer client 角色，使 SDK 使用独立客户端凭据访问同一用户名下的已启用代理，同时不替换 provider 客户端会话。
- 新增按用户返回已启用代理列表的控制通道消息，避免复用按 client 作用域的 `ProxySnapshot`。
- 扩展控制通道多路复用能力，使 consumer 可以主动打开数据流，由服务端桥接到对应 provider 客户端并连接固定 proxy target。
- 新增 SDK API：连接认证、代理列表、按 proxy ID 拨号、HTTP transport，以及后续本地固定目标代理入口。
- 为 SDK 桥接流引入服务端资源防护合同：provider open 超时必须随首批实现落地，其余全局/用户/连接限流先建立 hook 和可验证边界。
- 明确本地 SOCKS5/HTTP CONNECT 模式的语义：本批不实现任意目标正向代理，默认只暴露固定 proxy target；真正按目标匹配代理或任意目标转发作为后续扩展。

## Capabilities

### New Capabilities

- `goginx-sdk`: 定义 Go SDK 的配置、连接生命周期、代理列表、直接拨号、本地固定目标入口、错误处理和示例文档合同。

### Modified Capabilities

- `control-channel`: 增加 consumer/provider 会话语义、按用户代理列表消息、consumer 主动流接入、服务端到 provider 的桥接和流打开超时。
- `admin-resource-management`: 增加客户端 kind/role 管理合同，支持通过 CLI 创建 consumer client，默认保持 provider 兼容语义。
- `quotas-and-limits`: 增加 SDK 桥接流的服务端资源防护合同和未完成限流项的缺口跟踪。

## Impact

- 后端领域模型和存储：`internal/domain`、`internal/store`、`internal/store/sqlite` 增加 client kind 和按 user 查询 proxy。
- 控制协议：`internal/control/controlpb/control.proto`、生成的 `control.pb.go`、`internal/control/protocol.go` 增加 `ProxyList` 消息。
- 控制通道运行时：`internal/control/transport.go`、`internal/control/mux.go`、`internal/session` 增加 consumer 分支、stream acceptor、SDK 桥接和超时控制。
- 管理入口：`internal/admin` 和 `cmd/goginx-admin` 支持创建 consumer client；GraphQL/TUI consumer 入口不在首批交付内。
- 新增 SDK 包：`sdk/` 包含配置、客户端、连接、本地入口、错误和包文档。
- 测试：增加 store、protocol、session、control、SDK 单元测试和 SDK e2e；涉及 proto 变更后需要重新生成并验证。
