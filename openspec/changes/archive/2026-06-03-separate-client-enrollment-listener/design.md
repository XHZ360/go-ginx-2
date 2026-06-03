## Context

当前 `ConfirmJoinServiceDefaults` 使用 `admin_listen` 的端口拼接默认 `enrollment_url`，服务端也在 admin listener 上同时服务 admin-ui、`/api/admin/*` 和 `/api/client/enroll`。这让客户端 join 依赖 8080，而 8080 又因为承载管理前端和管理 API 被限制外网访问。

现有默认端口中，`admin_listen` 是 `127.0.0.1:8080`，`http_entry_listen` 是 `:8081`，`https_entry_listen` 默认未启用。enrollment 默认改为 `:8081` 且监听所有地址后，HTTP 反向代理默认端口必须调整；同时 configless 业务入口应更贴近公网 Web 服务习惯，默认使用 HTTP `:80` 和 HTTPS `:443`。

## Goals / Non-Goals

**Goals:**

- 为客户端 token 兑换新增独立 enrollment listener，默认 `:8081`，监听所有地址。
- 默认 join token 的 `enrollment_url` 使用独立 enrollment listener，而不是 admin listener。
- 移除 admin listener 上的 `/api/client/enroll` 路由，不兼容指向旧 admin listener 的 join token。
- 允许通过 server JSON 和环境变量覆盖 enrollment 监听地址。
- admin listener 继续仅承担 admin-ui 和 `/api/admin/*` 管理职责，保持内网或受保护访问策略。
- 将 configless `http_entry_listen` 默认值设为 `:80`，`https_entry_listen` 默认值设为 `:443`。
- 保证端口冲突检测覆盖新增静态 listener，并避免 configless 默认端口冲突。
- 更新 CLI/TUI/Admin API 默认 join 参数、启动日志、测试和文档。

**Non-Goals:**

- 不引入新的 join token 格式或数据库 schema。
- 不改变控制通道 QUIC/TCP+TLS 认证、证书校验或客户端受管状态格式。
- 不实现完整公网 TLS 证书自动配置；生产 TLS 终止仍可由反向代理或后续专门 change 处理。
- 不把 admin-ui 暴露到 enrollment listener 上。

## Decisions

1. 新增 `client_enrollment_listen` 配置字段，默认值为 `:8081`。

   该字段只控制服务端监听地址。`:` 或 `0.0.0.0` 形式都表示全地址监听，默认采用 Go 监听习惯中的 `:8081`，与现有 `control_quic_listen`、`http_entry_listen` 风格一致。环境变量新增 `GOGINX_CLIENT_ENROLLMENT_LISTEN`，显式 server JSON 和环境覆盖都参与 `LoadJoinServiceDefaults`。

   备选方案是只新增 `-enrollment-url` 文档或反向代理建议，但这不能解决 Admin API/TUI 默认生成 token 时仍指向 8080 的问题。

2. enrollment listener 只注册 `/api/client/enroll`。

   独立 listener 使用现有 `enrollment.Service.Redeem`，返回与当前 admin listener 上的兑换接口相同的响应。它不加载 admin frontend、不注册 `/api/admin/*`、不使用管理员会话 Cookie，也不提供 GraphQL。

   admin listener 上的 `/api/client/enroll` 路由必须移除。指向旧 admin listener 的 join token 或显式 `enrollment_url` 不再可兑换，操作者需要重新生成 token，或显式生成指向专用 enrollment listener/外部反向代理 URL 的 token。这个取舍让监听职责更清楚，也避免将 admin listener 作为客户端入网的隐式后门。

3. 默认 `enrollment_url` 从 `client_enrollment_listen` 端口生成。

   默认 host 仍来自现有 `join_service_host` 推断流程：显式 `join_service_host`、控制监听 host、本机非回环地址、回环兜底。端口来自 `client_enrollment_listen`，默认 scheme 暂用 `http`，路径保持 `/api/client/enroll`。操作者仍可通过 `create-client-join -enrollment-url` 或 Admin API/TUI 表单显式覆盖完整 URL。

   备选方案是新增 `client_enrollment_url` 配置字段。它能更好支持 TLS 反向代理外部 URL，但会引入“监听地址”和“公开 URL”双配置。本 change 先保持最小配置面；如后续需要固定公网 HTTPS URL，可单独扩展。

