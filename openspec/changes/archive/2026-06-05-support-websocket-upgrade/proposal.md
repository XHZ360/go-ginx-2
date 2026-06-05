## Why

当前 HTTP 反向代理和 HTTPS TLS 终止路径只按普通 HTTP 请求/响应处理，遇到 WebSocket Upgrade 时无法在 `101 Switching Protocols` 后切换为双向隧道，导致 Nuxt/Vite HMR 等依赖 WebSocket 的场景转发失败。

该能力需要在默认 Host/Origin 改写机制之后补齐，使 HTTP 类反向代理既能正确完成 WebSocket 握手，也能在握手后稳定透传帧数据。

## What Changes

- 在 HTTP 反向代理中识别 WebSocket Upgrade 请求，并把握手转发到客户端本地 HTTP 目标。
- 在 HTTPS TLS 终止后的 HTTP 转发中支持同样的 WebSocket Upgrade 行为。
- 当本地目标返回 `101 Switching Protocols` 时，运行时 MUST 把外部连接、控制通道流和本地目标连接切换为双向字节隧道。
- WebSocket 握手请求继续应用默认 HTTP target header rewrite，即 `Host` 和可解析 `Origin` 按目标地址改写；握手成功后的 WebSocket 帧不解析、不改写。
- HTTPS SNI 透传路径保持现有加密 TCP 透传行为，不新增 HTTP 层解析或头改写。
- 非 WebSocket HTTP 请求继续使用现有普通请求/响应语义。

## Capabilities

### New Capabilities

- 无。

### Modified Capabilities

- `reverse-proxy-runtime`: 扩展 HTTP 反向代理和 HTTPS TLS 终止后的 HTTP 转发，声明并要求支持 WebSocket Upgrade 握手和 `101` 后的双向隧道。

## Impact

- 影响服务端 HTTP 入口：需要在可升级请求上接管外部连接并进入隧道。
- 影响服务端 HTTPS TLS 终止入口：需要在 TLS 连接上完成握手响应写回后进入隧道。
- 影响客户端控制通道 HTTP stream 出口：需要为 WebSocket Upgrade 使用可隧道化的目标连接，而不是只依赖 `http.RoundTripper` 的普通响应模型。
- 影响双向拷贝 helper：需统一现有两个分叉实现并采用 full-close 关闭语义（不实现 TCP half-close），关闭语义的取舍、成本与后续演进路径见 design 决策 7「关闭语义：为何选 full-close，何时引入 half-close」。
- 影响运行时测试：需要覆盖 HTTP WebSocket、HTTPS TLS 终止 WebSocket、非 Upgrade HTTP 回归、目标拒绝 Upgrade、HTTPS SNI 透传保持不变。
- 不引入新的外部 API、数据库字段或管理 UI 行为。
