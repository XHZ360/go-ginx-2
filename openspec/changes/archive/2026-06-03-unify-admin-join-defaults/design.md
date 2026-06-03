## Context

当前 `internal/config.ConfirmJoinServiceDefaults` 已能从 `config.Server` 计算默认 join 参数，daemon 启动后会把结果放入 `admin.Service.DefaultJoin`，所以 Admin API 生成 join token 时能使用 server 启动确认的地址。独立 `goginx-admin` CLI 和 TUI 则在各自进程中调用本地默认构造逻辑，当前更偏向 `127.0.0.1`，无法自然复用生产部署里配置的 `join_service_host`。

这次变更跨越 `internal/config`、`cmd/goginx-admin`、`internal/admintui`、文档和测试，但不改变 enrollment token、SQLite schema 或控制通道协议。

## Goals / Non-Goals

**Goals:**

- 为 daemon、Admin API、admin CLI 和 TUI 建立一致的 join 默认参数解析模型。
- 让 `join_service_host` 和 `GOGINX_JOIN_SERVICE_HOST` 成为所有 join token 生成入口的默认地址来源。
- 支持 admin CLI/TUI 显式指定 server 配置路径，用于读取与 server 相同的监听端口、server name、CA 文件和默认 host。
- 保留每次 token 创建时的显式 join 参数覆盖能力。
- 对本地 configless 开发保留可工作的兜底默认值，并在文档里清楚区分本地兜底和远程可达地址。

**Non-Goals:**

- 不把 daemon 运行时确认结果写入 SQLite 或引入新的运行时设置表。
- 不改变 join token payload、token hash、enrollment 兑换、客户端受管状态格式或控制通道认证。
- 不要求 admin CLI 远程调用正在运行的 server 来获取默认值。
- 不把 admin CLI/TUI 变成远程管理客户端；它们仍是面向本地部署根和 SQLite 的运维入口。

## Decisions

### 1. 抽出可复用的 join 默认值解析入口

在 `internal/config` 内保留 `ConfirmJoinServiceDefaults(Server)` 作为核心纯计算函数，并新增或拆分面向部署根的解析 helper。该 helper 负责：

- 加载默认 managed server 配置或显式 server 配置文件。
- 应用受支持的 `GOGINX_*` 环境覆盖。
- 按部署根解析 CA 文件、证书路径、SQLite 等路径字段。
- 调用 `ConfirmJoinServiceDefaults` 得到 `JoinServiceDefaults`。

这样 daemon 和 admin CLI/TUI 共用同一套配置结构和校验，而不是让 `cmd/goginx-admin` 手写端口拼接逻辑。

替代方案是把 daemon 启动时确认的 join 默认值持久化到 SQLite，admin CLI 再读取数据库。这个方案能反映运行中 server 的实际状态，但会引入设置生命周期、并发写入、回滚和“server 未启动但 admin CLI 要生成 token”的问题；当前需求只需要统一配置来源，不需要新的持久化模型。

### 2. 明确 admin CLI 默认值来源优先级

admin CLI/TUI 生成 join 参数时采用下面的来源优先级：

1. 单次命令显式 join 参数：`-enrollment-url`、`-server-address`、`-server-tls-address`、`-server-name`、`-server-ca-file`。
2. 显式 server 配置路径，例如新增的 `-server-config <path>`，用于读取 `join_service_host`、监听端口、server name 和 CA 文件。
3. 当前进程环境变量，特别是 `GOGINX_JOIN_SERVICE_HOST` 以及相关监听、server name、CA 文件环境覆盖。
4. 部署根下约定的 server 配置文件，如果存在且可读取。
5. managed/default server 配置推断，包括本机接口和 `127.0.0.1` 兜底。

第 1 层只覆盖对应字段，不要求管理员一次性填写全部字段。第 2 到 5 层负责构造完整默认值，再交给服务层已有校验处理。

替代方案是新增 `-join-service-host` 作为独立 admin flag。它更直接，但会让端口、server name 和 CA 文件仍分散在多个来源中；优先读取 server 配置可以让 admin 命令与 daemon 部署配置保持一组事实来源。

### 3. TUI 只展示解析后的默认值，并允许提交前编辑

TUI 启动时由 `cmd/goginx-admin` 注入解析后的 `JoinServiceDefaults`。客户端快速向导继续展示 enrollment URL、控制通道地址、TLS 地址、server name、CA 文件和 TTL，操作者可以在提交前编辑。查看或重置不可用 join token 时，TUI 使用同一组默认值生成替代 token；若 CA 文件缺失或默认值无法构造，错误保持在当前 TUI 流程内展示。

替代方案是在 TUI 内部重新读取 server 配置。这样会让 TUI 和非交互式 CLI 的解析路径分叉，测试和错误语义更难统一。

### 4. 文档把本地兜底和远程部署分开说明

README 和运维文档应说明：生产或跨主机 join 推荐配置 `join_service_host` 或 `GOGINX_JOIN_SERVICE_HOST`，admin CLI/TUI 会在未显式覆盖时复用该默认值；`127.0.0.1` 只表示本地开发或未能确认更好地址时的兜底。

## Risks / Trade-offs

- [Risk] admin CLI 是独立进程，无法知道 daemon 进程实际监听的动态端口或只存在于 systemd 服务环境中的变量。 → Mitigation: 文档要求使用固定监听端口和可复用配置/env；需要动态端口的测试继续显式传入 join 参数。
- [Risk] 自动读取部署根 server 配置可能让旧脚本在存在样例配置时得到不同默认值。 → Mitigation: 只读取约定的真实配置路径或显式 `-server-config`，文档继续把样例配置标注为可选参考；现有显式 join 参数保持最高优先级。
- [Risk] CA 文件路径解析错误会导致 token 生成失败。 → Mitigation: 复用现有部署根路径解析，并为 admin CLI/TUI 添加覆盖测试。
- [Risk] IPv6、空 host、未指定 host 的监听地址容易产生拼接歧义。 → Mitigation: 继续使用 `net.JoinHostPort`、现有 host 校验和 `ConfirmJoinServiceDefaults` 的推断规则。

## Migration Plan

这是非破坏性变更。已有脚本如果显式传入 `-server-address`、`-server-tls-address`、`-enrollment-url`、`-server-name` 或 `-server-ca-file`，行为保持不变。未显式传入 join 地址的 admin CLI/TUI 在部署配置存在时会生成更适合远程客户端的默认值；若需要旧的本地行为，可显式传入 `127.0.0.1` 相关参数。

回滚时可恢复 admin CLI 的本地默认构造逻辑；已经生成的 join token 不需要迁移，未消费 token 仍按自身 payload 中的地址工作。

## Open Questions

- 是否把 `-server-config` 做成只在 join 相关子命令上可用，还是提升为 `goginx-admin` 的通用 flag？倾向先限制在 `create-client-join`、`client-join-command` 和 `tui`，避免影响其他资源管理命令。
- 部署根默认 server 配置文件的自动发现路径是否应只认 `config/server.json`？倾向只认这一条约定路径，避免误读示例或临时文件。
