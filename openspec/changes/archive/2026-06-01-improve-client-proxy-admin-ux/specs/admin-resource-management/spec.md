## ADDED Requirements

### Requirement: Administrator client-to-proxy creation frontend baseline
系统 MUST 在专用 admin 前端中支持从客户端上下文进入代理创建流程，并在代理创建表单中默认携带可修改的用户和客户端上下文。

#### Scenario: Client shortcut opens proxy creation with context
- **WHEN** 已认证管理员在 `Clients` 列表或 `ClientDetail` 页面触发为某个客户端创建代理的入口
- **THEN** 前端导航到代理创建入口，并携带该客户端的 `clientId` 以及所属 `userId` 作为创建代理默认上下文

#### Scenario: Proxy create form defaults selectors from client context
- **WHEN** 已认证管理员从客户端上下文打开代理创建表单
- **THEN** 代理创建表单的用户选择器默认选中携带的 `userId`，客户端选择器默认选中携带的 `clientId`，并在提交时把选择后的 ID 传给 `createProxy` mutation

#### Scenario: Proxy create selectors remain editable
- **WHEN** 已认证管理员在代理创建表单中修改用户或客户端选择器
- **THEN** 前端使用管理员最终选择的 `userId` 和 `clientId` 创建代理，而不是强制保留跳转时携带的默认值

#### Scenario: Client selector stays consistent with selected user
- **WHEN** 已认证管理员在代理创建表单中改变用户选择器，且当前客户端不属于新选择的用户
- **THEN** 前端清空或重新要求选择客户端，避免提交不一致的用户和客户端组合

#### Scenario: Proxy create context survives refresh
- **WHEN** 已认证管理员直接打开、刷新或登录后恢复带有 `clientId` 与 `userId` 的代理创建 URL
- **THEN** 前端从 URL 或路由状态恢复默认上下文，并在候选选项尚未加载完成时保留 ID fallback

### Requirement: Shared administrator modal interaction baseline
系统 MUST 在专用 admin 前端中为所有弹窗提供统一的视口约束、内容滚动、token 展示和蒙版关闭语义，避免内容不可达或误关闭正在操作的弹窗。

#### Scenario: Modal content scrolls within viewport
- **WHEN** 已认证管理员打开任意 admin-ui 弹窗，且弹窗内容高度超过当前视口可用高度
- **THEN** 弹窗容器保持在视口最大高度内，弹窗内容区域在容器内部滚动，使标题、主体和可用操作不会因为溢出而不可访问

#### Scenario: Token area scrolls after three lines
- **WHEN** 弹窗或动作结果 surface 展示 join token、客户端凭据、secret 或类似长文本，且内容超过 3 行
- **THEN** 该 token 区域最多显示 3 行高度，超出内容在 token 区域内部滚动，而不是撑高整个弹窗

#### Scenario: Overlay click closes only when full click stays on overlay
- **WHEN** 管理员在弹窗蒙版层按下鼠标并在同一蒙版层释放鼠标
- **THEN** 前端可以关闭当前弹窗

#### Scenario: Drag release from dialog to overlay does not close
- **WHEN** 管理员在弹窗内容内部按下鼠标，随后在蒙版层释放鼠标
- **THEN** 前端 MUST NOT 把该释放动作视为蒙版完整点击，也不得关闭当前弹窗

#### Scenario: Dialog internal interaction does not trigger overlay close
- **WHEN** 管理员在弹窗内部点击、选择文本、拖动滚动条或操作表单控件
- **THEN** 前端保持弹窗打开，除非管理员触发显式关闭、取消、提交成功或完整蒙版点击关闭动作

## MODIFIED Requirements

### Requirement: Administrator client-management baseline
系统 MUST 提供管理员专用客户端列表、详情、创建、凭据轮换和完全删除合同，其中凭据处理为一次性返回，删除处理为永久资源销毁。

#### Scenario: List clients
- **WHEN** 已认证管理员查询 V1 管理面中的客户端
- **THEN** 系统返回适合管理员控制 console 的分页、可过滤、可排序客户端列表数据

#### Scenario: View client detail with runtime state
- **WHEN** 已认证管理员查看客户端详情
- **THEN** 系统组合持久化客户端数据与当前可用运行时/会话状态，并包含该客户端管理的代理上下文

#### Scenario: Create or rotate client credential returns secret once
- **WHEN** 已认证管理员创建客户端或轮换客户端凭据
- **THEN** 新凭据可以在该变更 payload 中精确返回一次，后续列表或详情查询 MUST NOT 暴露该凭据

#### Scenario: Delete client with no enabled proxies
- **WHEN** 已认证管理员请求完全删除某个客户端，且该客户端不存在已启用代理
- **THEN** 系统在一个受控变更中永久删除该客户端、客户端凭据和允许级联清理的从属客户端资源，记录审计事件，并使后续列表、详情、认证和 join/enrollment 消费都不能再使用该客户端

#### Scenario: Delete client rejects enabled proxy dependency
- **WHEN** 已认证管理员请求完全删除某个客户端，但该客户端仍存在已启用代理
- **THEN** 系统以可由前端消费的 `CONFLICT` 或等价结构化语义拒绝删除，并说明必须先禁用相关代理

#### Scenario: Delete missing client returns not found
- **WHEN** 已认证管理员请求完全删除不存在或已删除的客户端
- **THEN** 系统返回可由前端消费的 `NOT_FOUND` 语义，而不是报告删除成功

#### Scenario: Delete client errors remain scoped
- **WHEN** 客户端删除 mutation 返回认证、授权、冲突、不存在、校验或后端失败语义
- **THEN** 前端在当前客户端列表、详情或确认弹窗动作 surface 内展示对应失败，不丢弃当前已加载的页面内容
