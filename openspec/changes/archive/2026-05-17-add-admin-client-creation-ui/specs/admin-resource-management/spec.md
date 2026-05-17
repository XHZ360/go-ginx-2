## ADDED Requirements

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
