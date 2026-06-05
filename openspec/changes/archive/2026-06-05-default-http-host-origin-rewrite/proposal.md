## Why

当前 HTTP 反向代理和 HTTPS TLS 终止后的 HTTP 转发会按公网 `Host`/SNI 选择代理，但客户端出口转发到本地目标时仍可能保留外部入口的 `Host` 和 `Origin`。这会让依赖目标主机名、虚拟主机或同源校验的本地 HTTP 服务误判请求来源，导致默认代理体验不稳定。

本变更让 HTTP 类转发默认呈现为发往配置目标的请求：路由仍使用外部入口域名，但本地目标默认看到目标地址对应的 `Host` 和 `Origin`。

## What Changes

- HTTP 反向代理在把请求转发到客户端本地 HTTP 目标时，默认将请求 `Host` 改写为配置的 `targetHost:targetPort`。
- HTTPS TLS 终止后的 HTTP 转发复用同一默认改写行为。
- 当请求包含可解析的 `Origin` 时，默认将其 scheme/host 改写为目标 HTTP origin；不主动新增缺失的 `Origin`。
- 对特殊或不可解析的 `Origin` 值保持原样，避免破坏 `Origin: null` 等浏览器语义。
- HTTPS SNI 透传继续作为加密 TCP 字节流转发，不声明也不尝试改写 HTTP 请求头。
- 暂不引入 per-proxy 配置开关；后续如需要兼容保留外部 `Host` 的目标服务，可单独扩展管理配置。

## Capabilities

### New Capabilities

- 无。

### Modified Capabilities

- `reverse-proxy-runtime`: 扩展 HTTP 反向代理与 HTTPS TLS 终止转发的请求头语义，要求默认改写本地目标可见的 `Host` 与可解析 `Origin`。

## Impact

- 运行时：客户端 HTTP stream 出口需要在 `RoundTrip` 前规范化 `Host`、URL target 和可解析 `Origin`。
- HTTPS：TLS 终止路径通过现有 `kind: "http"` 转发自动适用；SNI 透传路径不变。
- 测试：需要覆盖 HTTP 代理、HTTPS TLS 终止、缺失/特殊 `Origin`、以及 HTTPS SNI 透传不改写的边界。
- 文档/规格：更新反向代理运行时合同，明确入口路由域名和目标请求头语义的分工。
