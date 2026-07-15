# 技术架构

运行时、协议、数据与集成设计。产品验收口径见 [../requirements/README.md](../requirements/README.md)；部署操作见 [../operations/README.md](../operations/README.md)。

## 文档列表

| 文档 | 说明 |
| --- | --- |
| [system-architecture.md](system-architecture.md) | 系统组成、数据流、代码入口与边界 |
| [control-channel.md](control-channel.md) | 控制通道、join/enrollment、consumer SDK |
| [reverse-proxy.md](reverse-proxy.md) | TCP/UDP/HTTP/HTTPS 运行时、路径路由与访问激活 |
| [certificate-management.md](certificate-management.md) | 证书来源、生命周期与失败语义 |
| [admin-and-observability.md](admin-and-observability.md) | Admin API/UI、统计、日志、审计范围 |
| [engineering-quality-guardrails.md](engineering-quality-guardrails.md) | 外部集成与生命周期工程质量护栏 |

路径路由与访问激活的实施记录见 [../changes/completed/http-path-routing-and-https-access-activation.md](../changes/completed/http-path-routing-and-https-access-activation.md)。

## 阅读顺序建议

1. system-architecture
2. control-channel → reverse-proxy → certificate-management
3. admin-and-observability
4. engineering-quality-guardrails（涉及 ACME/Cloudflare/异步生命周期时）
