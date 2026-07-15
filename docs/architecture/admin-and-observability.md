# Admin 与可观测性

## Admin

管理员通过 `goginx-admin init-admin` 初始化 SQLite 中的管理员，不存在默认密码。管理 listener 提供 `/api/admin/login`、`/api/admin/session`、`/api/admin/logout` 和 `/api/admin/graphql`；登录态是固定 8 小时的签名 JWT HttpOnly Cookie，mutation 需要 JWT 中的 CSRF token。Admin UI 与 API 同源，默认从部署根目录 `admin-ui/` 提供。

当前管理范围包括用户、provider/consumer 客户端、客户端凭据轮换、TCP/UDP/HTTP/HTTPS 代理、托管证书、dashboard 和近期审计。删除客户端前必须处理启用代理依赖；入口 listener 冲突以结构化管理错误返回。

## 统计、日志与审计

TCP、UDP、HTTP 运行时记录基础累计字节、连接/包/请求和错误计数，干净关闭时刷入 SQLite；活跃连接数和会话数重启后重置。server/client 日志写入部署根 `logs/` 并按大小、数量、天数和压缩配置轮换，同时保留 stderr 输出。

管理创建、生命周期和证书操作在当前支持路径记录轻量审计事件，Admin UI 只展示近期时间线。日志查询/导出、完整指标聚合、告警、长期审计和统一错误分类尚未实现。

实现入口为 `internal/admin*`、`internal/store` 和运行时统计/日志相关包；验证使用 `go test ./internal/admin/... ./internal/store/... -count=1`。部署、日志轮换和故障排查见 [../operations/daemon-runtime.md](../operations/daemon-runtime.md)。
