## Purpose

定义管理员资源管理契约，覆盖里程碑一非交互式 CLI 资源种子写入、首次管理员初始化、SQLite 管理员认证、同源会话与管理 API、面向专用前端的 GraphQL 合同、默认内置同源前端交付，以及 dashboard、用户、客户端、代理、托管证书和近期审计视图；同时显式跟踪配额、设置、告警、更完整可观测性、RBAC、正向代理和普通用户自助等剩余管理缺口。
## Requirements
### Requirement: Admin CLI user seeding baseline
系统 MUST 支持通过 admin CLI 非交互式创建里程碑一用户记录。

#### Scenario: Create user record
- **WHEN** 操作者使用数据库路径、用户 ID 和用户名运行 admin CLI `create-user` 流程
- **THEN** CLI 把用户资源持久化到 SQLite

### Requirement: Admin CLI client credential seeding baseline
系统 MUST 支持通过 admin CLI 非交互式创建里程碑一客户端凭据。

#### Scenario: Create client credential record
- **WHEN** 操作者使用用户 ID、客户端 ID、显示名称和凭据运行 admin CLI `create-client` 流程
- **THEN** CLI 为该用户把客户端凭据资源持久化到 SQLite

### Requirement: Admin CLI proxy seeding baseline
系统 MUST 支持通过 admin CLI 非交互式创建当前已支持的反向代理记录。

#### Scenario: Create TCP proxy record
- **WHEN** 操作者使用用户、客户端、入口端口和本地目标设置运行 admin CLI `create-tcp-proxy` 流程
- **THEN** CLI 把 TCP 代理资源持久化到 SQLite

#### Scenario: Create UDP proxy record
- **WHEN** 操作者使用用户、客户端、入口端口和本地目标设置运行 admin CLI `create-udp-proxy` 流程
- **THEN** CLI 把 UDP 代理资源持久化到 SQLite

#### Scenario: Create HTTP proxy record
- **WHEN** 操作者使用用户、客户端、`Host` 和本地目标设置运行 admin CLI `create-http-proxy` 流程
- **THEN** CLI 把 HTTP 代理资源持久化到 SQLite

#### Scenario: Create HTTPS proxy record
- **WHEN** 操作者使用用户、客户端、HTTPS 主机、本地目标设置，以及可选证书/私钥文件运行 admin CLI `create-https-proxy` 流程
- **THEN** CLI 把 HTTPS 代理资源持久化到 SQLite，并保留透传或 TLS 终止所需配置

### Requirement: Admin CLI audit baseline
系统 MUST 在当前实现路径支持时，把成功的里程碑一 admin 创建操作记录为审计事件。

#### Scenario: Create operation audit event
- **WHEN** admin CLI 创建操作成功，并且当前实现为该操作记录审计事件
- **THEN** 审计事件携带已创建资源上下文并持久化

### Requirement: First-run administrator bootstrap
系统 SHALL 提供不依赖额外管理员凭据配置文件的首次管理员初始化路径，并且 MUST NOT 提供默认管理员密码。

#### Scenario: Initialize first administrator
- **WHEN** SQLite 中不存在可用于管理面登录的启用管理员用户，且操作者执行文档化的本地初始化流程并提供用户名和密码
- **THEN** 系统创建或更新一个启用的管理员用户，保存密码校验材料，并记录可审计的初始化结果

#### Scenario: Reject implicit default administrator
- **WHEN** 服务端首次以 configless 模式启动且操作者尚未初始化管理员
- **THEN** 管理面不接受任何默认用户名或默认密码登录

#### Scenario: Prevent accidental remote open setup
- **WHEN** 管理面没有可登录管理员
- **THEN** 系统不暴露无需认证即可远程设置管理员密码的通用浏览器写入口

### Requirement: Administrator authentication baseline
系统 MUST 使用与客户端凭据分离的管理员专用认证来保护管理前端和 API；默认管理员凭据来源 MUST 是 SQLite 中启用的管理员用户密码校验材料，而不是独立服务端凭据配置文件。

#### Scenario: Administrator credentials loaded from SQLite admin users
- **WHEN** 管理面启动
- **THEN** 管理员用户名、角色、状态和密码校验材料从 SQLite 用户存储加载，并且只有启用的管理员角色用户可用于管理面登录

#### Scenario: Ordinary users cannot authenticate as administrators
- **WHEN** SQLite 中存在启用的普通用户但该用户不具备管理员角色
- **THEN** 管理面 MUST 拒绝该用户登录管理员 console

#### Scenario: Administrator credentials remain separate from client credentials
- **WHEN** 管理面认证管理员
- **THEN** 管理员浏览器登录语义与运行时客户端凭据保持分离，不把机器客户端身份或客户端 credential 当作浏览器管理员身份

#### Scenario: Protected credentials file remains optional compatibility input
- **WHEN** 实现保留 `admin_credentials_file` 作为兼容路径
- **THEN** 该路径 MUST 被文档化为显式覆盖或迁移辅助，而不是 configless 基础部署的必需输入

#### Scenario: Management access requires protected transport
- **WHEN** 使用管理员凭据访问管理面
- **THEN** 管理端点预期运行在 TLS 保护之后；本地回环明文仅用于开发和自动化测试

### Requirement: Administrator session endpoint baseline
系统 MUST 为专用 admin console 暴露同源管理员会话端点，并使用 SQLite 管理员用户凭据作为默认登录校验来源。

#### Scenario: Login creates an administrator browser session
- **WHEN** 启用的 SQLite 管理员用户向 `/api/admin/login` 提交有效凭据
- **THEN** 系统校验管理员用户密码材料，创建服务端管理的浏览器会话，设置会话 Cookie，并返回前端 shell 所需的最小启动信息

#### Scenario: Login rejects missing administrator bootstrap
- **WHEN** 管理面尚未初始化任何可登录管理员用户
- **THEN** 登录端点拒绝认证，并返回可区分的认证失败或初始化缺失语义，且不泄露默认凭据

