# 反向代理运行时

## 支持类型

- TCP：入口连接通过控制通道流转发到客户端 TCP target。
- UDP：按外部源地址维护会话，并在空闲后清理。
- HTTP：按 listener 和 `Host` 路由，转发前按 target 改写可解析的 `Host`/`Origin`。
- HTTPS：按 listener 和 TLS SNI 路由，要求静态或健康的托管证书后终止 TLS，再转发解密后的 HTTP。

HTTP/HTTPS 支持 HTTP/1.1 WebSocket Upgrade，并在升级后进入双向隧道。每类代理都可以指定入口 bind host、port；空值回退到全局入口配置。启用、禁用、更新或删除代理后，服务端会协调所需 listener，冲突或绑定失败作为管理操作错误返回。

## 路径路由

> 当前事实：Domain + Path 模型已落地（见 [../decisions/domain-path-proxy-routing.md](../decisions/domain-path-proxy-routing.md) 与 [../changes/completed/domain-path-proxy-routing.md](../changes/completed/domain-path-proxy-routing.md)）。

HTTP 与终止 TLS 的 HTTPS 使用共享路径映射：

- Domain 独立资源：Host 全局唯一，持有 HTTP/HTTPS entry 与可选证书绑定。
- Web Proxy 使用 `(domain_id, path_prefix)`；最长路径段前缀命中（`/api` 匹配 `/api/users`，不匹配 `/apix`）。
- HTTP 与 HTTPS 对相同 Domain+Path 命中同一 Proxy。
- `PathPrefix` 必须以 `/` 开头；Query/Fragment 不参与匹配。
- 系统保留 `/.well-known/goginx/`，用户路由不得占用。
- `StripPrefix` + `UpstreamPathPrefix` 显式改写路径；Query 原样保留。
- Domain、Proxy、Client 必须同用户；Client 离线时仅该 Proxy 返回 `503`。
- listener 由 DomainEntry 驱动；Proxy 启停不直接创建 listener。
- 流量统计按最终命中的 Proxy 聚合；迁移后 `/` Proxy 可携带 legacy aggregate 标记。

未命中任何 Proxy 返回 `404`。

## HTTPS 访问激活

访问激活归最终命中的 Web Proxy：

1. 管理员原子开启认证并生成一次性激活链接（HTTPS Domain entry）。
2. 访问者 `GET` 确认页后 `POST` 兑换；成功后设置 HttpOnly Cookie（Path 为该 Proxy 的 PathPrefix）。
3. 路径选择后再校验 Cookie；失败返回 `401`，且不得转发。
4. 转发前移除 go-ginx 认证 Cookie；WebSocket 同样先认证。
5. 统一撤销递增 `AccessAuthVersion`，使全部 Token 与 Cookie 立即失效。
6. 经 HTTP entry 命中启用认证的 Proxy：有可用 HTTPS entry 则 `308` 重定向，否则 `403`。

激活路径：`https://<domain>/.well-known/goginx/activate/<token>`。Token/secret 仅存 hash；完整 URL 只在创建响应返回一次。关闭认证或删除 Proxy 时统一撤销。

## HTTPS 证书边界

证书绑定到 Domain（**1:n**：一证可被多 Domain 引用，每 Domain 最多一证）。HTTPS entry 没有可服务证书时 fail closed；不会把加密 TLS 字节隐式透传到 target。证书缺失、不可读、过期、域名不匹配或 key 不匹配时同样失败关闭。

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
