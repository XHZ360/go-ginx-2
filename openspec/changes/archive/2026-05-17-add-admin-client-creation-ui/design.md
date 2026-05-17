## Context

当前 admin 后端已经通过 `/api/admin/graphql` 暴露 `createClient` 和 `rotateClientCredential`，命令服务负责持久化客户端、生成或保存凭据、记录审计事件，并保证列表/详情查询不返回明文凭据。React admin 前端已经在 Users 和 Proxies 页面具备创建表单、结构化校验错误展示、CSRF mutation 和 mutation 后局部刷新模式；Clients 页面目前只实现只读列表、过滤、轮询和详情，并且已有 `userId` 过滤输入。

这个变更应当把客户端管理前端补齐到现有后端合同，而不是重新设计客户端注册、join token 或运行时认证。

## Goals / Non-Goals

**Goals:**

- 在 `Clients` 列表页提供创建客户端表单，并复用现有 session、CSRF、GraphQL client 和结构化错误处理模式。
- 在 `Clients` 列表页为 `userId` 过滤和创建表单所属用户提供用户选择器，减少手输用户 ID。
- 支持从用户列表和用户详情进入用户上下文客户端列表，只显示指定用户的客户端。
- 在用户上下文客户端列表中创建客户端时，默认使用传入的用户 ID。
- 创建成功后展示一次性返回的客户端凭据，并刷新客户端列表。
- 在 `ClientDetail` 页面提供凭据轮换动作，轮换成功后展示一次性新凭据并刷新详情/列表。
- 保证列表和详情查询继续不显示凭据材料。
- 为创建和轮换路径补充前端测试覆盖。

**Non-Goals:**

- 不新增后端 GraphQL mutation 或 HTTP endpoint。
- 不新增后端客户端过滤能力；本次复用已有 clients `filter.userId` 合同。
- 不把 `create-client-join` / join token 生成流程迁入本次 UI。
- 不实现客户端删除、编辑名称、RBAC 重设计或批量操作。
- 不改变 CLI、客户端守护进程、控制通道认证或 enrollment 行为。

## Decisions

### 复用现有 GraphQL client mutation 模式

前端新增 `mutateCreateClient` 和 `mutateRotateClientCredential` 封装，形状与 `mutateCreateUser`、`mutateCreateProxy` 一致，继续通过 `useMutationWithAuth` 注入认证失败处理和 CSRF token。

替代方案是直接在页面中内联 GraphQL 字符串；这会让 `admin-graphql.ts` 的查询/变更集中封装模式变差，也更难复用类型。

### 创建表单放在客户端列表页

创建客户端是集合级动作，入口放在 `Clients` 页头，与 Users/Proxies 的创建入口一致。表单字段保持最小：所属用户、`name`、可选 `credential`。所属用户通过用户选择器产生 `userId`，后端已经支持未提供 credential 时生成凭据，因此前端不需要本地生成 secret。

替代方案是在用户详情页创建客户端，但当前用户详情页没有关联客户端管理区域；这会扩大范围并引入页面结构调整。

### userId 交互使用用户选择器，状态仍保存用户 ID

Clients 页面中的 `userId` 过滤和创建表单所属用户字段使用同一类用户选择器。选择器展示适合管理员识别的用户信息，例如用户名和用户 ID，但提交给 clients 查询和 `createClient` mutation 的值仍是用户 ID。选择器数据复用现有 `users` GraphQL 列表合同，不为本次变更新增用户查找 API。

从 `/clients?userId=<id>` 进入时，即使对应用户详情尚未加载到选择器选项，前端也必须保留该 ID 作为当前过滤状态，并在选择器中以 ID fallback 表示，避免深链状态被丢弃。

替代方案是继续使用自由文本 `userId` 输入；这会让用户上下文快捷入口之外的创建流程仍然依赖操作者记住 ID，和本次管理面易用性目标不一致。

### 用户上下文使用 URL 查询参数承载

把用户上下文状态 `client-userid` 表示为 `/clients?userId=<id>`。这样从用户列表或详情页跳转后，刷新、复制链接、登录后恢复目标地址时都能保留过滤上下文。`ClientsPage` 初始化时从查询参数填充 `filter.userId`，查询 GraphQL 时继续使用现有 `ClientFilter.userId`，创建表单打开时用同一个值预填 `userId`。

替代方案是使用 React Router location state；它不能可靠跨刷新、复制链接或会话恢复，不适合管理面深链。

### 用户页面只提供快捷跳转，不复制客户端列表

用户列表行和用户详情页操作区增加“Clients”类快捷入口，目标是 `/clients?userId=<id>`。用户页面不内嵌客户端列表、不承担客户端创建表单，这样客户端管理行为仍集中在 Clients 页面。

替代方案是在用户详情页嵌入客户端子表；这会引入重复列表状态、轮询和创建表单，超出当前补齐范围。

### 一次性凭据用 mutation 成功态展示

创建或轮换成功后，页面在当前 dialog 或详情页动作区域显示成功 payload 中的 `credential`，并使用明确文案提示该值只显示一次。后续列表和详情查询不请求 credential 字段。

替代方案是把凭据写入持久前端状态或通知中心；这会增加泄露面，也与“一次性返回”的合同相冲突。

### 轮换入口放在客户端详情页

凭据轮换是具体客户端生命周期动作，放在 `ClientDetail` 页头操作区，并使用确认动作避免误触。成功后刷新 `client` 与 `clients` query。

替代方案是在列表每行加入轮换按钮；当前 Clients 表格以整行跳转为主，行内敏感动作会增加误操作风险。

## Risks / Trade-offs

- 一次性凭据可能因用户关闭 dialog 或刷新页面而丢失 -> 成功态文案明确提示只显示一次；丢失后需要再次轮换。
- 纯文本展示凭据有复制便利性与泄露风险 -> 仅在 mutation 成功后的局部 surface 展示，不写入列表、详情或长期缓存。
- 用户选择器依赖 users 列表合同，极大用户集可能需要分页或搜索策略 -> 本次复用现有 users 分页/过滤能力，保持选择器查询范围受控。
- URL 中的 `userId` 可能无效或对应用户不存在 -> 复用现有 clients 过滤查询和空状态，不在前端额外阻断。
- 后端 payload 字段必须与前端封装一致 -> 用前端测试覆盖 create/rotate 成功和校验失败路径。
