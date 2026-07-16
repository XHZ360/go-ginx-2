# 证书绑定迁移与兼容说明

本篇说明证书集中管理与后续 Domain 绑定改造下的迁移路径。当前权威绑定为 **Domain**（`domains.certificate_id`），证书与 Domain 为 **1:n**。与 UI 相关见 [../requirements/admin-ui/certificates.md](../requirements/admin-ui/certificates.md)、[../requirements/admin-ui/domain-detail.md](../requirements/admin-ui/domain-detail.md)。

> 当前产品行为以 Domain 绑定为准。下文第 1–5 节保留 Proxy 绑定期历史说明，供旧数据排障；Domain 路由迁移见 [../operations/domain-path-routing-migration.md](../operations/domain-path-routing-migration.md)。

## 1. 绑定模型变化（历史）

- 改造前：`ManagedCertificate` 以 `proxy_id` 唯一绑定 HTTPS proxy；HTTPS proxy 表单直接保存 `certFile` / `keyFile`。
- Proxy 绑定期：证书是独立资源；HTTPS proxy 通过 `proxies.certificate_id` 显式绑定。
- **当前：** 权威绑定迁到 Domain（`domains.certificate_id`）；一证可服务多 Domain；Web Proxy 不再持有权威证书绑定。

## 2. 迁移策略（启动时幂等）

- **旧托管证书（`managed_certificates.proxy_id`）**：先收敛为独立证书资源与 proxy 绑定，再随 Domain Path 迁移写入 `domains.certificate_id`。
- **旧静态文件证书（proxy 上的 `certFile` / `keyFile`）**：登记为 file-backed certificate 资源后绑定，再迁到 Domain。
- Domain Path 迁移将 HTTP/HTTPS Proxy 的证书绑定合并到按 Host 聚合的 Domain；同一 Domain 多证书冲突时停止并要求人工处理。

## 3. 私钥不入库

迁移和登记 file-backed 证书时，私钥材料始终只以文件路径或受管文件形式存在，绝不进入 SQLite。SQLite 仅存储文件路径、状态、指纹、有效期和脱敏错误。UI 不回显私钥内容。

## 4. 运行时解析（当前）

- HTTPS 按 listener + SNI 选择 Domain，再按 `Domain.CertificateID` 解析可服务证书。
- 校验证书 hostnames 覆盖 Domain Host（含单级通配）。
- Domain 未绑定或证书不可服务时 HTTPS fail closed。

## 5. 证书删除与解绑（当前）

- 证书是独立资源；删除 Domain 或 Web Proxy 不会自动删除证书文件。
- Domain 解绑后证书资源保留，可再绑到其他 Domain。
- 删除仍被 Domain 引用的证书会解除相关 Domain 绑定，HTTPS 在重新绑定前不可用。

## 6. 兼容层接口边界

- 管理面新路径使用 Domain 绑定 mutation（`bindDomainCertificate` / `unbindDomainCertificate`）。
- 证书删除只清理受管证书目录中可安全归属的 active/previous material。
- 回滚 Domain Path 迁移只能回到数据库备份点，不会反向重建旧 `proxy_routes` 或 Proxy 证书绑定语义。
