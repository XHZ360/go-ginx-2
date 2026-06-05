## Context

admin API 当前使用 `sessionManager` 在进程内 map 保存管理员浏览器会话。登录成功后服务端生成随机 session id 和 CSRF token，把 session id 写入 `goginx_admin_session` HttpOnly Cookie；后续 `/api/admin/session` 和 `/api/admin/graphql` 通过该 session id 回查内存状态。默认生命周期包含 15 分钟 idle timeout 和 8 小时 absolute lifetime。

这种实现简单，但它把管理员登录态绑定到单个进程内存：管理员短时间不操作会被 idle timeout 踢下线，服务端重启后内存 session 全部丢失。目标是改为 8 小时无状态 JWT 登录态，同时尽量保留现有同源 admin-ui API 和 CSRF 保护模型。

## Goals / Non-Goals

**Goals:**

- 管理员登录成功后获得 8 小时绝对有效期的无状态 JWT 登录态。
- 服务端重启后，未过期 JWT 在签名密钥稳定的前提下继续可用。
- 前端继续通过同源 HttpOnly Cookie 携带登录态，不把 JWT 暴露给 JavaScript 持久存储。
- `/api/admin/session` 继续返回 `authenticated`、`username`、`csrfToken` 和轮询间隔，保持路由守卫和 mutation 调用模型稳定。
- 保留 GraphQL mutation 的 CSRF header 校验。
- configless/managed 部署自动生成并持久化签名密钥，显式配置部署可以指定密钥文件。

**Non-Goals:**

- 不引入 refresh token、滑动续期或 idle timeout。
- 不实现服务端 JWT 黑名单、全局登出、每用户 token version 或密码变更后的立即吊销。
- 不把 admin-ui 改成 `Authorization: Bearer` 或 `localStorage/sessionStorage` token 模型。
- 不改变管理员账号来源、RBAC、普通用户自助登录或客户端 join token 语义。

## Decisions

### 1. 使用 HttpOnly Cookie 承载 JWT

JWT 继续写入现有 `goginx_admin_session` Cookie，保持 `Path=/api/admin`、`HttpOnly`、`SameSite=Lax`，在 TLS 或 `X-Forwarded-Proto: https` 语境下继续设置 `Secure`。Cookie 增加 `MaxAge`/`Expires`，与 JWT `exp` 对齐为 8 小时。

备选方案是把 JWT 返回给前端并由前端放入 `Authorization: Bearer`。该方案会要求前端保存 token，增加 XSS 读取风险，也会重写当前 `credentials: include` 调用模型；本变更不采用。

### 2. 固定 claims 和标准库 HMAC-SHA256

新增内部 admin JWT issuer/verifier，优先使用标准库实现固定用途 JWT，避免为了一个小的管理面 token 引入可配置过多的通用 JWT 行为。JWT header 固定为 `{"alg":"HS256","typ":"JWT"}`，claims 至少包含：

- `typ`: 固定为 `admin`
- `ver`: claims 版本，首版为 `1`
- `sub`: 管理员用户名
- `iat`: 签发时间
- `exp`: 过期时间
- `csrf`: 登录时生成的随机 CSRF token

验签必须拒绝未知算法、`none` 算法、格式错误、签名不匹配、过期、未来签发时间明显异常、类型不为 `admin` 或缺少必需 claims 的 token。时间来源沿用 `Entry.Now` 注入能力，便于测试过期和重启恢复语义。

### 3. 签名密钥通过文件稳定持久化

新增 server 配置字段 `admin_jwt_secret_file`，默认值为 `data/admin-jwt.key`。configless/managed 启动路径在准备受管目录时确保该文件存在：不存在则生成至少 32 字节随机密钥，以 base64url 文本加换行保存，文件权限使用 `0600`。显式配置和环境变量 `GOGINX_ADMIN_JWT_SECRET_FILE` 可覆盖路径。

服务端启动 admin API 时读取密钥文件并传入 `adminapi.Entry`。密钥文件缺失、无法读取、无法解码或解码后长度不足时，admin listener 启动失败并给出明确错误。密钥不得写入日志、审计事件、前端响应或错误 payload。

