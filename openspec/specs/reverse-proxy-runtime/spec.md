## Purpose

定义反向代理运行时契约，覆盖 TCP、UDP、HTTP、HTTPS SNI 透传，以及使用静态文件或托管证书进行 HTTPS TLS 终止后，经已认证控制通道路由到客户端目标的转发；同时显式跟踪正向代理、访问控制、配额、更丰富错误响应和生产级可观测性的缺口。
## Requirements
### Requirement: TCP reverse proxy baseline
系统 MUST 支持从公网 TCP 入口到客户端本地目标的 TCP 反向代理转发，转发路径使用已认证控制通道流。

#### Scenario: TCP traffic reaches local target
- **WHEN** 已启用的 TCP 代理配置给在线且已认证的客户端
- **THEN** 发往代理入口的外部 TCP 流量通过客户端转发到配置的本地目标

#### Scenario: TCP traffic statistics are recorded
- **WHEN** TCP 流量经过代理运行时
- **THEN** 运行时记录该代理的基础 TCP 连接和字节计数

### Requirement: UDP reverse proxy baseline
系统 MUST 支持从公网 UDP 入口到客户端本地 UDP 目标的 UDP 反向代理转发，转发路径使用已认证控制通道流。

#### Scenario: UDP packet reaches local target
- **WHEN** 已启用的 UDP 代理配置给在线且已认证的客户端
- **THEN** 发往代理入口的外部 UDP 包通过客户端转发到配置的本地 UDP 目标

#### Scenario: UDP per-source session is maintained
- **WHEN** UDP 流量来自某个外部源地址
- **THEN** 运行时维护该源地址对应的转发会话，用于把响应发回原始源地址，直到空闲清理

#### Scenario: UDP traffic statistics are recorded
- **WHEN** UDP 流量经过代理运行时
- **THEN** 运行时记录该代理的基础 UDP 包和字节计数

### Requirement: HTTP reverse proxy baseline
系统 MUST 支持按请求 `Host` 匹配已启用 HTTP 代理，并把请求转发到客户端本地 HTTP 目标。

#### Scenario: HTTP request reaches local target
- **WHEN** 请求 `Host` 存在已启用 HTTP 代理，且其客户端在线
- **THEN** 运行时通过客户端把 HTTP 请求转发到配置的本地 HTTP 目标

#### Scenario: HTTP response returns to caller
- **WHEN** 本地 HTTP 目标返回响应
- **THEN** 运行时把响应状态、响应头和响应体返回给外部调用方

#### Scenario: HTTP traffic statistics are recorded
- **WHEN** HTTP 流量经过代理运行时
- **THEN** 运行时记录该代理的基础 HTTP 请求、状态码、字节和错误计数

### Requirement: Daemon runtime proxy startup
服务端守护进程 MUST 从已启用代理记录启动反向代理入口，客户端守护进程 MUST 认证、接收代理配置，并把反向代理流服务到本地目标。

#### Scenario: Server starts configured entries
- **WHEN** 服务端守护进程启动时 SQLite 中存在已启用的 TCP、UDP、HTTP 或 HTTPS 代理记录
- **THEN** 服务端启动对应的反向代理入口监听器

#### Scenario: Client serves proxy streams
- **WHEN** 客户端守护进程认证并接收代理配置
- **THEN** 客户端把 TCP、UDP、HTTP 和 HTTPS 代理流服务到配置的本地目标

### Requirement: HTTPS reverse proxy passthrough baseline
系统 MUST 支持 HTTPS 反向代理 SNI 透传：按 TLS ClientHello SNI 主机路由到已启用 HTTPS 代理，并在公网服务端不终止 TLS 的情况下，把加密 TCP 流转发到客户端本地 HTTPS 目标。

#### Scenario: HTTPS passthrough reaches local target
- **WHEN** TLS ClientHello SNI 主机存在已启用 HTTPS 代理，且其客户端在线
- **THEN** 运行时通过客户端把加密 TLS 流转发到配置的本地 HTTPS 目标

#### Scenario: Public server does not terminate passthrough TLS
- **WHEN** HTTPS 透传流量经过代理运行时
- **THEN** 公网服务端仅使用 SNI 进行路由，且 MUST NOT 要求为该透传连接选择代理证书或访问私钥

### Requirement: HTTPS TLS termination baseline
系统 MUST 支持对带有文件型证书/私钥路径或可热加载托管证书的已启用 HTTPS 代理执行 TLS 终止。服务端 MUST 按 SNI 选择代理和证书，完成公网 TLS 握手，并把解密后的 HTTP 请求转发到客户端本地 HTTP 目标。

