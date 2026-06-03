## MODIFIED Requirements

### Requirement: Local admin TUI client setup
系统 MUST 通过 TUI 支持快速配置客户端，并优先通过用户选择、生成凭据和统一解析的默认 join 参数减少手动输入。TUI 展示和提交的默认 join 参数 MUST 来自与 admin CLI/server 配置兼容的默认 join 解析结果。

#### Scenario: Quick create client join token with selected owner
- **WHEN** 操作者在 TUI 默认客户端快速向导中从现有用户列表选择所属用户，输入有效客户端名称，并确认加入令牌参数
- **THEN** 系统创建客户端和加入令牌，并在结果页展示 token
- **AND** 结果页展示可在管理端终端执行的 `goginx-admin client-join-command -client <id>` 指令，用于获取客户端 join 指令

#### Scenario: Review active client join token after creation
- **WHEN** 操作者从 TUI 客户端操作菜单选择查看 join token，且该客户端存在未使用且未过期的 join token
- **THEN** 系统再次展示该 token，并提示该 token 仍只能被客户端消费一次
- **AND** 结果页展示可在管理端终端执行的 `goginx-admin client-join-command -client <id>` 指令，用于获取客户端 join 指令

#### Scenario: Reset unavailable client join token on review
- **WHEN** 操作者从 TUI 客户端操作菜单选择查看 join token，且该客户端没有可查看的可用 join token
- **THEN** 系统使用统一解析的默认 join 参数轮换客户端凭据，生成新的未使用 join token，并展示新 token、过期时间和单次消费提示
- **AND** 旧的已过期或已使用 join token 仍不可被客户端消费

#### Scenario: Reject reviewing unavailable client join token
- **WHEN** 操作者尝试查看 join token，但系统无法使用默认 join 参数生成替代 token
- **THEN** TUI 展示可操作错误，并引导操作者重新生成 join token

#### Scenario: Create client credential from secondary path
- **WHEN** 操作者在 TUI 客户端配置中选择仅创建客户端凭据，从现有用户列表选择所属用户，并输入有效客户端名称
- **THEN** 系统为该用户创建客户端记录，生成或保存客户端凭据，并在结果页展示新凭据

#### Scenario: Client owner uses selection instead of manual ID
- **WHEN** TUI 需要客户端所属用户
- **THEN** 系统展示现有用户选项供操作者选择，并把选择结果作为用户 ID 提交给客户端配置流程

#### Scenario: Client credential defaults to generated value
- **WHEN** 操作者创建客户端凭据且未选择手动输入凭据
- **THEN** 系统使用服务层生成的客户端凭据，而不是要求操作者手动填写 secret

#### Scenario: Join token defaults are reviewable before submit
- **WHEN** 操作者创建客户端加入令牌
- **THEN** TUI 展示从 server 配置、环境覆盖或 managed 默认配置统一解析出的 enrollment URL、控制通道地址、TLS 地址、server name、CA 文件和 TTL 默认值，并要求操作者在提交前确认或编辑

#### Scenario: Reject client setup when no owner exists
- **WHEN** SQLite 中不存在可作为客户端所属者的用户，且操作者进入客户端配置
- **THEN** TUI 阻止创建客户端，并引导操作者先创建用户

#### Scenario: Reject invalid client setup input
- **WHEN** 操作者提交空客户端名称、无效用户选择、空 join 必填地址、缺失 CA 文件或服务层返回的校验错误
- **THEN** TUI 在当前客户端流程展示错误，并且不创建无效客户端或加入令牌

#### Scenario: Client secrets display policy distinguishes credentials and join tokens
- **WHEN** 客户端凭据创建或轮换成功
- **THEN** TUI 仅在当前结果页展示凭据，并提示离开后需要重新创建或轮换才能再次获得明文值
- **AND** join token 在未使用且未过期期间可以由管理员重复查看

### Requirement: Local admin TUI client maintenance
系统 MUST 通过 TUI 支持客户端启用、禁用、凭据轮换、join token 查看和受保护删除。TUI 重置不可用 join token 时 MUST 使用统一解析的默认 join 参数。

#### Scenario: Disable selected client
- **WHEN** 操作者从 TUI 客户端列表选择一个客户端并确认禁用
- **THEN** 系统禁用该客户端，记录审计事件，并在结果页展示该客户端的新状态

#### Scenario: Enable selected client
- **WHEN** 操作者从 TUI 客户端列表选择一个禁用客户端并确认启用
- **THEN** 系统启用该客户端，记录审计事件，并在结果页展示该客户端的新状态

#### Scenario: Rotate selected client credential
- **WHEN** 操作者从 TUI 客户端列表选择一个客户端并确认轮换凭据
- **THEN** 系统轮换该客户端凭据，记录审计事件，并仅在当前结果页展示新凭据

#### Scenario: Review selected client join token
- **WHEN** 操作者从 TUI 客户端列表选择一个客户端并请求查看 join token
- **THEN** 系统查询该客户端最新可查看 join token；若 token 不可用，则使用统一解析的默认 join 参数重置 join token
- **AND** 结果页展示 token、过期时间和单次消费提示
- **AND** 结果页展示可在管理端终端执行的 `goginx-admin client-join-command -client <id>` 指令，用于获取客户端 join 指令

#### Scenario: Delete selected client
- **WHEN** 操作者从 TUI 客户端列表选择一个客户端并完成强确认
- **THEN** 系统删除该客户端记录，记录审计事件，并刷新客户端列表

#### Scenario: Reject client delete when service blocks it
- **WHEN** 客户端存在服务层禁止删除的依赖或状态，例如仍有关联启用代理
- **THEN** TUI 展示阻塞原因，并且不得删除该客户端

#### Scenario: Client delete requires strong confirmation
- **WHEN** 操作者在 TUI 中触发客户端删除
- **THEN** TUI 在执行删除前要求摘要确认和客户端 ID 级别的强确认
