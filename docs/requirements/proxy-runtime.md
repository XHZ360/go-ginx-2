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

- 同一 Host/SNI 下可配置多条路径前缀路由到不同 Client/target。
- 默认 `/` 后端由父 Proxy 的 Client/target 提供。
- 最长前缀优先，路径段边界匹配。
- 支持保留路径或剥离前缀后拼接上游前缀。
- 子路由 Client 必须与父 Proxy 同用户。
- 不支持正则、Header、Method 匹配与负载均衡。

## HTTPS 访问激活

- 可选；作用于整个 HTTPS Proxy（同 Domain 全部业务路径）。
- 管理员生成一次性激活链接/二维码；访问者确认后获得 Cookie。
- 未激活请求不得转发上游。
- 支持统一撤销全部访问；第一版不支持逐设备撤销。

## 证书

- HTTPS 必须绑定可服务证书（静态文件或托管证书）。
- 无可用证书时不得对外提供 HTTPS。
- 证书生命周期操作集中在证书管理能力，见 [certificate-lifecycle.md](certificate-lifecycle.md)。

## 验收口径

- TCP/UDP/HTTP/HTTPS 流量经 server+client 可达本地 target。
- 禁用代理、provider 离线、未知代理或用户不匹配时拒绝桥接。
- 路径路由按最长前缀命中正确后端；Client 离线仅影响对应路径。
- 开启访问激活后未激活返回 `401`；激活后业务请求与 WebSocket 可通过；撤销后立即失效。
- 认证 Cookie 不得到达上游。

## 相关文档

- 运行时设计：[../architecture/reverse-proxy.md](../architecture/reverse-proxy.md)
- UI：[admin-ui/proxies-list.md](admin-ui/proxies-list.md)、[admin-ui/proxy-detail.md](admin-ui/proxy-detail.md)
- 实施记录：[../changes/completed/http-path-routing-and-https-access-activation.md](../changes/completed/http-path-routing-and-https-access-activation.md)
