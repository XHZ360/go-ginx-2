# ACME 证书创建前置条件可见性

## 元信息

| 项 | 值 |
| --- | --- |
| 状态 | `completed` |
| 最后更新 | `2026-07-16` |
| 相关需求 | [../../requirements/certificate-lifecycle.md](../../requirements/certificate-lifecycle.md)、[../../requirements/admin-ui/certificates.md](../../requirements/admin-ui/certificates.md) |
| 相关架构 | [../../architecture/certificate-management.md](../../architecture/certificate-management.md)、[../../architecture/engineering-quality-guardrails.md](../../architecture/engineering-quality-guardrails.md) |
| 相关决策 | 无 |
| 实现提交 | 未提交 |

> 本文描述尚未完成的目标与实施过程，不代表当前代码已经具备目标行为。

## 背景

生产环境尝试创建 ACME DNS-01 证书时失败。排查确认服务配置中 `acme_enabled` 为 `false`、`acme_account_email` 为空、`acme_terms_accepted` 为 `false`，且运行进程未持有 `CF_DNS_API_TOKEN`。现有 Certificates 页面仍允许选择并提交 ACME 创建请求，操作者只能在请求失败后推断服务器配置缺失。

ACME DNS-01 的可用性取决于服务端配置与进程环境，不能由浏览器直接验证。Cloudflare Origin CA credential 由管理面维护，但 ACME 的 Cloudflare DNS Token 当前只从服务端环境变量读取；两条 provider 链路的前置条件和修复路径不同。

## 当前实现

- `internal/daemon/server.go` 仅在 `ACMEEnabled` 为真时初始化 ACME issuer 与 Cloudflare DNS provider。
- `internal/certmanager/service.go` 的 `IssueCertificate` 根据 `providerType` 调用 ACME 或 Origin CA 签发；ACME 依赖已初始化的 issuer、DNS provider 与账户配置。
- `internal/certmanager/acme.go` 从 `acme_cloudflare_token_env` 指定的环境变量读取 Cloudflare DNS Token。
- `admin-ui/src/routes/CertificatesPage.tsx` 的创建对话框允许选择 ACME，但没有服务端就绪状态，也不能在提交前说明缺少哪一项。
- 证书生命周期状态只会在可创建/可更新的证书资源上记录；当前配置类失败可能发生在记录创建前，列表中的 `lastError` 无法完整解释失败。

## 问题

- 未启用或未配置 ACME 时，UI 仍提供可提交的 ACME 操作，失败反馈滞后且不可操作。
- 操作者无法区分“服务端未准备好”“Cloudflare Token 缺失/权限不足”“ACME CA 或 DNS challenge 失败”。
- 失败可能没有对应证书记录，导致 Certificates 列表不能成为完整的诊断入口。
- Cloudflare Origin CA credential 的就绪状态容易被误认为可复用到 ACME DNS-01。

## 目标

- 在 Certificates 页面明确展示 ACME DNS-01 与 Cloudflare Origin CA 的独立就绪状态和缺失项。
- ACME 不可用时阻止提交，并给出不含敏感数据的服务器侧修复说明。
- 让 API 返回结构化、可消费的 provider readiness 信息与签发失败分类。
- 在 ACME 可用但 Cloudflare 或 CA 调用失败时保留脱敏错误、失败次数与可重试状态。
- 保持 Origin CA 现有 credential 管理与 ACME 环境变量 Token 的安全边界。

## 非目标

- 不将 ACME Cloudflare DNS Token 写入 SQLite、GraphQL、浏览器存储或 Admin UI 表单。
- 不在本 Change 中实现 Cloudflare Universal SSL、Edge Certificate 或 DNS 代理状态管理。
- 不改变 ACME provider、DNS-01 challenge 类型或证书与 Domain 的 1:n 绑定模型。
- 不自动修改生产环境配置、环境变量或重启服务。

## 核心不变量

- readiness 响应只能包含布尔状态、配置的环境变量名称和脱敏诊断，绝不包含 Token、私钥、完整上游响应或环境变量值。
- 当 ACME 未就绪时，不得发起 ACME CA 或 Cloudflare DNS API 调用，也不得创建会被运行时选中的坏证书材料。
- ACME 与 Origin CA 的就绪状态、凭据来源和修复提示必须分别展示，不能以任一方可用推断另一方可用。
- 已有健康 active material 在续期失败时继续服务；首次创建失败不能影响已有证书或 Domain 绑定。
- 结构化错误须区分配置缺失、provider 未就绪、权限/业务拒绝与临时不可用。

