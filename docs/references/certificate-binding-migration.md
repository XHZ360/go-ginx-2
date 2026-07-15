# 证书绑定迁移与兼容说明

本篇说明证书集中管理改造下的绑定模型迁移与兼容策略：旧 `managed_certificates.proxy_id` 与旧 HTTPS proxy 直接保存的 `certFile` / `keyFile` 如何收敛为“独立证书资源 + 显式绑定”，以及迁移期保留的运行时兼容路径。与 UI 行为相关的部分见 [../requirements/admin-ui/certificates.md](../requirements/admin-ui/certificates.md)、[../requirements/admin-ui/proxies-list.md](../requirements/admin-ui/proxies-list.md) 和 [../requirements/admin-ui/proxy-detail.md](../requirements/admin-ui/proxy-detail.md)。

## 1. 绑定模型变化

- 改造前：`ManagedCertificate` 以 `proxy_id` 唯一绑定 HTTPS proxy；HTTPS proxy 表单直接保存 `certFile` / `keyFile`，运行时在没有静态文件路径时按 host 查托管证书。
- 改造后：证书是独立资源；HTTPS proxy 通过 `proxies.certificate_id` 显式绑定证书。本次实现保持一个证书最多绑定一个 proxy。

## 2. 迁移策略（启动时幂等迁移）

迁移为幂等操作，保留旧字段读取兼容：

- **旧托管证书（`managed_certificates.proxy_id`）**：旧的 1:1 绑定语义迁移为 `proxies.certificate_id`，即为每条旧记录生成（或对应）独立证书资源，并把对应 HTTPS proxy 绑定到该 certificate ID。
- **旧静态文件证书（proxy 上的 `certFile` / `keyFile`）**：在启动时把 proxy 上完整的证书文件 / 私钥文件路径登记为 file-backed certificate 资源，并绑定到该 proxy，使 proxy 可继续使用该证书。

## 3. 私钥不入库

迁移和登记 file-backed 证书时，私钥材料始终只以文件路径或受管文件形式存在，绝不进入 SQLite。SQLite 仅存储文件路径、状态、指纹、有效期和脱敏错误。UI 不回显私钥内容。

## 4. 运行时兼容 fallback

- 运行时优先按 proxy 绑定的 certificate ID 解析 TLS active material，并校验证书覆盖该 proxy 的 SNI host。
- 迁移期保留按 host 查找旧托管证书和按旧 proxy 文件路径的 fallback，以兼容尚未迁移完成的数据。
- 该 fallback 仅为迁移期兼容路径，不是新 AdminUI 的主路径；新写入路径统一使用显式 certificate ID。新的 AdminUI HTTPS proxy 表单只提交 `certificateId`，不再提交 `certFile` / `keyFile`。

## 5. 证书在 proxy 删除后保留

证书是独立资源，生命周期与 proxy 解耦：

- 删除对应 proxy 后，证书保留为未绑定资源，不随 proxy 一并删除；需在证书页显式删除。
- 把 proxy 的证书选择改为其他证书或清空后，原证书同样保留为未绑定资源。
- 反向地，在证书页删除仍在服务的绑定证书会解除绑定，并把受影响 proxy 标记为需要重新配置。

## 6. 兼容层接口边界

- 兼容层可以继续接受旧 GraphQL/CLI 输入中的 `certFile` / `keyFile`，但应把它们迁移或登记为 file-backed certificate 资源后再绑定给 proxy。
- 证书删除只清理受管证书目录中可安全归属的 active/previous material，不会删除不属于受管目录或无法安全归属的任意外部文件路径。
- 回滚时保留旧字段 fallback，可将 proxy 绑定派生回 `certFile` / `keyFile` 或旧 `managed_certificates.proxy_id` 语义；但已创建的独立证书资源不应删除，避免丢失 active material 元数据。
