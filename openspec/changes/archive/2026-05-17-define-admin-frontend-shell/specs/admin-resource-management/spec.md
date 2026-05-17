## ADDED Requirements

### Requirement: Admin frontend route shell baseline
系统 MUST 把专用管理员前端定义为一个同源路由 shell，通过 `/api/admin/session` 做守卫决策，通过 `/api/admin/login` 和 `/api/admin/logout` 管理会话生命周期，通过 `/api/admin/graphql` 获取业务数据。

#### Scenario: Shell separates public and protected route groups
- **WHEN** 为已确认首批页面定义专用管理员前端路由模型
- **THEN** 它包含一个用于 `login` 的未认证路由组，以及一个用于 `dashboard`、`users`、`clients`、`proxies`、`certificates` 和 `audit` 的已认证应用 shell 路由组

#### Scenario: Shell keeps backend contract boundaries intact
- **WHEN** 前端路由 shell 把浏览器交互映射到后端调用
- **THEN** 登录、会话启动和登出使用同源 admin HTTP 端点，页面业务数据继续使用 `/api/admin/graphql` 上的规范 GraphQL 合同

### Requirement: Guarded-route bootstrap semantics
系统 MUST 定义受保护管理员路由先通过同源会话端点解析会话状态，再激活受保护页面内容。

#### Scenario: Initial protected navigation with valid session
- **WHEN** 管理员带有效既有会话加载或深链打开受保护路由
- **THEN** 前端启动流程检查 `/api/admin/session`，初始化已认证 shell，并渲染请求的受保护页面，而不经过 `login`

#### Scenario: Initial protected navigation without valid session
- **WHEN** 管理员在没有有效会话的情况下加载或深链打开受保护路由
- **THEN** 前端不渲染受保护页面内容，而是把浏览器路由到 `login` 页面

#### Scenario: Session expires while using a protected page
- **WHEN** 受保护页面请求或轮询周期从同源 admin API 收到认证过期结果
- **THEN** 前端把该条件视为会话过期，清理已认证 shell 状态，并让浏览器回到 `login` 流程，而不是展示通用页面错误

### Requirement: Confirmed admin page hierarchy baseline
系统 MUST 围绕已对齐的后端合同定义已确认首批 admin 前端页面层级，且 MUST NOT 要求未支持的页面详情流程。

#### Scenario: Confirmed top-level pages are present
- **WHEN** 定义专用 admin 前端信息架构
- **THEN** 已确认顶层页面是 `login`、`dashboard`、`users`、`clients`、`proxies`、`certificates` 和 `audit`

#### Scenario: List and detail hierarchy applies only to supported resource pages
- **WHEN** 定义首批页面层级
- **THEN** `users`、`clients` 和 `proxies` 包含列表与详情页面定义，而 `dashboard`、`certificates` 和 `audit` 保持为顶层页面，除非未来规格显式添加独立详情行为

### Requirement: Admin shell navigation baseline
系统 MUST 定义扁平的首批管理员导航模型，只暴露已确认的运维区域。

#### Scenario: Navigation exposes confirmed pages only
- **WHEN** 已认证管理员使用专用前端 shell
- **THEN** 主导航暴露 `Dashboard`、`Users`、`Clients`、`Proxies`、`Certificates` 和 `Audit`，且不暴露配额、设置、告警、更完整可观测性、域名工作流、RBAC 重设计或普通用户自助入口

#### Scenario: Navigation is shell-owned rather than page-specific
- **WHEN** 专用 admin 前端渲染受保护页面
- **THEN** 已认证 shell 拥有共享导航 chrome、当前管理员上下文展示和登出入口，而不是每个页面独立重定义这些控制

### Requirement: Shared page-state semantics baseline
系统 MUST 为专用 admin 前端定义一致的页面级加载、空状态、错误和未找到行为。

#### Scenario: Protected shell preserves navigation during page loading
- **WHEN** 已认证管理员导航到仍在解析主数据的受保护页面
- **THEN** 前端保留已认证 shell 可见，并展示路由级或页面级加载状态，而不是清空整个应用框架

