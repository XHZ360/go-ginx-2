## ADDED Requirements

### Requirement: SDK public API surface
GoGinX SDK MUST 在 `github.com/simp-frp/go-ginx-2/sdk` 包中提供稳定的首批公开 API。该 API MUST 至少包含配置结构、client 构造、连接生命周期、proxy 列表、直接拨号、TCP 适配、HTTP transport、本地固定目标入口和 proxy 信息类型。

#### Scenario: Public constructors and lifecycle methods exist
- **WHEN** 调用方导入 `github.com/simp-frp/go-ginx-2/sdk`
- **THEN** 调用方可以使用 `New(Config)` 创建 SDK client
- **AND** 调用方可以调用 `Connect(ctx)` 和 `Close()` 管理控制通道生命周期

#### Scenario: Public proxy access methods exist
- **WHEN** 调用方持有已连接 SDK client
- **THEN** 调用方可以调用 `Proxies(ctx)` 获取 `[]ProxyInfo`
- **AND** 调用方可以调用 `Dial(ctx, proxyID)` 获取 `io.ReadWriteCloser`
- **AND** 调用方可以调用 `DialTCP(ctx, proxyID)` 获取 `net.Conn`

#### Scenario: Public integration helpers exist
- **WHEN** 调用方需要集成标准库 HTTP 或本地入口
- **THEN** 调用方可以调用 `HTTPTransport(proxyID)` 获取 `*http.Transport`
- **AND** 调用方可以调用 `StartLocalProxy(ctx, addr, proxyID)` 启动固定目标本地入口

#### Scenario: Proxy info fields are available
- **WHEN** 调用方读取 SDK 返回的 `ProxyInfo`
- **THEN** 调用方可以访问 proxy ID、名称、类型、入口 host、入口 port、目标 host、目标 port 和描述字段

### Requirement: SDK client configuration
GoGinX SDK SHALL 提供 Go library 配置结构，使调用方可以指定控制通道地址、TLS 信任材料、客户端凭据和允许协议。SDK MUST 使用安全的服务端证书校验，除非未来规格显式定义受控的不安全测试模式。

#### Scenario: Configure secure control connection
- **WHEN** 调用方创建 SDK client 并提供服务端地址、服务端名称、CA 文件、client ID 和 credential
- **THEN** SDK 使用这些配置建立已验证的控制通道连接

#### Scenario: Missing required configuration is rejected
- **WHEN** 调用方尝试连接但缺少服务端地址、client ID、credential 或必要 TLS 信任配置
- **THEN** SDK MUST 返回可由调用方处理的配置错误

#### Scenario: Allowed protocol order is honored
- **WHEN** 调用方配置允许协议列表
- **THEN** SDK 按配置允许范围尝试 QUIC 或 TCP+TLS 控制通道
- **AND** SDK MUST NOT 使用未被允许的协议作为回退

### Requirement: SDK authentication lifecycle
SDK SHALL 使用与 GoGinX client 兼容的 client ID 和 credential 完成控制通道认证，并提供明确的连接和关闭生命周期。

#### Scenario: Connect authenticates consumer credential
- **WHEN** 调用方使用有效 consumer client 凭据调用 SDK connect
- **THEN** SDK 建立控制通道并完成认证
- **AND** 服务端把该连接作为 consumer 会话处理

#### Scenario: Invalid credential fails connect
- **WHEN** 调用方使用未知 client ID 或错误 credential 调用 SDK connect
- **THEN** SDK connect 返回认证失败错误
- **AND** SDK MUST NOT 暴露 credential 明文到错误字符串

#### Scenario: Close releases connection
- **WHEN** 调用方关闭已连接 SDK client
- **THEN** SDK 关闭底层控制通道和相关后台处理资源

### Requirement: SDK proxy listing
SDK MUST 提供获取当前 consumer 可访问代理列表的 API。该 API MUST 返回服务端按 user 作用域授权并过滤后的代理信息。

#### Scenario: List proxies after connect
- **WHEN** SDK 已连接且调用方请求 proxy 列表
- **THEN** SDK 返回服务端提供的 consumer proxy list
- **AND** 列表只包含当前 consumer 可访问的已启用 proxy

#### Scenario: List proxies before connect fails
- **WHEN** 调用方在 SDK 未连接时请求 proxy 列表
- **THEN** SDK 返回未连接错误

#### Scenario: Proxy info contains stable metadata
- **WHEN** SDK 返回 proxy 列表
- **THEN** 每个 proxy info 至少包含 proxy ID、名称、类型、入口信息、目标信息和描述
- **AND** SDK MUST NOT 在 proxy info 中包含 provider credential、consumer credential、私钥或 token 明文

### Requirement: SDK direct proxy dialing
SDK MUST 提供按 proxy ID 打开远端服务流的直接连接 API。返回的连接 SHALL 通过控制通道多路复用，由服务端桥接到 proxy 所属 provider。

