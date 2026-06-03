## 1. 统一默认值解析

- [x] 1.1 在 `internal/config` 中新增面向 admin CLI/TUI 的 join 默认值解析入口，复用 `ConfirmJoinServiceDefaults` 的 host 校验、端口组合和 fallback 规则。
- [x] 1.2 支持从显式 server 配置路径、部署根默认 server 配置、managed 默认配置和 `GOGINX_*` 环境覆盖构造 `JoinServiceDefaults`。
- [x] 1.3 确保解析后的 `ServerCAFile`、SQLite 等部署相关路径按部署根规则解析，避免 admin CLI 在其他工作目录运行时使用 cwd-relative 路径。
- [x] 1.4 为显式 `join_service_host`、环境变量覆盖、部署根配置、invalid host、本地 fallback 和 CA 文件路径解析补充 config 单元测试。

## 2. Admin CLI 接入

- [x] 2.1 为 `create-client-join`、`client-join-command` 和 `tui` 增加受支持的 server 配置来源参数，优先使用统一解析结果作为默认 join 参数。
- [x] 2.2 保持 `create-client-join` 的单次显式参数最高优先级，显式传入的 enrollment URL、server address、TLS address、server name 和 CA 文件不得被默认值覆盖。
- [x] 2.3 让 `client-join-command` 在重置不可查看 token 时使用统一解析出的默认 join 参数。
- [x] 2.4 为 admin CLI 补充测试，覆盖部署根 server 配置默认值、环境变量默认值、显式参数覆盖和旧本地 fallback。

## 3. TUI 接入

- [x] 3.1 在 `goginx-admin tui` 启动时注入统一解析出的 `JoinServiceDefaults`，并保持 TUI 表单提交前可查看和编辑默认 join 参数。
- [x] 3.2 确认 TUI 查看或重置不可用 join token 时复用同一组默认 join 参数，并在 CA 文件缺失或默认值无效时显示当前流程内错误。
- [x] 3.3 为 TUI 补充测试，覆盖默认值展示来自 server 配置、提交前编辑仍生效、重置 token 使用统一默认值。

## 4. 文档与验证

- [x] 4.1 更新 README 和守护进程运维文档，说明 `join_service_host`、`GOGINX_JOIN_SERVICE_HOST` 与 admin CLI/TUI 默认 join 参数的关系。
- [x] 4.2 调整示例，明确 `127.0.0.1` 是本地兜底而不是远程客户端推荐默认值。
- [x] 4.3 运行 `go test ./...`，确认 config、admin CLI、TUI、Admin API 和 enrollment 相关测试通过。
- [x] 4.4 运行 `openspec validate unify-admin-join-defaults --strict`，确认 proposal、design、spec delta 和 tasks 可归档。
