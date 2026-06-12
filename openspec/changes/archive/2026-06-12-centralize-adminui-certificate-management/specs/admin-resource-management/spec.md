## MODIFIED Requirements

### Requirement: Managed certificate admin baseline
系统 MUST 在 Admin API/UI 中为 HTTPS 证书资源提供统一的管理员状态、创建、删除和生命周期动作；Certificates 页 MUST 是管理员浏览器中执行证书增删和生命周期动作的唯一主入口。

#### Scenario: View certificate inventory
- **WHEN** 已认证管理员查看 Certificates 页或查询证书列表
- **THEN** 系统返回 HTTPS 证书资源清单，包含证书 ID、host、hostnames、provider、绑定 proxy、serving status、operation status、provider status、有效期、最近同步/检查时间、失败次数、指纹和脱敏错误

#### Scenario: Create or import certificate from Certificates page
- **WHEN** 已认证管理员从 Certificates 页创建 ACME DNS-01、Cloudflare Origin CA 或受支持的 file-backed 证书资源
- **THEN** 系统在证书管理上下文中创建证书资源或启动签发流程，并记录控制面操作
- **AND** HTTPS proxy 表单 MUST NOT 成为证书创建或证书文件路径维护的主入口

#### Scenario: Delete certificate from Certificates page
- **WHEN** 已认证管理员从 Certificates 页请求删除证书资源
- **THEN** 系统按证书引用状态和可服务状态计算删除风险，删除证书元数据和可安全清理的受管材料，并记录控制面操作
- **AND** 无效、失效/过期或未被使用的 certificate 删除 MUST NOT 要求二次确认或输入式强确认
- **AND** 如果删除会影响仍在使用且可服务的 HTTPS 证书，系统 MUST 要求强确认并在结果中返回受影响 proxy

#### Scenario: Run provider lifecycle actions from Certificates page
- **WHEN** 已认证管理员触发证书签发、续期、Cloudflare Origin CA 轮换、同步或撤销
- **THEN** 系统执行受支持的 provider-specific 生命周期动作，并记录控制面操作
- **AND** AdminUI MUST 按证书 provider、状态、引用关系和 active material 可用性控制动作可用性

#### Scenario: Destructive certificate actions require strong confirmation
- **WHEN** 已认证管理员请求撤销 Cloudflare Origin CA 证书、删除仍被 HTTPS proxy 使用且可服务的证书资源，或执行其他会移除当前可服务 active material 的破坏性动作
- **THEN** AdminUI MUST 要求管理员输入 host、certificate ID、Cloudflare certificate ID 或等价强确认材料
- **AND** UI MUST NOT 仅通过预填隐藏字段或单击确认来满足后端强确认语义

#### Scenario: Low-risk certificate delete does not require secondary confirmation
- **WHEN** 已认证管理员删除无效、失效/过期或未被使用的 certificate
- **THEN** AdminUI MUST 允许该删除通过普通删除动作完成
- **AND** UI MUST NOT 要求额外二次确认、输入证书 ID、输入 host 或输入 Cloudflare certificate ID

#### Scenario: Certificate lifecycle actions do not expose secret material
- **WHEN** 已认证管理员通过 `/api/admin/graphql` 触发证书创建、删除、签发、续期、轮换、同步或撤销
- **THEN** 变更合同返回生命周期结果、证书元数据和脱敏错误，且 MUST NOT 暴露私钥、Cloudflare API token 或其他 secret material

### Requirement: Administrator proxy entry configuration
系统 MUST 在管理员 API 和专用前端中支持为当前反向代理类型配置入口监听地址、入口端口和适用的路由域名；HTTPS proxy 表单 MUST 只选择证书资源或跳转创建证书，并提供可操作的错误反馈。

#### Scenario: HTTP create includes listener and domain
- **WHEN** 已认证管理员创建 HTTP 代理
- **THEN** 管理面允许提交监听地址、入口端口、HTTP Host 域名、本地目标主机和本地目标端口

#### Scenario: HTTPS create includes listener, SNI domain, and certificate selection
- **WHEN** 已认证管理员创建 HTTPS 代理
- **THEN** 管理面允许提交监听地址、入口端口、SNI 域名、本地目标主机、本地目标端口以及选中的证书 ID
- **AND** 专用前端 MUST NOT 在 HTTPS proxy 创建主流程中要求管理员填写证书文件和私钥文件路径

#### Scenario: HTTPS create links to certificate creation
- **WHEN** 已认证管理员在 HTTPS proxy 创建表单中没有可选择的合适证书或需要新建证书
- **THEN** 专用前端提供跳转到 Certificates 页证书创建流程的入口，并携带返回目标和当前 proxy 表单草稿引用

