## Purpose

定义管理员资源管理契约，覆盖里程碑一非交互式 CLI 资源种子写入、管理员专用认证、同源会话与管理 API、面向专用前端的 GraphQL 合同、可选同源前端交付，以及 dashboard、用户、客户端、代理、托管证书和近期审计视图；同时显式跟踪配额、设置、告警、更完整可观测性、RBAC、正向代理和普通用户自助等剩余管理缺口。

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

### Requirement: Administrator authentication baseline
系统 MUST 使用与客户端凭据分离的管理员专用认证来保护管理前端和 API。

#### Scenario: Administrator credentials loaded from protected configuration
- **WHEN** 管理面启动
- **THEN** 管理员用户名和密码校验材料从受保护的服务端配置文件加载，而不是从现有 SQLite `users` 表加载

#### Scenario: Administrator credentials remain separate from product users and client credentials
- **WHEN** 管理面认证管理员
- **THEN** 管理员身份与产品用户和客户端凭据保持分离，不把浏览器登录语义耦合到运行时机器身份

#### Scenario: Management access requires protected transport
- **WHEN** 使用管理员凭据访问管理面
- **THEN** 管理端点预期运行在 TLS 保护之后；本地回环明文仅用于开发和自动化测试

### Requirement: Administrator session endpoint baseline
系统 MUST 为专用 admin console 暴露同源管理员会话端点。

#### Scenario: Login creates an administrator browser session
- **WHEN** 有效管理员向 `/api/admin/login` 提交凭据
- **THEN** 系统根据受保护管理员凭据源校验凭据，创建服务端管理的浏览器会话，设置会话 Cookie，并返回前端 shell 所需的最小启动信息

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
系统 MUST 把专用管理员前端和管理员 API 以一个外部同源呈现给浏览器。

#### Scenario: Frontend assets are served when configured
- **WHEN** `admin_frontend_dir` 指向包含 `index.html` 的专用前端构建目录
- **THEN** admin listener 在同源上服务 `/`、`/login`、`/dashboard`、`/users`、`/clients`、`/proxies`、`/certificates`、`/audit` 和受支持深链，同时继续保留 `/api/admin/*` 作为后端 API 命名空间

#### Scenario: Missing frontend keeps transitional not-found behavior
- **WHEN** `admin_frontend_dir` 未配置
- **THEN** 非 API 浏览器路由继续返回 `404 Not Found`，而会话认证的管理员 API 保持可用

#### Scenario: Missing asset-like paths return not found
- **WHEN** 浏览器请求配置前端目录中不存在的资源型路径，例如 `/assets/missing.js`
- **THEN** admin listener 返回 `404 Not Found`，而不是把缺失资源错误伪装成前端深链

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
系统 MUST 提供管理员专用客户端列表、详情、创建和凭据轮换合同，其中凭据处理为一次性返回。

#### Scenario: List clients
- **WHEN** 已认证管理员查询 V1 管理面中的客户端
- **THEN** 系统返回适合管理员控制 console 的分页、可过滤、可排序客户端列表数据

#### Scenario: View client detail with runtime state
- **WHEN** 已认证管理员查看客户端详情
- **THEN** 系统组合持久化客户端数据与当前可用运行时/会话状态，并包含该客户端管理的代理上下文

#### Scenario: Create or rotate client credential returns secret once
- **WHEN** 已认证管理员创建客户端或轮换客户端凭据
- **THEN** 新凭据可以在该变更 payload 中精确返回一次，后续列表或详情查询 MUST NOT 暴露该凭据

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
