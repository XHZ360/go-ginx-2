# 控制通道与 SDK

## 控制通道

服务端同时支持 QUIC 和 TCP+TLS。客户端先完成服务端证书链和 server name 校验，再使用 client ID 与 credential 认证。TCP+TLS 回退连接上的多个 framed substream 共用一条 TCP 连接，可能发生队头阻塞。

认证成功后：

- provider 接收自身用户范围内的代理快照，发送心跳，并等待服务端打开代理子流。
- consumer 不替换同一 client 用户的 provider 会话，接收已启用代理列表，并可以主动打开指定代理流。
- 服务端始终选择同一 client 的最新有效会话；provider 不在线、代理禁用、未知代理或用户不匹配时拒绝桥接。

固定系统客户端 `server-local` 是 server 内注册的常驻 virtual provider session，不执行 join、认证、心跳或远程 enrollment。它复用 session/stream 接口和既有 `OpenStream` 内存帧，但不建立 QUIC、TCP+TLS 或回环网络连接，因此 control wire protocol 没有新增消息或兼容分支。常驻 session 不参与心跳过期，server shutdown 时显式注销。

客户端 join 通过专用 `client_enrollment_listen` 的 `/api/client/enroll` 兑换一次性 token。兑换结果写入客户端受管状态和 CA 文件，后续可无 `-config` 启动。token 过期、已消费、撤销或篡改后不可重用，日志不得记录 credential 或完整 token。

## Consumer SDK

`github.com/simp-frp/go-ginx-2/sdk` 提供：

- `New(Config)`、`Connect(ctx)`、`Close()` 生命周期。
- `Proxies(ctx)` 获取当前用户可访问的已启用代理。
- `Dial(ctx, proxyID)`、`DialTCP(ctx, proxyID)` 打开固定代理 target。
- `HTTPTransport(proxyID)` 适配标准库 HTTP 客户端。
- `StartLocalProxy(ctx, addr, proxyID)` 提供直接 TCP、SOCKS5 或 HTTP CONNECT 外壳。

SDK 的 proxy ID 是唯一远端选择；本地 SOCKS5/CONNECT 请求中的目标地址只用于兼容握手，不会覆盖服务端配置的 target。因此 SDK 不提供任意目标正向代理。SDK 配置必须提供控制地址、server name、CA、client ID 和 credential，并仅尝试 `AllowedProtocols` 中的协议。

Consumer/SDK 的用户授权检查会拒绝保留系统用户所属的代理；本机代理只能由管理员配置的外部反向代理入口使用。

示例位于 `sdk/example/main.go`；验证使用 `go test ./sdk -count=1` 和 `go test ./e2e -run TestSDK -count=1`。

## 安全与失败边界

控制通道不得使用跳过 TLS 校验的回退。SDK 错误可区分配置、未连接、认证和代理访问失败，但不得包含 credential、私钥或 token。provider open 有界超时；更完整的全局、用户和连接级限流尚未实现。