#### Scenario: Session bootstrap returns current auth context
- **WHEN** 专用前端携带有效浏览器会话调用 `/api/admin/session`
- **THEN** 系统返回路由守卫、shell 初始化和后续 CSRF 感知请求所需的最小管理员上下文

#### Scenario: Logout invalidates the administrator browser session
- **WHEN** 专用前端为当前浏览器会话调用 `/api/admin/logout`
- **THEN** 系统失效对应的服务端会话，并清除浏览器会话 Cookie

### Requirement: Administrator session lifecycle baseline
系统 MUST 执行专用 console 管理员会话生命周期规则。

#### Scenario: Session expiry rejects further access
- **WHEN** 管理员浏览器会话缺失、过期或无效
- **THEN** 会话启动端点和基于会话认证的 API 操作拒绝访问，且不暴露受保护的管理员管理资源

#### Scenario: Process restart invalidates in-memory sessions
- **WHEN** 服务端进程重启，而当前会话实现使用内存会话存储
- **THEN** 之前签发的管理员会话不再有效，管理员 MUST 重新认证

### Requirement: Browser mutation CSRF baseline
系统 MUST 保护基于会话认证的专用 console 变更请求，防止 CSRF。

#### Scenario: Session-authenticated mutation requires a valid CSRF token
- **WHEN** 专用 console 浏览器变更请求使用有效管理员会话
- **THEN** 系统要求除会话 Cookie 外还携带有效 CSRF 令牌，才允许变更继续

#### Scenario: Session-authenticated query access does not require CSRF
- **WHEN** 专用 console 浏览器请求执行只读操作
- **THEN** 只要管理员会话有效，系统可以不要求 CSRF 令牌

### Requirement: API namespace and legacy route baseline
系统 MUST 把管理员浏览器 API 保留在显式命名空间下，并停止服务旧的服务端渲染管理员 UI 和旧浏览器 GraphQL 路由。

#### Scenario: API paths remain distinct from frontend routes
- **WHEN** 同源 admin console 同时处理浏览器路由和 API 请求
- **THEN** 管理员 API 保留在 `/api/admin/*` 命名空间，使前端路由和 API 路由不产生歧义

#### Scenario: Legacy browser admin flow is not retained as fallback
- **WHEN** 基于会话认证的管理员 API surface 被引入
- **THEN** 系统不再保留服务端渲染管理员页面、浏览器表单处理器或重复 Basic Auth 提示作为回退路径

#### Scenario: Legacy browser-facing GraphQL route is removed
- **WHEN** 浏览器客户端访问管理员管理数据
- **THEN** 浏览器使用基于会话认证的 `/api/admin/graphql`，旧的浏览器 `POST /graphql` 管理入口不再服务

### Requirement: Same-origin frontend delivery baseline
系统 MUST 把专用管理员前端和管理员 API 以一个外部同源呈现给浏览器，并且 configless 基础部署 MUST 默认使用部署根目录中的 `admin-ui/` 前端构建目录，而不是要求配置 `admin_frontend_dir`、依赖进程工作目录或静默使用二进制内嵌前端资源。

#### Scenario: Default admin-ui directory is served
- **WHEN** 管理面启用、未配置 `admin_frontend_dir`，且服务端二进制所在部署根目录中的 `admin-ui/` 包含专用前端构建产物入口 `index.html`
- **THEN** admin listener 从该部署根目录的 `admin-ui/` 目录在同源上服务 `/`、`/login`、`/dashboard`、`/users`、`/clients`、`/proxies`、`/certificates`、`/audit` 和受支持深链，同时继续保留 `/api/admin/*` 作为后端 API 命名空间

#### Scenario: Configured frontend directory overrides default admin-ui directory
- **WHEN** `admin_frontend_dir` 显式指向包含 `index.html` 的专用前端构建目录
- **THEN** admin listener 使用该目录服务前端路由和资源，而不是使用默认 `admin-ui/` 目录

#### Scenario: Missing selected frontend fails clearly
- **WHEN** 管理面启用，且当前选定的前端目录缺失、不是目录或缺少 `index.html`
- **THEN** 系统启动失败或拒绝启用管理 listener，并返回明确的 admin frontend 目录错误，而不是继续服务旧的内嵌前端资源

#### Scenario: Embedded frontend assets are not the default fallback
- **WHEN** 未配置 `admin_frontend_dir` 且默认 `admin-ui/` 目录不可用，即使服务端二进制包含内嵌前端资源
- **THEN** 系统也不得静默回退到内嵌前端作为基础部署的浏览器管理面来源

#### Scenario: Missing asset-like paths return not found
- **WHEN** 浏览器请求当前选定前端目录中不存在的资源型路径，例如 `/assets/missing.js`
- **THEN** admin listener 返回 `404 Not Found`，而不是把缺失资源错误伪装成前端深链或回退到其他前端资源来源

### Requirement: Dedicated administrator frontend baseline
系统 MUST 把管理员管理 console 视为专用浏览器前端应用，而不是嵌在管理后端中的逐页服务端渲染 HTML。

#### Scenario: Frontend owns browser routing and presentation
- **WHEN** 已认证管理员使用专用管理 console
- **THEN** 浏览器路由渲染、页面组合、加载状态、空状态和表单交互行为由前端应用处理

#### Scenario: Root route redirects by auth state
- **WHEN** 浏览器请求专用管理员前端根路由 `/`
- **THEN** 前端通过会话启动端点解析管理员会话状态，并把已认证管理员导向 `/dashboard`，把未认证访问者导向 `/login`

#### Scenario: Protected routes bootstrap before rendering content
- **WHEN** 浏览器加载、刷新或直接打开 `/dashboard`、`/users/:id`、`/clients/:id` 或 `/proxies/:id` 等受保护路由
- **THEN** 前端在渲染受保护资源内容前，先通过专用会话启动端点校验当前管理员会话

### Requirement: Frontend route shell baseline
系统 MUST 把专用管理员前端定义为一个同源路由 shell，使用会话端点处理守卫，使用 GraphQL 处理业务数据。

