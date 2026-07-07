## Context

GoGinX 当前控制通道面向 provider 客户端：客户端认证后接收按 `client_id` 过滤的 `ProxySnapshot`，服务端在公网入口收到流量后通过该客户端的最新会话打开代理子流。SDK 场景的方向相反：第三方应用作为 consumer 连接控制通道，主动打开到某个 proxy 的数据流，由服务端桥接到拥有该 proxy 的 provider 客户端。

现有模型有两个关键限制。第一，`ProxySnapshot` 只返回当前 client 自己的 proxy，无法满足 consumer 获取同一 user 下所有已启用 proxy 的需求。第二，会话管理按 `client_id` 维护 latest session；如果 SDK 复用 provider client ID，会替换 provider 会话并造成桥接自环。因此本变更先补齐 consumer/provider 角色、按 user proxy list 和桥接流，再在后续阶段落地 SDK 包和本地入口。

## Goals / Non-Goals

**Goals:**

- 为客户端资源增加 provider/consumer kind，默认兼容现有 provider 行为。
- 为 consumer 新增按 user 返回已启用代理列表的控制通道协议。
- 允许 consumer 在已认证控制通道上主动打开数据流，并由服务端桥接到 proxy 所属 provider 的活跃会话。
- SDK 复用现有控制通道认证、TLS/QUIC/TCP+TLS、多路复用和消息编解码机制。
- 首批实现 provider open 超时，避免 provider 阻塞时服务端 goroutine 和流资源无限挂起。
- 建立 SDK 包的公开 API 合同，覆盖连接、代理列表、直接拨号、HTTP transport 和本地固定目标入口。

**Non-Goals:**

- 本变更不实现任意目标正向代理。proxy 的 target 仍由服务端从持久化 proxy 配置注入，SDK 不能指定任意 host:port。
- 首批不要求 GraphQL 或 TUI 提供 consumer 创建入口；CLI 入口足够支撑端到端验证。
- 首批不完整实现全局/用户/连接级限流和令牌桶，只保留 hook、常量和规格缺口跟踪。
- 首批不改变 provider 客户端的 `ProxySnapshot` 语义，不破坏现有 goginx-client 行为。
- 首批不实现代理级 consumer ACL；consumer 可访问同一 user 名下所有已启用 proxy。

## Architecture

SDK 作为嵌入式 Go library 运行在第三方应用进程内，不单独部署服务。它复用 GoGinX 控制通道，通过 QUIC 或 TCP+TLS 连接 `goginx-server`，并在同一控制连接上打开多路复用数据流。服务端收到 consumer 数据流后，不直接连接远端 target，而是查找 proxy 所属 provider client 的活跃会话，再向 provider 打开代理子流。最终流量路径如下：

```text
第三方应用 / Android Go / Go 程序
    |
    v
goginx-sdk
    |  consumer control connection: Auth + ProxyList + OpenStream
    |  QUIC 或 TCP+TLS mux
    v
goginx-server control listener
    |  校验 proxy.UserID、proxy.Status、provider session
    |  注入 proxy.TargetHost / proxy.TargetPort
    v
goginx-client provider session
    |
    v
provider 本地 target service
```

该架构保持两个不变量。第一，consumer 不需要公网入口，也不要求服务端为 SDK 暴露新的数据端口。第二，consumer 不决定远端 target，target 仍来自服务端持久化 proxy 配置。

### Runtime Components

- `sdk.Client`：SDK 主入口，负责配置校验、控制通道连接、认证、proxy list 缓存或刷新、dial 和关闭。
- `sdk.ProxyConn`：单个 proxy 数据流包装，面向调用方表现为 `io.ReadWriteCloser`，TCP 场景可适配为 `net.Conn`。
- `sdk.LocalProxy`：本地固定目标入口，监听本机地址并把所有连接转发到一个指定 proxy ID。
- `control.ClientConn`：现有控制通道客户端连接抽象，新增 `OpenStream(ctx)` 供 SDK 主动打开数据流。
- `session.StreamAcceptor`：服务端侧能力接口，表示某个控制连接既能被服务端 `OpenStream`，也能接受 consumer 主动打开的 stream。
- `control.Server` consumer 分支：认证后发送 `ProxyListResponse`，处理 heartbeat/proxy list request，并启动 consumer stream accept loop。
- provider `goginx-client`：保持现有 provider 行为，接收服务端 open-stream 消息并连接本地 target。

### SDK Package Shape

SDK 包位于仓库根 `sdk/`，保持公开 API 小而稳定：

