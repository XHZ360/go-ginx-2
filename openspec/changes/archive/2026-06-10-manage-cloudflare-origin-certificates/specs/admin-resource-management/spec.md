## MODIFIED Requirements

### Requirement: Managed certificate admin baseline
系统 MUST 在首批 API/UI 中为托管 HTTPS 证书提供管理员证书状态、provider 选择和受支持 provider 的生命周期动作。

#### Scenario: Maintain Cloudflare Origin CA credential
- **WHEN** 已认证管理员在 Admin UI 中创建、更新、验证、禁用或删除 Cloudflare Origin CA credential
- **THEN** 系统执行对应 credential 管理动作，返回 metadata-only 结果，并记录控制面审计事件

#### Scenario: Credential token is write-only in admin surface
- **WHEN** 已认证管理员查看 Cloudflare Origin CA credential 列表、详情、审计事件或 GraphQL 响应
- **THEN** 系统 MUST NOT 返回 Cloudflare API Token 明文，只能返回 token 指纹、状态、作用域、最近验证时间和脱敏错误

#### Scenario: View managed certificate status
- **WHEN** 已认证管理员查看 V1 中某个 HTTPS 代理的托管证书状态
- **THEN** 系统返回当前证书管理行为已支持的托管证书状态 surface，包括 provider 类型、credential ID、服务状态、操作状态和脱敏错误

#### Scenario: Issue or renew managed certificate
- **WHEN** 已认证管理员触发 V1 托管证书签发、续期或 provider-specific 轮换
- **THEN** 系统执行受支持的托管证书生命周期动作，并记录控制面操作

#### Scenario: Issue Cloudflare Origin CA certificate
- **WHEN** 已认证管理员为 HTTPS proxy 选择 Cloudflare Origin CA provider 并触发签发
- **THEN** 系统执行 Origin CA 签发流程，返回 provider 元数据、证书健康状态和操作结果，且记录控制面审计事件

#### Scenario: Sync or revoke Cloudflare Origin CA certificate
- **WHEN** 已认证管理员触发 Origin CA provider sync 或带强确认的 revoke 动作
- **THEN** 系统执行对应 provider 操作，返回脱敏结果，并记录控制面审计事件

#### Scenario: Certificate lifecycle actions do not expose secret material
- **WHEN** 已认证管理员通过 `/api/admin/graphql` 触发证书签发、续期、轮换、同步或撤销
- **THEN** 变更合同返回运行生命周期结果，且不暴露私钥、Cloudflare API Token 或其他私钥材料
