## Context

当前反向代理运行时有三条相关路径：

- HTTP 反向代理：服务端 HTTP handler 把请求写入控制通道 stream，客户端 `handleHTTPStream` 使用 `http.DefaultTransport.RoundTrip` 请求本地 HTTP 目标，再把响应写回服务端。
- HTTPS TLS 终止：服务端完成公网 TLS 握手后读取一个 HTTP 请求，再复用 `Kind: "http"` 控制通道路径转发到客户端本地 HTTP 目标。
- HTTPS SNI 透传：服务端只读取 TLS ClientHello SNI 路由，然后使用 `Kind: "tcp"` 执行双向字节拷贝。

WebSocket over HTTP/1.1 先是普通 HTTP Upgrade 握手，只有当本地目标返回 `101 Switching Protocols` 后，代理才应停止普通 HTTP 响应体模型，切换为连接级双向隧道。当前 HTTP 和 HTTPS 终止路径没有这个切换点，因此 HMR、实时通知等 WebSocket 场景会失败；HTTPS SNI 透传路径已经是加密 TCP 隧道，不需要理解 WebSocket。

## Goals / Non-Goals

**Goals:**

- 支持 HTTP/1.1 WebSocket Upgrade 请求经 HTTP 反向代理到达客户端本地 HTTP 目标。
- 支持 HTTPS TLS 终止后的 HTTP/1.1 WebSocket Upgrade 请求到达客户端本地 HTTP 目标。
- 在本地目标返回 `101 Switching Protocols` 后，将公网侧连接、控制通道 stream 和本地目标 TCP 连接切换为双向字节隧道。
- 复用现有 `Kind: "http"` 控制通道语义，避免新增控制协议类型。
- 在 WebSocket 握手请求上继续应用默认 `Host` 和可解析 `Origin` 改写；握手成功后的 WebSocket 帧保持字节透传。
- 原样返回本地目标 `101` 响应中的 WebSocket 协商头，避免浏览器握手因缺失 `Sec-WebSocket-Accept`、子协议或扩展协商结果而失败。
- 保持普通 HTTP 请求/响应路径和 HTTPS SNI 透传路径行为不变。

**Non-Goals:**

- 不支持 HTTP/2 Extended CONNECT WebSocket。
- 不实现 WebSocket 协议解析、帧级统计、压缩协商改写或子协议选择逻辑。
- 不新增管理 UI、配置项、数据库字段或外部 API。
- 不改变 HTTPS SNI 透传的加密字节流模型。

## Decisions

### 1. 用请求头检测 HTTP/1.1 WebSocket Upgrade

服务端和客户端出口都使用同一类判断逻辑：请求必须包含 `Upgrade: websocket`，并且 `Connection` token 中包含 `upgrade`。判断 `Connection` 时按逗号分隔 token 并大小写不敏感匹配，以兼容 `keep-alive, Upgrade` 这类常见写法。

替代方案是只看 `Upgrade` 头或只按路径匹配。这样会误判普通请求，也无法覆盖不同框架的 WebSocket 路径，因此不采用。

NGINX 的反向代理默认会移除或重设 hop-by-hop 头，WebSocket 配置需要显式把 `Upgrade` 和 `Connection` 传给 upstream。本实现也把这两个头作为 WebSocket 分支的有意转发对象：普通 HTTP 请求不因为该功能改变 hop-by-hop 头处理；被识别为 WebSocket Upgrade 的请求必须把 `Upgrade: websocket` 和包含 `upgrade` token 的 `Connection` 语义转发到本地目标，必要时可规范化 `Connection` 为 `Upgrade`。

### 2. 继续复用 `Kind: "http"` 控制通道

WebSocket Upgrade 握手仍是 HTTP 请求，因此服务端继续发送 `OpenStream{Kind: "http"}` 和原始 HTTP 请求。客户端 `handleHTTPStream` 在 `http.ReadRequest` 后根据请求头分支：

- 非 WebSocket Upgrade：保留现有 `RoundTrip` 路径。
- WebSocket Upgrade：使用带超时的拨号连接本地目标，写入改写后的 HTTP/1.1 握手请求，读取目标响应；如果是 `101`，写回响应后清除目标连接上的读写 deadline，并进入 `stream <-> target` 双向拷贝。

