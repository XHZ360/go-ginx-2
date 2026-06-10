## ADDED Requirements

### Requirement: Cloudflare Origin CA provider credential management
系统 MUST 支持管理员通过 Admin UI 维护 Cloudflare Origin CA API Token credential，并且 MUST 把 token 明文保存在 SQLite 之外。

#### Scenario: Admin creates Cloudflare Origin CA credential
- **WHEN** 已认证管理员在 Admin UI 中创建 Cloudflare Origin CA credential 并提交 API Token
- **THEN** 系统将 token material 写入 SQLite 之外的 secret store，在 SQLite 中仅保存 credential metadata、token 指纹和 secret 引用，并且 MUST NOT 在响应中回显 token 明文

#### Scenario: Admin updates Cloudflare Origin CA credential
- **WHEN** 已认证管理员在 Admin UI 中更新 Cloudflare Origin CA credential 的 API Token
- **THEN** 系统替换 secret store 中的 token material，更新 token 指纹和验证状态，记录审计事件，并且 MUST NOT 把旧 token 或新 token 明文写入 SQLite 或普通日志

#### Scenario: Admin verifies Cloudflare Origin CA credential
- **WHEN** 已认证管理员请求验证 Cloudflare Origin CA credential
- **THEN** 系统使用 secret store 中的 token 调用 Cloudflare API 验证权限，记录 `last_verified_at`、credential status 和脱敏错误

#### Scenario: Admin query returns credential metadata only
- **WHEN** 已认证管理员查看 Cloudflare Origin CA credential 列表或详情
- **THEN** 响应只包含 credential ID、名称、状态、作用域、token 指纹、最近验证时间和脱敏错误，且 MUST NOT 包含 token 明文

#### Scenario: Missing Cloudflare Origin CA token blocks operation
- **WHEN** 管理员请求 Origin CA 签发、轮换、同步或撤销，但所选 credential 的 secret material 缺失、为空、禁用或验证失败
- **THEN** 操作以 provider 凭据配置错误失败，记录脱敏错误，并且 MUST NOT 修改当前 active certificate material

#### Scenario: Origin CA Service Key is not accepted
- **WHEN** 管理员在 Admin UI 中提交 Cloudflare Origin CA Service Key 或旧式 service key 凭据
- **THEN** 系统 MUST 拒绝把该凭据用于新 Origin CA 生命周期操作，并提示使用 Cloudflare API Token

### Requirement: Cloudflare Origin CA certificate issuance
系统 SHALL 为 HTTPS proxy 主机通过 Cloudflare Origin CA API 签发 origin 证书，同时保持私钥只在本地受管证书目录中生成和保存。

#### Scenario: Successful Origin CA issuance writes managed certificate files
- **WHEN** 已启用的 HTTPS proxy 请求 Cloudflare Origin CA 签发，proxy 主机非空，引用的 Cloudflare credential 有效，且 Cloudflare API 返回覆盖该主机的证书
- **THEN** 系统本地生成私钥和 CSR，调用 Cloudflare Origin CA create 操作，校验证书/私钥对，写入 active certificate material，并记录托管证书元数据

#### Scenario: Origin CA private key remains local
- **WHEN** 系统为 Origin CA 签发生成私钥和 CSR
- **THEN** 系统 MUST 只向 Cloudflare 发送 CSR 和非敏感请求元数据，且 MUST NOT 向 Cloudflare、SQLite、管理 API 或普通日志发送或保存私钥字节

#### Scenario: Issued certificate must cover proxy host
- **WHEN** Cloudflare Origin CA API 返回的证书不覆盖 HTTPS proxy 主机、证书已过期、证书/key 不匹配或证书无法解析
- **THEN** 系统 MUST 拒绝激活该证书，记录签发失败操作状态，并保持当前 active certificate material 不变

#### Scenario: Origin CA metadata is persisted without secrets
- **WHEN** Origin CA 签发成功
- **THEN** SQLite 保存 provider 类型、provider 名称、credential ID、Cloudflare certificate ID、hostnames、request type、requested validity、过期时间、fingerprint 和文件路径，且 MUST NOT 保存私钥字节或 Cloudflare API Token 值

### Requirement: Cloudflare Origin CA rotation
系统 MUST 支持对 Cloudflare Origin CA 托管证书执行显式轮换和基于过期时间的调度轮换候选选择。

#### Scenario: Origin CA certificate enters rotation window
- **WHEN** Origin CA 托管证书的 active certificate material 仍可服务，且 `not_after` 进入配置的 Origin CA rotation window
- **THEN** daemon 证书 controller 将该证书纳入轮换候选，并遵守失败退避和同一 proxy/host 单飞约束