#### Scenario: HTTPS termination reaches local HTTP target
- **WHEN** TLS ClientHello SNI 主机存在已启用 HTTPS 代理，代理有生效的静态或托管证书和私钥，且其客户端在线
- **THEN** 运行时终止公网 TLS，并通过客户端把解密后的 HTTP 请求转发到配置的本地 HTTP 目标

#### Scenario: HTTPS certificate selected by SNI
- **WHEN** HTTPS 终止流量到达已配置的 HTTPS 代理主机
- **THEN** 服务端使用该代理主机对应的生效静态或托管证书及私钥

#### Scenario: Managed certificate hot reload applies to new handshakes
- **WHEN** HTTPS 代理主机的托管证书替换件被激活
- **THEN** 该 SNI 主机的新 TLS 握手无需重启 HTTPS 监听器即可使用替换证书

#### Scenario: HTTPS passthrough remains available
- **WHEN** 已启用 HTTPS 代理没有配置生效的静态或托管证书和私钥
- **THEN** 运行时保留 SNI 透传行为，直接转发加密 TLS 字节，而不要求访问代理私钥

### Requirement: HTTPS certificate lifecycle gap tracking
反向代理运行时规格 MUST 跟踪当前基线未实现的更丰富 HTTPS 证书生命周期和策略行为。

#### Scenario: Advanced HTTPS behavior planned but not implemented
- **WHEN** 产品或设计文档提到访问密码页面、临时分享流程、HTTP 状态检查、丰富 HTTPS 错误响应、外部告警或管理 UI 生命周期行为
- **THEN** 在存在实现证据前，本规格 MUST 把该行为标识为未来缺口

### Requirement: Runtime policy gap tracking
反向代理运行时规格 MUST 把访问密码、临时分享链接、配额、限速、更丰富错误响应和生产级可观测性，作为当前基线未实现的需求/设计行为跟踪。

#### Scenario: Policy behavior planned but not implemented
- **WHEN** 产品或设计文档提到访问控制、配额、限速、分享链接、丰富错误响应或生产可观测性行为
- **THEN** 在存在实现证据前，本规格 MUST 把该行为标识为未来缺口

#### Scenario: Future policy implementation
- **WHEN** 未来实现某项策略或可观测性行为
- **THEN** 在声明该行为已实现前，MUST 用有实现证据的场景更新本规格

### Requirement: Forward proxy exclusion
反向代理运行时基线 MUST NOT 声明正向代理行为已经实现。

#### Scenario: Forward proxy remains separate
- **WHEN** 产品或设计文档提到正向代理行为
- **THEN** 该行为 MUST 作为未来或独立能力跟踪，且 MUST NOT 包含在反向代理 MVP 基线中

### Requirement: Per-proxy entry listener configuration
系统 MUST 为 TCP、UDP、HTTP 和 HTTPS 反向代理支持按代理配置有效入口监听地址与端口，并为 HTTP/HTTPS 区分监听地址和路由域名。

#### Scenario: TCP proxy binds configured host and port
- **WHEN** 已启用 TCP 代理配置了入口监听地址和入口端口
- **THEN** 服务端在该监听地址和端口上启动 TCP 代理 listener，并把流量转发到该代理的客户端本地目标

#### Scenario: UDP proxy binds configured host and port
- **WHEN** 已启用 UDP 代理配置了入口监听地址和入口端口
- **THEN** 服务端在该监听地址和端口上启动 UDP 代理 listener，并把 UDP 流量转发到该代理的客户端本地目标

#### Scenario: HTTP proxy routes by domain on configured listener
- **WHEN** 已启用 HTTP 代理配置了入口监听地址、入口端口和路由域名
- **THEN** 服务端在该监听地址和端口上的 HTTP listener 按请求 `Host` 匹配该域名，并把请求转发到该代理的客户端本地 HTTP 目标

#### Scenario: HTTPS proxy routes by SNI on configured listener
- **WHEN** 已启用 HTTPS 代理配置了入口监听地址、入口端口和 SNI 域名
- **THEN** 服务端在该监听地址和端口上的 HTTPS listener 按 TLS ClientHello SNI 匹配该域名，并执行透传或 TLS 终止转发

#### Scenario: Existing proxy records use default listeners
- **WHEN** 已启用代理记录缺少新的监听地址或 HTTP/HTTPS 入口端口字段
- **THEN** 服务端使用当前 server 配置中的默认监听地址和端口解释该代理，而不是要求操作者迁移后才能继续服务

### Requirement: Runtime listener reconciliation
服务端运行时 MUST 在代理入口有效监听配置变化后及时协调实际运行的 listener 服务，使运行状态与已启用代理集合保持一致。

