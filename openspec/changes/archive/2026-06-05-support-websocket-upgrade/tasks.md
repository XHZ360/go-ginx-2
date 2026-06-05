## 1. 共享 Upgrade 基础能力

- [x] 1.1 新增 HTTP/1.1 WebSocket Upgrade 检测 helper，按大小写不敏感方式匹配 `Upgrade: websocket` 和 `Connection` token `upgrade`
- [x] 1.2 新增 reader-backed `io.ReadWriteCloser`/`net.Conn` 包装，确保 `bufio.Reader` 中已缓冲字节在隧道切换后先被转发
- [x] 1.3 统一现有两个分叉的双向拷贝实现（`internal/control/transport.go` 只 `<-done` 一次、`internal/proxy/https/entry.go` 等两次），抽取单一双向拷贝 helper 供 HTTP、HTTPS 终止、客户端出口复用，并满足：
  - 必须等待两个方向都终止后再返回，确保隧道关闭顺序确定、且双向字节统计在返回前已汇总完整（供 3.5/5.13 使用）
  - 关闭语义按 design 决策 7：采用 full-close（任一方向 EOF/错误后关闭两端），本次不实现 TCP half-close；据此 close frame 往返仅在双方 TCP 连接仍打开时保证（见 5.6/5.7 建模约束）
- [x] 1.4 新增握手请求规范化 helper，确保 Upgrade 分支显式转发或规范化 `Upgrade`/`Connection`，并强制目标侧请求为 HTTP/1.1

## 2. 客户端 HTTP Stream 出口

- [x] 2.1 在 `handleHTTPStream` 读取请求并完成目标 URL、`Host`、`Origin` 改写后识别 WebSocket Upgrade 请求
- [x] 2.2 为 WebSocket Upgrade 分支使用带超时的 `DialContext`/`DialTimeout` 连接 `targetHost:targetPort`，写入改写后的 HTTP/1.1 握手请求，并读取本地目标响应
- [x] 2.3 当本地目标返回非 `101` 响应时，把响应作为普通 HTTP 响应写回控制通道并结束请求
- [x] 2.4 当本地目标返回 `101` 响应时，把响应写回控制通道，清除本地目标连接 deadline，并使用保留 buffered bytes 的包装执行 `stream <-> target` 双向隧道
- [x] 2.5 保持非 WebSocket 请求继续使用现有 `http.DefaultTransport.RoundTrip` 路径

## 3. 服务端 HTTP 入口

- [x] 3.1 在 HTTP 入口写入代理请求后，使用可保留 buffered bytes 的 response reader 读取客户端出口响应
- [x] 3.2 对非 WebSocket 请求或 WebSocket 非 `101` 响应保持现有普通 HTTP 响应写回行为
- [x] 3.3 对 WebSocket `101` 响应在 Hijack 前落定 `statusCode=101` 和 `failed=false`，然后使用 `http.ResponseController(w).Hijack()` 接管公网连接
- [x] 3.4 完整写回目标 `101` 响应状态和响应头，包括 `Sec-WebSocket-Accept`、`Sec-WebSocket-Protocol`、`Sec-WebSocket-Extensions`，然后进入公网连接与控制通道 stream 的双向隧道
- [x] 3.5 隧道切换时转发 hijack reader 和 response reader 中的已缓冲字节，并通过双向拷贝 helper 累加上传/下载字节统计

## 4. 服务端 HTTPS TLS 终止入口

- [x] 4.1 保留 HTTPS 终止读取请求时使用的 request reader，以便隧道切换后转发公网客户端已缓冲字节
- [x] 4.2 对 HTTPS 终止后的 WebSocket 非 `101` 响应保持普通 HTTPS 响应写回
- [x] 4.3 对 HTTPS 终止后的 WebSocket `101` 响应完整写回状态和响应头，包括 `Sec-WebSocket-Accept`、`Sec-WebSocket-Protocol`、`Sec-WebSocket-Extensions`
- [x] 4.4 清除 `tls.Conn` 握手/读写阶段 deadline，然后使用保留 buffered bytes 的包装执行 `tls.Conn <-> stream` 双向隧道
- [x] 4.5 确认 HTTPS SNI 透传路径保持 `Kind: "tcp"` 原始双向拷贝，不增加 HTTP 层解析或头改写

## 5. 测试与验证

