## ADDED Requirements

### Requirement: Local admin TUI entrypoint
系统 MUST 为 admin CLI 提供本地 TUI 模式，作为面向服务器终端的交互式运维入口。

#### Scenario: Start local TUI with default database
- **WHEN** 操作者运行 `goginx-admin tui` 且未显式提供数据库路径
- **THEN** 系统使用与现有 admin CLI 相同的部署根默认 SQLite 路径启动 TUI

#### Scenario: Start local TUI with explicit database
- **WHEN** 操作者运行 `goginx-admin tui -db <path>`
- **THEN** 系统使用指定 SQLite 数据库路径启动 TUI，并按现有部署相对路径规则解析该路径

#### Scenario: Preserve non-interactive command behavior
- **WHEN** 操作者继续运行 `init-admin`、`create-user`、`create-client` 或其他现有 admin CLI 子命令
- **THEN** 系统保持这些子命令的参数、输出和错误行为，不要求经过 TUI

#### Scenario: Reject non-interactive terminal for TUI
- **WHEN** 操作者在不支持交互式终端控制的环境中运行 `goginx-admin tui`
- **THEN** 系统拒绝进入 TUI，并提示使用现有非交互式子命令完成同等配置

### Requirement: Local admin TUI navigation baseline
系统 MUST 在 TUI 中提供简单可发现的本地运维导航，并限制首版范围为管理员、用户和客户端配置。

#### Scenario: Main menu exposes confirmed operations
- **WHEN** TUI 启动成功
- **THEN** 主菜单提供管理员设置、用户管理、客户端配置和退出选项

#### Scenario: Main menu excludes broader admin surfaces
- **WHEN** 操作者查看 TUI 主菜单
- **THEN** 系统不展示代理、证书、审计、配额、系统设置、告警或远程登录管理入口

#### Scenario: Return to main menu after action
- **WHEN** 操作者完成或取消管理员、用户或客户端配置流程
- **THEN** TUI 返回主菜单或上一级菜单，而不是退出整个进程

### Requirement: Local admin TUI administrator setup
系统 MUST 通过 TUI 支持快速配置管理员信息，并且不得引入默认管理员密码。

#### Scenario: Create first administrator from TUI
- **WHEN** SQLite 中不存在可登录管理员，且操作者在 TUI 中提交有效用户名、密码和确认密码
- **THEN** 系统创建一个启用的管理员用户，保存密码校验材料，并记录可审计的初始化结果

#### Scenario: Select existing administrator for update
- **WHEN** SQLite 中已存在管理员用户，且操作者进入管理员设置
- **THEN** TUI 优先展示可选择的现有管理员列表，而不是要求操作者手动输入管理员用户 ID

#### Scenario: Update administrator password from TUI
- **WHEN** 操作者选择现有管理员并提交有效的新密码和确认密码
- **THEN** 系统更新该管理员密码校验材料，并保持该用户可用于管理员登录

#### Scenario: Enable disabled administrator from TUI
- **WHEN** 操作者选择一个已禁用的管理员并确认启用
- **THEN** 系统启用该管理员用户，并记录对应审计事件

#### Scenario: Reject invalid administrator setup input
- **WHEN** 操作者在管理员设置中提交空用户名、空密码、不一致的确认密码或无效管理员选择
- **THEN** TUI 在当前表单阻止提交，并展示字段级错误

### Requirement: Local admin TUI user setup
系统 MUST 通过 TUI 支持快速配置用户，并优先使用选项和默认值减少手动输入。

#### Scenario: Create user with role selection
- **WHEN** 操作者在 TUI 用户管理中输入有效用户名并从角色选项中选择 `admin` 或 `user`
- **THEN** 系统创建对应角色的启用用户，并持久化到 SQLite

#### Scenario: Default ordinary user role
- **WHEN** 操作者打开创建用户表单
- **THEN** TUI 默认选择普通用户角色，并允许操作者显式改为管理员角色

#### Scenario: Show existing users as selectable context
- **WHEN** 操作者进入用户管理
- **THEN** TUI 展示现有用户的可识别信息，至少包括用户名、用户 ID、角色和状态

#### Scenario: Reject invalid user setup input
- **WHEN** 操作者提交空用户名、无效角色或与服务层校验冲突的用户配置
- **THEN** TUI 在当前用户表单展示错误，并且不创建无效用户记录

### Requirement: Local admin TUI user maintenance
系统 MUST 通过 TUI 支持用户启用、禁用和受保护删除，并防止级联误删用户下的客户端或代理资源。

#### Scenario: Disable selected user
- **WHEN** 操作者从 TUI 用户列表选择一个启用用户并确认禁用
- **THEN** 系统禁用该用户，记录审计事件，并在结果页展示该用户的新状态