备选方案是每次进程启动生成内存密钥；这无法解决重启后登录态丢失。另一个备选方案是从管理员密码 hash 或控制通道 TLS 私钥派生密钥；这会耦合无关安全材料，并让轮换和故障排查更难。

### 4. Session 语义变为 8 小时固定有效期

登录成功后签发一个 8 小时 JWT，不再记录服务端 session map，也不再刷新 last seen。`/api/admin/session` 只验证 Cookie 中 JWT；GraphQL 认证也只验证 JWT。服务端重启后，只要密钥文件不变且 token 未过期，会话继续有效。

logout 清除浏览器 Cookie。由于本变更选择纯无状态 JWT，logout 不承诺让外部保存的旧 JWT 在服务端立刻失效；旧 JWT 在 8 小时过期前仍可被验签通过。该行为需要在规格和文档中明确。

### 5. CSRF token 放入 JWT claims

登录时继续生成随机 CSRF token，但不再存进内存 session，而是写入 JWT `csrf` claim。`/api/admin/session` 验证 JWT 后返回该 CSRF token。GraphQL mutation 和 authenticated logout 继续要求 `X-GoGinx-CSRF-Token` 与 claim 中的值常量时间相等；GraphQL query 不要求 CSRF。

这保留了“Cookie 自动发送 + 自定义 header 双提交”防护模型。由于 Cookie 为 HttpOnly，前端不能读取 JWT；前端仍通过 session bootstrap 获取 CSRF token。

### 6. 管理员状态不做每请求回查

JWT 验证不在每个请求重新查询 SQLite 管理员状态。这样才能保持管理面认证路径无状态，也与当前进程内 session 在登录后不持续回查用户状态的行为接近。禁用管理员、修改密码或删除管理员不会立即吊销已签发 JWT，最长影响窗口为 8 小时。未来如果需要更强吊销语义，可以引入管理员 token version 或服务端吊销表作为单独变更。

## Risks / Trade-offs

- [Risk] 被窃取的 JWT 在过期前可重放。 -> Mitigation: 保持 HttpOnly Cookie、受保护传输要求、8 小时固定有效期和 CSRF header；文档强调 admin listener 应置于 TLS 或可信内网后。
- [Risk] logout 不再服务端吊销 token。 -> Mitigation: 明确纯无状态 logout 语义；如需要强吊销，后续以独立变更引入 token version 或黑名单。
- [Risk] 签名密钥丢失会让所有未过期 JWT 失效。 -> Mitigation: 把 `data/admin-jwt.key` 作为受管状态纳入备份/恢复说明；启动错误必须指向密钥文件问题。
- [Risk] 签名密钥泄露会允许伪造管理员 JWT。 -> Mitigation: 文件权限 `0600`，不输出密钥内容；部署文档要求限制服务账号和备份访问权限。
- [Risk] 修改管理员密码或禁用管理员后旧 JWT 仍有效。 -> Mitigation: 固定 8 小时窗口并记录为已知取舍；未来可通过 token version 扩展。

## Migration Plan

1. 新版本启动时 managed 路径自动生成 `data/admin-jwt.key`，不要求操作者手写配置。
2. 老版本进程内 session 无法迁移为 JWT；升级重启后旧 session Cookie 会因格式不匹配而被清除，管理员需要登录一次获取 JWT。
3. 之后的重启只要保留 `admin-jwt.key`，未过期 JWT 继续有效。
4. 回滚到旧版本时，JWT Cookie 会被旧 session manager 视为未知 session id 并清除，管理员重新登录即可；`admin-jwt.key` 文件可保留供再次升级使用。

## Open Questions

- 是否需要在实现阶段把 `admin_jwt_secret_file` 写入示例配置文件，还是只在文档中说明默认值和高级覆盖？
- 是否需要为 JWT 增加 `jti` claim 以便未来无兼容成本地接入吊销表？首版可以签发但不使用。
