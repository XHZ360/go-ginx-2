## Why

当前后端 GraphQL 合同和基线规格已经把客户端创建、凭据一次性返回和凭据轮换列为 V1 管理能力，但专用 admin 前端的 `Clients` 页面仍只提供列表、筛选、刷新和详情查看。这个缺口会导致操作者在浏览器管理面中无法完成客户端接入准备，必须退回 CLI。

## What Changes

- 在 admin Web 面板的客户端列表页增加创建客户端入口。
- 创建客户端时通过用户选择器选择所属用户，并收集客户端名称和可选显式初始凭据；未提供凭据时由后端生成并在成功 payload 中返回一次。
- 创建成功后在当前表单 surface 内展示一次性客户端凭据，并刷新客户端列表。
- 在客户端详情页增加凭据轮换动作，轮换成功后同样只展示新凭据一次。
- 客户端列表页支持用户上下文状态 `client-userid`，通过传入用户 ID 只显示该用户的客户端。
- 客户端列表页的用户过滤和创建表单的所属用户字段使用用户选择器，选择器提交和保存的值仍为用户 ID。
- 从带用户上下文的客户端列表创建客户端时，创建表单默认选中该用户 ID。
- 在用户列表页和用户详情页增加查看该用户客户端的快捷入口，跳转到带用户上下文的客户端列表。
- 保持客户端列表和详情查询不暴露任何明文凭据。
- 不改变现有 GraphQL 后端 mutation 名称、会话/CSRF 机制或客户端 join token 流程。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `admin-resource-management`: 补齐专用 admin 前端对客户端创建、凭据轮换和用户上下文客户端列表导航合同的消费要求，并明确一次性凭据展示、默认用户 ID、刷新和错误处理语义。

## Impact

- 影响 `admin-ui` 的客户端列表、客户端详情、用户列表、用户详情、GraphQL 客户端封装、表单状态、导航状态和测试。
- 复用现有 `/api/admin/graphql` 的 `createClient`、`rotateClientCredential`、`enableClient`、`disableClient` 合同；不新增后端 API。
- 不影响 `goginx-admin create-client`、`goginx-admin create-client-join` 或 `/api/client/enroll` 运行路径。
