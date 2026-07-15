# 反向代理产品需求

## 范围

系统提供 TCP、UDP、HTTP、HTTPS 反向代理。HTTPS 在服务端终止 TLS 后按 HTTP 语义转发。不提供任意目标正向代理。

## 代理类型

| 类型 | 入口选择 | 后端 |
| --- | --- | --- |
| TCP | bind host + port | Client 本地 TCP target |
| UDP | bind host + port | Client 本地 UDP target，按外部源会话 |
| HTTP | bind host + port + Host | Client 本地 HTTP target |
| HTTPS | bind host + port + SNI | 终止 TLS 后转发到 Client 本地 HTTP target |

管理员可创建、更新、启用、禁用、删除代理。启用态与入口冲突必须以管理错误返回，不得静默失败。

## 路径路由（HTTP/HTTPS）

- Domain 是独立资源，由单一用户拥有；同一 Domain 可以被多个独立 Proxy 使用。
- 路由模型是 `(Domain, PathPrefix) => Proxy`，HTTP 与 HTTPS 共享同一套映射。
- 每个 Web Proxy 选择一个 Domain，并直接保存自己的 PathPrefix、Client、target 与改写配置。
- 最长前缀优先，路径段边界匹配。
- 支持保留路径或剥离前缀后拼接上游前缀。
- Domain、Proxy 与 Client 必须属于同一用户。
- 不支持正则、Header、Method 匹配与负载均衡。

> 实现状态：当前代码仍使用“一个父 Proxy + 多个 ProxyRoute 子路由”的旧模型，尚未满足上述需求。改造计划见 [../changes/active/domain-path-proxy-routing.md](../changes/active/domain-path-proxy-routing.md)。

## HTTPS 访问激活

- 可选；作用于最终由 Domain + Path 命中的单个 Proxy。
- 管理员生成一次性激活链接/二维码；访问者确认后获得 Cookie。
- 未激活请求不得转发上游。
- 支持统一撤销全部访问；第一版不支持逐设备撤销。

## 证书

- Domain 与证书可选一对一绑定；启用 HTTPS entry 的 Domain 必须绑定可服务证书（静态文件或托管证书）。
- 无可用证书时不得对外提供 HTTPS。
- 证书生命周期操作集中在证书管理能力，见 [certificate-lifecycle.md](certificate-lifecycle.md)。

## 验收口径

- TCP/UDP/HTTP/HTTPS 流量经 server+client 可达本地 target。
- 禁用代理、provider 离线、未知代理或用户不匹配时拒绝桥接。
- 同一 Domain 的多个独立 Proxy 按最长前缀命中；Client 离线仅影响命中的 Proxy。
- HTTP 与 HTTPS 对相同 Domain+Path 命中同一个 Proxy。
- 开启访问激活后未激活返回 `401`；激活后业务请求与 WebSocket 可通过；撤销后立即失效。
- 认证 Cookie 不得到达上游。

## 相关文档

- 运行时设计：[../architecture/reverse-proxy.md](../architecture/reverse-proxy.md)
- UI：[admin-ui/proxies-list.md](admin-ui/proxies-list.md)、[admin-ui/proxy-detail.md](admin-ui/proxy-detail.md)
- 实施记录：[../changes/completed/http-path-routing-and-https-access-activation.md](../changes/completed/http-path-routing-and-https-access-activation.md)
