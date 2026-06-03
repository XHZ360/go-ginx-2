## Why

当前 server/Admin API 已能使用服务端启动时确认的默认 join 服务域名或 IP，但独立 `goginx-admin` CLI 和 TUI 仍倾向本机默认值或仅使用调用进程内的临时默认值。这样同一部署里不同 join token 生成入口可能产生不同的客户端连接参数，远程部署、NAT、容器和负载均衡场景容易误生成不可达的 token。

本变更把默认 join 参数来源统一到服务端配置和环境覆盖，使操作者只需配置一次默认服务域名或 IP，所有管理入口在未显式覆盖时都生成一致的客户端 join 参数。

## What Changes

- 让 `goginx-admin create-client-join`、`goginx-admin client-join-command` 和 `goginx-admin tui` 使用与 server 启动路径兼容的默认 join 参数解析规则。
- 支持 admin CLI/TUI 从部署根默认 server 配置、显式 server 配置参数或受支持环境变量中确认 `join_service_host`，并组合默认 enrollment URL、QUIC 地址、TCP+TLS 地址、server name 和 CA 文件。
- 保留每次创建 join token 时的显式参数覆盖能力；显式传入的 enrollment URL、server address、TLS address、server name 或 CA 文件仍优先于默认值。
- 在无法读取 server 配置或未配置公共地址时，继续提供清晰的本地开发兜底行为，并让远程部署需要覆盖的事实可见。
- 更新文档和测试，避免 `127.0.0.1` 被误认为远程部署的推荐默认值。
- 不改变 join token 格式、enrollment 兑换语义、客户端受管状态格式或控制通道认证协议。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `control-channel`: 扩展 join/enrollment 材料默认服务地址要求，覆盖 Admin API、admin CLI、TUI 等所有受支持的生成入口。
- `deployment-operations`: 扩展默认服务地址确认要求，使 admin CLI/TUI 能复用 server 配置或环境覆盖，而不是只在 daemon 进程内可用。
- `admin-resource-management`: 扩展 TUI 客户端 join 默认值展示和重置 token 行为，要求其默认参数来自统一解析结果。

## Impact

- 影响 `cmd/goginx-admin` 的 join 默认值构造、相关 flags、TUI 启动注入和测试。
- 影响 `internal/config` 中默认 join 参数解析 API 的可复用边界，可能需要拆分或新增面向部署根/server 配置路径的 helper。
- 影响 `internal/admintui` 的默认 join 参数展示、提交和错误反馈。
- 影响 README、`docs/daemon-runtime.md` 或部署示例中对 `join_service_host`、`GOGINX_JOIN_SERVICE_HOST`、`create-client-join` 默认行为的说明。
- 不需要数据库迁移，不新增外部依赖，不改变现有 Admin API 的 GraphQL 字段形状。