#### Scenario: Shell separates public and protected route groups
- **WHEN** 专用管理员前端路由模型被实现
- **THEN** 它包含一个未认证的 `/login` 路由，以及一个已认证应用 shell，覆盖 `/dashboard`、`/users`、`/users/:id`、`/clients`、`/clients/:id`、`/proxies`、`/proxies/:id`、`/certificates` 和 `/audit`

#### Scenario: Navigation exposes confirmed pages only
- **WHEN** 已认证管理员使用专用前端 shell
- **THEN** 主导航只暴露 Dashboard、Users、Clients、Proxies、Certificates 和 Audit，不暴露配额、设置、告警、更完整可观测性、域名工作流、RBAC 重设计或普通用户自助入口

#### Scenario: Intended destination restores after login
- **WHEN** 未认证访问者从有效受保护路由被重定向到 `/login`
- **THEN** 成功登录后，前端恢复该受保护目标；缺失、不安全、不支持或无效目标回退到 `/dashboard`

### Requirement: API-first management backend baseline
系统 MUST 通过可由专用前端消费的 API 合同暴露管理员管理面。

#### Scenario: GraphQL remains the primary resource contract
- **WHEN** 专用前端读取或变更管理员管理资源
- **THEN** dashboard、用户、客户端、代理、证书和审计资源操作通过基于会话认证的 `/api/admin/graphql` 暴露，并由 admin query 与 command 服务支撑

#### Scenario: Auxiliary HTTP endpoints remain narrow
- **WHEN** 专用前端或启动流程需要登录、登出、会话启动或类似浏览器会话行为
- **THEN** 后端只为这些关注点暴露最小非 GraphQL HTTP 端点，不复制核心资源管理合同

### Requirement: Administrator dashboard baseline
系统 MUST 提供与当前可信运行时聚合对齐的最小管理员 dashboard 摘要。

#### Scenario: Dashboard summary fields
- **WHEN** 管理员加载 V1 dashboard 摘要
- **THEN** 响应包含 `onlineClientCount`、`enabledProxyCount`、`activeTCPConnectionCount`、`cumulativeUploadBytes`、`cumulativeDownloadBytes`、`cumulativeTCPErrorCount`、`cumulativeUDPErrorCount` 和 `cumulativeHTTPErrorCount`

#### Scenario: Dashboard excludes unfinished observability projections
- **WHEN** 管理员查看 V1 dashboard
- **THEN** dashboard 不声明缺少实现证据的告警状态汇总、时间窗口流量摘要或更丰富可观测性投影

### Requirement: Administrator user-management baseline
系统 MUST 为首批 API/UI 提供管理员专用用户管理。

#### Scenario: List and view users
- **WHEN** 已认证管理员查询 V1 用户管理面
- **THEN** 系统返回托管用户列表和详情视图

#### Scenario: Create user
- **WHEN** 已认证管理员在 V1 管理面创建用户
- **THEN** 系统持久化用户资源，不要求初始配额或限制字段

#### Scenario: Disable user
- **WHEN** 已认证管理员在 V1 管理面禁用用户
- **THEN** 系统更新用户状态，使后续运行时准入检查可以把该用户视为禁用

#### Scenario: Modify user password
- **WHEN** 已认证管理员更新托管用户密码
- **THEN** 系统存储更新后的密码校验材料，并且不在管理响应中暴露明文密码材料

### Requirement: Administrator client-management baseline
系统 MUST 提供管理员专用客户端列表、详情、创建、凭据轮换和完全删除合同，其中凭据处理为一次性返回，删除处理为永久资源销毁。

#### Scenario: List clients
- **WHEN** 已认证管理员查询 V1 管理面中的客户端
- **THEN** 系统返回适合管理员控制 console 的分页、可过滤、可排序客户端列表数据

#### Scenario: View client detail with runtime state
- **WHEN** 已认证管理员查看客户端详情
- **THEN** 系统组合持久化客户端数据与当前可用运行时/会话状态，并包含该客户端管理的代理上下文

#### Scenario: Create or rotate client credential returns secret once
- **WHEN** 已认证管理员创建客户端或轮换客户端凭据
- **THEN** 新凭据可以在该变更 payload 中精确返回一次，后续列表或详情查询 MUST NOT 暴露该凭据

#### Scenario: Delete client with no enabled proxies
- **WHEN** 已认证管理员请求完全删除某个客户端，且该客户端不存在已启用代理
- **THEN** 系统在一个受控变更中永久删除该客户端、客户端凭据和允许级联清理的从属客户端资源，记录审计事件，并使后续列表、详情、认证和 join/enrollment 消费都不能再使用该客户端

#### Scenario: Delete client rejects enabled proxy dependency
- **WHEN** 已认证管理员请求完全删除某个客户端，但该客户端仍存在已启用代理
- **THEN** 系统以可由前端消费的 `CONFLICT` 或等价结构化语义拒绝删除，并说明必须先禁用相关代理

#### Scenario: Delete missing client returns not found
- **WHEN** 已认证管理员请求完全删除不存在或已删除的客户端
- **THEN** 系统返回可由前端消费的 `NOT_FOUND` 语义，而不是报告删除成功

#### Scenario: Delete client errors remain scoped
- **WHEN** 客户端删除 mutation 返回认证、授权、冲突、不存在、校验或后端失败语义
- **THEN** 前端在当前客户端列表、详情或确认弹窗动作 surface 内展示对应失败，不丢弃当前已加载的页面内容

### Requirement: Administrator proxy-management baseline
系统 MUST 为当前支持的反向代理资源类型提供完整管理员 CRUD 与生命周期控制，并尽可能在接受无效状态前拒绝会违反活跃运行时监听器准入的 TCP 或 UDP 生命周期变更。

#### Scenario: Manage supported reverse-proxy types
- **WHEN** 已认证管理员在 V1 创建或更新代理
- **THEN** 管理面支持当前已实现的 `TCP`、`UDP`、`HTTP` 和 `HTTPS` 反向代理类型，且不声明本批次支持正向代理创建

#### Scenario: View proxy list and detail
- **WHEN** 已认证管理员查询 V1 代理
- **THEN** 系统返回组合持久化配置、可用运行时状态和聚合状态信息的代理列表与详情视图

