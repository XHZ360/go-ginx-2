## Context

当前 AdminUI 的证书行为有三类入口交织在一起：

- `/certificates` 展示 HTTPS proxy 派生出的证书状态，并承载 ACME、Cloudflare Origin CA 和 provider credential 操作。
- HTTPS proxy 创建/编辑表单直接暴露 `certFile` / `keyFile`，使用户误以为 proxy 页面也是证书维护入口。
- 后端 `ManagedCertificate` 以 `proxy_id` 唯一绑定 HTTPS proxy，HTTPS runtime 在没有静态文件路径时按 host 查托管证书，并要求证书记录属于当前 proxy。

这使“创建证书、删除证书、选择证书、修复证书、撤销 Origin CA”缺少单一管理边界。用户从 proxy 表单跳转去创建证书时，也没有可恢复的 proxy 草稿状态。

## Goals / Non-Goals

**Goals:**

- 让 Certificates 页成为所有证书增删和生命周期动作的唯一 AdminUI 管理入口。
- 让 HTTPS proxy 表单只选择证书，或跳转到证书创建流程。
- 支持从 proxy 表单跳转创建证书后返回原表单，并恢复已填写状态且自动选择新证书。
- 把证书建模为可被选择的独立资源，同时保留 secret-safe active material 边界。
- 对证书删除按风险分级处理：无效、失效/过期或未被使用的证书允许直接删除；仍被使用且可服务的证书删除需要强确认并明确后果。
- 保持现有 ACME DNS-01、Cloudflare Origin CA、静态文件型证书材料不进入 SQLite 的安全边界。

**Non-Goals:**

- 本变更不引入普通用户自助证书管理。
- 本变更不解决完整自定义域名所有权验证或平台代理域名证书策略。
- 本变更不要求实现多 proxy 共享同一证书；该能力可作为后续 SAN/wildcard 复用扩展。
- 本变更不要求在浏览器中上传私钥明文；如需要手动证书导入，优先登记服务端可访问的文件路径或受管存储结果。

## Decisions

### 1. 证书成为独立资源，proxy 显式选择证书

将证书生命周期资源从“隐式附着在 proxy 上”调整为“独立 certificate resource”。HTTPS proxy 记录保存选中的 certificate ID，证书记录保留 host、hostnames、provider、active material 文件路径、状态、失败信息和 provider metadata。

本次实现保持一个证书最多被一个 HTTPS proxy 绑定。证书可以先创建为未绑定状态，随后从 proxy 表单选择；也可以从 proxy 草稿带入 host 创建后回填。

备选方案：

- 继续让 `ManagedCertificate.proxy_id` 作为唯一主键语义：实现成本低，但无法满足“proxy 只选择证书”和“先创建证书再返回选择”的交互。
- 立即支持多 proxy 共享证书：对 wildcard/SAN 更灵活，但需要更完整的覆盖范围、引用计数、删除保护和运行时缓存设计，本次先不扩大范围。

### 2. HTTPS proxy 表单不再直接维护证书文件路径

AdminUI 的 HTTPS proxy 创建/编辑表单展示 certificate select、证书摘要和 `Create certificate` 跳转。`certFile` / `keyFile` 不作为主表单字段出现。

兼容层可以继续接受旧 GraphQL/CLI 输入中的 `certFile` / `keyFile`，但应把它们迁移或登记为 file-backed certificate resource，再绑定给该 proxy。前端新流程只提交 certificate ID。

备选方案：

- 在 proxy 表单中同时保留文件路径和 certificate select：短期兼容性好，但继续制造“证书在哪里维护”的混乱。

### 3. Certificates 页拥有证书创建、删除和生命周期动作

Certificates 页提供证书列表、创建/导入入口、provider credential 管理、ACME/Origin CA 生命周期动作、删除和引用状态。动作按钮必须按 provider、状态、是否有引用和 active material 可用性收敛。

