## Why

admin 后端已经提供同源 API-only 管理面，包含专用会话端点和面向已确认管理员视图的会话认证 GraphQL 合同。前端 shell 与页面层级也已定义，包括受保护路由集合、根路由方向和页面级职责。

剩余缺口是把该模型落成可运行的前端基础：应用工作区、请求层、会话启动流程、共享页面状态、页面级轮询归属，以及本地开发和生产同源交付集成。没有这层基础，页面实现会在根路径交付、路由守卫、登录后目标恢复、CSRF 感知 GraphQL 变更、加载/失败语义、页面刷新归属和同源构建集成上反复做横切决策。

## What Changes

- 建立 `admin-ui` 前端工作区，采用 React、React Router、TanStack Query、Vite 和 TypeScript。
- 实现 admin origin 根路径交付模型，保留 `/api/admin/*` 作为后端 API 命名空间。
- 实现按认证状态处理的根路由、受保护路由启动、`/login` 行为和登录后目标恢复。
- 实现 `/api/admin/login`、`/api/admin/session`、`/api/admin/logout` 与 `/api/admin/graphql` 的共享请求模型，包括 CSRF 变更令牌处理。
- 实现共享页面状态、受保护 shell、页面容器、页面级轮询和变更后定向刷新。
- 实现 dashboard、users、clients、proxies、certificates 和 audit 的首批页面容器。
- 增加前端会话/导航测试，并通过 Vite 开发代理保持本地同源 API 语义。

## Capabilities

### New Capabilities
- 无独立新能力；本变更把既有 `admin-resource-management` 合同落成专用前端基础。

### Modified Capabilities
- `admin-resource-management`：定义并实现专用 admin UI 的前端基础和浏览器行为模型，用于消费既有同源会话与 GraphQL 合同。

## Impact

- 受影响系统：admin 前端应用结构、浏览器会话启动与守卫导航、前端 GraphQL transport、根路径路由处理、同源交付集成。
- 受影响代码区域：`admin-ui` 工作区、路由入口、认证和请求层、共享页面状态组件、前端构建集成、admin listener 静态路由协调。
- 依赖和外部系统：依赖既有 `/api/admin/login`、`/api/admin/session`、`/api/admin/logout` 和 `/api/admin/graphql` 后端合同，以及此前定义的 admin 前端 shell 和页面层级。

## Explicitly Excluded

- 重设计后端会话合同、GraphQL schema 或 admin command/query 服务边界。
- 扩展到配额、设置、告警、更完整可观测性、RBAC 重设计或普通用户自助。
- 引入实时订阅或 shell 全局后台刷新。
- 把最终视觉设计语言或精修 UI 表现作为本变更的主要关注点。
