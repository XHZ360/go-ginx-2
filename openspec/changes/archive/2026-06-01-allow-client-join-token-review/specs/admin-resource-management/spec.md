## MODIFIED Requirements

### Requirement: Local admin TUI client setup
系统 MUST 通过 TUI 支持快速配置客户端，并优先通过用户选择、生成凭据和默认 join 参数减少手动输入。

#### Scenario: Quick create client join token with selected owner
- **WHEN** 操作者在 TUI 默认客户端快速向导中从现有用户列表选择所属用户，输入有效客户端名称，并确认加入令牌参数
- **THEN** 系统创建客户端和加入令牌，并在结果页展示 token
- **AND** 结果页展示可在管理端终端执行的 `goginx-admin client-join-command -client <id>` 指令，用于获取客户端 join 指令

#### Scenario: Review active client join token after creation
- **WHEN** 操作者从 TUI 客户端操作菜单选择查看 join token，且该客户端存在未使用且未过期的 join token
- **THEN** 系统再次展示该 token，并提示该 token 仍只能被客户端消费一次
- **AND** 结果页展示可在管理端终端执行的 `goginx-admin client-join-command -client <id>` 指令，用于获取客户端 join 指令

#### Scenario: Reject reviewing unavailable client join token
- **WHEN** 操作者尝试查看不存在、已使用、已过期或历史记录缺少明文的客户端 join token
- **THEN** TUI 展示可操作错误，并引导操作者重新生成 join token

#### Scenario: Client secrets display policy distinguishes credentials and join tokens
- **WHEN** 客户端凭据创建或轮换成功
- **THEN** TUI 仅在当前结果页展示凭据，并提示离开后需要重新创建或轮换才能再次获得明文值
- **AND** join token 在未使用且未过期期间可以由管理员重复查看

### Requirement: Local admin TUI client maintenance
系统 MUST 通过 TUI 支持客户端启用、禁用、凭据轮换、join token 查看和受保护删除。

#### Scenario: Review selected client join token
- **WHEN** 操作者从 TUI 客户端列表选择一个客户端并请求查看 join token
- **THEN** 系统查询该客户端最新可查看 join token，并在结果页展示 token、过期时间和单次消费提示
- **AND** 结果页展示可在管理端终端执行的 `goginx-admin client-join-command -client <id>` 指令，用于获取客户端 join 指令