删除证书前必须计算风险级别。未被任何 HTTPS proxy 绑定的证书、不可服务证书、过期/失效证书或缺失 active material 的证书可以直接删除，不要求输入式强确认或二次确认。仍被 HTTPS proxy 使用且当前可服务的证书删除会解除绑定并使 proxy 进入需要配置/证书无效状态，因此必须要求管理员输入 host、certificate ID 或等价强确认材料。Origin CA 撤销仍必须要求输入 host 或 Cloudflare certificate ID 等强确认，不由 UI 静默填充所有确认字段。

### 4. Proxy 到 certificate 创建使用草稿恢复

从 proxy 表单跳转到证书创建时，前端生成 draft ID，把当前表单状态写入 sessionStorage 或等价同源短期存储，并在 URL 中传递 `returnTo` 和 `draftId`。证书创建成功后返回 proxy 表单，读取 draft，恢复用户已填字段，并把新 certificate ID 写入选择器。

草稿只保存 proxy 配置输入和新证书选择结果，不保存私钥材料、Cloudflare token 或其他 secret。

备选方案：

- 全部表单状态放在 URL query：便于深链，但目标 host、描述等字段可能过长且容易泄漏不必要信息。
- 先创建 proxy 再创建证书：避免草稿，但会产生未配置证书的 proxy，和“创建后返回原表单”的体验目标不一致。

### 5. Runtime 通过绑定证书解析 TLS active material

HTTPS runtime 优先使用 proxy 绑定的 certificate ID 加载证书 active material，并验证证书覆盖该 proxy 的 SNI host。迁移期可以保留按 host 查找旧托管证书和旧 proxy file path 的 fallback，但新写入路径应使用显式 certificate ID。

## Risks / Trade-offs

- [Risk] 数据迁移触及 proxy 和 managed certificate 表结构，可能影响既有部署。  
  → Mitigation: 增加兼容读取路径和幂等迁移，把旧 `proxy.cert_file/key_file` 以及旧 `managed_certificates.proxy_id` 转换为 certificate resource + proxy binding。

- [Risk] 用户从 proxy 表单跳转期间关闭浏览器或 sessionStorage 清理，草稿无法恢复。  
  → Mitigation: 返回时检测 draft 缺失并显示明确提示；证书仍已创建，用户可手动选择。

- [Risk] 证书 hostnames 与 proxy SNI 不匹配会导致运行时失败。  
  → Mitigation: 绑定前由管理 API 校验，runtime 再做防御性校验并报告 certificate invalid/needs_config。

- [Risk] Origin CA 证书容易被误用为直连公网浏览器证书。  
  → Mitigation: Certificates 页展示 provider-specific deployment hints，并在选择 Origin CA 证书时提示其只适用于 Cloudflare 到源站 TLS。

- [Risk] 删除仍在使用的可服务证书可能让 HTTPS proxy 立即进入需要配置状态。  
  → Mitigation: 对该风险级别要求强确认，并在删除结果中明确返回受影响 proxy；对无效、失效/过期或未使用证书不增加额外确认负担。

## Migration Plan

1. 增加 certificate resource 与 proxy binding 字段/查询接口，保留旧字段读取兼容。
2. 迁移旧托管证书：为每条旧 `managed_certificates` 生成独立 certificate resource，并把对应 HTTPS proxy 绑定到该 certificate ID。
3. 迁移旧静态文件证书：把 proxy 上完整 `certFile/keyFile` 登记为 file-backed certificate resource，并绑定到 proxy。
4. 更新 GraphQL：新增/调整证书创建、删除、绑定、引用查询和 proxy create/update 输入。
5. 更新 AdminUI：Certificates 页集中动作，proxy 表单改为证书选择和创建跳转。
6. 更新 HTTPS runtime：优先按 certificate ID 解析 active material，保留迁移 fallback。
7. 增加测试覆盖后移除新 AdminUI 对 proxy 文件路径字段的依赖。

Rollback 时保留旧字段 fallback，可将 proxy binding 派生回 `certFile/keyFile` 或旧 `managed_certificates.proxy_id` 语义；但已创建的独立证书资源不应删除，避免丢失 active material 元数据。
