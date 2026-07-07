## ADDED Requirements

### Requirement: Client kind management
系统 MUST 为客户端资源记录 provider/consumer kind。未显式指定 kind 的既有和新建客户端 MUST 按 provider 处理，以保持现有 goginx-client 和代理管理行为兼容。

#### Scenario: Default client kind is provider
- **WHEN** 管理员或迁移路径创建或读取未显式指定 kind 的客户端
- **THEN** 系统把该客户端视为 provider client

#### Scenario: Persist provider client kind
- **WHEN** 管理员创建 provider client
- **THEN** 系统持久化该客户端的 provider kind
- **AND** 该客户端可以继续作为 proxy 所属 provider 使用

#### Scenario: Persist consumer client kind
- **WHEN** 管理员创建 consumer client
- **THEN** 系统持久化该客户端的 consumer kind
- **AND** 该客户端可用于 SDK consumer 控制通道认证

#### Scenario: Reject invalid client kind
- **WHEN** 管理员或内部服务尝试创建或更新包含未知 kind 的客户端
- **THEN** 系统 MUST 拒绝该变更并返回可操作的校验错误

### Requirement: Admin CLI consumer client creation
admin CLI MUST 支持创建 consumer client 凭据，用于 SDK 连接控制通道。未使用 consumer 标志的现有 `create-client` 流程 MUST 继续创建 provider client。

#### Scenario: Create provider client from CLI by default
- **WHEN** 操作者运行 admin CLI `create-client` 且未指定 consumer 标志
- **THEN** CLI 创建 provider client 凭据

#### Scenario: Create consumer client from CLI
- **WHEN** 操作者运行 admin CLI `create-client` 并指定 consumer 标志
- **THEN** CLI 创建 consumer client 凭据
- **AND** 该凭据可被 SDK 用于 consumer 控制通道认证

#### Scenario: Consumer client creation keeps secret display policy
- **WHEN** admin CLI 成功创建 consumer client 并生成或保存 credential
- **THEN** CLI 的 credential 明文展示策略 MUST 与现有 client credential 创建策略一致
- **AND** 普通日志和审计事件 MUST NOT 明文记录可重放 credential

### Requirement: Consumer client management surface boundary
首批 consumer client 管理入口 SHALL 由 admin CLI 提供。GraphQL 管理 API、Admin UI 和本地 TUI 可以继续按 provider 默认语义创建客户端，直到它们显式实现 consumer kind 选择。

#### Scenario: Existing browser client creation remains provider
- **WHEN** 管理员通过尚未实现 consumer kind 选择的 GraphQL、Admin UI 或 TUI 创建客户端
- **THEN** 系统创建 provider client

#### Scenario: Future UI support requires explicit kind
- **WHEN** 未来 GraphQL、Admin UI 或 TUI 暴露 consumer client 创建入口
- **THEN** 该入口 MUST 明确展示并提交 client kind
- **AND** 它 MUST 继续遵守客户端 credential 一次性展示和 secret-safe 响应策略