#### Scenario: Create proxy
- **WHEN** 已认证管理员创建有效代理
- **THEN** 系统持久化代理资源并记录控制面动作

#### Scenario: Update proxy without type mutation
- **WHEN** 已认证管理员更新现有代理
- **THEN** 系统允许更新受支持代理字段，但拒绝原地变更代理类型

#### Scenario: Enable or disable proxy
- **WHEN** 已认证管理员启用或禁用代理
- **THEN** 系统把该动作视为显式生命周期操作，而不是偶然的状态字段编辑

#### Scenario: Delete requires disabled proxy
- **WHEN** 已认证管理员请求删除代理
- **THEN** 系统只允许先禁用后删除

### Requirement: Proxy listener-admission semantics
系统 MUST 通过共享 ListenerClaim 模型在活跃运行时监听空间上评估 TCP 和 UDP 代理 socket 准入，并把活跃监听冲突作为显式合同行为暴露。

#### Scenario: ListenerClaim conflict rejects create, update, or enable operations
- **WHEN** 已认证管理员创建、更新或启用 TCP/UDP 代理，且请求的活跃监听器与现有活跃 claim 在 V1 `same network + same port` 规则下冲突
- **THEN** 操作以 `ENTRY_CONFLICT` 语义被拒绝，而不是落为通用持久化失败

#### Scenario: Active ListenerClaim set includes configured static listeners
- **WHEN** 为 TCP 或 UDP 代理活动评估监听器准入
- **THEN** 活跃 ListenerClaim 集合包含来自 `control_quic_listen`、`control_tls_listen`、`admin_listen`、`http_entry_listen` 和 `https_entry_listen` 的已配置静态监听器，只要这些监听器参与运行时绑定

#### Scenario: Active ListenerClaim set includes enabled TCP and UDP proxies
- **WHEN** 为 TCP 或 UDP 代理活动评估监听器准入
- **THEN** 活跃 ListenerClaim 集合包含会占用活跃运行时监听器的已启用 TCP 代理和已启用 UDP 代理

#### Scenario: Disabled proxies do not participate in active ListenerClaim admission
- **WHEN** 为 TCP 或 UDP 代理活动评估监听器准入
- **THEN** 禁用代理不参与用于冲突检测的活跃 claim 集合

### Requirement: Managed certificate admin baseline
系统 MUST 在首批 API/UI 中为托管 HTTPS 证书提供管理员证书状态和生命周期动作。

#### Scenario: View managed certificate status
- **WHEN** 已认证管理员查看 V1 中某个 HTTPS 代理的托管证书状态
- **THEN** 系统返回当前证书管理行为已支持的托管证书状态 surface

#### Scenario: Issue or renew managed certificate
- **WHEN** 已认证管理员触发 V1 托管证书签发或续期
- **THEN** 系统执行受支持的托管证书生命周期动作，并记录控制面操作

#### Scenario: Certificate lifecycle actions do not expose secret material
- **WHEN** 已认证管理员通过 `/api/admin/graphql` 触发证书签发或续期
- **THEN** 变更合同返回运行生命周期结果，且不暴露私钥或其他私钥材料

### Requirement: Minimal administrator audit list baseline
系统 MUST 为首批 admin API/UI 暴露最小近期审计事件列表。

#### Scenario: Recent audit events list
- **WHEN** 已认证管理员请求 V1 审计视图
- **THEN** 系统按倒序时间返回近期事件列表，包含 actor type、actor ID、资源类型、资源 ID、动作、结果和时间戳字段

#### Scenario: Audit view excludes advanced query behavior
- **WHEN** 已认证管理员使用 V1 审计 surface
- **THEN** 系统不声明首批支持高级过滤、导出或日志关联行为

### Requirement: Frontend-grade list interaction baseline
系统 MUST 支持专用 console 管理员列表视图所需的前端级交互。

#### Scenario: Primary list views support pagination and filtering
- **WHEN** 已认证管理员使用 `users`、`clients`、`proxies`、`certificates` 或 `audit` 列表视图
- **THEN** 管理 API 支持分页和视图适配的过滤，使前端不依赖整表转储

#### Scenario: Primary list views support explicit sorting semantics
- **WHEN** 已认证管理员改变受支持列表视图排序
- **THEN** 管理 API 应用显式排序字段和排序方向，而不是依赖隐式存储顺序

### Requirement: Frontend-consumable error semantics baseline
系统 MUST 为专用管理员 console 暴露可由前端消费的认证、授权、校验和失败语义。

#### Scenario: GraphQL errors expose machine-readable contract codes
- **WHEN** `/api/admin/graphql` 管理操作失败
- **THEN** 响应暴露 `UNAUTHENTICATED`、`FORBIDDEN`、`VALIDATION_FAILED`、`NOT_FOUND`、`CONFLICT`、`UNSUPPORTED`、`ENTRY_CONFLICT`、`INVALID_CSRF` 和 `INTERNAL` 等机器可读错误语义

#### Scenario: Validation failures include field-level details
- **WHEN** admin GraphQL 操作因为一个或多个字段输入校验失败
- **THEN** 响应在适用时包含字段级校验详情，使前端无需只解析人类文本即可把失败映射到表单字段

#### Scenario: Authentication failures are distinguishable
- **WHEN** 专用 console 请求因为管理员会话缺失、过期或无效而失败
- **THEN** 前端可以把该认证失败与授权拒绝、校验失败和意外后端错误区分开

### Requirement: Shared frontend page-state semantics baseline
系统 MUST 在专用管理员前端中保持共享页面状态语义，使受保护路由、列表、详情和刷新行为一致。

#### Scenario: Empty state distinguishes no-data from no-match
- **WHEN** `users`、`clients`、`proxies`、`certificates` 或 `audit` 等列表页没有可显示条目
- **THEN** 前端区分资源集合为空的基线空状态和当前过滤/搜索导致的无匹配状态

#### Scenario: Detail pages distinguish missing resource from backend failure
- **WHEN** 已认证管理员打开 `users/:id`、`clients/:id` 或 `proxies/:id` 等有效详情路由
- **THEN** 前端把托管资源缺失与通用后端失败区分开，而不是折叠成单一通用错误状态