## UX 原则

本 Change 遵循项目级 [UX 原则](../../project/ux-principles.md)；以下内容说明其在 ACME 创建流程中的具体约束。

- **将可预见失败前置化：** 能在提交前判断的配置缺失不得留到请求失败后才提示。
- **把系统能力与用户动作绑定：** 只有后端真实可执行时才允许提交；不可执行时说明原因与恢复路径。
- **状态必须可解释、可行动：** 错误信息应说明缺少什么、由谁修复以及修复后的下一步。
- **区分责任边界：** 表单输入、服务器配置与外部 provider 故障使用不同状态、文案和操作入口。
- **安全不等于不可观察：** 不暴露 Token、私钥或原始响应，但要提供安全的就绪状态、错误类别和诊断标识。
- **避免错误的能力暗示：** Origin CA credential 可用不代表 ACME DNS-01 可用；相似 provider 的依赖必须独立展示。
- **失败不丢失上下文：** 签发失败后保留域名、provider、返回路径和已填写字段，使修复后可以直接重试。
- **服务端是最终裁决者：** 前端 readiness 只用于引导，执行前必须由后端重新校验。
- **把诊断设计成产品能力：** 提供脱敏、可复制的诊断信息，避免操作者在 UI、日志和配置之间反复跳转。
- **默认保护运行中服务：** 首次创建失败不能污染运行时；续期失败优先保留健康 active material。

## 目标设计

### 数据模型

不新增或持久化 ACME Token。新增仅用于查询的 provider readiness 视图，至少包含：

- `providerType`
- `ready`
- `missingRequirements`（稳定枚举，如 `acme_disabled`、`account_email_missing`、`terms_not_accepted`、`dns_token_missing`）
- `tokenEnvName`（仅 ACME，环境变量名称）
- `guidance`（脱敏、面向操作者的修复摘要）

签发失败仍复用现有证书生命周期字段；在生成证书资源前发现的 readiness 失败通过结构化 API 错误返回，不伪造证书记录。

### 运行时流程

1. daemon 在构建证书服务时从实际配置和依赖初始化结果派生 ACME readiness。
2. 创建/签发 API 在执行 provider 调用前检查 readiness。
3. 未就绪时返回稳定错误码和缺失项，不调用 ACME 或 Cloudflare。
4. 已就绪后，ACME DNS-01 仍按既有流程创建 TXT 记录、签发、清理 challenge，并将提供方失败写入脱敏生命周期状态。
5. Origin CA 继续通过 provider credential 与 secret store 计算自己的 readiness；不得依赖 ACME Token 状态。

### API 与协议

- 增加管理员只读查询，用于返回各证书 provider 的 readiness。
- 为 ACME 未就绪定义结构化契约错误，例如 `provider_not_ready`，并携带可安全展示的 `missingRequirements`。
- 保留当前创建与生命周期 mutation；客户端在提交前查询 readiness，服务端仍必须强制校验以避免竞态或绕过。
- GraphQL、审计事件与日志不得回显 token 值、环境变量内容或 Cloudflare 原始响应。

### Admin UI

- Certificates 页面新增“提供方运行条件”区域，分别显示 ACME DNS-01 和 Cloudflare Origin CA 的就绪状态。
- ACME 条目显示邮箱、条款、启用状态与 `CF_DNS_API_TOKEN` 等已配置环境变量名称的检查结果；缺失时给出“修改服务端配置/注入环境变量并重启”的说明。
- 创建对话框保留 ACME 选项，但未就绪时禁用提交并在选项附近显示具体原因；不让操作者误以为功能不存在。
- Origin CA 显示 credential 就绪状态与既有部署边界提示，不将其状态混入 ACME 诊断。
- 创建失败后保留已输入域名和 provider 选择，并展示结构化错误的脱敏摘要；提供“复制诊断信息”操作，仅复制错误码、缺失项和修复提示。

### 安全与失败处理

