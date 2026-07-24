# 管理 API 与前端

管理 API 保留在 `/api/admin/*` 命名空间：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/admin/login` | 管理员登录 |
| `GET` | `/api/admin/session` | 当前会话上下文 |
| `POST` | `/api/admin/logout` | 清除当前浏览器 Cookie |
| `POST` | `/api/admin/graphql` | GraphQL 管理操作 |

客户端加入接口仅由专用 enrollment listener 提供，不由 admin listener 提供：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/client/enroll` | 客户端 join token 兑换 |

登录成功后服务端签发 8 小时绝对有效期的管理员 JWT，并写入现有的 HttpOnly Cookie。`/api/admin/session` 验证 Cookie 后返回前端需要的管理员上下文和 CSRF token。

GraphQL query 只要求 JWT 有效，mutation 继续要求 `X-GoGinx-CSRF-Token` 与 JWT claim 匹配。短时间不操作不会触发 idle timeout。

只要 `admin_jwt_secret_file` 指向的签名密钥不变，服务端重启后未过期的管理员 JWT 仍可继续使用。`POST /api/admin/logout` 会清除当前浏览器 Cookie；纯无状态 JWT 不承诺服务端吊销外部保存的未过期 token。

首次从旧版本升级时，旧的进程内 session Cookie 无法迁移，管理员需要重新登录一次；之后请把 `data/admin-jwt.key` 与 SQLite、证书一起备份，删除或轮换该文件会让既有管理员 JWT 失效。

当前 GraphQL 管理范围包括仪表盘汇总、用户管理、客户端列表和详情、反向代理 CRUD 与生命周期操作、托管证书状态/签发/续期、最近审计列表，以及 `localTargetAllowlist`、`replaceLocalTargetAllowlist` 和本机代理专用 CRUD/启停操作。浏览器侧 legacy `/graphql` 路由和旧的服务端渲染管理页不再作为本阶段入口。

客户端列表中的 `server-local` 带 `isSystem` 标记。前端在其详情页提供白名单和本机 TCP/UDP 代理管理；系统 client 的删除、禁用、凭据轮换、join，以及系统 proxy 的通用 mutation 都会由后端返回 `FORBIDDEN`。相关成功、失败和 forbidden 结果写入审计，失败摘要只保存稳定错误码。

管理前端源码位于 `admin-ui/`：

```powershell
Set-Location admin-ui
corepack enable
pnpm install --frozen-lockfile
pnpm test
pnpm build
```

服务端默认使用部署根目录下的 `admin-ui/` 构建产物目录。若服务端二进制位于 `bin/`，部署根目录就是 `bin/` 的上一级；开发或自定义部署时，可将其他构建产物目录配置到 `admin_frontend_dir`。