#### Scenario: Dashboard zero values remain content
- **WHEN** 已认证管理员查看 dashboard，且当前可信聚合均为零值
- **THEN** 前端把零值字段渲染为 dashboard 内容，而不是替换为通用空状态

#### Scenario: Validation failure remains scoped to active form
- **WHEN** 创建、更新或生命周期动作以结构化校验语义失败
- **THEN** 前端在当前表单或动作 surface 内展示校验失败，而不是把整页折叠成通用错误

### Requirement: Shared administrator modal interaction baseline
系统 MUST 在专用 admin 前端中为所有弹窗提供统一的视口约束、内容滚动、token 展示和蒙版关闭语义，避免内容不可达或误关闭正在操作的弹窗。

#### Scenario: Modal content scrolls within viewport
- **WHEN** 已认证管理员打开任意 admin-ui 弹窗，且弹窗内容高度超过当前视口可用高度
- **THEN** 弹窗容器保持在视口最大高度内，弹窗内容区域在容器内部滚动，使标题、主体和可用操作不会因为溢出而不可访问

#### Scenario: Token area scrolls after three lines
- **WHEN** 弹窗或动作结果 surface 展示 join token、客户端凭据、secret 或类似长文本，且内容超过 3 行
- **THEN** 该 token 区域最多显示 3 行高度，超出内容在 token 区域内部滚动，而不是撑高整个弹窗

#### Scenario: Overlay click closes only when full click stays on overlay
- **WHEN** 管理员在弹窗蒙版层按下鼠标并在同一蒙版层释放鼠标
- **THEN** 前端可以关闭当前弹窗

#### Scenario: Drag release from dialog to overlay does not close
- **WHEN** 管理员在弹窗内容内部按下鼠标，随后在蒙版层释放鼠标
- **THEN** 前端 MUST NOT 把该释放动作视为蒙版完整点击，也不得关闭当前弹窗

#### Scenario: Dialog internal interaction does not trigger overlay close
- **WHEN** 管理员在弹窗内部点击、选择文本、拖动滚动条或操作表单控件
- **THEN** 前端保持弹窗打开，除非管理员触发显式关闭、取消、提交成功或完整蒙版点击关闭动作

### Requirement: Polling refresh baseline
系统 MUST 为首批专用 admin console 使用固定客户端轮询，而不是实时订阅。

#### Scenario: Runtime-oriented pages poll frequently
- **WHEN** 管理员打开 dashboard、clients 或 proxies 等运行时导向页面
- **THEN** 前端按 5 秒节奏刷新相关 GraphQL 查询，而不是整页刷新或使用实时推送

#### Scenario: Low-churn pages avoid aggressive refresh
- **WHEN** 管理员使用 users、audit 或 certificates 页面
- **THEN** 这些页面使用手动刷新、低频轮询或动作驱动刷新，并且刷新作用域保持在当前页面

#### Scenario: Targeted post-mutation refresh
- **WHEN** 创建、更新、启用、禁用、删除、签发或续期等变更成功
- **THEN** 前端重新查询受影响的列表、详情或两者，而不是依赖浏览器整页 reload 同步状态

### Requirement: Admin GraphQL implementation alignment baseline
系统 MUST 把现有管理员 GraphQL/API 实现与规范化 admin-resource-management 合同对齐，同时保留当前 admin read 和 command 服务边界。

#### Scenario: Admin GraphQL reads and commands preserve canonical boundaries
- **WHEN** 实现在 `/api/admin/graphql` 对齐 dashboard、用户、客户端、代理、证书或审计行为
- **THEN** 读操作继续使用 `internal/adminquery` 的面向页面列表和详情模型，命令操作继续使用 `internal/admin` 的生命周期与变更行为

#### Scenario: Admin GraphQL contracts align with canonical frontend semantics
- **WHEN** 实现更新 admin GraphQL 列表、详情或变更操作
- **THEN** 结果合同保留规范化页面导向列表/详情行为、共享分页/过滤/排序输入、单 input/单 payload 变更语义、客户端凭据一次性返回行为、带校验详情的结构化 GraphQL 错误码，以及审计 actor type 加 actor ID 的身份方向

#### Scenario: Alignment excludes unrelated admin redesign work
- **WHEN** 实现变更限定在 admin GraphQL 合同对齐范围
- **THEN** 不把范围扩大到配额或限速、可观测性重做、管理员会话持久化重设计、RBAC 重设计、正向代理支持、无关部署工作或备份/恢复工作

### Requirement: Management policy gap tracking
admin-resource-management 基线 MUST 把已确认的 V1 管理员资源管理 surface 视为当前能力，同时继续排除更广泛的策略和管理域。

#### Scenario: V1 management surface is limited to confirmed resources
- **WHEN** 操作者描述 V1 管理员管理面
- **THEN** 它包含 dashboard 摘要、用户列表/详情/创建/禁用/密码修改、客户端列表/详情/创建/凭据轮换、代理 CRUD 与生命周期控制、托管证书状态/签发/续期，以及最小近期审计列表

#### Scenario: Adjacent policy behavior remains a gap
- **WHEN** 产品或设计文档提到配额编辑、限制执行 UI、高级资源过滤、系统设置变更、日志搜索、审计导出、告警中心工作流、域名生命周期管理或正向代理管理行为
- **THEN** 在存在证据支持的实现前，该行为 MUST 作为本规格中的缺口保留

### Requirement: Administrator client creation frontend baseline
系统 MUST 在专用 admin 前端中提供客户端创建入口，并通过现有会话认证的 `/api/admin/graphql` 客户端创建合同完成持久化。

#### Scenario: Create client from clients list
- **WHEN** 已认证管理员在 `Clients` 列表页打开创建客户端表单，通过用户选择器选定所属用户，并提交有效的客户端名称和可选初始凭据
- **THEN** 前端通过 `createClient` mutation 创建客户端，创建成功后关闭或完成表单状态，并刷新受影响的客户端列表查询