#### Scenario: TCP and UDP create include listener host
- **WHEN** 已认证管理员创建 TCP 或 UDP 代理
- **THEN** 管理面允许提交监听地址、入口端口、本地目标主机和本地目标端口

#### Scenario: Proxy edit validates type-specific entry fields
- **WHEN** 已认证管理员更新现有代理
- **THEN** 系统按该代理类型校验必填入口字段，并拒绝会导致该代理无法路由、无法监听或无法使用所选证书终止 TLS 的配置

#### Scenario: Listener host options are provided
- **WHEN** 专用前端渲染代理创建或编辑表单
- **THEN** 它从管理员 API 获取可选择的监听地址选项，并使用选择器提交所选监听地址

#### Scenario: Proxy lifecycle errors remain visible
- **WHEN** 创建、更新、启用、禁用或删除代理返回入口冲突、监听启动失败、证书绑定校验失败、校验失败或资源不存在错误
- **THEN** 专用前端在当前代理列表、详情或表单动作 surface 中展示该错误，并保留当前已加载内容

### Requirement: Certificate management query responses remain secret-safe
系统 MUST 在优化管理员证书查询、credential 加载路径和证书绑定路径后继续保持证书与 provider credential 响应 secret-safe。

#### Scenario: Credential summaries omit token material
- **WHEN** 管理员查询 provider credential 列表、详情或证书动作结果
- **THEN** 响应只包含 credential metadata、token 指纹、状态、最近校验时间和脱敏错误
- **AND** 响应 MUST NOT 包含 Cloudflare API Token 明文、secret store 文件内容或私钥材料

#### Scenario: Certificate summaries keep lifecycle fields compatible
- **WHEN** 管理员查询证书列表、代理详情或生命周期动作结果
- **THEN** 响应继续包含 provider type、credential ID、provider status、Cloudflare certificate ID、hostnames、request type、requested validity、last synced time、serving status、operation status、failure count、next attempt time、绑定 proxy 引用和脱敏错误
- **AND** 查询路径优化 MUST NOT 删除或重命名现有管理端可见字段，除非提供兼容字段或迁移说明

#### Scenario: Proxy summaries expose selected certificate metadata
- **WHEN** 管理员查询 HTTPS proxy 列表、详情或 create/update 结果
- **THEN** 响应包含所选证书 ID 和足以渲染证书摘要的 secret-safe 元数据
- **AND** 响应 MUST NOT 暴露私钥材料或 provider token

## ADDED Requirements

### Requirement: AdminUI certificate workflow preserves proxy draft state
系统 MUST 在从 HTTPS proxy 表单跳转到证书创建流程时保留管理员已填写的 proxy 表单状态，并在证书创建后恢复该状态。

#### Scenario: Certificate creation returns to proxy create form
- **WHEN** 管理员在 HTTPS proxy 创建表单中已填写用户、客户端、名称、监听、SNI 域名和目标配置，并点击创建证书入口
- **THEN** 专用前端保存不含 secret material 的 proxy 表单草稿，并导航到证书创建流程

#### Scenario: Created certificate is selected after return
- **WHEN** 证书创建或签发流程成功，并且请求携带有效返回目标和草稿引用
- **THEN** 专用前端返回原 proxy 表单，恢复已填写字段，并自动选择新创建的证书

#### Scenario: Missing draft degrades safely
- **WHEN** 证书创建成功后返回 proxy 表单但草稿不存在、过期或无法解析
- **THEN** 专用前端显示可理解的恢复失败提示，并允许管理员手动选择刚创建的证书继续配置

### Requirement: Certificates page exposes clear status and action model
系统 MUST 在 Certificates 页把证书健康状态、生命周期操作状态和 provider 远端状态作为不同维度展示，并据此收敛可用动作。

#### Scenario: Status dimensions are separated
- **WHEN** 管理员查看证书列表
- **THEN** UI 分别展示 serving status、operation status 和 provider status，而不是把不同维度合并成一个含糊状态

#### Scenario: Filters identify status dimension
- **WHEN** 管理员使用证书状态筛选
- **THEN** UI 明确筛选的是 serving、operation、provider 或综合视图，并避免让同一个选项在多个状态维度中产生不可解释的匹配

#### Scenario: Inapplicable actions are unavailable
- **WHEN** 某个证书的 provider、状态、引用关系或 active material 不支持某个动作
- **THEN** UI 禁用或隐藏该动作，并在需要时提供可理解原因

#### Scenario: Origin CA deployment hints are visible
- **WHEN** 管理员查看或选择 Cloudflare Origin CA 证书
- **THEN** UI 展示该证书只适用于 Cloudflare 到源站 TLS、需要合适 SSL 模式、直连浏览器不信任 Origin CA 的部署提示
