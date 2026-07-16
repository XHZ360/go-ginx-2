# 证书页 UI 设计

## 1. 页面定位

证书页是管理后台中证书增删和生命周期动作的唯一管理入口。所有证书的创建、删除、签发、续期、轮换、同步、撤销以及 Cloudflare provider credential 管理都集中在本页完成；HTTPS proxy 表单只负责选择已存在证书或跳转到本页创建证书，不再维护证书文件路径。

## 2. 路由

- ` /certificates `
- ` /certificates?create=1 `：自动打开“创建证书”对话框；从 Domain/Proxy 表单跳转时还会携带 `returnTo`、`draftId`、`host`、`providerType` 参数

## 3. 页面目标

- 浏览证书资源清单及其多维状态
- 创建 ACME DNS-01、Cloudflare Origin CA、file-backed（登记已有证书文件路径）三类证书
- 执行签发、续期、轮换、同步、撤销和按风险分级的删除
- 管理 Cloudflare provider credential（创建、更新、校验、禁用、删除）
- 支持搜索、按多个状态维度筛选和分页
- 支持从 Domain/Proxy 表单跳转创建证书并在成功后自动返回原表单

## 4. 页面结构

- 页面标题区（含“创建证书”和“刷新”）
- Cloudflare 凭据管理区
- 工具栏（搜索 + 多维状态筛选）
- 证书列表区
- 分页区
- 创建证书对话框
- 强确认删除对话框（仅高风险删除时弹出）
- 提供方运行条件区（ACME DNS-01 与 Cloudflare Origin CA 分别展示）

## 5. 证书作为独立资源

证书是可独立管理的资源；权威绑定在 Domain，不再附着在 Web Proxy 上。

- 证书可以先创建为未绑定状态，随后从 Domain 详情绑定。
- Domain 通过 `certificateId` 显式绑定证书（数据层为 `domains.certificate_id`）。
- 证书与 Domain 为 **1:n**：同一证书可绑定多个 Domain（hostnames 覆盖时，例如通配证书）；每个 Domain 最多一张证书。
- Domain 解绑或改绑后，证书资源保留，除非在本页显式删除。
- 运行时按 Domain 绑定的 certificate ID 解析 TLS active material，并校验证书覆盖该 Domain Host。
- 私钥材料始终只以文件路径或受管文件形式存在，绝不进入 SQLite，UI 也不回显私钥内容。

历史 Proxy 绑定迁移与私钥不入库策略见 [../../references/certificate-binding-migration.md](../../references/certificate-binding-migration.md)；Domain 模型见 [../../changes/completed/domain-path-proxy-routing.md](../../changes/completed/domain-path-proxy-routing.md)。

## 6. 创建证书

“创建证书”对话框按证书来源动态展示字段：

- **ACME DNS-01（自动签发）**：填写主机名后启动自动签发流程。
- **Cloudflare Origin CA（源站 TLS）**：选择 Origin CA 凭据、请求类型（`origin-ecc` / `origin-rsa`）、有效期（天），并展示 Origin CA 部署提示（见第 11 节）。
- **已有文件路径登记（file-backed）**：登记服务器上已存在的证书文件路径和私钥文件路径，只录入路径，不接受粘贴私钥文本。

当从 HTTPS proxy 表单点击“创建证书”跳转过来时，对话框会用 `host` / `providerType` 参数预填，并提示“创建成功后将自动返回代理表单并选中该证书”。创建成功后携带新证书 ID 返回原 proxy 表单（见第 9 节）。

ACME 的运行条件显示服务端启用状态、账户邮箱、条款和 DNS Token 环境变量名称的脱敏检查结果。未就绪时仍保留 ACME 选项，但禁用创建并说明需修改服务端配置、注入环境变量后重启；Origin CA 的 credential 状态独立展示，不能用任一方状态推断另一方可用。未就绪诊断可复制，但只包含 provider、错误缺失项和修复提示。

## 7. 状态维度

证书状态按三个独立维度展示，避免合并成一个含糊状态：

- **Serving（serving status）**：证书当前能否为 HTTPS 提供服务，取值如 usable / expiring_soon / expired / missing / invalid。后端单一状态筛选器覆盖该维度。
- **Operation（operation status）**：生命周期操作状态，取值如 idle / pending / issue_failed / renewal_failed。
- **Provider（provider status）**：provider 远端状态，取值如 active / revoked / missing_remote / unknown。

工具栏的筛选项明确区分维度：搜索框、Status（serving）、Operation status、Provider status、Provider type（acme_dns01 / cloudflare_origin_ca / file）、Origin credential。同一选项不会在多个维度间产生不可解释的匹配。

## 8. 列表字段

