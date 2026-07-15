# 反向代理运行时

## 支持类型

- TCP：入口连接通过控制通道流转发到客户端 TCP target。
- UDP：按外部源地址维护会话，并在空闲后清理。
- HTTP：按 listener 和 `Host` 路由，转发前按 target 改写可解析的 `Host`/`Origin`。
- HTTPS：按 listener 和 TLS SNI 路由，要求静态或健康的托管证书后终止 TLS，再转发解密后的 HTTP。

HTTP/HTTPS 支持 HTTP/1.1 WebSocket Upgrade，并在升级后进入双向隧道。每类代理都可以指定入口 bind host、port；空值回退到全局入口配置。启用、禁用、更新或删除代理后，服务端会协调所需 listener，冲突或绑定失败作为管理操作错误返回。

## HTTPS 证书边界

HTTPS proxy 没有可服务的静态证书或托管 active material 时标记为 `needs_config`，新 TLS 连接关闭；不会把加密 TLS 字节隐式透传到 target。显式配置但证书缺失、不可读、过期、域名不匹配或 key 不匹配时同样失败关闭。

证书健康检查使用 TLS hostname 语义：`*.example.com` 只覆盖单层子域，不覆盖 apex 或多层子域。托管证书的签发、续期、热加载和失败保留规则见 `docs/daemon-runtime.md` 与 `docs/engineering-quality-guardrails.md`。

## 排查与验证

确认 proxy 已启用、client ID 正确、provider 在线，以及实际 listener 的 bind host、port、Host/SNI 和 target 地址。运行时实现位于 `internal/proxy/`，listener 协调位于 `internal/daemon/proxy_listeners.go`；验证使用 `go test ./internal/proxy/... ./internal/daemon -count=1`。

当前不支持正向代理、访问密码、分享链接或配额限速。