替代方案是新增 `Kind: "websocket"` 或 `Kind: "http-upgrade"`。这会扩大控制协议和测试面，但并不会减少握手处理复杂度，因此不采用。

### 3. 客户端 Upgrade 出口直接拨号本地目标

普通 `http.Transport` 的主要价值是连接池、代理、重试和响应体模型；WebSocket Upgrade 成功后需要保留底层连接进行读写。直接拨号目标 TCP 连接再 `Request.Write` 握手请求，能够明确控制 `101` 后的连接所有权，也避免依赖 `Transport` 对 `101` response body 的内部实现。

直接拨号必须使用有界连接超时，例如 `net.Dialer{Timeout: ...}` 或等价 `DialContext`，避免本地目标不可达时长期占用控制通道 stream。连接建立后可在握手阶段设置读写 deadline，但一旦目标返回 `101` 并进入 WebSocket 隧道，必须清除目标连接 deadline，避免把短握手超时遗留到长连接。

握手请求在写入目标前必须沿用现有改写：

- `RequestURI = ""`
- `URL.Scheme = "http"`
- `URL.Host = targetHost:targetPort`
- `Host = targetHost:targetPort`
- 可解析 HTTP/HTTPS `Origin` 改写为 `http://targetHost:targetPort`
- `ProtoMajor = 1`、`ProtoMinor = 1`、`Proto = "HTTP/1.1"`，确保发往本地目标的是 HTTP/1.1 Upgrade 握手

`Upgrade`、`Connection` 作为本功能需要的 hop-by-hop 头显式转发或规范化；其他 WebSocket 头，例如 `Sec-WebSocket-Key`、`Sec-WebSocket-Version`、`Sec-WebSocket-Protocol`、`Sec-WebSocket-Extensions`，保持透传。

### 4. 服务端 HTTP 入口在 `101` 后 Hijack 外部连接

HTTP handler 收到目标响应后：

- 如果原请求不是 WebSocket Upgrade，保持现有响应写回。
- 如果原请求是 WebSocket Upgrade 但目标没有返回 `101`，把目标响应按普通 HTTP 响应写回调用方。
- 如果原请求是 WebSocket Upgrade 且目标返回 `101`，使用 `http.ResponseController(w).Hijack()` 接管外部连接，把目标 `101` 响应写入 hijacked connection，然后进入外部连接和控制通道 stream 的双向拷贝。

`101` 响应必须保留目标返回的 WebSocket 协商头，尤其是 `Sec-WebSocket-Accept`、`Sec-WebSocket-Protocol` 和 `Sec-WebSocket-Extensions`。这些响应头是浏览器确认握手、子协议和扩展协商结果的依据，不能只写状态码后直接进入隧道。

替代方案是先用 `ResponseWriter.WriteHeader(101)` 再尝试拷贝 body。这仍停留在 HTTP response writer 模型中，无法可靠获得连接级读写权，因此不采用。

### 5. HTTPS TLS 终止直接在 TLS conn 上切换隧道

HTTPS 终止路径已经持有 `tls.Conn`，不需要 `Hijack`。当目标返回 `101` 时，将响应写回 `tls.Conn`，然后清除 `tls.Conn` 上握手、读请求或写响应阶段留下的 deadline，再执行 `tls.Conn <-> stream` 双向拷贝。非 `101` 响应继续使用现有超时写回逻辑。

HTTPS SNI 透传路径继续使用 `Kind: "tcp"` 和现有双向拷贝。该路径对 `wss` 是透明的，不参与 Host/Origin 改写，也不声明 WebSocket 握手可见性。

### 6. 隧道切换时保留 `bufio.Reader` 已缓冲字节

`http.ReadRequest` 和 `http.ReadResponse` 都可能在解析握手时从底层连接多读到后续 WebSocket 帧。进入隧道前必须把这些 buffered bytes 纳入拷贝链路：

