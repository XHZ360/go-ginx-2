# 证书生命周期需求

## 证书类型

| 类型 | 用途 |
| --- | --- |
| 控制通道 CA/证书 | 仅服务端与客户端控制面 TLS，非公网 HTTPS |
| HTTPS 静态证书 | 文件路径绑定到 HTTPS proxy |
| 托管证书 ACME DNS-01 | Cloudflare DNS-01 自动签发/续期 |
| 托管证书 Origin CA | Cloudflare Origin CA 签发/轮换/同步 |

## 管理原则

- 证书是独立资源；HTTPS proxy 通过 `certificateId` 显式绑定。
- 证书增删与生命周期动作集中在证书管理入口；proxy 表单只负责选择或跳转创建。
- SQLite 只存路径、元数据与脱敏状态；私钥与 provider token 不得入库。
- 首次创建失败且无可用材料时，不得让坏状态进入运行时。
- 续期/轮换失败时保留旧的可用 active material。

> 目标模型已改为 Domain 与证书可选一对一绑定，HTTPS 在选择 Proxy 前按 Domain 解析证书。当前 Proxy 绑定语义仍是已实现行为；迁移见 [../changes/active/domain-path-proxy-routing.md](../changes/active/domain-path-proxy-routing.md)。

## 用户可见状态

- `serving_status`：active 材料是否可服务（`usable`、`expiring_soon`、`expired`、`missing`、`invalid`）。
- `operation_status`：最近签发/续期/轮换/同步结果。
- Origin CA 额外展示 provider 同步相关字段。

健康 active 材料即使最近操作失败仍可继续服务；新材料通过校验后才替换 active 文件。

## 操作

- 签发、续期、状态查询（Admin UI 或 `goginx-admin`）。
- Origin CA：同步、显式撤销（高风险，不因轮换自动发生）。
- 删除仍在服务的绑定证书会解除绑定并使对应 HTTPS proxy 需要重新配置。

## 验收口径

- 无可用证书的 HTTPS proxy 不对外服务。
- 续期失败不覆盖健康 active 证书。
- API、日志、审计、UI 不泄露私钥、token 或完整敏感响应。
- 绑定冲突（一证书多 proxy）以可消费错误返回。

## 已知缺口

- 平台域名证书范围与自定义域名所有权验证。
- 完整手动上传证书生命周期。

## 相关文档

- 设计：[../architecture/certificate-management.md](../architecture/certificate-management.md)
- 操作：[../operations/certificate-operations.md](../operations/certificate-operations.md)
- 绑定迁移：[../references/certificate-binding-migration.md](../references/certificate-binding-migration.md)
- UI：[admin-ui/certificates.md](admin-ui/certificates.md)
