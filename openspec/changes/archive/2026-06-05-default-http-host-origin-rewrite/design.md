## Context

当前 HTTP 入口在服务端按请求 `Host` 匹配代理记录，然后把原始 HTTP 请求写入控制通道。HTTPS 入口有两条路径：没有可用代理证书时按 SNI 透传加密 TCP 字节；有可用证书时在服务端终止 TLS，读取解密后的 HTTP 请求，再通过 `kind: "http"` 控制通道转给客户端。

客户端 `handleHTTPStream` 读取控制通道内的 HTTP 请求后，会把 `RequestURI` 清空，并把 `URL.Scheme` / `URL.Host` 指向配置的 `targetHost:targetPort`，再调用 `http.DefaultTransport.RoundTrip`。但是 Go 的请求转发会优先使用 `Request.Host` 作为 HTTP `Host` 头，因此本地目标仍可能看到公网入口域名；请求已有 `Origin` 时也会继续携带公网 origin。

## Goals / Non-Goals

**Goals:**

- HTTP 反向代理默认让本地 HTTP 目标看到配置目标地址对应的 `Host`。
- HTTPS TLS 终止后的 HTTP 转发默认复用同一 `Host` / `Origin` 改写行为。
- 仅在请求原本包含可解析 `Origin` 时改写 origin，不新增缺失的 `Origin`。
- 保持 HTTP 入口按外部 `Host` 路由、HTTPS 按 SNI 选路和选证书的现有行为。
- 明确 HTTPS SNI 透传不适用请求头改写。

**Non-Goals:**

- 不新增数据库字段、GraphQL 字段、CLI 参数或 UI 开关。
- 不实现 per-proxy 保留外部 `Host` / `Origin` 的兼容模式。
- 不为 HTTPS SNI 透传解密或修改 TLS 内部 HTTP 请求。
- 不改变 `X-Forwarded-*`、`Forwarded` 或其他代理头策略。

## Decisions

### 1. 在客户端 HTTP stream 出口改写

改写发生在客户端 `handleHTTPStream` 读取 inbound request 后、调用 `RoundTrip` 前。该位置同时覆盖 HTTP 代理和 HTTPS TLS 终止后的 HTTP 转发，因为两者都会写入 `OpenStream{Kind: "http"}`。

备选方案是在服务端 HTTP/HTTPS 入口写入控制通道前改写。这个方案会把入口路由域名和目标请求语义混在一起，也容易影响后续审计、日志和 route miss 诊断。因此选择在客户端出口集中处理。

### 2. 使用目标 authority 作为 Host

客户端出口以 `net.JoinHostPort(targetHost, targetPort)` 构造 target authority，并同时用于 `inbound.URL.Host` 和 `inbound.Host`。这样 `RoundTrip` 发往本地目标时，URL 路由和 HTTP `Host` 头一致。

备选方案是只改 `URL.Host`。这正是当前行为的缺口：`Request.Host` 存在时会覆盖写出的 `Host` 头，导致目标仍看到公网域名。

### 3. Origin 只做保守改写

当请求包含 `Origin` 且它是可解析的绝对 HTTP/HTTPS origin 时，将其改成目标 origin：`http://targetHost:targetPort`。缺失的 `Origin` 不新增；`Origin: null`、空值、非 HTTP scheme 或不可解析值保持原样。

备选方案是总是覆盖或新增 `Origin`。这会破坏无 origin 请求、浏览器特殊 origin 和非浏览器客户端的语义；默认代理行为应该解决常见同源误判，而不是制造新的跨源信号。

### 4. HTTPS 透传保持字节级语义

HTTPS SNI 透传路径继续使用 `OpenStream{Kind: "tcp"}`，服务端只读取 ClientHello SNI 用于路由，之后把 TLS 字节原样搬到客户端本地目标。该路径不能声明请求头改写，因为代理没有解密 HTTP 层。

备选方案是在透传模式下尝试终止 TLS 或中间人式改写。那会改变安全边界、证书要求和现有透传合同，超出本变更范围。

### 5. 暂不提供配置开关

本变更把改写作为 HTTP 类转发的默认运行时行为，不扩展管理模型。这样不会触发 SQLite 迁移、GraphQL schema/codegen 和 UI 表单改动。

备选方案是新增 `changeHost` / `changeOrigin` per-proxy 开关并默认 true。当前需求只要求默认启用，且现有系统还没有此类字段；先落运行时默认值更小、更容易验证。若后续用户需要保留外部 host，可基于新的行为合同追加可配置策略。

## Risks / Trade-offs

- **[Risk] 某些目标服务依赖公网 Host** -> Mitigation: 本变更先明确默认行为；如出现兼容诉求，再增加 per-proxy 保留外部 Host 的配置开关。
- **[Risk] Origin 改写过度破坏浏览器特殊语义** -> Mitigation: 只改写可解析的 HTTP/HTTPS Origin，保留 `null`、缺失和异常值。
- **[Risk] HTTPS 透传用户误以为也会改写请求头** -> Mitigation: 规格和文档明确透传是加密 TCP 字节流，不适用 HTTP header rewrite。
- **[Risk] IPv6 target authority 格式错误** -> Mitigation: 继续使用 `net.JoinHostPort` 生成 Host/URL authority，并增加测试覆盖。

## Migration Plan

该变更不需要数据迁移或配置迁移。部署新版本后，HTTP 代理和 HTTPS TLS 终止代理的新请求立即使用默认改写行为。

回滚到旧版本会恢复旧行为：本地目标可能继续看到公网 `Host` / `Origin`。如果后续已依赖新默认行为，回滚前需要确认目标服务可以接受公网入口域名。

## Open Questions

- 后续是否需要提供 per-proxy 开关，让个别 HTTP/HTTPS TLS 终止代理保留外部 `Host` 或 `Origin`。
- Origin 改写是否应保留原 scheme 中的 `https`，还是始终表达客户端出口实际访问本地目标的 `http`。本设计选择后者，因为当前客户端 HTTP stream 固定以 `http` 访问 target。