```text
sdk/
    doc.go          包文档和安全边界
    config.go       Config、默认值和配置校验
    client.go       Client、Connect、Close、Proxies、Dial、DialTCP、HTTPTransport
    proxy_conn.go   ProxyInfo、ProxyConn、net.Conn 适配
    local_proxy.go  本地固定目标入口、SOCKS5/HTTP CONNECT 兼容握手
    errors.go       可判断错误类型和脱敏错误包装
```

公开配置包含控制通道地址、TCP+TLS 备选地址、TLS server name、CA 文件、client ID、credential 和允许协议。`AllowedProtocols` 只限制 SDK 允许尝试的传输，不改变服务端可用监听器。

### SDK Public API Draft

SDK 首批公开 API 以 `github.com/simp-frp/go-ginx-2/sdk` 包暴露。设计文档保留 API 草案，实际实现可以在不破坏规格行为的前提下做小幅 Go 习惯性调整；若需要删除、重命名或改变语义，必须先更新本 OpenSpec change。

```go
package sdk

type Config struct {
    ServerAddress    string
    ServerTLSAddress string
    ServerName       string
    ServerCAFile     string
    ClientID         string
    Credential       string
    AllowedProtocols []string
}
```

字段语义：

- `ServerAddress`：优先使用的控制通道地址，通常为 QUIC 地址，例如 `control.example.com:8443`。
- `ServerTLSAddress`：TCP+TLS 回退控制通道地址。
- `ServerName`：TLS server name，用于服务端证书校验。
- `ServerCAFile`：服务端 CA 文件路径。
- `ClientID`：consumer client ID。
- `Credential`：consumer client credential。
- `AllowedProtocols`：允许 SDK 尝试的协议，首批值为 `quic` 和 `tcp_tls`。

```go
type Client struct {
    // unexported fields
}

func New(cfg Config) *Client
func (c *Client) Connect(ctx context.Context) error
func (c *Client) Proxies(ctx context.Context) ([]ProxyInfo, error)
func (c *Client) Dial(ctx context.Context, proxyID string) (io.ReadWriteCloser, error)
func (c *Client) DialTCP(ctx context.Context, proxyID string) (net.Conn, error)
func (c *Client) HTTPTransport(proxyID string) *http.Transport
func (c *Client) StartLocalProxy(ctx context.Context, addr string, proxyID string) error
func (c *Client) Close() error
```

方法语义：

- `New` 只创建 client 对象，不进行网络连接。
- `Connect` 建立控制通道、发送认证请求，并接收初始 consumer proxy list。
- `Proxies` 返回当前 consumer 可访问的 proxy 列表；实现可以使用缓存，也可以发送 `ProxyListRequest` 刷新。
- `Dial` 打开到指定 proxy ID 的多路复用数据流，返回 `io.ReadWriteCloser`。
- `DialTCP` 面向 TCP proxy 返回 `net.Conn` 兼容对象；deadline 和 addr 行为应在代码注释中明确。
- `HTTPTransport` 返回通过指定 proxy ID 固定 target 转发请求的 `http.Transport`。
- `StartLocalProxy` 启动本地固定目标入口；名称保留原方案，但文档必须说明它不是任意目标正向代理。
- `Close` 关闭控制通道和 SDK 内部后台资源。

```go
type ProxyInfo struct {
    ID          string
    Name        string
    Type        string
    EntryHost   string
    EntryPort   int
    TargetHost  string
    TargetPort  int
    Description string
}
```

`ProxyInfo.Type` 必须与服务端 proxy type 对齐。`EntryHost` 和 `EntryPort` 是管理和展示信息；SDK 数据流不通过公网 entry 连接 target。

SDK 使用示例应覆盖以下最小路径：

```go
client := sdk.New(sdk.Config{
    ServerAddress:    "control.example.com:8443",
    ServerName:       "go-ginx-control.local",
    ServerCAFile:     "data/certs/server-ca.crt",
    ClientID:         "sdk-client-1",
    Credential:       "secret",
    AllowedProtocols: []string{"quic", "tcp_tls"},
})

ctx := context.Background()
if err := client.Connect(ctx); err != nil {
    return err
}
defer client.Close()

proxies, err := client.Proxies(ctx)
if err != nil {
    return err
}

conn, err := client.Dial(ctx, proxies[0].ID)
if err != nil {
    return err
}
defer conn.Close()
```

### Control Messages

provider 继续使用现有 `AuthRequest`、`AuthResponse`、`ProxySnapshot`、`Heartbeat` 和 `OpenStream`。consumer 新增专用 proxy list 消息：

