## Why

当前 admin-ui 浏览器登录态由服务端进程内 session map 管理，虽然已有 8 小时绝对生命周期，但还叠加 15 分钟 idle timeout，并且服务端重启后已登录浏览器会话全部失效。管理面需要更符合运维使用预期的 8 小时无状态登录，使短暂不操作和服务端重启不再频繁打断管理员。

## What Changes

- 将 admin-ui 登录态从进程内 session id 改为由服务端签名的 8 小时 JWT。
- 保持 `/api/admin/login`、`/api/admin/session`、`/api/admin/logout` 和 `/api/admin/graphql` 的同源浏览器 API 形状，前端继续通过 HttpOnly Cookie 携带登录态。
- 管理端 JWT 使用固定 8 小时绝对过期时间，不再执行 15 分钟 idle timeout。
- 服务端重启后，只要 JWT 签名密钥稳定且 token 未过期，管理员会话继续有效。
- mutation CSRF 保护继续保留，CSRF token 由 JWT claims 承载并通过 `/api/admin/session` 返回给前端。
- logout 清除浏览器 Cookie；纯无状态模式下不承诺服务端吊销已经签发但被外部保留的未过期 JWT。
- 为 configless/managed 部署生成并持久化 admin JWT 签名密钥，显式配置部署可指定密钥文件路径。

## Capabilities

### New Capabilities

- 无

### Modified Capabilities

- `admin-resource-management`: 管理员浏览器会话从进程内 session 改为 JWT 无状态登录，更新登录、session bootstrap、logout、过期、重启恢复和 CSRF 合同。
- `deployment-operations`: 增加 admin JWT 签名密钥的配置、managed 默认生成、权限和重启持久化要求。

## Impact

- 后端：`internal/adminapi` 的 session manager、login/session/logout/graphql 认证路径、Cookie 写入和测试需要更新。
- 配置：`internal/config` 需要新增 admin JWT secret 文件字段、环境覆盖和 managed 准备逻辑。
- 运行时：`internal/daemon` 启动 admin API 时需要传入稳定签名密钥材料。
- 前端：`admin-ui` API 形状可保持不变，最多更新测试对登录态语义的描述。
- 文档与规格：OpenSpec、README 和 `docs/daemon-runtime.md` 需要说明 8 小时 JWT、密钥文件和 logout 语义。
- 依赖：优先使用标准库 HMAC-SHA256 实现固定 claims JWT；若后续实现选择外部 JWT 库，需要同步更新 `go.mod`。