#### Scenario: Create client defaults user from user-scoped clients list
- **WHEN** 已认证管理员从带用户上下文的客户端列表打开创建客户端表单
- **THEN** 创建表单的用户选择器默认选中当前用户上下文中的用户 ID，并允许管理员在提交前显式修改该选择

#### Scenario: Create client validation remains scoped to form
- **WHEN** 客户端创建 mutation 返回结构化校验失败
- **THEN** 前端在创建客户端表单内展示字段级或表单级错误，而不是把整个客户端列表页替换为通用错误状态

#### Scenario: Create client credential displays once
- **WHEN** 客户端创建 mutation 成功并返回一次性客户端凭据
- **THEN** 前端在当前创建结果 surface 中展示该凭据及一次性提示，且后续客户端列表或详情查询 MUST NOT 请求或显示该凭据

### Requirement: Administrator client user selector frontend baseline
系统 MUST 在专用 admin 前端中为客户端列表过滤和客户端创建提供用户选择器，并把选择结果作为用户 ID 传递给现有 clients 查询与创建 mutation。

#### Scenario: Client list filters by selected user
- **WHEN** 已认证管理员在 `Clients` 列表页通过用户选择器选择某个用户
- **THEN** 前端把该用户 ID 应用于客户端列表过滤，并只请求和显示该用户的客户端

#### Scenario: Client create submits selected user ID
- **WHEN** 已认证管理员在创建客户端表单中通过用户选择器选择所属用户
- **THEN** 前端向 `createClient` mutation 提交该用户的 ID，而不是显示名称或用户名

#### Scenario: User selector displays recognizable user options
- **WHEN** 前端渲染客户端列表过滤或创建表单中的用户选择器
- **THEN** 选择器选项展示足以识别用户的信息，包括用户名和用户 ID，并复用现有管理员用户列表合同获取候选用户

#### Scenario: Scoped user ID remains preserved before option hydration
- **WHEN** 已认证管理员直接打开带用户 ID 的客户端列表 URL，而该用户尚未出现在已加载的选择器选项中
- **THEN** 前端保留该用户 ID 作为当前过滤和创建默认值，并在选择器中以用户 ID fallback 表示该状态

#### Scenario: Clearing selected user restores global client list
- **WHEN** 已认证管理员清空客户端列表的用户选择器
- **THEN** 前端清除用户 ID 过滤，恢复全局客户端列表，并使后续创建客户端表单不再默认选中之前的用户

### Requirement: Administrator client credential rotation frontend baseline
系统 MUST 在专用 admin 前端中提供客户端凭据轮换动作，并且只在轮换 mutation 成功结果中展示新凭据。

#### Scenario: Rotate client credential from client detail
- **WHEN** 已认证管理员在 `ClientDetail` 页面确认轮换某个客户端的凭据
- **THEN** 前端通过 `rotateClientCredential` mutation 轮换该客户端凭据，并在成功后刷新受影响的客户端详情和客户端列表查询

#### Scenario: Rotated credential displays once
- **WHEN** 客户端凭据轮换 mutation 成功并返回新凭据
- **THEN** 前端在当前详情页动作 surface 中展示该凭据及一次性提示，且后续客户端列表或详情查询 MUST NOT 请求或显示该凭据

#### Scenario: Rotate client credential errors remain scoped
- **WHEN** 客户端凭据轮换 mutation 返回认证、校验、不存在或后端失败语义
- **THEN** 前端在客户端详情页的轮换动作 surface 内展示对应失败，不丢弃当前已加载的客户端详情内容

### Requirement: Administrator user-scoped clients navigation baseline
系统 MUST 支持从用户管理页面进入带用户上下文的客户端列表，并把该上下文用于客户端列表过滤和创建默认值。

#### Scenario: User list shortcut opens scoped clients
- **WHEN** 已认证管理员在 `Users` 列表页对某个用户触发查看客户端快捷入口
- **THEN** 前端导航到带有该用户 ID 的客户端列表 URL，并且 `Clients` 页面只请求和显示该用户的客户端

#### Scenario: User detail shortcut opens scoped clients
- **WHEN** 已认证管理员在 `UserDetail` 页面触发查看该用户客户端快捷入口
- **THEN** 前端导航到带有该用户 ID 的客户端列表 URL，并且 `Clients` 页面只请求和显示该用户的客户端

#### Scenario: Scoped clients state survives refresh
- **WHEN** 已认证管理员直接打开、刷新或登录后恢复带用户 ID 的客户端列表 URL
- **THEN** 前端从 URL 中恢复用户上下文状态，初始化客户端用户选择器，并在创建客户端表单中默认选中该用户 ID

#### Scenario: Scoped clients state remains clearable
- **WHEN** 已认证管理员清除客户端列表中的用户选择器
- **THEN** 前端恢复全局客户端列表视图，后续创建客户端表单不再默认使用之前的用户 ID

### Requirement: Administrator client-to-proxy creation frontend baseline
系统 MUST 在专用 admin 前端中支持从客户端上下文进入代理创建流程，并在代理创建表单中默认携带可修改的用户和客户端上下文。

#### Scenario: Client shortcut opens proxy creation with context
- **WHEN** 已认证管理员在 `Clients` 列表或 `ClientDetail` 页面触发为某个客户端创建代理的入口
- **THEN** 前端导航到代理创建入口，并携带该客户端的 `clientId` 以及所属 `userId` 作为创建代理默认上下文

#### Scenario: Proxy create form defaults selectors from client context
- **WHEN** 已认证管理员从客户端上下文打开代理创建表单
- **THEN** 代理创建表单的用户选择器默认选中携带的 `userId`，客户端选择器默认选中携带的 `clientId`，并在提交时把选择后的 ID 传给 `createProxy` mutation

#### Scenario: Proxy create selectors remain editable
- **WHEN** 已认证管理员在代理创建表单中修改用户或客户端选择器
- **THEN** 前端使用管理员最终选择的 `userId` 和 `clientId` 创建代理，而不是强制保留跳转时携带的默认值

#### Scenario: Client selector stays consistent with selected user
- **WHEN** 已认证管理员在代理创建表单中改变用户选择器，且当前客户端不属于新选择的用户
- **THEN** 前端清空或重新要求选择客户端，避免提交不一致的用户和客户端组合

