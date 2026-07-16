# Domain + Path 到 Proxy 的路由模型

## 状态

已采纳，尚未完成实现迁移。

## 背景

当前实现把 HTTP/HTTPS Domain 绑定到一个父 Proxy，再通过 `ProxyRoute` 把多个 Path 挂到该 Proxy 下。这使 Domain 成为 Proxy 的属性，阻止同一 Domain 被多个独立 Proxy 使用，也把证书、TLS、路径路由和后端目标耦合在同一个资源中。

产品需要以 `domain + path => proxy` 作为 Web 反向代理的入口模型：Domain 负责公网主机身份和 TLS，Proxy 负责某个路径对应的 Client 与 target。

## 决策

- Domain 提取为独立资源，并由单一用户拥有。
- 同一 Domain 下的所有 Proxy 必须属于 Domain 所有者。
- HTTP 与 HTTPS 共享同一套 `(Domain, PathPrefix) => Proxy` 映射。
- 同一 Domain 下规范化后的 `PathPrefix` 唯一，运行时按最长路径段前缀选择 Proxy。
- Domain 与证书采用可选 **1:n** 关系：每个 Domain 最多绑定一张证书；同一张证书可被多个 Domain 引用（例如 `*.example.com` 通配证书服务多个子域）。HTTPS entry 启用时 Domain 必须绑定可服务证书。
- HTTPS 先根据 SNI 解析 Domain 和证书并完成 TLS，再根据请求 Path 选择 Proxy。
- HTTPS 访问激活归最终命中的 Proxy，不归 Domain；路径选择完成后再执行访问认证。
- TCP/UDP Proxy 不受该模型影响。

## 核心不变量

- Domain 的规范化 Host 在系统内唯一，不能由多个用户分别声明。
- Domain 可以暴露 HTTP、HTTPS 或两者的一个或多个 entry；所有 entry 共享该 Domain 的路径映射。
- HTTPS entry 没有可服务证书时必须 fail closed。
- `(domain_id, normalized_path_prefix)` 唯一；`/api` 匹配 `/api` 和 `/api/users`，不匹配 `/apix`。
- `/.well-known/goginx/` 是系统保留路径，不能被普通 Proxy 占用。
- 未命中任何 Proxy 时返回 `404`，不存在隐式默认 target。
- Proxy 的 Domain 或 PathPrefix 变化时，必须使该 Proxy 的旧访问 Token/Cookie 失效。

## 补充决定（2026-07-15）

- **HTTP 入口与访问认证：** 命中启用认证的 Web Proxy 时，若 Domain 存在可用 HTTPS entry，则 HTTP 请求 `308` 到对应 HTTPS URL；否则返回 `403`。禁止经 HTTP 明文完成 Cookie 认证或转发上游。
- **历史统计迁移：** 父 Proxy ID 保留给 `/` Web Proxy，其迁移前累计统计作为 legacy aggregate 保留并在 UI 标注；由旧 `ProxyRoute` 生成的新 Proxy 从零计数。新流量按最终命中的 Proxy 聚合。

## 后果

- Web Proxy 不再以 `http` / `https` 类型区分公网协议；Domain entry 决定 HTTP/HTTPS 暴露方式，Proxy 表达 Web 路径后端。
- `ProxyRoute` 子资源和 `proxy_routes` 表将被移除或迁移为独立 Proxy。
- `EntryHost`、Web entry 配置和 `CertificateID` 从 Proxy 迁到 Domain/Domain entry。
- listener 协调、证书解析、GraphQL、Admin UI 和数据迁移都需要调整。
- 旧模型已经实现，迁移期间必须明确区分当前行为和目标行为。

## 相关文档

- 实施 Change：[../changes/active/domain-path-proxy-routing.md](../changes/active/domain-path-proxy-routing.md)
- 产品需求：[../requirements/proxy-runtime.md](../requirements/proxy-runtime.md)
- 当前运行时：[../architecture/reverse-proxy.md](../architecture/reverse-proxy.md)
- 被替代的实施记录：[../changes/completed/http-path-routing-and-https-access-activation.md](../changes/completed/http-path-routing-and-https-access-activation.md)