- readiness 检查不得读取或传输 Token 明文；只判断环境变量是否存在且非空。
- Cloudflare DNS API 的权限、zone 查找、TXT 创建和清理失败映射为 provider 错误，并继续使用安全错误清洗。
- 由于服务重载或环境变化造成的 readiness 变化由服务端校验兜底；UI 缓存不是授权或安全边界。
- 首次签发失败不创建可服务材料；续期失败沿用当前 active material 与退避规则。

## 兼容与迁移

- 不涉及数据库 schema 或既有证书数据迁移。
- 旧客户端不调用 readiness 查询时，创建 mutation 仍返回结构化 readiness 错误，不出现假成功。
- 新 UI 需要与后端 readiness 查询同时发布；若 UI 先发布，显示“无法检查服务端就绪状态”并由 mutation 错误兜底。
- 回滚时移除 UI 前置检查即可，既有配置和证书状态不受影响；服务端不得回滚到泄露敏感配置的错误响应。

## 实施步骤

- [x] 定义 provider readiness 领域模型、ACME 缺失项枚举与错误映射。
- [x] 在 daemon/certmanager 暴露只读 readiness 并在签发前强制校验。
- [x] 增加 GraphQL 查询、错误扩展字段及 API 测试。
- [x] 更新 Certificates 页面、创建对话框和安全诊断复制交互。
- [x] 补充 ACME 配置缺失、Token 缺失、Origin CA 独立就绪、provider 失败与 secret safety 测试。
- [x] 同步 requirements、architecture、operations、Admin UI 文档和 worklog。

## 验收条件

- [x] `acme_enabled=false` 时，API 返回 `PROVIDER_NOT_READY` 与 `acme_disabled`，且不会调用 ACME 或 Cloudflare DNS。
- [x] ACME 邮箱、条款或 Token 环境变量缺失时，API 能逐项返回不含 secret 的缺失项。
- [x] UI 在提交前显示 ACME 未就绪原因并禁用创建；Origin CA 仍可按自身 readiness 操作。
- [x] ACME 运行条件满足后，创建流程可继续发起 DNS-01 签发；Cloudflare 权限或网络失败沿用现有脱敏、可操作的 provider 错误。
- [x] 创建前失败不会生成可服务证书或影响已有 Domain 绑定；续期失败沿用既有 active material 保留行为。
- [x] API、UI、日志、审计和测试快照均不包含 Token、私钥或完整 Cloudflare 响应。
- [x] 从 Domain/Proxy 跳转创建证书失败后，域名、provider 和返回上下文仍被保留。

## 验证记录

| 日期 | 命令/步骤 | 结果 | 说明 |
| --- | --- | --- | --- |
| 2026-07-16 | 生产只读排查 | 通过 | 确认 ACME 被关闭、邮箱/条款/Token 缺失；现有 Origin CA 证书可服务。 |
| 2026-07-16 | `go test ./internal/admin ./internal/adminquery ./internal/adminapi ./internal/store/sqlite -count=1` | 通过 | 当前证书管理相关回归测试通过；尚未包含本 Change 的目标测试。 |
| 2026-07-16 | `pnpm test -- certificates-page.test.tsx proxy-certificate-flow.test.tsx` | 通过 | 当前证书 UI 测试通过；尚未包含 readiness 交互。 |
| 2026-07-16 | `go test ./internal/adminapi -run 'TestServerReportsACMEReadinessAndBlocksCreate' -count=1` | 通过 | GraphQL readiness 与 `PROVIDER_NOT_READY` mutation 契约。 |
| 2026-07-16 | `go test ./internal/certmanager -run 'TestACMEReadiness' -count=1` | 通过 | 配置缺失阻断、无记录副作用及 Token 不回显。 |
| 2026-07-16 | `pnpm test -- certificates-page.test.tsx proxy-certificate-flow.test.tsx`、`pnpm build` | 通过 | Certificates UI 回归、GraphQL codegen、TypeScript 与生产构建。 |
| 2026-07-16 | `go test ./internal/certmanager ./internal/admin ./internal/adminapi -count=1` | 未通过 | `internal/certmanager` 既有 `TestFileSecretStoreRoundTripAndPathSafety` 在目录权限上失败；admin/adminapi 通过。 |

## 文档同步

- [x] requirements 已更新
- [x] architecture 已更新
- [x] operations 已更新
- [x] Admin UI 文档已更新
- [x] worklog 已更新

## 结果

已完成；实现提交待创建。