#### Scenario: Dial enabled proxy succeeds
- **WHEN** SDK 已连接且调用方 dial 一个可访问的已启用 TCP proxy ID
- **THEN** SDK 返回可读写连接
- **AND** 调用方写入该连接的数据被转发到 proxy 固定 target
- **AND** target 响应被转发回调用方

#### Scenario: Dial inaccessible proxy fails
- **WHEN** SDK 调用方 dial 不存在、未授权或已禁用的 proxy ID
- **THEN** SDK 返回拨号失败错误或在流建立阶段返回可识别失败

#### Scenario: Dial does not choose arbitrary target
- **WHEN** SDK 调用方 dial 某个 proxy ID
- **THEN** SDK MUST NOT 允许调用方覆盖该 proxy 的远端 target host 或 target port

#### Scenario: Dial before connect fails
- **WHEN** 调用方在 SDK 未连接时调用 dial
- **THEN** SDK 返回未连接错误

### Requirement: SDK net and HTTP integration
SDK SHALL 为 Go 调用方提供符合常见标准库集成方式的连接适配能力，包括 `io.ReadWriteCloser`、TCP 风格连接和 HTTP transport。

#### Scenario: Dial returns read write closer
- **WHEN** 调用方调用 SDK 通用 dial API
- **THEN** SDK 返回可用于读写和关闭的连接对象

#### Scenario: DialTCP returns net connection compatible object
- **WHEN** 调用方对 TCP proxy 调用 SDK TCP dial API
- **THEN** SDK 返回可作为 `net.Conn` 使用的连接对象

#### Scenario: HTTP transport uses selected proxy
- **WHEN** 调用方从 SDK 获取绑定某个 proxy ID 的 HTTP transport
- **THEN** 该 transport 发起的 HTTP 请求通过该 proxy 的固定 target 转发

### Requirement: SDK local fixed-target entry
SDK SHALL 提供本地固定目标入口，使调用方可以在本机监听地址上接收连接并把所有连接转发到指定 proxy ID。该入口可以支持 SOCKS5 或 HTTP CONNECT 握手外壳，但 MUST 明确不提供任意目标正向代理语义。

#### Scenario: Start local fixed target entry
- **WHEN** SDK 已连接且调用方启动绑定某个 proxy ID 的本地入口
- **THEN** SDK 在指定本地地址监听连接
- **AND** 每个接受的连接被转发到该 proxy ID 对应的固定 target

#### Scenario: SOCKS5 requested target is not authoritative
- **WHEN** 本地入口收到 SOCKS5 请求且请求中携带目标地址
- **THEN** SDK 可以完成兼容握手
- **AND** 远端连接目标仍由指定 proxy ID 的服务端配置决定

#### Scenario: HTTP CONNECT requested target is not authoritative
- **WHEN** 本地入口收到 HTTP CONNECT 请求且请求中携带目标地址
- **THEN** SDK 可以完成兼容握手
- **AND** 远端连接目标仍由指定 proxy ID 的服务端配置决定

#### Scenario: Local entry stops with context
- **WHEN** 启动本地入口时传入的 context 被取消
- **THEN** SDK 停止接受新连接并释放监听资源

### Requirement: SDK error safety
SDK MUST 返回可由调用方判断的错误类别，同时避免在错误字符串、日志辅助信息或测试快照中泄露 credential、私钥、token 或完整敏感 payload。

#### Scenario: Authentication error is classified
- **WHEN** SDK 连接因 credential 无效被拒绝
- **THEN** SDK 返回可区分的认证错误
- **AND** 错误内容 MUST NOT 包含 credential 明文

#### Scenario: Not connected error is classified
- **WHEN** 调用方在未连接状态调用需要控制通道的 API
- **THEN** SDK 返回可区分的未连接错误

#### Scenario: Proxy access error is classified
- **WHEN** SDK dial 因 proxy 不存在、未授权、禁用或 provider 离线失败
- **THEN** SDK 返回可由调用方处理的代理访问或拨号错误

### Requirement: SDK examples and verification
SDK SHALL 提供最小使用示例和测试，证明 consumer 凭据连接、proxy 列表、直接拨号和本地固定目标入口的核心路径。

#### Scenario: Example demonstrates connect list and dial
- **WHEN** 开发者阅读 SDK 使用示例
- **THEN** 示例展示创建 SDK client、连接、获取 proxy 列表、dial proxy、读写数据和关闭连接

#### Scenario: E2E verifies bridged echo
- **WHEN** 测试环境启动服务端、provider client、TCP proxy 和 echo target，并使用 consumer SDK 凭据连接
- **THEN** SDK 能获取包含该 proxy 的列表
- **AND** SDK dial 该 proxy 后发送的数据能收到 echo 响应

#### Scenario: Local entry verification uses fixed target semantics
- **WHEN** 测试启动 SDK 本地入口并向该本地地址发送请求
- **THEN** 请求被转发到绑定 proxy ID 的固定 target
- **AND** 测试 MUST NOT 声明已支持任意目标正向代理
