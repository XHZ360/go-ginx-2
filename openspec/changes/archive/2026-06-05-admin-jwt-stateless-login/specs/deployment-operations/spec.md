## ADDED Requirements

### Requirement: Admin JWT signing key deployment
系统 MUST 为管理员 JWT 登录态提供稳定、受保护且可配置的签名密钥来源，使 configless 和显式配置部署都能在服务端重启后继续验证未过期的管理员 JWT。

#### Scenario: Managed startup creates admin JWT signing key
- **WHEN** 服务端以 configless 或 managed 默认路径启动，且默认 admin JWT 签名密钥文件尚不存在
- **THEN** 系统在受管数据目录中生成新的随机签名密钥文件，并使用仅服务账号可读写的文件权限保存

#### Scenario: Managed startup reuses admin JWT signing key
- **WHEN** 服务端以 configless 或 managed 默认路径重启，且默认 admin JWT 签名密钥文件已存在且有效
- **THEN** 系统复用该密钥验证重启前签发的未过期管理员 JWT，而不是生成新密钥使会话失效

#### Scenario: Explicit config overrides admin JWT signing key path
- **WHEN** 操作者通过 server JSON 或受支持环境变量配置 admin JWT 签名密钥文件路径
- **THEN** 服务端使用该显式路径加载签名密钥，并在路径无效、文件不可读、密钥格式无效或密钥强度不足时拒绝启动 admin listener

#### Scenario: Admin JWT signing key remains secret
- **WHEN** 服务端启动、登录、验证 JWT、记录日志、返回 API 错误或展示前端页面
- **THEN** 系统不得在日志、审计事件、HTTP 响应、GraphQL 错误或前端可见文本中暴露 admin JWT 签名密钥明文

#### Scenario: Admin JWT signing key is part of managed state
- **WHEN** 文档描述部署备份、恢复、升级或回滚注意事项
- **THEN** 文档 MUST 把 admin JWT 签名密钥文件列为影响管理员重启后登录态连续性的受管状态，并说明丢失或轮换该文件会让既有管理员 JWT 失效