- `ProxyListRequest`：consumer 请求刷新当前可访问 proxy 列表，首批不带过滤字段。
- `ProxyListResponse`：服务端返回版本号和 proxy 列表，列表按 consumer 所属 user 过滤，并排除 disabled proxy。

consumer 数据流打开后，首个消息仍使用 `OpenStream` 表示请求连接某个 `ProxyID`。服务端只信任 `ProxyID` 和连接标识；provider 侧收到的 `OpenStream` 由服务端重新构造，包含服务端注入的 kind、target host 和 target port。

## Core Flows

### Consumer Connect and Proxy List

1. 第三方应用创建 `sdk.Client`。
2. SDK 根据 `AllowedProtocols` 建立 QUIC 或 TCP+TLS 控制连接。
3. SDK 使用 consumer client ID 和 credential 发送认证请求。
4. 服务端验证 client，读取 `Client.Kind`。
5. kind 为 `consumer` 时，服务端注册 consumer session，但不替换任何 provider session。
6. 服务端查询该 user 名下已启用 proxy，发送 `ProxyListResponse`。
7. SDK 保存或返回 proxy list，供调用方选择 proxy ID。

### Direct Dial

1. 调用方执行 `Dial(ctx, proxyID)`。
2. SDK 通过 `ClientConn.OpenStream(ctx)` 在控制连接上打开新 stream。
3. SDK 在该 stream 上发送 consumer `OpenStream{ProxyID, ConnectionID}`。
4. 服务端接受 consumer stream，读取 open-stream 请求。
5. 服务端按 proxy ID 查询持久化 proxy。
6. 服务端校验 proxy 属于 consumer user，且状态为 enabled。
7. 服务端查找 `proxy.ClientID` 对应的活跃 provider session，并确认 session kind 为 provider。
8. 服务端使用带超时的 context 向 provider session 打开子流。
9. 服务端向 provider 发送由 proxy 配置派生的 `OpenStream`，注入 target host/port。
10. provider 连接本地 target，服务端双向桥接 consumer stream 和 provider stream。

### Local Fixed-Target Entry

本地入口是 SDK 进程内功能，不改变服务端协议。调用方指定本地监听地址和 proxy ID 后，SDK 接受本机连接并为每个连接调用 `Dial(ctx, proxyID)`。SOCKS5 和 HTTP CONNECT 只作为本地客户端兼容层：SDK 可以完成握手，但握手请求里的目标地址不参与远端 target 决策。

### Failure Handling

- 认证失败：SDK `Connect` 返回认证错误，错误文本不包含 credential。
- proxy 不存在、越权或禁用：服务端关闭或重置 consumer 数据流；SDK 将该失败映射为 proxy 访问或拨号错误。
- provider 离线：服务端拒绝桥接，且不得回退到 consumer 自身或其他 client。
- provider open 超时：服务端释放该次桥接资源，SDK 收到拨号失败。
- consumer 连接关闭：服务端停止该 consumer 的 accept loop，已建立流按连接关闭自然释放。

## Decisions

### 使用 client kind 区分 provider 和 consumer

`domain.Client` 增加 `Kind`，取值为 `provider` 或 `consumer`，空值在校验和迁移中视为 `provider`。这样旧数据库、旧 CLI 和旧客户端配置保持兼容；SDK 使用独立 consumer client 记录，不复用 provider credential。

替代方案是把 SDK 身份建成单独表或在会话层临时标记角色。单独表会复制 credential 生命周期和管理逻辑；临时标记无法解决持久化创建和审计语义。client kind 是最小且可迁移的模型。

### 新增 ProxyList 消息，不复用 ProxySnapshot

consumer 认证成功后接收 `ProxyListResponse`，内容为该 consumer 所属 user 名下的已启用 proxy。`ProxySnapshot` 继续表示 provider 自己拥有的 proxy 配置快照，不改变它的按 client 作用域语义。

替代方案是在 `ProxySnapshot` 增加 scope 字段。该方案会让已有消息同时承担 provider 配置同步和 consumer 可访问列表两种语义，容易使旧客户端或未来版本错误解释字段。独立消息边界更清晰。

### 服务端注入 proxy target

consumer 的 `OpenStream` 只表达要连接的 `ProxyID` 和连接标识。服务端查出 proxy 后校验 `UserID`、`Status` 和 provider 会话，并把 `TargetHost`、`TargetPort`、由 proxy type 派生出的 kind 写给 provider。SDK 请求中的目标和 kind 不作为授权依据。

