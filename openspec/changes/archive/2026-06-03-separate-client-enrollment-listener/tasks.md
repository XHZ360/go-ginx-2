## 1. 配置与默认值

- [x] 1.1 在 `config.Server` 中新增 `client_enrollment_listen` 字段，默认值设为 `:8081`，并加入 JSON 加载、校验和路径无关的 managed 配置流程
- [x] 1.2 新增 `GOGINX_CLIENT_ENROLLMENT_LISTEN` 环境变量覆盖，并确保 `LoadManagedServer` 与 `LoadJoinServiceDefaults` 都应用该覆盖
- [x] 1.3 将 configless `http_entry_listen` 内置默认值设为 `:80`，`https_entry_listen` 内置默认值设为 `:443`
- [x] 1.4 更新 `ConfirmJoinServiceDefaults`，使默认 `enrollment_url` 使用默认 join host 和 `client_enrollment_listen` 端口组合
- [x] 1.5 扩展配置单元测试，覆盖 enrollment 默认 `:8081`、HTTP/HTTPS 默认 `:80`/`:443`、环境覆盖、JSON 覆盖、无冲突默认监听和默认 enrollment URL

## 2. 独立 Enrollment Listener

- [x] 2.1 提取或新增只服务 `/api/client/enroll` 的 enrollment HTTP server，复用现有 `enrollment.Service.Redeem` 行为
- [x] 2.2 在 daemon 启动流程中监听 `client_enrollment_listen`，并把 listener 纳入 `ServerRuntime` 生命周期和关闭路径
- [x] 2.3 确认 enrollment listener 不注册 admin-ui、`/api/admin/*`、管理员登录、管理员会话或 GraphQL 管理 API
- [x] 2.4 从 admin listener 移除 `/api/client/enroll` 路由，并新增旧 admin enrollment URL 不能兑换 token 的测试
- [x] 2.5 更新服务端启动日志，输出 enrollment listener 地址和默认 enrollment URL

## 3. ListenerClaim 与管理入口

- [x] 3.1 将 `client_enrollment_listen` 加入静态 ListenerClaim 集合，确保 TCP/UDP 代理创建、更新和启用时能检测端口冲突
- [x] 3.2 更新 admin service/admin API 相关测试，覆盖 enrollment listener 端口参与冲突检测
- [x] 3.3 增加 admin/enrollment 边界测试，证明 enrollment listener 不暴露管理前端或管理 API

## 4. Join 默认入口

- [x] 4.1 更新 Admin API、admin CLI 和 TUI 的默认 join 参数展示与提交路径，确保未显式覆盖时使用专用 enrollment listener 端口
- [x] 4.2 保持显式 `-enrollment-url`、GraphQL `enrollmentUrl` 和 TUI 可编辑 enrollment URL 覆盖优先级不变
- [x] 4.3 更新 `client-join-command` 和 join token review/reset 流程测试，覆盖默认 URL 从 admin 8080 迁移到 enrollment 8081，且旧 admin URL token 不再兼容

## 5. 端到端验证与文档

- [x] 5.1 增加或更新 e2e configless join 测试，证明客户端通过默认 `:8081` enrollment listener 完成 join
- [x] 5.2 增加端口冲突测试，证明默认 `client_enrollment_listen` 与默认 HTTP/HTTPS entry 不冲突，显式冲突会失败或被准入检测拒绝
- [x] 5.3 更新 README、`docs/daemon-runtime.md` 和部署示例，说明 admin 8080、enrollment 8081、HTTP entry 80、HTTPS entry 443、低端口权限要求、环境变量覆盖、旧 token 不兼容和生产 TLS 反代建议
- [x] 5.4 运行相关 Go 单元测试、e2e smoke 测试和 OpenSpec 校验，确认 change 可进入实现归档流程
