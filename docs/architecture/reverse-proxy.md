# 反向代理运行时

## 支持类型

- TCP：入口连接通过控制通道流转发到客户端 TCP target。
- UDP：按外部源地址维护会话，并在空闲后清理。
- HTTP：按 listener 和 `Host` 路由，转发前按 target 改写可解析的 `Host`/`Origin`。
- HTTPS：按 listener 和 TLS SNI 路由，要求静态或健康的托管证书后终止 TLS，再转发解密后的 HTTP。

HTTP/HTTPS 支持 HTTP/1.1 WebSocket Upgrade，并在升级后进入双向隧道。每类代理都可以指定入口 bind host、port；空值回退到全局入口配置。启用、禁用、更新或删除代理后，服务端会协调所需 listener，冲突或绑定失败作为管理操作错误返回。

## 路径路由

> **当前实现说明：** 以下父 Proxy + `ProxyRoute` 是现有代码行为，但目标模型已调整为 `(Domain, PathPrefix) => Proxy`。新模型决策见 [../decisions/domain-path-proxy-routing.md](../decisions/domain-path-proxy-routing.md)，实施计划见 [../changes/active/domain-path-proxy-routing.md](../changes/active/domain-path-proxy-routing.md)。在 Change 完成前，本节继续记录实际运行行为。

HTTP 与终止 TLS 的 HTTPS 支持路径前缀路由（类似 nginx `location` + 显式 `proxy_pass` 语义）：

- 父 Proxy 仍是虚拟主机（Host/SNI、证书、默认 `/` 后端）。
- 子路由 `ProxyRoute` 覆盖特定 `PathPrefix` 的 Client 与 target。
- 最长前缀优先，且要求路径段边界：`/api` 匹配 `/api`、`/api/users`，不匹配 `/apix`。
- `PathPrefix` 必须以 `/` 开头；Query/Fragment 不参与匹配。
- 系统保留 `/.well-known/goginx/`，用户路由不得占用。
- `StripPrefix` + `UpstreamPathPrefix` 显式改写路径；Query 原样保留。
- 子路由 Client 必须与父 Proxy 同用户；Client 离线时该路由返回 `503`，不影响其他路由。
- 路由更新落库后立即生效，不要求 listener reconcile。
- 流量统计按父 Proxy 聚合，不按子路由拆分。

未匹配子路由时使用父 Proxy 默认后端；无可用默认后端时返回 `404`。

## HTTPS 访问激活

HTTPS Proxy 可启用整站访问激活（不按单条路径配置）：

1. 管理员原子开启认证并生成一次性激活链接（同 Domain）。
2. 访问者 `GET` 确认页后 `POST` 兑换；成功后设置 HttpOnly Cookie。
3. 后续请求在路径路由前校验 Cookie；失败返回 `401`，且不得转发。
4. 转发前移除 go-ginx 认证 Cookie；WebSocket 同样先认证。
5. 统一撤销递增 `AccessAuthVersion`，使全部 Token 与 Cookie 立即失效。

激活路径：`https://<domain>/.well-known/goginx/activate/<token>`。Token/secret 仅存 hash；完整 URL 只在创建响应返回一次。关闭认证或删除 Proxy 时统一撤销。

## HTTPS 证书边界

HTTPS proxy 没有可服务的静态证书或托管 active material 时标记为 `needs_config`，新 TLS 连接关闭；不会把加密 TLS 字节隐式透传到 target。显式配置但证书缺失、不可读、过期、域名不匹配或 key 不匹配时同样失败关闭。

证书健康检查使用 TLS hostname 语义：`*.example.com` 只覆盖单层子域，不覆盖 apex 或多层子域。托管证书的签发、续期、热加载和失败保留规则见 [../operations/daemon-runtime.md](../operations/daemon-runtime.md)、[../operations/certificate-operations.md](../operations/certificate-operations.md) 与 [engineering-quality-guardrails.md](engineering-quality-guardrails.md)。

## 错误语义（HTTP/HTTPS）

| 场景 | 响应 |
| --- | --- |
| 路由不存在 | `404` |
| 路由 Client 离线 | `503` |
| 上游开流/响应失败 | `502` |
| 未激活或 Cookie 无效 | `401` |
| 认证存储不可用 | fail closed（不绕过） |

## 排查与验证

确认 proxy 已启用、client ID 正确、provider 在线，以及实际 listener 的 bind host、port、Host/SNI 和 target 地址。路径路由与访问激活的实施记录见 [../changes/completed/http-path-routing-and-https-access-activation.md](../changes/completed/http-path-routing-and-https-access-activation.md)。

运行时实现位于 `internal/proxy/`，listener 协调位于 `internal/daemon/proxy_listeners.go`；验证使用 `go test ./internal/proxy/... ./internal/daemon -count=1`。

当前不支持正向代理、按路径的访问密码、分享链接、正则/Header/Method 路由、逐设备撤销或配额限速。