- HTTP handler Hijack 返回的 `bufio.ReadWriter` 可能包含公网客户端已发送的帧，外部连接读方向应先消费该 reader。
- HTTPS 终止读取请求时应保留请求 reader，进入隧道时先转发其中已缓冲的公网客户端字节。
- 服务端从控制通道读取 `101` 响应时应保留响应 reader，进入隧道时先转发其中已缓冲的客户端目标字节。
- 客户端出口从本地目标读取 `101` 响应时应保留目标 reader，进入隧道时先转发其中已缓冲的目标字节。

实现上可以抽取一个包装 `io.ReadWriteCloser` 的 reader-backed stream/conn helper：当关联的 `bufio.Reader.Buffered() > 0` 时先从 reader 读取，否则回落到底层连接。

### 7. 统计和生命周期

WebSocket 握手成功后必须记录状态码 `101`，并且在 HTTP handler `Hijack` 或 HTTPS 终止进入隧道前把请求结果落定为 `failed=false`。这要发生在隧道拷贝开始前，避免 deferred stats 在长连接关闭、异常关闭或 hijack 分支中沿用初始化的 `502`/失败状态。

隧道期间的上传/下载字节不能继续依赖 `r.Body` 的 counting wrapper，因为 hijack 后 WebSocket 帧不再通过 HTTP request body。若要计入字节统计，双向拷贝 helper 必须在公网侧到目标侧和目标侧到公网侧两个方向分别累加字节；即使暂时不完整统计隧道字节，也 MUST 记录 `101` 且 `failed=false`。

连接关闭遵循现有双向拷贝模型：任一方向 EOF 或拷贝错误后关闭两端，等待另一方向退出，释放控制通道 stream 和目标连接。本次不实现 TCP half-close 语义。正常 WebSocket close frame 往返应在 TCP 连接仍打开时完成；如果某一端发送 close frame 后立即关闭 TCP，当前 full-close 模型可能截断对端回程 close frame，这是本次接受的取舍。

测试必须分别覆盖公网侧主动关闭和目标侧主动关闭，确保另一端连接被清理；同时覆盖正常 WebSocket close frame 往返，避免隧道代码在双方仍保持 TCP 连接时破坏协议级关闭握手。

#### 关闭语义：为何选 full-close，何时引入 half-close

本次明确选择 **full-close**（任一方向 EOF/错误即关闭两端），不实现 TCP half-close。决策依据与后续演进路径记录如下，避免将来重复讨论。

**half-close 是什么**：A→B 方向读到 A 的 EOF 时，只对 B 做 `CloseWrite()`（发 FIN、告知对端"我侧不再写"），保留 B→A 方向继续搬运，待 B 也 EOF 才整体释放——即 `shutdown(SHUT_WR)` 语义。

**half-close 的收益**：

- 唯一真正修复的问题——对端 close frame 不被截断：规范 WebSocket 客户端发完 Close 帧会等回程再关 TCP，full-close 对它们无损；但"发完 Close 就立即 `shutdown(SHUT_WR)`"的非常规对端，在 full-close 下回程 Close 帧会被丢弃。half-close 对这类对端也能完成干净关闭握手。
- 通用正确性：任何"一个方向先结束、另一方向仍在传"的协议（如未来用隧道承载 gRPC streaming、请求体发完而响应仍流式返回）都需要 half-close；full-close 假设双向同生共死，仅对纯 WebSocket 够用。

**half-close 的成本（为何本次不做）**：

- mux 协议扩展是最大成本：当前 `muxStream.Close()` 只发单个 `MuxFrameClose` 且立即拆读写两个方向，mux 协议没有"单向 FIN"概念。要支持 half-close 必须新增 `MuxFrameCloseWrite` 帧、为 `muxStream` 维护 `readClosed`/`writeClosed` 独立状态、仅当两方向都关才回收槽位。QUIC stream、hijack 的 `*net.TCPConn`、`*tls.Conn` 都已原生支持 `CloseWrite()`，唯独自研 mux 需要改协议，回归风险集中于此。
- 半开泄漏撞上 mux 容量上限：一端 FIN、另一端永不关闭的半开连接会无限期占用 goroutine 和稀缺的 mux 槽位（`maxMuxStreams=256`）。引入 half-close 必须同时引入空闲超时/最大存活时间兜底，否则把 256 槽资源池暴露给"对端不老实即泄漏"的风险。
- `tls.Conn.CloseWrite()` 发送 `close_notify`，部分 TLS 栈会将其当作整条连接结束，半关闭行为需按对端实测。
- 状态机与测试面变大，且必须严格区分 clean EOF（传播 FIN）与 error/reset（硬关两端）。