该决策保留现有反向代理安全模型：目标地址由管理员配置和服务端持久化状态决定，而不是由 consumer 流量动态指定。

### consumer 流从同一控制连接 accept

QUIC 使用连接上的额外 stream；TCP+TLS 使用现有 mux 的 `AcceptStream`。`session.StreamAcceptor` 扩展 `StreamOpener`，服务端只在 opener 支持 accept 时启动 SDK 流循环。TCP+TLS 的 accept 循环必须在 mux `Start` 后运行，以确保 read loop 已启动。

### 资源防护分阶段落地

首批实现 provider `OpenStream` 超时，避免 provider 不响应时服务端无限等待。全局桥接流上限、每 user 上限、open 速率限制、连接数准入和旧连接关闭作为 hook 与 TODO 进入代码，后续按压测结果启用。

完整限流过早落地会扩大变更面并影响现有控制通道稳定性；但不放置 hook 会导致 SDK 桥接一旦可用便缺少清晰的治理位置。

### 本地代理入口定义为固定目标隧道

SDK 的本地入口可以兼容 SOCKS5/HTTP CONNECT 握手外壳，但首批语义是所有本地连接都转发到一个指定 proxy ID 的固定 target。请求中携带的目标地址不改变远端连接目标。

真正的 SOCKS5/CONNECT 需要按请求目标匹配用户 proxy，或允许 SDK 传目标并重新设计授权模型；这超出首批 SDK 安全边界。

## Phase Boundaries

### Phase 1: Control Channel Foundation

Phase 1 交付 consumer/provider kind、按 user proxy list、consumer stream accept、服务端桥接到 provider、provider open 超时和对应测试。完成后，即使还没有公开 SDK 包，也可以用测试客户端验证 consumer 控制连接、proxy list 和桥接 echo 往返。

### Phase 2: SDK Core

Phase 2 在 `sdk/` 包中交付配置、连接、代理列表、直接拨号、TCP/net.Conn 适配、HTTP transport、错误类型和单元测试。SDK 只依赖 Phase 1 稳定后的控制通道 API。

### Phase 3: Local Fixed-Target Entry

Phase 3 交付本地监听入口、直接 TCP 转发、SOCKS5 握手兼容和 HTTP CONNECT 握手兼容。该阶段必须在文档和测试中维持固定 target 语义，不宣称任意目标正向代理。

### Phase 4: E2E and Documentation

Phase 4 覆盖 server、provider client、TCP proxy、echo target、consumer SDK 凭据、SDK dial 和本地入口的端到端验证，并补齐使用示例和操作文档。

## Risks / Trade-offs

- consumer 默认可访问同一 user 的所有已启用 proxy → 后续需要引入 per-proxy consumer ACL；首批通过 user 边界和 enabled 状态限制访问范围。
- 桥接流增加服务端资源压力 → 首批实现 provider open 超时，并为全局/用户/连接级限制预留 hook；完整限流进入后续任务。
- TCP+TLS mux 接受双向流的时序更敏感 → consumer accept 循环必须在 mux 启动之后，测试覆盖 QUIC 和 TCP+TLS 两种路径。
- 本地 SOCKS5/HTTP CONNECT 名称可能误导使用者 → 公开文档和 API 注释必须说明这是固定目标入口，不是任意目标正向代理。
- proto 变更需要工具链一致 → 实施前确认 `protoc` 和 `protoc-gen-go`，生成文件纳入同一变更并通过 `go build ./...` 验证。

## Migration Plan

1. SQLite schema 增加 `clients.kind text not null default 'provider'`，迁移使用幂等补列；旧 client 自动成为 provider。
2. 领域层 `Client.Validate` 对空 kind 默认 provider，避免旧测试和旧创建路径失败。
3. CLI `create-client` 增加 consumer flag；未传 flag 仍创建 provider。
4. 控制通道先保持 provider 分支逻辑不变，再新增 consumer 分支和新消息。
5. SDK 包在控制通道测试通过后实现，避免 SDK 直接依赖尚未稳定的协议扩展。
6. 回滚时保留 `clients.kind` 列无害；旧代码忽略该列但不能使用 consumer 功能。

## Open Questions

- consumer ACL 是按 proxy 绑定 client、按 user role，还是引入单独 access policy 表。
- 本地入口最终是否保留 `StartLocalProxy` 名称，还是改为更准确的 `ExposeLocal` 或 `StartLocalTunnel`。
- 完整资源防护参数的默认值需要压测后确定，包括全局桥接流、每 user 桥接流、open 速率和连接数上限。
