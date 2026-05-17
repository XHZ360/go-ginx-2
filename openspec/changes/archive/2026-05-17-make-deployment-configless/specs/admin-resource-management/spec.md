## ADDED Requirements

### Requirement: First-run administrator bootstrap
系统 SHALL 提供不依赖额外管理员凭据配置文件的首次管理员初始化路径，并且 MUST NOT 提供默认管理员密码。

#### Scenario: Initialize first administrator
- **WHEN** SQLite 中不存在可用于管理面登录的启用管理员用户，且操作者执行文档化的本地初始化流程并提供用户名和密码
- **THEN** 系统创建或更新一个启用的管理员用户，保存密码校验材料，并记录可审计的初始化结果

#### Scenario: Reject implicit default administrator
- **WHEN** 服务端首次以 configless 模式启动且操作者尚未初始化管理员
- **THEN** 管理面不接受任何默认用户名或默认密码登录

#### Scenario: Prevent accidental remote open setup
- **WHEN** 管理面没有可登录管理员
- **THEN** 系统不暴露无需认证即可远程设置管理员密码的通用浏览器写入口

## MODIFIED Requirements

### Requirement: Administrator authentication baseline
系统 MUST 使用与客户端凭据分离的管理员专用认证来保护管理前端和 API；默认管理员凭据来源 MUST 是 SQLite 中启用的管理员用户密码校验材料，而不是独立服务端凭据配置文件。

#### Scenario: Administrator credentials loaded from SQLite admin users
- **WHEN** 管理面启动
- **THEN** 管理员用户名、角色、状态和密码校验材料从 SQLite 用户存储加载，并且只有启用的管理员角色用户可用于管理面登录

#### Scenario: Ordinary users cannot authenticate as administrators
- **WHEN** SQLite 中存在启用的普通用户但该用户不具备管理员角色
- **THEN** 管理面 MUST 拒绝该用户登录管理员 console

#### Scenario: Administrator credentials remain separate from client credentials
- **WHEN** 管理面认证管理员
- **THEN** 管理员浏览器登录语义与运行时客户端凭据保持分离，不把机器客户端身份或客户端 credential 当作浏览器管理员身份

#### Scenario: Protected credentials file remains optional compatibility input
- **WHEN** 实现保留 `admin_credentials_file` 作为兼容路径
- **THEN** 该路径 MUST 被文档化为显式覆盖或迁移辅助，而不是 configless 基础部署的必需输入

#### Scenario: Management access requires protected transport
- **WHEN** 使用管理员凭据访问管理面
- **THEN** 管理端点预期运行在 TLS 保护之后；本地回环明文仅用于开发和自动化测试

### Requirement: Administrator session endpoint baseline
系统 MUST 为专用 admin console 暴露同源管理员会话端点，并使用 SQLite 管理员用户凭据作为默认登录校验来源。

#### Scenario: Login creates an administrator browser session
- **WHEN** 启用的 SQLite 管理员用户向 `/api/admin/login` 提交有效凭据
- **THEN** 系统校验管理员用户密码材料，创建服务端管理的浏览器会话，设置会话 Cookie，并返回前端 shell 所需的最小启动信息

#### Scenario: Login rejects missing administrator bootstrap
- **WHEN** 管理面尚未初始化任何可登录管理员用户
- **THEN** 登录端点拒绝认证，并返回可区分的认证失败或初始化缺失语义，且不泄露默认凭据

#### Scenario: Session bootstrap returns current auth context
- **WHEN** 专用前端携带有效浏览器会话调用 `/api/admin/session`
- **THEN** 系统返回路由守卫、shell 初始化和后续 CSRF 感知请求所需的最小管理员上下文

#### Scenario: Logout invalidates the administrator browser session
- **WHEN** 专用前端为当前浏览器会话调用 `/api/admin/logout`
- **THEN** 系统失效对应的服务端会话，并清除浏览器会话 Cookie

### Requirement: Same-origin frontend delivery baseline
系统 MUST 把专用管理员前端和管理员 API 以一个外部同源呈现给浏览器，并且 configless 基础部署 MUST 不要求配置外部 admin 前端目录。

#### Scenario: Embedded frontend assets are served by default
- **WHEN** 服务端包含内置专用前端资源且未配置 `admin_frontend_dir`
- **THEN** admin listener 在同源上服务 `/`、`/login`、`/dashboard`、`/users`、`/clients`、`/proxies`、`/certificates`、`/audit` 和受支持深链，同时继续保留 `/api/admin/*` 作为后端 API 命名空间

#### Scenario: Configured frontend directory overrides embedded assets
- **WHEN** `admin_frontend_dir` 显式指向包含 `index.html` 的专用前端构建目录
- **THEN** admin listener 使用该目录服务前端路由和资源，作为开发或定制覆盖路径

#### Scenario: Missing frontend has explicit fallback behavior
- **WHEN** 未配置 `admin_frontend_dir` 且服务端构建不包含内置前端资源
- **THEN** 系统提供明确的 API-only 或启动失败行为，并在文档中说明该构建不满足默认 configless 浏览器管理面交付

#### Scenario: Missing asset-like paths return not found
- **WHEN** 浏览器请求已配置或内置前端资源中不存在的资源型路径，例如 `/assets/missing.js`
- **THEN** admin listener 返回 `404 Not Found`，而不是把缺失资源错误伪装成前端深链
