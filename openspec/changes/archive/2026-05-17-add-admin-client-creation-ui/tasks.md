## 1. GraphQL 前端合同

- [x] 1.1 在 `admin-ui/src/lib/admin-graphql.ts` 增加 `mutateCreateClient`，调用现有 `createClient` mutation 并返回 `clientId`、`status`、`credential` 和客户端摘要字段。
- [x] 1.2 在 `admin-ui/src/lib/admin-graphql.ts` 增加 `mutateRotateClientCredential`，调用现有 `rotateClientCredential` mutation 并返回 `clientId`、`status`、`credential` 和客户端摘要字段。
- [x] 1.3 确认 `queryClients` 和 `queryClient` 不请求 credential 字段，保持凭据只在 mutation payload 中出现。
- [x] 1.4 复用现有 `queryUsers`/users 列表合同为客户端用户选择器提供候选用户，不新增后端用户查找 API。

## 2. 客户端创建 UI

- [x] 2.1 在 `ClientsPage` 页头增加 `Create client` 入口，并使用现有 `Dialog`、`FormField`、`ValidationBanner` 和 `useMutationWithAuth` 模式实现创建表单。
- [x] 2.2 为 `ClientsPage` 的用户过滤增加用户选择器，选择器展示用户名和用户 ID，选择后把用户 ID 写入 `filter.userId` 并刷新客户端列表。
- [x] 2.3 让 `ClientsPage` 从 `/clients?userId=<id>` 初始化用户选择器和 `filter.userId`，并在该状态下只请求和显示当前用户的客户端。
- [x] 2.4 处理深链用户 ID 尚未出现在选择器候选项中的情况，保留该 ID 并以 ID fallback 显示当前选择。
- [x] 2.5 创建表单使用用户选择器收集所属用户 ID，并收集 `name` 和可选 `credential`；当列表带有用户上下文时，默认选中该用户 ID。
- [x] 2.6 把结构化校验错误限制在创建表单 surface 内展示。
- [x] 2.7 创建成功后刷新 `clients` 查询，并在创建结果 surface 中展示一次性 credential 与只显示一次提示。
- [x] 2.8 创建表单关闭或再次打开时清理旧的字段错误、表单错误和一次性 credential 状态，并按当前用户上下文重新计算默认用户选择。

## 3. 用户到客户端快捷导航

- [x] 3.1 在 `UsersPage` 为每个用户增加查看客户端快捷入口，导航到 `/clients?userId=<id>`，并避免与现有行点击进入详情行为冲突。
- [x] 3.2 在 `UserDetailPage` 操作区增加查看该用户客户端快捷入口，导航到 `/clients?userId=<id>`。
- [x] 3.3 确认登录后目标恢复和安全路由清洗允许 `/clients?userId=<id>` 保留查询参数。

## 4. 客户端详情轮换 UI

- [x] 4.1 在 `ClientDetailPage` 页头增加凭据轮换确认动作，复用现有确认交互模式。
- [x] 4.2 轮换成功后刷新当前 `client` 详情和 `clients` 列表查询，并在详情页动作 surface 中展示一次性新 credential。
- [x] 4.3 轮换失败时在详情页动作 surface 内展示错误，同时保留当前已加载的客户端详情内容。

## 5. 测试与校验

- [x] 5.1 增加或更新 admin-ui 测试，覆盖客户端创建成功、创建校验失败、创建后一次性 credential 展示和列表刷新。
- [x] 5.2 增加或更新 admin-ui 测试，覆盖客户端用户选择器选项展示、选择后过滤、`/clients?userId=<id>` 初始化、创建表单默认选择、ID fallback 和清除过滤恢复全局视图。
- [x] 5.3 增加或更新 admin-ui 测试，覆盖用户列表和用户详情页的查看客户端快捷跳转。
- [x] 5.4 增加或更新 admin-ui 测试，覆盖客户端凭据轮换成功、轮换失败、详情内容保留和一次性 credential 展示。
- [x] 5.5 运行 admin-ui 测试和 OpenSpec 校验，确认变更满足规格并且没有破坏现有 Users/Proxies mutation 流程。