4. configless 默认业务入口端口改为 HTTP `:80`、HTTPS `:443`。

   这是为了满足 enrollment 默认 `:8081` 且 HTTP/HTTPS entry 仍可直接作为公网业务入口的约束。显式 JSON 或环境变量中的 `http_entry_listen`/`https_entry_listen` 不变；只有未配置时的内置默认值变化。文档必须标明这是 configless 默认端口变更，并说明 Linux/Unix 上绑定 80/443 通常需要 root、`CAP_NET_BIND_SERVICE` 或服务管理器能力配置。

   备选方案是把 HTTP entry 迁到 `:8082`、默认禁用 HTTPS entry，或允许 enrollment 与 HTTP proxy 共用 8081。`8082` 不符合公网 Web 入口预期；默认禁用 HTTPS 会削弱当前 HTTPS 代理入口的开箱即用性；共用 8081 需要路径/Host 复用并改变 HTTP proxy 语义，都会偏离本 change 的安全边界。

5. 静态 ListenerClaim 和启动日志纳入 enrollment listener。

   `RuntimeListenerClaims` 需要包含 `client_enrollment_listen`，避免管理员创建 TCP proxy 或其他静态监听时占用 enrollment 端口。服务端启动日志应显示 enrollment listener 地址和默认 `enrollment_url`，便于操作者确认外网 join 地址是否正确。

## Risks / Trade-offs

- [Risk] 默认 HTTP proxy 端口从 8081 迁到 80，且 HTTPS entry 从默认未启用改为 443，会影响依赖 configless 默认端口或普通用户权限启动的脚本、演示环境和测试。 → Mitigation: 标注为默认行为变更，文档说明低端口权限要求，并给出 `GOGINX_HTTP_ENTRY_LISTEN`、`GOGINX_HTTPS_ENTRY_LISTEN` 或 JSON 显式配置的覆盖方式。
- [Risk] 指向 admin listener 的旧 join token 会立即失效。 → Mitigation: 在文档和 release notes 中说明需要重新生成 join token；join token 本来是单次、限时材料，因此不做跨监听器兼容。
- [Risk] 公开 `http://host:8081/api/client/enroll` 会让 join token 兑换流量暴露在明文网络中。 → Mitigation: enrollment listener 不暴露 admin 资源；token 继续保持单次、限时和 secret/hash 校验；生产文档推荐在 8081 前使用 TLS 反向代理并显式覆盖 `-enrollment-url`。
- [Risk] NAT、容器或负载均衡环境中，监听地址端口和客户端可达 URL 可能不同。 → Mitigation: 保留显式 `-enrollment-url`、Admin API/TUI 可编辑默认值，并在日志和文档中展示默认来源。
- [Risk] 双 listener 关闭顺序或测试 fixture 不完整可能导致端口泄露。 → Mitigation: 把 enrollment server 纳入 `ServerRuntime.Close`，新增 unit/e2e 测试覆盖启动、兑换、关闭和端口冲突。

## Migration Plan

1. 添加新配置字段和环境变量，更新默认端口分配。
2. 新增独立 enrollment listener 并接入 daemon lifecycle。
3. 从 admin listener 移除 `/api/client/enroll` 路由，并更新旧 token 不兼容说明。
4. 更新默认 join 参数解析，使 CLI/TUI/Admin API 使用新 enrollment 端口。
5. 更新 ListenerClaim、日志、文档和测试。
6. 发布说明中强调：需要继续让 HTTP 反向代理占用非标准端口或让 HTTPS entry 保持禁用的部署必须显式配置 `http_entry_listen`/`https_entry_listen`；需要继续使用 8081 作为 HTTP 业务入口的部署必须显式改 enrollment 端口。

Rollback 到旧版本时，显式 `http_entry_listen`/`https_entry_listen` 可恢复原端口行为；已经生成且指向专用 enrollment listener 的 token 需要重新生成或使用显式 enrollment URL。

## Open Questions

- 是否在本 change 中新增 `client_enrollment_url` 作为 server 级公开 URL 覆盖，还是继续依赖现有 join 命令/API 的显式 `enrollment_url` 覆盖？当前设计选择后者以控制范围。
