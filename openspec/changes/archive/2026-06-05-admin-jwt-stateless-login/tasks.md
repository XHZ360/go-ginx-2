## 1. 配置与签名密钥

- [x] 1.1 在 server 配置中新增 `admin_jwt_secret_file` 字段、默认值 `data/admin-jwt.key`、路径解析和 JSON/env 覆盖。
- [x] 1.2 在 managed server 准备流程中确保 admin JWT 签名密钥文件存在：不存在时生成至少 32 字节随机密钥，以受保护权限保存。
- [x] 1.3 增加配置测试，覆盖默认路径、环境变量覆盖、路径解析、managed 生成、重启复用和无效密钥失败。
- [x] 1.4 在 daemon 启动 admin API 前加载签名密钥，并把密钥材料传入 `adminapi.Entry`；加载失败时拒绝启动 admin listener。

## 2. JWT 核心

- [x] 2.1 新增固定用途 admin JWT issuer/verifier，使用 HS256、固定 header、`typ=admin`、`ver=1`、`sub`、`iat`、`exp` 和 `csrf` claims。
- [x] 2.2 为 JWT issuer/verifier 增加表驱动测试，覆盖成功签发验证、过期、错误签名、错误算法、缺失 claims、错误类型、格式错误和时间注入。
- [x] 2.3 移除或替换进程内 admin session manager 的认证职责，确保不再使用 idle timeout 或内存 session map 判断管理员登录态。

## 3. Admin API 行为

- [x] 3.1 更新 `/api/admin/login`：凭据校验成功后签发 8 小时 JWT，写入 HttpOnly Cookie，并返回包含 CSRF token 的 bootstrap 响应。
- [x] 3.2 更新 `/api/admin/session`：验证 JWT Cookie，返回当前管理员上下文；缺失、过期或无效 JWT 时清理 Cookie 并返回未认证 bootstrap。
- [x] 3.3 更新 `/api/admin/logout`：缺失或无效 JWT 时仍清理 Cookie；有效 JWT 请求继续要求匹配的 CSRF header 后返回未认证 bootstrap。
- [x] 3.4 更新 `/api/admin/graphql`：使用 JWT 验证结果设置 actor，上游 query 不要求 CSRF，mutation 要求 header 与 JWT CSRF claim 匹配。
- [x] 3.5 更新 Cookie 写入/清理逻辑，设置与 JWT 过期一致的 `MaxAge`/`Expires`，并保持现有 `Secure`、`HttpOnly`、`SameSite=Lax` 和 `/api/admin` path 语义。

## 4. 测试覆盖

- [x] 4.1 更新 `internal/adminapi` 测试，覆盖登录、bootstrap、GraphQL query、mutation CSRF、logout、过期清理和无效 JWT 错误语义。
- [x] 4.2 新增 admin API 重启恢复测试：同一签名密钥下，重启前登录得到的 Cookie 在重启后仍可 bootstrap 和访问 GraphQL。
- [x] 4.3 新增签名密钥轮换测试：更换签名密钥后，旧 JWT 被拒绝并要求重新登录。
- [x] 4.4 更新 `internal/daemon` 和 external process smoke 测试，确保 configless/packaged 启动会生成并复用 admin JWT 签名密钥。
- [x] 4.5 检查 admin-ui 单元测试 mock，保持 `SessionBootstrap` 合同不变，必要时只更新测试描述。

## 5. 文档与验证

- [x] 5.1 更新 README 和 `docs/daemon-runtime.md`，说明 admin-ui 使用 8 小时 JWT、重启保持登录、logout 清 Cookie 和密钥文件部署语义。
- [x] 5.2 更新配置示例或部署说明，记录 `admin_jwt_secret_file`、`GOGINX_ADMIN_JWT_SECRET_FILE` 和 `data/admin-jwt.key` 备份/恢复注意事项。
- [x] 5.3 运行 `go test ./...`，并运行 admin-ui 测试套件确认前端会话模型未破坏。
- [x] 5.4 运行 `openspec validate admin-jwt-stateless-login --strict`，确认 proposal、design、specs 和 tasks 可被 OpenSpec 接受。