#### Scenario: Create or enable starts required listener
- **WHEN** 管理员创建或启用代理，且该代理的有效入口监听地址尚未运行对应协议 listener
- **THEN** 服务端在管理操作完成前启动所需 listener，使该代理无需重启服务端即可接收外部流量

#### Scenario: Update moves listener
- **WHEN** 管理员更新已启用代理的有效入口监听地址、端口或 HTTP/HTTPS 路由域名
- **THEN** 服务端及时让新有效入口可用，并在旧入口不再被任何已启用代理使用时关闭旧 listener

#### Scenario: Disable or delete closes unused listener
- **WHEN** 管理员禁用或删除代理，且该代理原有效入口 listener 不再被任何已启用代理使用
- **THEN** 服务端及时关闭该 listener，而不是让不再需要的服务继续监听

#### Scenario: Shared HTTP listener remains active
- **WHEN** 多个 HTTP 代理共享同一监听地址和端口，且其中一个代理被禁用或删除
- **THEN** 服务端保持该 HTTP listener 运行，以继续服务同监听地址上的其他已启用 HTTP 代理

#### Scenario: Reconcile failure is visible
- **WHEN** 代理管理操作要求启动新的 listener 但实际绑定失败
- **THEN** 管理操作返回可消费的失败语义，并且系统不得静默声明代理入口已经可用

### Requirement: Default HTTP target header rewrite
系统 MUST 在 HTTP 类反向代理转发到客户端本地 HTTP 目标前，默认把目标可见的请求 `Host` 改写为配置的目标地址，并在安全可解析时改写已有 `Origin`。该行为 MUST 适用于 HTTP 反向代理和 HTTPS TLS 终止后的 HTTP 转发；HTTPS SNI 透传作为加密 TCP 字节流 MUST NOT 声明或尝试改写 HTTP 请求头。

#### Scenario: HTTP proxy rewrites Host to target authority
- **WHEN** HTTP 代理按公网请求 `Host` 匹配已启用代理，并把请求转发到配置的本地 HTTP 目标
- **THEN** 本地目标接收到的 HTTP `Host` MUST 是该代理的 `targetHost:targetPort`

#### Scenario: HTTP proxy rewrites parseable Origin
- **WHEN** HTTP 代理请求包含可解析的 HTTP 或 HTTPS `Origin`
- **THEN** 本地目标接收到的 `Origin` MUST 使用目标 HTTP origin `http://targetHost:targetPort`

#### Scenario: HTTP proxy preserves missing or special Origin
- **WHEN** HTTP 代理请求没有 `Origin`，或 `Origin` 为 `null`、空值、非 HTTP scheme、不可解析值
- **THEN** 运行时 MUST NOT 新增 `Origin`，且 MUST 保留特殊或不可解析的原始 `Origin` 值

#### Scenario: HTTPS termination uses HTTP header rewrite
- **WHEN** HTTPS 代理选择证书并终止公网 TLS，然后把解密后的 HTTP 请求转发到客户端本地 HTTP 目标
- **THEN** 本地目标接收到的 `Host` 和可解析 `Origin` MUST 按默认 HTTP target header rewrite 规则改写

#### Scenario: HTTPS passthrough does not rewrite HTTP headers
- **WHEN** HTTPS 代理没有生效证书并按 SNI 透传加密 TLS 字节到客户端本地 HTTPS 目标
- **THEN** 运行时 MUST 保持透传字节流，不得声明或尝试修改 TLS 内部的 HTTP `Host` 或 `Origin`

### Requirement: HTTP WebSocket Upgrade tunneling
系统 MUST 支持 HTTP/1.1 WebSocket Upgrade 请求经 HTTP 反向代理和 HTTPS TLS 终止后的 HTTP 转发到达客户端本地 HTTP 目标，并在本地目标返回 `101 Switching Protocols` 后进入双向字节隧道。运行时 MUST 使用大小写不敏感的 `Upgrade: websocket` 和 `Connection` token `upgrade` 识别 WebSocket Upgrade 请求，并 MUST 使用有界目标连接超时建立本地目标连接。

#### Scenario: HTTP proxy forwards WebSocket handshake
- **WHEN** HTTP 代理按公网请求 `Host` 匹配已启用代理，请求包含 WebSocket Upgrade 头，且其客户端在线
- **THEN** 运行时 MUST 通过客户端把 WebSocket 握手请求转发到配置的本地 HTTP 目标

#### Scenario: WebSocket handshake uses default target header rewrite
- **WHEN** WebSocket 握手请求经 HTTP 代理或 HTTPS TLS 终止后的 HTTP 转发到客户端本地 HTTP 目标
- **THEN** 本地目标接收到的握手请求 `Host` 和可解析 `Origin` MUST 按默认 HTTP target header rewrite 规则改写，其他 WebSocket 协商头 MUST 保持透传

