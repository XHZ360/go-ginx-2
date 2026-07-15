# 产品需求

产品能力、业务边界与 Admin UI 验收口径。实现细节见 [../architecture/README.md](../architecture/README.md)。

## 文档列表

| 文档 | 说明 |
| --- | --- |
| [proxy-runtime.md](proxy-runtime.md) | 反向代理类型、路径路由、访问激活与验收 |
| [client-access.md](client-access.md) | Provider/Consumer、join、授权边界 |
| [certificate-lifecycle.md](certificate-lifecycle.md) | 证书来源、绑定、生命周期与可见状态 |
| [limits.md](limits.md) | 已执行限制与明确的配额/限速/容量缺口 |
| [admin-ui/README.md](admin-ui/README.md) | 管理后台逐页 UI 设计与交互约束 |

## 当前产品范围（摘要）

- 管理员管理用户、客户端、代理与托管证书。
- Provider 客户端暴露本地 target；Consumer 访问已授权代理。
- TCP/UDP/HTTP/HTTPS 反向代理；HTTP/HTTPS 支持路径前缀路由。
- HTTPS 可启用访问激活；需可用证书终止 TLS。
- 基础累计统计与轻量审计；完整配额与告警未实现。

## 相关架构

- 系统组成：[../architecture/system-architecture.md](../architecture/system-architecture.md)
- 反向代理运行时：[../architecture/reverse-proxy.md](../architecture/reverse-proxy.md)
- 管理与可观测性：[../architecture/admin-and-observability.md](../architecture/admin-and-observability.md)