**折中方案（若需要时优先采用）**：不改 mux 协议，在双向拷贝 helper 层给 full-close 加**有界 linger**——第一个方向 EOF 后，对另一方向设短读 deadline（如 2–5s）再关，给回程 Close 帧排空时间。可在不碰 256 槽泄漏雷区的前提下，拿到 half-close 绝大部分的正确性收益；对 WebSocket close 帧（极小、即时）足够。

**重新评估触发条件**：

- 出现非常规客户端（发完 Close 立即半关闭）丢回程 Close 帧的实证 → 先上"有界 linger"。
- 需要用 `Kind:"tcp"`/`http` 隧道承载更一般的双向流（gRPC streaming 等）→ 才值得给 mux 加 `CloseWrite` 帧 + 半开状态机 + 空闲超时，作为独立变更立项。

### 8. 测试矩阵

WebSocket Upgrade 的端到端测试需要按路径和控制传输展开：

- 入口路径：HTTP 反向代理、HTTPS TLS 终止。
- 控制传输：QUIC 原生 stream、TCP+TLS mux stream。
- 数据方向：公网客户端到目标、目标到公网客户端。
- 缓冲时机：目标在 `101` 后立即发送首帧、公网客户端在握手后立即发送首帧。
- 关闭方向：目标侧主动关闭、公网侧主动关闭、正常 WebSocket close frame 往返。

测试还应覆盖多条并发 WebSocket 隧道经同一客户端转发，尤其是 TCP+TLS mux 路径，验证 stream 隔离、无串扰和长连接占用下的基本并发行为。

## Risks / Trade-offs

- WebSocket 长连接占用控制通道 stream 时间更长 → 复用现有 stream-per-connection 模型，并在测试中覆盖目标侧主动关闭、公网侧主动关闭和正常 WebSocket close frame 往返。
- 本次不实现 TCP half-close，任一方向 EOF 会触发两端关闭 → 对常规 WebSocket 关闭流程，依靠协议 close frame 在 TCP 关闭前完成往返；把半关闭优化留给独立变更。
- TCP+TLS 控制通道的 mux 有 `maxMuxStreams = 256` 硬上限，WebSocket 长连接会长期占用 stream，达到上限后新的代理流会失败为 `too many mux streams`；QUIC 路径不共享这个具体限制 → 本次不改变 mux 上限，但在设计和后续文档中明确该容量边界，必要时单独设计连接池、限流或多 mux 连接。
- TCP+TLS mux 每个 stream 的 inbound channel 缓冲为 32，且多条流复用单个 TCP+TLS 连接，没有逐流窗口流控；高吞吐 WebSocket，例如大文件 over WebSocket，会受单连接串行化和背压模型影响 → 本次保证功能正确性，不承诺高吞吐优化。
- `bufio.Reader` 缓冲字节处理遗漏会造成偶发首帧丢失 → 抽取 reader-backed wrapper，并在测试中让目标或客户端紧跟握手发送数据。
- HTTP server Hijack 后无法再使用 `ResponseWriter` 错误处理 → 只有在已经拿到目标 `101` 后才 Hijack；Hijack 失败时关闭控制流和目标流。
- 直接拨号 Upgrade 目标绕过 `http.Transport` 连接池 → WebSocket 本身是长连接，连接池收益有限；普通 HTTP 仍使用 `RoundTrip`。
- 直接拨号如果没有连接超时会让不可达目标长期占用控制通道 stream → 使用有界拨号超时，并在成功进入隧道后清除目标连接 deadline。
- HTTPS 终止如果把握手阶段 deadline 带入隧道，会导致长连接被短超时误杀 → 在 `101` 写回后、双向拷贝前清除 `tls.Conn` deadline。
- HTTPS 终止目前只读取一个 HTTP 请求 → WebSocket 长连接符合单请求升级模型；普通 keep-alive 多请求不在本次范围。
