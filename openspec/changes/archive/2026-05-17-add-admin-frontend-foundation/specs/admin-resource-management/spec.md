## ADDED Requirements

### Requirement: Admin frontend route-entry and guard semantics
系统 MUST 为交付在 admin origin 上的专用管理员前端保持规范路由入口和会话守卫行为。

#### Scenario: Root route redirects by authentication state
- **WHEN** 浏览器请求专用管理员前端根路由 `/`
- **THEN** 前端通过专用会话启动端点解析管理员会话状态，并把已认证管理员重定向到 `/dashboard`，把未认证访问者重定向到 `/login`

#### Scenario: Login is the only public frontend route
- **WHEN** 浏览器在没有有效管理员会话的情况下请求专用管理员前端路由
- **THEN** `/login` 是唯一公开前端页面，其他管理员前端路由在会话校验完成前 MUST NOT 渲染受保护页面内容

#### Scenario: Authenticated visit to login redirects to dashboard
- **WHEN** 已认证管理员请求 `/login`
- **THEN** 前端把该管理员重定向到 `/dashboard`，而不是再次渲染登录表单

#### Scenario: Protected routes bootstrap before rendering content
- **WHEN** 浏览器加载、刷新或直接打开 `/dashboard`、`/users`、`/clients/:id` 或 `/proxies/:id` 等受保护管理员前端路由
- **THEN** 前端通过专用会话启动端点校验当前管理员会话，然后才渲染受保护的管理员资源内容

#### Scenario: Protected deep link restores after successful login
- **WHEN** 未认证访问者从有效受保护管理员前端路由被重定向到 `/login`
- **THEN** 成功管理员认证后，前端恢复该预期受保护目标

#### Scenario: Invalid intended destination falls back safely
- **WHEN** 登录后的预期目标缺失、不安全、不受支持或不是有效受保护管理员前端路由
- **THEN** 前端把管理员重定向到 `/dashboard`，而不是跟随无效目标

### Requirement: Admin frontend page-state semantics
系统 MUST 在专用管理员前端中保持共享页面状态语义，使受保护路由行为、列表视图、详情视图和刷新行为一致。

#### Scenario: Authentication expiry remains distinct from generic backend failure
- **WHEN** 受保护管理员前端查询或变更因为管理员会话缺失、过期或无效而失败
- **THEN** 前端把该结果视为认证过期行为，并把浏览器带回登录流程，而不是只展示通用后端失败状态

#### Scenario: List pages distinguish baseline empty state from filtered empty state
- **WHEN** 已认证管理员使用 `users`、`clients`、`proxies`、`certificates` 或 `audit` 等列表页
- **THEN** 前端区分资源集合未填充的基线空状态和当前过滤或搜索范围导致的过滤空状态

#### Scenario: Detail pages distinguish missing resource from generic backend failure
- **WHEN** 已认证管理员打开 `users/:id`、`clients/:id` 或 `proxies/:id` 等有效详情路由
- **THEN** 前端区分托管资源缺失和通用后端失败，而不是把两者折叠成一个通用错误状态

#### Scenario: Runtime summary pages do not use empty-state semantics for zero-value summaries
- **WHEN** 已认证管理员查看 dashboard 摘要，且当前可信聚合均为零值
- **THEN** 前端把 dashboard 摘要渲染为带零值字段的内容，而不是替换为通用空状态视图

#### Scenario: Validation failure remains scoped to the active form or action surface
- **WHEN** 专用 console 创建、更新或生命周期动作以结构化校验语义失败
- **THEN** 前端在当前表单或动作 surface 内展示该校验失败，而不是把整页折叠成通用错误状态

#### Scenario: Polling remains scoped to the active page
- **WHEN** 管理员前端通过轮询刷新运行时导向视图
- **THEN** 轮询保持在活跃页面上下文内，而不是作为一个 shell 全局刷新循环重新加载无关页面数据

## MODIFIED Requirements

### Requirement: API-only administrator browser surface baseline
系统 MUST 在引入基于会话认证的 admin surface 后停止服务旧的服务端渲染管理员 UI，并在专用管理员前端引入时允许该前端拥有浏览器侧管理员路由。

#### Scenario: Server-rendered administrator routes are removed
- **WHEN** 基于会话认证的管理员 API surface 被引入
- **THEN** 旧的服务端渲染管理员页面和浏览器表单处理器不再服务

#### Scenario: Dedicated frontend routes replace transitional browser not-found behavior
- **WHEN** 专用管理员前端被引入到 admin origin，且浏览器请求 `/`、`/login`、`/dashboard`、`/users`、`/clients`、`/proxies`、`/certificates` 或 `/audit` 等前端路径
- **THEN** 这些浏览器侧路径由专用管理员前端路由模型处理，而不是返回前端存在前的过渡性 `404 Not Found` 行为；管理员 API 行为仍保留在 `/api/admin/*` 等显式命名路径下

#### Scenario: Separated-console API routes use administrator session auth
- **WHEN** 专用 admin 前端调用新的同源 admin API 路由
- **THEN** 这些路由通过服务端管理的会话模型认证管理员，而不是重复 Basic Auth 提示

#### Scenario: Legacy browser-facing GraphQL route is removed
- **WHEN** 基于会话认证的管理员 API surface 被引入
- **THEN** 旧的浏览器侧 `POST /graphql` 路由不再服务管理员浏览器访问，浏览器客户端改用基于会话认证的 GraphQL 入口