#### Scenario: Successful Origin CA rotation activates replacement
- **WHEN** Origin CA 轮换成功签发替换证书，且替换证书/私钥对通过代理主机健康检查
- **THEN** 系统激活新的 active certificate material，保留上一组 previous material，更新 Cloudflare certificate ID、过期时间、fingerprint、最近轮换时间和调度状态，并热加载后续新 TLS 握手

#### Scenario: Failed Origin CA rotation preserves active certificate
- **WHEN** Origin CA 轮换因 API、CSR、证书校验、文件写入或 metadata 更新失败
- **THEN** 系统 MUST NOT 替换当前 active certificate material，并记录轮换失败、失败次数、下一次尝试时间和脱敏错误

#### Scenario: Rotation does not auto-revoke previous certificate
- **WHEN** Origin CA 轮换成功并产生 previous certificate material
- **THEN** 系统默认 MUST NOT 自动撤销 previous Cloudflare Origin CA 证书，而是保留元数据供管理员检查和显式撤销

### Requirement: Cloudflare Origin CA provider synchronization
系统 SHALL 支持同步 Cloudflare Origin CA 远端证书状态，并将已确认的 provider-side 不可用状态纳入管理端状态和 HTTPS proxy 可服务性判断。

#### Scenario: Sync records active remote certificate state
- **WHEN** 管理员或 daemon 对 Origin CA 托管证书执行 provider sync，且 Cloudflare API 返回 active certificate 对应的远端证书记录
- **THEN** 系统记录 provider 状态、最近同步时间、远端过期时间和脱敏响应摘要，并保持 active material 健康状态由本地证书检查决定

#### Scenario: Sync detects revoked active certificate
- **WHEN** provider sync 确认 active certificate 对应的 Cloudflare Origin CA 证书已撤销
- **THEN** 系统把 provider 状态标记为 revoked 或等价不可用状态，将该 HTTPS proxy 标记为需要证书配置，并且 MUST NOT 声明该证书可用于 Cloudflare Origin CA 服务

#### Scenario: Sync failure preserves serving material
- **WHEN** provider sync 因网络、权限或 Cloudflare API 错误失败，但当前 active certificate material 仍通过本地健康检查且未确认被撤销
- **THEN** 系统记录同步失败和 provider 状态未知，同时继续按照本地 active material 健康结果决定是否服务

### Requirement: Cloudflare Origin CA revocation safety
系统 MUST 把 Cloudflare Origin CA 证书撤销作为显式高风险管理动作处理，并要求强确认。

#### Scenario: Revoke requires explicit certificate identity confirmation
- **WHEN** 管理员请求撤销 Cloudflare Origin CA 证书
- **THEN** 请求 MUST 包含匹配的 proxy ID、host 和 Cloudflare certificate ID 或等价强确认字段，否则系统拒绝撤销

#### Scenario: Revoking previous certificate does not affect active material
- **WHEN** 管理员撤销的 Cloudflare certificate ID 对应 previous certificate material
- **THEN** 系统调用 Cloudflare revoke，记录撤销结果和审计事件，并且 MUST NOT 修改当前 active certificate material

#### Scenario: Revoking active certificate invalidates HTTPS proxy
- **WHEN** 管理员强确认撤销当前 active certificate 对应的 Cloudflare certificate ID 且 Cloudflare revoke 成功
- **THEN** 系统 MUST 更新 provider 状态，将该 HTTPS proxy 标记为需要证书配置，并拒绝把该证书继续声明为可服务 Cloudflare Origin CA 证书

### Requirement: Cloudflare Origin CA deployment guardrails
系统 MUST 在管理端和文档中明确 Cloudflare Origin CA 证书的适用边界和必要部署前提。

#### Scenario: Origin CA status shows Cloudflare deployment hints
- **WHEN** 管理员查看 Origin CA 托管证书状态
- **THEN** 管理端展示该证书适用于 Cloudflare 到 origin 的 TLS 连接，并提示 DNS record 应为 proxied 且 SSL mode 应为 Full (strict) 或等价严格校验

#### Scenario: Direct browser trust is not claimed
- **WHEN** 系统展示或记录 Origin CA 托管证书能力
- **THEN** 系统 MUST NOT 声明该证书适合公网浏览器直连 origin 的普通 WebPKI 信任场景

#### Scenario: Cloudflare environment checks are non-mutating
- **WHEN** 系统检查 Cloudflare DNS proxied 状态或 SSL mode
- **THEN** 第一阶段实现 MUST 只读展示检查结果或警告，且 MUST NOT 自动修改 Cloudflare DNS 记录、proxied 状态或 SSL mode
