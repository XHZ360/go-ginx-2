## Context

admin 后端已经暴露同源浏览器合同：`POST /api/admin/login`、`GET /api/admin/session`、`POST /api/admin/logout` 和 `POST /api/admin/graphql`。admin 前端 shell 变更也已经定义了 login、dashboard、users、clients、proxies、certificates 和 audit 的确认路由集合、受保护 shell 边界、共享页面状态语义和页面级轮询预期。

本变更把该模型落实为前端基础架构。该 admin UI 是服务端状态密集型管理 console，而不是离线优先本地应用，因此架构 MUST 把 admin API 视为事实来源，把 shell 关注点与资源页面关注点分离，并避免把所有业务资源集中进一个浏览器全局 store。

## Goals / Non-Goals

**目标：**

- 在同一 admin origin 下实现专用 admin UI 的前端应用基础。
- 实现 shell、路由入口和会话启动边界，使硬加载、深链和登录后导航行为一致。
- 实现页面容器对 `/api/admin/graphql` 的消费，而登录、登出和会话启动保持在专用 admin HTTP 端点上。
- 明确状态模型：shell 会话状态、页面拥有的服务端状态，以及本地 UI 状态。
- 实现页面级轮询和变更后定向刷新，保留规范 admin 页面模型。
- 定义并落地本地开发和生产交付形态，使前端资源与 `/api/admin/*` 共存于一个外部 origin。

**非目标：**

- 不重设计后端会话模型、GraphQL schema 或 admin read/command 服务边界。
- 不引入 shell 全局轮询、实时订阅、离线同步或全应用业务数据 store。
- 不扩展到配额、设置、告警、更完整可观测性、RBAC 重设计或普通用户自助。
- 不把最终视觉设计语言作为本变更重点。

## Decisions

1. 使用一个受保护 shell，拥有会话状态、导航 chrome 和路由级认证处理，但不拥有资源页面业务数据。
   - 决策：认证 shell 负责当前管理员上下文、登出入口、路由启动和认证过期处理；页面容器负责自己的资源查询、变更和刷新行为。
   - 理由：shell 关注点是横切且稳定的，而 dashboard、users、clients、proxies、certificates 和 audit 的查询生命周期不同，不应通过全局资源 store 耦合。

2. 只把会话状态作为必需全局应用状态。
   - 决策：全局前端状态仅包含会话启动状态、已认证管理员身份、CSRF token 和最小目标恢复上下文。业务数据保持页面级服务端状态。
   - 理由：admin console 由后端事实驱动，不需要为首批 users、clients、proxies、certificates 或 audit 建立大型客户端领域 store。

3. 页面容器是主要数据消费边界。
   - 决策：每个已确认路由映射到页面容器，由容器拥有规范 GraphQL 读取、变更、空/错误状态、轮询节奏和变更后刷新逻辑。
   - 理由：保持资源关注点与页面模型一致，避免共享布局代码吸收资源特定行为。

4. 服务端状态和本地 UI 状态分离。
   - 决策：GraphQL 响应和会话启动结果是服务端状态；过滤器、排序、分页、对话框状态和表单输入是本地页面状态。
   - 理由：防止 UI 关注点泄漏进 transport 缓存，使页面刷新行为可预测。

5. 轮询保持页面级，只在对应页面活跃时执行。
   - 决策：dashboard、clients 和 proxies 采用规范 5 秒节奏；certificates 使用低频或动作驱动刷新；users 和 audit 使用手动或低频刷新；login 不轮询。
   - 理由：后端合同和页面模型已经预期不同新鲜度，页面级归属避免无关 reload 或 shell-wide timer。

6. 使用变更后定向刷新，而不是完整 shell 或完整路由 reload。
   - 决策：成功变更后，根据当前页面上下文重新查询受影响详情、受影响列表或两者；架构 MUST NOT 依赖浏览器 reload 同步页面状态。
   - 理由：保留当前导航上下文、过滤状态和 shell 连续性，同时显式保持服务端状态新鲜。

7. 在小型请求层集中 transport 关注点，并分离会话端点和 GraphQL 业务流量。
   - 决策：前端基础提供会话客户端处理 login/session/logout，提供 GraphQL transport 处理资源读写，并包含 CSRF 变更令牌和共享错误转换。
   - 理由：认证生命周期流量与资源管理流量关注点不同，不应散落在各页面的 ad hoc fetch 调用中。

8. 围绕规范后端语义标准化前端错误处理。
   - 决策：`UNAUTHENTICATED`、`FORBIDDEN`、`VALIDATION_FAILED`、`NOT_FOUND`、`CONFLICT`、`UNSUPPORTED`、`ENTRY_CONFLICT`、`INVALID_CSRF` 和 `INTERNAL` 是页面容器映射到会话过期、表单校验、未找到、冲突或通用失败状态的稳定类别。
   - 理由：后端已经定义前端可消费错误语义，前端基础应保留这些区别，而不是折叠成通用异常。

