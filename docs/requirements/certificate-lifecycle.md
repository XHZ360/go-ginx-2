# 证书生命周期需求

## 证书类型

| 类型 | 用途 |
| --- | --- |
| 控制通道 CA/证书 | 仅服务端与客户端控制面 TLS，非公网 HTTPS |
| HTTPS 静态证书 | 文件路径绑定到 HTTPS proxy |
| 托管证书 ACME DNS-01 | Cloudflare DNS-01 自动签发/续期 |
| 托管证书 Origin CA | Cloudflare Origin CA 签发/轮换/同步 |

## 管理原则

- 证书是独立资源；权威绑定在 Domain（`domains.certificate_id`）。
- 证书与 Domain 为 **1:n**：一张证书可被多个 Domain 引用（例如通配证书）；每个 Domain 最多一张证书。
- 证书增删与生命周期动作集中在证书管理入口；Domain 页面负责绑定/解绑；Web Proxy 不持有权威证书绑定。
- SQLite 只存路径、元数据与脱敏状态；私钥与 provider token 不得入库。
- 首次创建失败且无可用材料时，不得让坏状态进入运行时。
- 续期/轮换失败时保留旧的可用 active material。

> 实现状态：HTTPS 在选择 Proxy 前按 Domain 解析证书。Change 见 [../changes/completed/domain-path-proxy-routing.md](../changes/completed/domain-path-proxy-routing.md)。

## 用户可见状态

- `serving_status`：active 材料是否可服务（`usable`、`expiring_soon`、`expired`、`missing`、`invalid`）。
- `operation_status`：最近签发/续期/轮换/同步结果。
- Origin CA 额外展示 provider 同步相关字段。

健康 active 材料即使最近操作失败仍可继续服务；新材料通过校验后才替换 active 文件。

## 操作

- 签发、续期、状态查询（Admin UI 或 `goginx-admin`）。
- Origin CA：同步、显式撤销（高风险，不因轮换自动发生）。
- 删除仍在服务的绑定证书会解除相关 Domain 绑定，并使依赖该证书的 HTTPS entry fail closed，直到重新绑定可服务证书。

## 验收口径

- 无可用证书的 Domain HTTPS entry 不对外服务。
- 续期失败不覆盖健康 active 证书。
- API、日志、审计、UI 不泄露私钥、token 或完整敏感响应。
- 证书 hostnames 不覆盖 Domain Host 时绑定失败并返回可消费错误；同一证书绑定多个 Domain 是允许行为。

## 已知缺口

- 平台域名证书范围与自定义域名所有权验证。
- 完整手动上传证书生命周期。

## 相关文档

- 设计：[../architecture/certificate-management.md](../architecture/certificate-management.md)
- 操作：[../operations/certificate-operations.md](../operations/certificate-operations.md)
- 绑定迁移：[../references/certificate-binding-migration.md](../references/certificate-binding-migration.md)
- UI：[admin-ui/certificates.md](admin-ui/certificates.md)
