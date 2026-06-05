## ADDED Requirements

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