#### Scenario: Proxy create context survives refresh
- **WHEN** 已认证管理员直接打开、刷新或登录后恢复带有 `clientId` 与 `userId` 的代理创建 URL
- **THEN** 前端从 URL 或路由状态恢复默认上下文，并在候选选项尚未加载完成时保留 ID fallback

### Requirement: Local admin TUI entrypoint
系统 MUST 为 admin CLI 提供本地 TUI 模式，作为面向服务器终端的交互式运维入口。

#### Scenario: Start local TUI with default database
- **WHEN** 操作者运行 `goginx-admin tui` 且未显式提供数据库路径
- **THEN** 系统使用与现有 admin CLI 相同的部署根默认 SQLite 路径启动 TUI

#### Scenario: Start local TUI with explicit database
- **WHEN** 操作者运行 `goginx-admin tui -db <path>`
- **THEN** 系统使用指定 SQLite 数据库路径启动 TUI，并按现有部署相对路径规则解析该路径

#### Scenario: Preserve non-interactive command behavior
- **WHEN** 操作者继续运行 `init-admin`、`create-user`、`create-client` 或其他现有 admin CLI 子命令
- **THEN** 系统保持这些子命令的参数、输出和错误行为，不要求经过 TUI

#### Scenario: Reject non-interactive terminal for TUI
- **WHEN** 操作者在不支持交互式终端控制的环境中运行 `goginx-admin tui`
- **THEN** 系统拒绝进入 TUI，并提示使用现有非交互式子命令完成同等配置

### Requirement: Local admin TUI navigation baseline
系统 MUST 在 TUI 中提供简单可发现的本地运维导航，并限制首版范围为管理员、用户和客户端配置。

#### Scenario: Main menu exposes confirmed operations
- **WHEN** TUI 启动成功
- **THEN** 主菜单提供管理员设置、用户管理、客户端配置和退出选项

#### Scenario: Main menu excludes broader admin surfaces
- **WHEN** 操作者查看 TUI 主菜单
- **THEN** 系统不展示代理、证书、审计、配额、系统设置、告警或远程登录管理入口

#### Scenario: Return to main menu after action
- **WHEN** 操作者完成或取消管理员、用户或客户端配置流程
- **THEN** TUI 返回主菜单或上一级菜单，而不是退出整个进程

### Requirement: Local admin TUI administrator setup
系统 MUST 通过 TUI 支持快速配置管理员信息，并且不得引入默认管理员密码。

#### Scenario: Create first administrator from TUI
- **WHEN** SQLite 中不存在可登录管理员，且操作者在 TUI 中提交有效用户名、密码和确认密码
- **THEN** 系统创建一个启用的管理员用户，保存密码校验材料，并记录可审计的初始化结果

#### Scenario: Select existing administrator for update
- **WHEN** SQLite 中已存在管理员用户，且操作者进入管理员设置
- **THEN** TUI 优先展示可选择的现有管理员列表，而不是要求操作者手动输入管理员用户 ID

#### Scenario: Update administrator password from TUI
- **WHEN** 操作者选择现有管理员并提交有效的新密码和确认密码
- **THEN** 系统更新该管理员密码校验材料，并保持该用户可用于管理员登录

#### Scenario: Enable disabled administrator from TUI
- **WHEN** 操作者选择一个已禁用的管理员并确认启用
- **THEN** 系统启用该管理员用户，并记录对应审计事件

#### Scenario: Reject invalid administrator setup input
- **WHEN** 操作者在管理员设置中提交空用户名、空密码、不一致的确认密码或无效管理员选择
- **THEN** TUI 在当前表单阻止提交，并展示字段级错误

### Requirement: Local admin TUI user setup
系统 MUST 通过 TUI 支持快速配置用户，并优先使用选项和默认值减少手动输入。

#### Scenario: Create user with role selection
- **WHEN** 操作者在 TUI 用户管理中输入有效用户名并从角色选项中选择 `admin` 或 `user`
- **THEN** 系统创建对应角色的启用用户，并持久化到 SQLite

#### Scenario: Default ordinary user role
- **WHEN** 操作者打开创建用户表单
- **THEN** TUI 默认选择普通用户角色，并允许操作者显式改为管理员角色

#### Scenario: Show existing users as selectable context
- **WHEN** 操作者进入用户管理
- **THEN** TUI 展示现有用户的可识别信息，至少包括用户名、用户 ID、角色和状态

#### Scenario: Reject invalid user setup input
- **WHEN** 操作者提交空用户名、无效角色或与服务层校验冲突的用户配置
- **THEN** TUI 在当前用户表单展示错误，并且不创建无效用户记录

### Requirement: Local admin TUI user maintenance
系统 MUST 通过 TUI 支持用户启用、禁用和受保护删除，并防止级联误删用户下的客户端或代理资源。

#### Scenario: Disable selected user
- **WHEN** 操作者从 TUI 用户列表选择一个启用用户并确认禁用
- **THEN** 系统禁用该用户，记录审计事件，并在结果页展示该用户的新状态

#### Scenario: Enable selected user
- **WHEN** 操作者从 TUI 用户列表选择一个禁用用户并确认启用
- **THEN** 系统启用该用户，记录审计事件，并在结果页展示该用户的新状态

#### Scenario: Delete user without dependent resources
- **WHEN** 操作者从 TUI 用户列表选择一个用户，且该用户没有客户端或代理等依赖资源，并完成强确认
- **THEN** 系统删除该用户记录，记录审计事件，并刷新用户列表

#### Scenario: Reject user delete with dependent resources
- **WHEN** 操作者尝试删除仍拥有客户端或代理资源的用户
- **THEN** 系统拒绝删除，展示依赖资源阻塞原因，并且不得级联删除该用户的客户端或代理

#### Scenario: User delete requires strong confirmation
- **WHEN** 操作者在 TUI 中触发用户删除
- **THEN** TUI 在执行删除前要求摘要确认和资源 ID 级别的强确认

