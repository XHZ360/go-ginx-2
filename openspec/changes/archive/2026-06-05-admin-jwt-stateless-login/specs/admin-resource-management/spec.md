## MODIFIED Requirements

### Requirement: Administrator session endpoint baseline
系统 MUST 为专用 admin console 暴露同源管理员会话端点，并使用 SQLite 管理员用户凭据作为默认登录校验来源。管理员浏览器登录态 MUST 使用服务端签名的 8 小时 JWT，并通过 HttpOnly Cookie 携带。

#### Scenario: Login creates an administrator browser session
- **WHEN** 启用的 SQLite 管理员用户向 `/api/admin/login` 提交有效凭据
- **THEN** 系统校验管理员用户密码材料，签发包含管理员上下文、过期时间和 CSRF 材料的 8 小时 JWT，设置会话 Cookie，并返回前端 shell 所需的最小启动信息

#### Scenario: Login rejects missing administrator bootstrap
- **WHEN** 管理面尚未初始化任何可登录管理员用户
- **THEN** 登录端点拒绝认证，并返回可区分的认证失败或初始化缺失语义，且不泄露默认凭据

#### Scenario: Session bootstrap returns current auth context
- **WHEN** 专用前端携带有效且未过期的管理员 JWT Cookie 调用 `/api/admin/session`
- **THEN** 系统验证 JWT 签名和生命周期，并返回路由守卫、shell 初始化和后续 CSRF 感知请求所需的最小管理员上下文

#### Scenario: Logout clears the administrator browser cookie
- **WHEN** 专用前端为当前浏览器会话调用 `/api/admin/logout`
- **THEN** 系统清除浏览器会话 Cookie，并返回未认证的启动信息
- **AND** 纯无状态 JWT logout 不承诺服务端吊销外部保存的未过期 JWT

### Requirement: Administrator session lifecycle baseline
系统 MUST 执行专用 console 管理员 JWT 生命周期规则。管理员登录态 MUST 使用固定 8 小时绝对有效期，不得使用进程内 session 存储或 idle timeout 作为认证所需状态。

#### Scenario: Session expiry rejects further access
- **WHEN** 管理员浏览器会话缺失、过期、签名无效、格式无效或 claims 不满足管理员 JWT 要求
- **THEN** 会话启动端点和基于会话认证的 API 操作拒绝访问，且不暴露受保护的管理员管理资源

#### Scenario: Inactivity does not expire a valid JWT before absolute expiry
- **WHEN** 管理员 JWT 签发后尚未达到 8 小时绝对过期时间，即使浏览器在此期间没有访问管理面
- **THEN** 系统在后续请求中继续接受该 JWT，前提是签名、claims 和传输保护要求均有效

#### Scenario: Process restart preserves unexpired JWT sessions
- **WHEN** 服务端进程重启，管理员浏览器携带重启前签发且尚未过期的 JWT，并且服务端继续使用同一 admin JWT 签名密钥
- **THEN** 会话启动端点和基于会话认证的 API 操作继续识别该管理员上下文，管理员不需要重新认证

#### Scenario: Signing key rotation invalidates previous JWT sessions
- **WHEN** 服务端使用不同 admin JWT 签名密钥启动
- **THEN** 使用旧密钥签发的管理员 JWT 不再有效，管理员 MUST 重新认证

### Requirement: Browser mutation CSRF baseline
系统 MUST 保护基于管理员 JWT Cookie 认证的专用 console 变更请求，防止 CSRF。

#### Scenario: JWT-authenticated mutation requires a valid CSRF token
- **WHEN** 专用 console 浏览器变更请求使用有效管理员 JWT Cookie
- **THEN** 系统要求除会话 Cookie 外还携带与 JWT claims 中 CSRF 材料匹配的有效 CSRF 令牌，才允许变更继续

#### Scenario: JWT-authenticated query access does not require CSRF
- **WHEN** 专用 console 浏览器请求执行只读操作
- **THEN** 只要管理员 JWT 有效，系统可以不要求 CSRF 令牌