#### Scenario: Empty state distinguishes no-data from no-match
- **WHEN** `users`、`clients`、`proxies`、`certificates` 或 `audit` 等列表页没有可显示条目
- **THEN** 页面模型区分基线无数据状态和过滤产生的无匹配状态，使后续实现可呈现正确的操作指引

#### Scenario: Error state distinguishes auth expiry from other failures
- **WHEN** 受保护页面请求失败
- **THEN** 页面模型区分会话过期、校验或变更失败、资源未找到和意外后端失败，而不是折叠成一个通用错误状态

### Requirement: Admin page consumption model baseline
系统 MUST 定义每个已确认页面如何消费规范同源 admin API 合同。

#### Scenario: Login page uses session endpoints only
- **WHEN** 前端定义 `login` 页面行为
- **THEN** 该页面使用 `/api/admin/login` 进行登录，并使用 `/api/admin/session` 做启动或重定向决策，不依赖 GraphQL 做认证启动

#### Scenario: Dashboard uses summary-oriented GraphQL reads
- **WHEN** 前端定义 `dashboard` 页面行为
- **THEN** 该页面消费规范 dashboard GraphQL 摘要合同，并把页面建模为运行时概览，而不是资源列表/详情流程

#### Scenario: Users page uses canonical list and detail contracts
- **WHEN** 前端定义 `users` 列表和详情页
- **THEN** 列表页消费规范分页、可过滤、可排序 users 合同，详情页消费规范 user detail 合同和受支持生命周期变更

#### Scenario: Clients page uses canonical list and detail contracts
- **WHEN** 前端定义 `clients` 列表和详情页
- **THEN** 列表页消费规范运行时感知 clients 列表合同，详情页消费包含 managed-proxy 上下文的规范 client detail 合同

#### Scenario: Proxies page uses canonical list and detail contracts
- **WHEN** 前端定义 `proxies` 列表和详情页
- **THEN** 列表页消费规范 proxies 列表合同，详情页消费规范 proxy detail 合同和受支持生命周期/变更流程，且不在前端重定义监听器准入语义

#### Scenario: Certificates and audit pages use top-level page contracts
- **WHEN** 前端定义 `certificates` 和 `audit` 页面
- **THEN** 每个页面作为顶层页消费规范 GraphQL 列表或状态合同，不要求未支持的详情路由假设

### Requirement: Page-scoped polling baseline for the dedicated frontend
系统 MUST 在已确认管理员前端视图的页面级定义轮询行为，而不是通过整页 reload 或 shell 全局刷新。

#### Scenario: Runtime-oriented pages poll on active view cadence
- **WHEN** 已认证管理员在专用前端中保持 `dashboard`、`clients` 或 `proxies` 打开
- **THEN** 这些页面在保持活跃时按既定 5 秒节奏轮询相关规范 GraphQL 查询

#### Scenario: Low-churn pages avoid aggressive polling
- **WHEN** 已认证管理员使用 `users`、`audit` 或 `certificates`
- **THEN** 页面模型使用适合该页面的手动刷新或低频轮询，而不是默认继承运行时页面的 5 秒节奏

#### Scenario: Polling remains page-scoped
- **WHEN** 某个受保护页面执行轮询刷新
- **THEN** 该刷新不重置无关页面状态，也不要求完整 shell 重载，因为轮询归属保持在活跃页面模型中

### Requirement: Admin frontend scope exclusions baseline
系统 MUST 把专用 admin 前端页面模型定义限定在已确认首批管理员视图内，且 MUST NOT 暗示更广泛的管理范围。

#### Scenario: Excluded future areas stay out of current shell definition
- **WHEN** 评审首批前端 shell 和页面定义
- **THEN** 配额和设置页、告警中心行为、更完整可观测性页、域名工作流、RBAC 重设计和普通用户自助保持明确范围外，直到未来变更定义它们

#### Scenario: Future touchpoints may be noted conceptually without becoming routes
- **WHEN** 设计中提到相邻的未来前端触点
- **THEN** 它们只被记录为可能的未来扩展区域，不成为当前变更中的活跃路由、导航项或必需页面合同