- Certificate（证书 ID + host）
- Provider（provider 类型、名称、credential ID、Cloudflare certificate ID、Origin CA 部署提示首条）
- Hostnames
- Bound proxy（绑定的 proxy ID，未绑定时显示“未绑定”）
- Use（是否被引用）
- Serving / Operation / Provider status（三列分维度状态）
- Expires（到期时间）
- Last sync（最近同步时间）
- Failures（失败次数）
- Fingerprint（指纹）
- Last error（脱敏错误）
- Actions（按 provider 与状态收敛的动作）

## 9. 生命周期动作与可用性收敛

动作按钮必须按 provider、状态、引用关系和 active material 可用性收敛；不适用的动作被禁用或隐藏，并通过 tooltip 给出可理解原因。

- **ACME DNS-01 证书**：支持 Issue、Renew、Issue Origin（无可用 Origin CA 凭据时禁用并提示）。
- **Cloudflare Origin CA 证书**：支持 Rotate、Sync、Revoke（缺少 Cloudflare 证书 ID 时无法吊销）。
- **File-backed 证书**：不支持签发/续期/轮换/同步/吊销，仅可删除；动作区显示“No lifecycle actions”并说明原因。
- **删除**：所有证书均可删除，按风险分级处理（见第 10 节）。

证书生命周期动作的契约保持 secret-safe：只返回生命周期结果、证书元数据和脱敏错误，绝不暴露私钥、Cloudflare API token 或其他 secret material。

## 10. 删除的风险分级

删除前按引用状态和可服务状态计算删除风险：

- **可直接删除（无二次确认）**：无效、失效/过期、缺失 active material、provider status 阻止服务，或未被任何 proxy 绑定的证书。这类删除通过普通删除按钮完成，不要求输入 host 或证书 ID，也不要求额外二次确认。若该证书仍有绑定，删除后会解除绑定并把受影响 proxy 标记为需要重新配置。
- **需要强确认**：仍被 HTTPS proxy 绑定且当前可服务的证书（`deletionRisk === requires_strong_confirmation`）。删除会弹出强确认对话框，要求管理员手动键入证书主机名或证书 ID 才能删除。UI 不得仅通过预填隐藏字段或单击确认满足强确认语义。
- 强确认删除成功后，系统解除该 proxy 的证书绑定，把受影响 proxy 标记为证书失效/需要重新配置，并在结果中返回受影响的 proxy ID 列表供页面提示。
- **防御回退**：若原以为是低风险删除，但后端返回 `CONFIRMATION_REQUIRED`，页面自动切换到强确认对话框并提示键入主机名或证书 ID。

Origin CA 撤销（Revoke）同样属于高风险动作，要求输入 host 或 Cloudflare certificate ID 等强确认材料，撤销后 Cloudflare 到源站 TLS 将中断，直到签发替换证书。

## 11. Origin CA 部署提示

查看或选择 Cloudflare Origin CA 证书时，必须展示其部署边界提示：

- Origin CA 证书仅用于 Cloudflare 到源站之间的 TLS，浏览器不会直接信任。
- 需要在 Cloudflare 侧使用 Full (strict) 等合适的 SSL 模式，否则源站连接可能不被校验。
- 直连公网浏览器不应使用 Origin CA 证书。

该提示在创建对话框（选择 Origin CA 来源时）、证书列表 Provider 列以及 proxy 表单的证书摘要中均会出现。

## 12. Cloudflare 凭据管理

凭据管理区用于维护 Cloudflare provider credential：

- 通过 Name、Scope、API token 表单创建或更新凭据。
- 列表展示 Name、Status、Scope、Fingerprint、Verified（最近校验时间）、Last error。
- 每条凭据支持 Edit、Verify、Disable、Delete。
- 凭据响应只包含 metadata、token 指纹、状态、最近校验时间和脱敏错误，绝不包含 API token 明文。

## 13. 状态设计

### 13.1 初始加载
- 表格骨架屏

### 13.2 空数据态
- 暂无证书

### 13.3 无结果态
- 当前筛选条件无匹配证书，提供清除筛选入口

### 13.4 操作失败
- 显示签发、续期、轮换、同步、撤销、删除或凭据操作的失败原因

### 13.5 列表错误态
- 显示证书列表加载失败

## 14. 安全要求

- UI 不展示任何私钥材料，file-backed 证书只录入文件路径。
- 错误提示和动作结果均为脱敏内容，不泄露私钥、Cloudflare API token 或敏感文件内容。

## 15. 验收标准

- 证书页是证书增删和所有生命周期动作的唯一入口
- 能创建 ACME DNS-01、Origin CA、file-backed 三类证书
- serving / operation / provider 三维状态分别展示且筛选维度明确
- 动作按 provider、状态、引用关系和 active material 可用性收敛
- 低风险删除无二次确认，高风险删除与 Origin CA 撤销要求输入式强确认
- 查看/选择 Origin CA 证书时展示部署提示
- 不暴露私钥或敏感凭据