#### Scenario: WebSocket handshake forwards upgrade headers over HTTP/1.1
- **WHEN** WebSocket 握手请求被转发到客户端本地 HTTP 目标
- **THEN** 运行时 MUST 使用 HTTP/1.1 发送握手请求，并 MUST 显式转发或规范化 `Upgrade: websocket` 和包含 `upgrade` token 的 `Connection` 语义

#### Scenario: Successful WebSocket upgrade enters tunnel
- **WHEN** 本地 HTTP 目标对转发的 WebSocket 握手返回 `101 Switching Protocols`
- **THEN** 运行时 MUST 把 `101` 响应返回给外部调用方，并在外部连接与本地目标连接之间建立双向字节隧道

#### Scenario: Successful WebSocket upgrade preserves response negotiation headers
- **WHEN** 本地 HTTP 目标对转发的 WebSocket 握手返回 `101 Switching Protocols`，且响应包含 `Sec-WebSocket-Accept`、`Sec-WebSocket-Protocol` 或 `Sec-WebSocket-Extensions`
- **THEN** 运行时 MUST 把这些 `101` 响应头原样返回给外部调用方

#### Scenario: Successful WebSocket upgrade records success before tunneling
- **WHEN** 本地 HTTP 目标对转发的 WebSocket 握手返回 `101 Switching Protocols`
- **THEN** 运行时 MUST 在接管公网连接或进入双向隧道前把该 HTTP 请求的统计状态落定为状态码 `101` 且 `failed=false`

#### Scenario: WebSocket tunnel does not inherit handshake deadlines
- **WHEN** WebSocket 握手成功并即将进入双向字节隧道
- **THEN** 运行时 MUST 清除本地目标连接和已终止公网 TLS 连接上用于拨号、握手、读请求或写响应阶段的临时 deadline

#### Scenario: WebSocket frames are not modified
- **WHEN** WebSocket 握手已经成功升级并进入双向隧道
- **THEN** 运行时 MUST 按字节顺序透传 WebSocket 帧数据，且 MUST NOT 解析、改写或重组 WebSocket 帧

#### Scenario: Non-101 upgrade response remains HTTP response
- **WHEN** 本地 HTTP 目标对 WebSocket Upgrade 请求返回非 `101` 响应
- **THEN** 运行时 MUST 按普通 HTTP 响应把状态、响应头和响应体返回给外部调用方，且 MUST NOT 进入 WebSocket 隧道

#### Scenario: Upgrade-like non-WebSocket request remains HTTP
- **WHEN** 请求缺少 `Connection` token `upgrade`，或 `Upgrade` 头不是 `websocket`
- **THEN** 运行时 MUST NOT 进入 WebSocket 隧道，且 MUST 按普通 HTTP 请求/响应路径处理该请求

#### Scenario: Target-side close tears down public side
- **WHEN** WebSocket 隧道已经建立，且本地目标连接主动关闭
- **THEN** 运行时 MUST 关闭对应的外部连接并释放控制通道 stream

#### Scenario: Public-side close tears down target side
- **WHEN** WebSocket 隧道已经建立，且外部连接主动关闭
- **THEN** 运行时 MUST 关闭对应的本地目标连接并释放控制通道 stream

#### Scenario: WebSocket close frame can round trip before TCP close
- **WHEN** WebSocket 隧道已经建立，一端发送 WebSocket close frame 且 TCP 连接仍保持打开
- **THEN** 运行时 MUST 按字节透传 close frame，并允许对端响应的 close frame 在连接拆除前返回

#### Scenario: HTTPS termination supports WebSocket upgrade
- **WHEN** HTTPS 代理选择证书并终止公网 TLS，解密后的 HTTP 请求包含 WebSocket Upgrade 头，且其客户端在线
- **THEN** 运行时 MUST 通过客户端把握手请求转发到配置的本地 HTTP 目标，并在目标返回 `101` 后通过已终止的 TLS 连接建立双向字节隧道

#### Scenario: HTTPS passthrough remains opaque for WebSocket over TLS
- **WHEN** HTTPS 代理没有生效证书并按 SNI 透传加密 TLS 字节，外部流量内部承载 WebSocket over TLS
- **THEN** 运行时 MUST 保持现有加密 TCP 透传行为，且 MUST NOT 检查、声明或改写 TLS 内部的 WebSocket 握手头

#### Scenario: Buffered bytes survive tunnel switch
- **WHEN** WebSocket 握手解析期间任一端在 HTTP 头之后立即发送了 WebSocket 数据字节
- **THEN** 运行时 MUST 在切换到双向隧道后按原始顺序转发这些已缓冲字节和后续字节

