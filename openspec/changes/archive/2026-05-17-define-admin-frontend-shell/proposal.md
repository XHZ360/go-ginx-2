## Why

管理员后端已经具备同源、API-only 的管理面，包含专用会话端点，以及 dashboard、用户、客户端、代理、证书和审计等已确认管理域的规范化 GraphQL 合同。前端实现前仍需要一个明确的页面模型和路由 shell 设计，用于约束 admin console 的结构、守卫、导航和刷新行为，避免后续代码在实现过程中临时解释后端行为。

本变更记录专用管理员前端的页面定义层，使后续实现可以基于一个已批准的 shell、层级和页面状态模型推进，而不是在框架代码中重新发现这些决策。

## What Changes

- 定义已确认视图的专用 admin 前端信息架构：login、dashboard、users、clients、proxies、certificates 和 audit。
- 定义同源 admin console 的认证路由 shell、受保护路由行为、页面层级和导航模型。
- 定义页面级加载、空状态、错误和会话过期行为，使后续前端工作在各页面保持一致 UX 语义。
- 定义各页面的轮询预期，以及列表/详情页如何消费 `/api/admin/graphql` 上已经对齐的规范 GraphQL 合同。
- 明确排除未来领域，避免此前端页面结构变更扩展到配额/设置、告警、更完整可观测性、域名工作流、RBAC 重设计或普通用户自助。

## Capabilities

### New Capabilities
- 无。

### Modified Capabilities
- `admin-resource-management`：定义专用 admin 前端页面模型和同源路由 shell，用于消费现有会话与 GraphQL 合同，并覆盖已确认管理员视图。

## Impact

- 受影响系统：后续 admin 前端应用结构、同源路由处理、会话启动与路由守卫、页面级 GraphQL 查询规划。
- 受影响代码区域：前端路由、导航 chrome、页面容器、GraphQL 查询接线和 admin UX 状态处理。
- 依赖和外部系统：依赖已实现的 `/api/admin/login`、`/api/admin/session`、`/api/admin/logout` 和 `/api/admin/graphql` 后端合同基线。