- [x] 5.1 增加 Upgrade 检测 helper 表驱动测试：`Connection: keep-alive, Upgrade` 命中；缺少 `upgrade` token 不命中；`Upgrade: h2c` 不命中；大小写变体命中
- [x] 5.2 增加客户端 HTTP stream 单元测试：WebSocket 握手会拨号本地目标、改写 `Host`/可解析 `Origin`、使用 HTTP/1.1、显式转发或规范化 `Upgrade`/`Connection`，并透传 WebSocket 协商头
- [x] 5.3 增加客户端 HTTP stream 单元测试：目标返回非 `101` 时按普通 HTTP 响应返回且不进入隧道
- [x] 5.4 增加 HTTP 反向代理端到端测试：外部 WebSocket Upgrade 返回 `101` 后可以双向收发字节，并断言 `Sec-WebSocket-Accept`、`Sec-WebSocket-Protocol`、`Sec-WebSocket-Extensions` 原样返回
- [x] 5.5 增加 HTTPS TLS 终止端到端测试：`wss` 请求在 TLS 终止后转发到本地 HTTP WebSocket 目标并完成双向隧道，并断言 `101` 响应协商头原样返回
- [x] 5.6 增加 HTTP 反向代理关闭测试：目标侧主动关闭会清理公网侧；公网侧主动关闭会清理目标侧；正常 WebSocket close frame 在 TCP 未关闭前可往返（往返用例必须模拟"发送 close frame 后保持连接、等待回程"的发起方，不能用立即半关闭，否则取决于 1.3 关闭语义）
- [x] 5.7 增加 HTTPS TLS 终止关闭测试：目标侧主动关闭会清理公网 TLS 侧；公网 TLS 侧主动关闭会清理目标侧；正常 WebSocket close frame 在 TCP/TLS 未关闭前可往返（往返建模约束同 5.6）
- [x] 5.8 增加 HTTPS TLS 终止非 `101` 回退测试：WebSocket 请求收到非 `101` 目标响应时按普通 HTTPS 响应写回且不进入隧道
- [x] 5.9 增加 buffered bytes 矩阵测试：目标握手后立即发帧可抵达公网客户端，公网客户端握手后立即发帧可抵达目标；HTTP 和 HTTPS TLS 终止路径均覆盖
- [x] 5.10 增加控制传输矩阵测试：限定为代表性子集，避免与缓冲方向/关闭方向做全交叉。仅"`101` + 双向收发 + 一个缓冲方向"这一核心用例在 QUIC stream 和 TCP+TLS mux stream 两种控制传输上各跑一次（覆盖 mux 分帧/背压差异）；缓冲字节、关闭方向等与传输无关的正交维度只在单一传输上覆盖
- [x] 5.11 增加并发 WebSocket 隧道测试：多条 WebSocket 经同一客户端同时转发，验证流隔离、无串扰，至少覆盖 TCP+TLS mux 路径
- [x] 5.12 增加拨号超时和 deadline 回归测试：目标不可达时 Upgrade 分支会及时失败，成功进入隧道后本地目标连接和 HTTPS 终止 TLS 连接不会继承握手阶段 deadline
- [x] 5.13 增加 HTTP 统计测试：WebSocket `101` 在 Hijack/隧道前记录为状态码 `101` 且 `failed=false`，隧道字节通过双向拷贝 helper 计入可用统计
- [x] 5.14 增加回归测试：Upgrade-like 但非 WebSocket 的请求走普通 HTTP 路径，普通 HTTP 请求、默认 Host/Origin rewrite、HTTPS SNI 透传行为保持不变
- [x] 5.15 复核 TCP+TLS mux 风险说明：确认本 change 不改变 `maxMuxStreams=256`、inbound channel 32 和单 mux 连接复用的容量/吞吐边界
- [x] 5.16 运行 `go test ./...` 和 `openspec status --change support-websocket-upgrade`，确认实现和规格状态通过

## 6. 可选扩展验证

- [x] 6.1 可选：构造 Hijack 失败路径，确认控制流和目标流会被关闭
- [x] 6.2 可选：增加超过 32MB 的 WebSocket 隧道传输测试，证明 HTTPS `maxHTTPBodyBytes` 只限制握手请求体，不截断升级后的隧道流量
- [x] 6.3 可选：增加空闲后恢复端到端测试，发送数据后空闲超过握手超时时间再发送，确认 deadline 清理生效