9. 本地开发与生产均保持同源交付语义。
   - 决策：生产在 admin origin 下服务专用前端，并保留 `/api/admin/*` 给后端 API；本地开发通过 Vite dev server proxy 或等价组合保持浏览器视角下的同源 API 合同。
   - 理由：admin 后端合同和 CSRF 模型有意是同源，开发环境应尽量贴近该合同。

## Architecture Shape

```text
Browser
  |
  v
Admin Frontend Router
  |- Public Route Group
  |  `- /login
  `- Protected Shell
     |- Session Bootstrap
     |- Navigation Chrome
     |- Current Administrator Context
     |- Logout Action
     |- Route-Level Auth Expiry Handling
     `- Page Containers
        |- Dashboard
        |- Users List / Detail
        |- Clients List / Detail
        |- Proxies List / Detail
        |- Certificates
        `- Audit
           |
           v
        Request Layer
        |- Session Client
        |- GraphQL Client
        |- CSRF Mutation Handling
        `- Error Mapping
           |
           v
        Same-Origin Admin API
        |- /api/admin/login
        |- /api/admin/session
        |- /api/admin/logout
        `- /api/admin/graphql
```

## State Model

前端基础使用三类状态。

1. **Shell 会话状态**
   - 拥有 `unknown`、`checking`、`authenticated` 和 `unauthenticated` 路由入口状态。
   - 只存储当前管理员身份、CSRF token、启动状态和目标恢复上下文。
   - 位于受保护 shell 边界。

2. **页面拥有的服务端状态**
   - 拥有 dashboard 摘要、分页列表、详情视图和变更刷新结果。
   - 与 `/api/admin/graphql` 上的规范合同对齐。
   - 按页面活跃状态刷新，而不是由 shell 全局编排。

3. **本地 UI 状态**
   - 拥有过滤输入、排序选择、分页控制、对话框状态和进行中的表单。
   - 可丢弃且作用域在页面内。

## Page Ownership Model

- `login`
  - 只使用 `/api/admin/login` 和 `/api/admin/session`。
  - 拥有成功认证后的目标恢复。
- `dashboard`
  - 拥有 dashboard 摘要查询和 5 秒轮询。
- `users` 和 `users/:id`
  - 拥有列表/详情查询，以及创建、禁用和密码修改动作。
- `clients` 和 `clients/:id`
  - 拥有运行时感知列表/详情查询和页面级轮询。
- `proxies` 和 `proxies/:id`
  - 拥有列表/详情查询，以及创建、更新、生命周期、禁用后删除和显式 `ENTRY_CONFLICT` 处理。
- `certificates`
  - 拥有生命周期导向列表/状态读取，以及签发和续期动作。
- `audit`
  - 拥有近期事件列表读取，以及轻量过滤和低频或手动刷新。

## Delivery Model

1. 生产交付
   - admin origin 从专用前端服务 `/`、`/login`、`/dashboard`、`/users`、`/clients`、`/proxies`、`/certificates` 和 `/audit` 等浏览器路由。
   - `/api/admin/*` 保留给后端 API 行为。
   - 前端范围内未知浏览器路由由前端路由模型处理，未知 API 路径保持 API not-found 行为。

2. 本地开发交付
   - 前端可以通过开发服务器运行，但浏览器对 `/api/admin/*` 的请求 MUST 通过代理或等价组合呈现同源语义。
   - 开发 MUST NOT 依赖与生产不同的跨源认证或 CSRF 行为。

## Risks / Trade-offs

- [Risk] 框架选择仍可能造成实现漂移。 -> Mitigation: 路由、transport、状态、轮询和交付边界已经定义为框架无关约束。
- [Risk] 页面拥有服务端状态会在页面间产生少量查询接线重复。 -> Mitigation: 只共享 transport、错误和表格/状态原语，资源查询归属保持在页面容器。
- [Risk] 变更后定向刷新可能在不同资源上不一致。 -> Mitigation: 按页面定义刷新预期，并通过实现验收验证。
- [Risk] 本地同源开发如果被推迟，会造成后期集成意外。 -> Mitigation: 本变更已把开发代理纳入前端基础。
- [Risk] 会话过期仍可能漏进通用页面错误。 -> Mitigation: 在请求层集中错误映射，并把认证过期结果路由回 shell。

## Migration Plan

1. 在此架构边界内选择前端工作区和框架，不改变已批准路由或合同模型。
2. 先实现路由 shell、路由入口处理和会话启动流程。
3. 实现会话端点、GraphQL 流量、CSRF 感知变更和规范错误映射请求层。
4. 实现共享 shell 和页面状态原语，再接入 dashboard 与一个列表/详情资源对验证模型。
5. 使用页面拥有查询、变更和轮询实现剩余资源页。
6. 集成本地开发和生产同源交付，使前端路由与 `/api/admin/*` 共存于一个 origin。
7. 增加启动、登录重定向、认证过期、页面状态、轮询归属和变更后定向刷新覆盖。

## Open Questions

- 查询缓存库可作为实现选择，只要支持页面拥有服务端状态、定向失效和页面级轮询，而不强制全局业务数据 store。
