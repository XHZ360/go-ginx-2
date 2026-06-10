## ADDED Requirements

### Requirement: Certificate management uses scoped certificate loading
系统 MUST 在管理员证书查询和生命周期动作中使用 scoped certificate loading，避免通过全量证书列表或 per-proxy 重复查询获取单个目标资源。

#### Scenario: Certificate list avoids per-proxy certificate lookups
- **WHEN** 已认证管理员查询证书列表
- **THEN** 管理查询可以使用批量证书加载或 store 层过滤加载证书摘要
- **AND** 系统 MUST NOT 为列表中的每个 HTTPS proxy 单独调用 proxy ID 证书查询

#### Scenario: Certificate detail loads target resources
- **WHEN** 已认证管理员查看单个代理或单个证书详情
- **THEN** 管理查询只加载目标 proxy、目标 managed certificate 和必要的运行时摘要
- **AND** 系统 MUST NOT 通过读取完整证书列表再在内存中过滤来获得该目标证书

#### Scenario: Certificate action reuses target certificate
- **WHEN** 已认证管理员触发托管证书签发、续期、轮换、同步或撤销动作
- **THEN** 管理 command 层加载目标证书后把该记录交给 certmanager 生命周期路径
- **AND** 同一动作链路 MUST NOT 在 provider 选择前重复按 proxy ID 读取目标证书

### Requirement: Provider credential resolution uses provider-scoped loading
系统 MUST 通过 provider-scoped credential loading 解析 Cloudflare Origin CA credential，避免读取全部 provider credential 后在内存中过滤目标凭据。

#### Scenario: Explicit credential loads by ID
- **WHEN** 管理员为 Cloudflare Origin CA 签发或轮换动作提供 credential ID
- **THEN** 系统通过 credential ID 定点读取该 credential
- **AND** 系统验证该 credential 属于 Cloudflare Origin CA 且状态允许使用

#### Scenario: Default credential uses provider and status filter
- **WHEN** 管理员未显式提供 Cloudflare Origin CA credential ID
- **THEN** 系统通过 provider type 和可用状态查询默认 credential 候选
- **AND** 系统 MUST NOT 通过读取全部 provider credential 后在内存中过滤默认候选

#### Scenario: Multiple default credential candidates require explicit selection
- **WHEN** provider-scoped credential 查询返回多个可用 Cloudflare Origin CA credential
- **THEN** 系统拒绝隐式选择并要求管理员显式提供 credential ID
- **AND** 错误响应不得暴露 token 明文或 secret store 路径内容

### Requirement: Certificate management query responses remain secret-safe
系统 MUST 在优化管理员证书查询和 credential 加载路径后继续保持证书与 provider credential 响应 secret-safe。

#### Scenario: Credential summaries omit token material
- **WHEN** 管理员查询 provider credential 列表、详情或证书动作结果
- **THEN** 响应只包含 credential metadata、token 指纹、状态、最近校验时间和脱敏错误
- **AND** 响应 MUST NOT 包含 Cloudflare API Token 明文、secret store 文件内容或私钥材料

#### Scenario: Certificate summaries keep lifecycle fields compatible
- **WHEN** 管理员查询证书列表、代理详情或生命周期动作结果
- **THEN** 响应继续包含 provider type、credential ID、provider status、Cloudflare certificate ID、hostnames、request type、requested validity、last synced time、serving status、operation status、failure count、next attempt time 和脱敏错误
- **AND** 查询路径优化 MUST NOT 删除或重命名现有管理端可见字段
