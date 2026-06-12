## Why

当前 AdminUI 中证书生命周期动作分散在证书页和 HTTPS proxy 表单语义之间：证书页同时承担状态表、Cloudflare 凭据管理和生命周期操作，HTTPS proxy 又直接暴露证书/私钥文件路径，导致用户难以判断何处创建、删除、撤销或绑定证书。

本变更把证书作为独立管理对象集中到 Certificates 页，并让 HTTPS proxy 只负责选择已存在证书或跳转创建证书，解决交互边界混乱、危险操作确认弱、表单状态丢失和证书绑定模型不清的问题。

## What Changes

- Certificates 页统一承载证书创建、删除、签发、续签、轮换、同步、撤销和 Cloudflare provider credential 管理。
- HTTPS proxy 创建/编辑表单移除直接维护证书/私钥文件路径的主流程，只保留证书选择控件和创建证书跳转入口。
- 从 HTTPS proxy 表单跳转到证书创建流程时，系统保留已填写的 proxy 表单草稿；证书创建成功后返回原表单并自动选中新证书。
- 证书资源和 HTTPS proxy 绑定语义从“proxy 内直接维护证书文件路径/托管证书隐式绑定”收敛为显式 certificate selection/binding。
- Certificates 页按证书类型和状态控制可用动作，展示证书 ID、provider、hostnames、有效期、使用位置、部署提示和脱敏错误。
- Origin CA 撤销和删除仍在使用且可服务的证书等高风险动作使用强确认；无效、失效/过期或未被使用的 certificate 允许直接删除，不要求二次确认。
- **BREAKING**：AdminUI 的 HTTPS proxy 表单不再把证书文件路径作为主要证书管理入口；已有 API/模型可能需要迁移或兼容层，以支持显式证书选择。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `admin-resource-management`: 调整 AdminUI 和 GraphQL 管理契约，使 Certificates 页成为证书生命周期唯一管理入口，并让 HTTPS proxy 表单只选择或跳转创建证书。
- `certificate-management`: 调整证书与 HTTPS proxy 的绑定契约，从 proxy 文件路径/隐式托管绑定收敛为显式证书资源选择，同时保留 secret-safe active material 边界。

## Impact

- AdminUI：`CertificatesPage`、`ProxiesPage`、`ProxyDetailPage`、路由状态恢复、表单草稿持久化和相关测试。
- Admin GraphQL：证书列表/详情、创建/删除/绑定、proxy create/update 输入、证书引用状态、删除风险分级和动作 payload。
- 后端管理服务：证书资源模型、proxy 绑定模型、迁移兼容、生命周期动作审计和引用校验。
- HTTPS runtime：SNI 证书解析需使用显式绑定或兼容映射，同时继续保持私钥材料只以文件路径/受管文件存在。
- 文档：AdminUI 证书页、HTTPS proxy 创建流程、证书 provider/Origin CA 使用边界和迁移说明。