### Requirement: Local admin TUI client setup
系统 MUST 通过 TUI 支持快速配置客户端，并优先通过用户选择、生成凭据和默认 join 参数减少手动输入。

#### Scenario: Quick create client join token with selected owner
- **WHEN** 操作者在 TUI 默认客户端快速向导中从现有用户列表选择所属用户，输入有效客户端名称，并确认加入令牌参数
- **THEN** 系统创建客户端和加入令牌，并在结果页展示 token
- **AND** 结果页展示可在管理端终端执行的 `goginx-admin client-join-command -client <id>` 指令，用于获取客户端 join 指令

#### Scenario: Review active client join token after creation
- **WHEN** 操作者从 TUI 客户端操作菜单选择查看 join token，且该客户端存在未使用且未过期的 join token
- **THEN** 系统再次展示该 token，并提示该 token 仍只能被客户端消费一次
- **AND** 结果页展示可在管理端终端执行的 `goginx-admin client-join-command -client <id>` 指令，用于获取客户端 join 指令

#### Scenario: Reject reviewing unavailable client join token
- **WHEN** 操作者尝试查看不存在、已使用、已过期或历史记录缺少明文的客户端 join token
- **THEN** TUI 展示可操作错误，并引导操作者重新生成 join token

#### Scenario: Create client credential from secondary path
- **WHEN** 操作者在 TUI 客户端配置中选择仅创建客户端凭据，从现有用户列表选择所属用户，并输入有效客户端名称
- **THEN** 系统为该用户创建客户端记录，生成或保存客户端凭据，并在结果页展示新凭据

#### Scenario: Client owner uses selection instead of manual ID
- **WHEN** TUI 需要客户端所属用户
- **THEN** 系统展示现有用户选项供操作者选择，并把选择结果作为用户 ID 提交给客户端配置流程

#### Scenario: Client credential defaults to generated value
- **WHEN** 操作者创建客户端凭据且未选择手动输入凭据
- **THEN** 系统使用服务层生成的客户端凭据，而不是要求操作者手动填写 secret

#### Scenario: Join token defaults are reviewable before submit
- **WHEN** 操作者创建客户端加入令牌
- **THEN** TUI 展示 enrollment URL、控制通道地址、TLS 地址、server name、CA 文件和 TTL 的默认值，并要求操作者在提交前确认或编辑

#### Scenario: Reject client setup when no owner exists
- **WHEN** SQLite 中不存在可作为客户端所属者的用户，且操作者进入客户端配置
- **THEN** TUI 阻止创建客户端，并引导操作者先创建用户

#### Scenario: Reject invalid client setup input
- **WHEN** 操作者提交空客户端名称、无效用户选择、空 join 必填地址、缺失 CA 文件或服务层返回的校验错误
- **THEN** TUI 在当前客户端流程展示错误，并且不创建无效客户端或加入令牌

#### Scenario: Client secrets display policy distinguishes credentials and join tokens
- **WHEN** 客户端凭据创建或轮换成功
- **THEN** TUI 仅在当前结果页展示凭据，并提示离开后需要重新创建或轮换才能再次获得明文值
- **AND** join token 在未使用且未过期期间可以由管理员重复查看

### Requirement: Local admin TUI client maintenance
系统 MUST 通过 TUI 支持客户端启用、禁用、凭据轮换、join token 查看和受保护删除。

#### Scenario: Disable selected client
- **WHEN** 操作者从 TUI 客户端列表选择一个客户端并确认禁用
- **THEN** 系统禁用该客户端，记录审计事件，并在结果页展示该客户端的新状态

#### Scenario: Enable selected client
- **WHEN** 操作者从 TUI 客户端列表选择一个禁用客户端并确认启用
- **THEN** 系统启用该客户端，记录审计事件，并在结果页展示该客户端的新状态

#### Scenario: Rotate selected client credential
- **WHEN** 操作者从 TUI 客户端列表选择一个客户端并确认轮换凭据
- **THEN** 系统轮换该客户端凭据，记录审计事件，并仅在当前结果页展示新凭据

#### Scenario: Review selected client join token
- **WHEN** 操作者从 TUI 客户端列表选择一个客户端并请求查看 join token
- **THEN** 系统查询该客户端最新可查看 join token，并在结果页展示 token、过期时间和单次消费提示
- **AND** 结果页展示可在管理端终端执行的 `goginx-admin client-join-command -client <id>` 指令，用于获取客户端 join 指令

#### Scenario: Delete selected client
- **WHEN** 操作者从 TUI 客户端列表选择一个客户端并完成强确认
- **THEN** 系统删除该客户端记录，记录审计事件，并刷新客户端列表

#### Scenario: Reject client delete when service blocks it
- **WHEN** 客户端存在服务层禁止删除的依赖或状态，例如仍有关联启用代理
- **THEN** TUI 展示阻塞原因，并且不得删除该客户端

#### Scenario: Client delete requires strong confirmation
- **WHEN** 操作者在 TUI 中触发客户端删除
- **THEN** TUI 在执行删除前要求摘要确认和客户端 ID 级别的强确认

### Requirement: Local admin TUI validation semantics
系统 MUST 在 TUI 中提供强校验和明确错误反馈，使无效配置不会静默写入本地数据库。

#### Scenario: Validate before command execution
- **WHEN** 操作者提交管理员、用户或客户端表单
- **THEN** TUI 在调用领域服务前校验必填字段、枚举选择、确认密码、选择项存在性和文件路径等可本地判断的条件

#### Scenario: Surface service validation errors
- **WHEN** 领域服务返回结构化校验、冲突或资源不存在错误
- **THEN** TUI 在当前流程展示可操作的错误信息，并保留操作者已填写的有效字段

#### Scenario: Confirm persistent changes
- **WHEN** 操作者即将创建、更新、启用、禁用、删除管理员、用户、客户端或加入令牌
- **THEN** TUI 在执行写入前展示摘要确认页，确认后才调用持久化操作

#### Scenario: Cancel without partial write
- **WHEN** 操作者在确认前取消管理员、用户或客户端配置流程
- **THEN** 系统不写入对应资源，并返回上一级菜单