#### Scenario: Enable selected user
- **WHEN** 操作者从 TUI 用户列表选择一个禁用用户并确认启用
- **THEN** 系统启用该用户，记录审计事件，并在结果页展示该用户的新状态

#### Scenario: Delete user without dependent resources
- **WHEN** 操作者从 TUI 用户列表选择一个用户，且该用户没有客户端或代理等依赖资源，并完成强确认
- **THEN** 系统删除该用户记录，记录审计事件，并刷新用户列表

#### Scenario: Reject user delete with dependent resources
- **WHEN** 操作者尝试删除仍拥有客户端或代理资源的用户
- **THEN** 系统拒绝删除，展示依赖资源阻塞原因，并且不得级联删除该用户的客户端或代理

#### Scenario: User delete requires strong confirmation
- **WHEN** 操作者在 TUI 中触发用户删除
- **THEN** TUI 在执行删除前要求摘要确认和资源 ID 级别的强确认

### Requirement: Local admin TUI client setup
系统 MUST 通过 TUI 支持快速配置客户端，并优先通过用户选择、生成凭据和默认 join 参数减少手动输入。

#### Scenario: Quick create client join token with selected owner
- **WHEN** 操作者在 TUI 默认客户端快速向导中从现有用户列表选择所属用户，输入有效客户端名称，并确认加入令牌参数
- **THEN** 系统创建客户端和加入令牌，并在结果页展示一次性 token

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
- **THEN** TUI 展示 enrollment URL、控制通道地址、TLS 地址、server name、CA 文件和 TTL 的默认值，并要求操作者在提交前确认或编辑

#### Scenario: Reject client setup when no owner exists
- **WHEN** SQLite 中不存在可作为客户端所属者的用户，且操作者进入客户端配置
- **THEN** TUI 阻止创建客户端，并引导操作者先创建用户

#### Scenario: Reject invalid client setup input
- **WHEN** 操作者提交空客户端名称、无效用户选择、空 join 必填地址、缺失 CA 文件或服务层返回的校验错误
- **THEN** TUI 在当前客户端流程展示错误，并且不创建无效客户端或加入令牌

#### Scenario: Client secrets display once
- **WHEN** 客户端凭据或加入令牌创建成功
- **THEN** TUI 仅在当前结果页展示凭据或 token，并提示离开结果页后需要重新生成或轮换才能再次获得明文值

### Requirement: Local admin TUI client maintenance
系统 MUST 通过 TUI 支持客户端启用、禁用、凭据轮换和受保护删除。

#### Scenario: Disable selected client
- **WHEN** 操作者从 TUI 客户端列表选择一个客户端并确认禁用
- **THEN** 系统禁用该客户端，记录审计事件，并在结果页展示该客户端的新状态

#### Scenario: Enable selected client
- **WHEN** 操作者从 TUI 客户端列表选择一个禁用客户端并确认启用
- **THEN** 系统启用该客户端，记录审计事件，并在结果页展示该客户端的新状态

#### Scenario: Rotate selected client credential
- **WHEN** 操作者从 TUI 客户端列表选择一个客户端并确认轮换凭据
- **THEN** 系统轮换该客户端凭据，记录审计事件，并仅在当前结果页展示新凭据

#### Scenario: Delete selected client
- **WHEN** 操作者从 TUI 客户端列表选择一个客户端并完成强确认
- **THEN** 系统删除该客户端记录，记录审计事件，并刷新客户端列表

#### Scenario: Reject client delete when service blocks it
- **WHEN** 客户端存在服务层禁止删除的依赖或状态，例如仍有关联启用代理
- **THEN** TUI 展示阻塞原因，并且不得删除该客户端

#### Scenario: Client delete requires strong confirmation
- **WHEN** 操作者在 TUI 中触发客户端删除
- **THEN** TUI 在执行删除前要求摘要确认和客户端 ID 级别的强确认

### Requirement: Local admin TUI validation semantics
系统 MUST 在 TUI 中提供强校验和明确错误反馈，使无效配置不会静默写入本地数据库。

#### Scenario: Validate before command execution
- **WHEN** 操作者提交管理员、用户或客户端表单
- **THEN** TUI 在调用领域服务前校验必填字段、枚举选择、确认密码、选择项存在性和文件路径等可本地判断的条件

#### Scenario: Surface service validation errors
- **WHEN** 领域服务返回结构化校验、冲突或资源不存在错误
- **THEN** TUI 在当前流程展示可操作的错误信息，并保留操作者已填写的有效字段

#### Scenario: Confirm persistent changes
- **WHEN** 操作者即将创建、更新、启用、禁用、删除管理员、用户、客户端或加入令牌
- **THEN** TUI 在执行写入前展示摘要确认页，确认后才调用持久化操作

#### Scenario: Cancel without partial write
- **WHEN** 操作者在确认前取消管理员、用户或客户端配置流程
- **THEN** 系统不写入对应资源，并返回上一级菜单
