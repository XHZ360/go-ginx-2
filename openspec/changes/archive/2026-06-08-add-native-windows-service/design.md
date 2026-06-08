## Context

当前 Windows 发布包包含 `bin/`、`config/`、`data/`、`logs/` 和 `admin-ui/`，但没有原生服务管理能力。操作者若要开机自启和受监督运行，需要自行选择 WinSW、NSSM 或任务计划程序；这会带来第三方工具依赖、参数约定不一致、工作目录错误、环境变量未继承和客户端 join 顺序不清晰等问题。

现有 server/client 入口已经适合接入 Windows Service：

- `goginx-server` 加载配置后调用 `daemon.StartServer(ctx, cfg)`，并在 `ctx.Done()` 后通过 `runtime.Close()` 收尾。
- `goginx-client` 加载配置后调用 `daemon.RunClient(ctx, cfg)`，临时连接问题由客户端重连逻辑处理。
- 默认路径按二进制所在部署根目录推导，适合服务进程不依赖当前工作目录。
- 日志已写入部署根目录下的 `logs/server.log` 和 `logs/client.log`。

## Goals / Non-Goals

**Goals:**

- 让 Windows 发布包内的 server/client 二进制可以自行安装、启动、停止、重启、查询状态和卸载 Windows Service。
- 在 Windows Service 停止或系统关闭时进入现有 context cancel 和 runtime close 路径，保持 graceful shutdown。
- 保持 configless/managed 默认部署路径，同时允许高级部署传入 `-config`。
- 明确 Windows 服务安装前置条件，尤其是 server 的 `join_service_host` 和 client 的 `join` 受管状态。
- 提供可测试的服务命令解析、服务注册参数、非 Windows 提示和 Windows 文档。

**Non-Goals:**

- 不实现 MSI/MSIX、winget、Chocolatey、签名安装器或图形化安装程序。
- 不实现 Windows Event Log 集成、服务恢复策略 UI、防火墙规则自动写入、自定义服务账户或账户密码托管。
- 不改变 server/client 的数据目录、日志目录、控制协议、数据库 schema 或管理 API。
- 不要求 Windows CI 能真实启动 SCM 服务；真实 SCM 启停可作为手动或后续 E2E 验证。

## Decisions

### 1. 使用 Go 原生 Windows Service API

使用 `golang.org/x/sys/windows/svc` 处理 SCM 运行入口，使用 `golang.org/x/sys/windows/svc/mgr` 处理安装、卸载和服务控制命令。

原因：

- 项目已经间接依赖 `golang.org/x/sys`，不会引入第三方运行时 wrapper。
- `svc.Handler` 能直接把 `Stop` 和 `Shutdown` 映射为 `context.CancelFunc`。
- 可保留单二进制发布体验，用户不需要额外下载 WinSW/NSSM。

备选方案：

- WinSW/NSSM：实现快，但会把核心部署体验交给外部工具。
- 直接 `sc.exe create`：当前 exe 不是 Windows Service 进程，不能正确响应 SCM。
- 任务计划程序：适合自启动，不提供完整服务生命周期。

### 2. 在 server/client 中复用同一套 service 子命令框架

两个入口都增加 `service` 子命令：

```text
goginx-server service install [flags]
goginx-server service uninstall [flags]
goginx-server service start [flags]
goginx-server service stop [flags]
goginx-server service restart [flags]
goginx-server service status [flags]
goginx-server service run [flags]
```

`goginx-client` 使用相同形状。`service run` 由 SCM 调用，普通控制台运行仍沿用当前无子命令路径。

原因：

- 让 server/client 的 Windows 体验一致。
- 便于共享安装参数解析、服务控制和错误处理。
- 不影响 Linux `systemd` 包和现有命令。

### 3. 把实际 daemon 运行抽成可复用函数

将当前 `main()` 中的运行逻辑拆成小函数，例如：

- `runServer(ctx context.Context, configPath string) error`
- `runClient(ctx context.Context, configPath string) error`

控制台模式使用 `signal.NotifyContext` 调用这些函数；Windows Service handler 使用 SCM stop/shutdown 触发的 context 调用同一函数。

原因：

- 避免 Windows Service 路径复制启动逻辑。
- 保持配置加载、日志输出、runtime close 和错误处理一致。
- 测试时可以替换 runner，验证 service handler 取消行为。

### 4. 服务安装默认使用部署根和 managed 路径

安装命令默认把服务 `binPath` 注册为当前 exe 加 `service run`，不默认附带 `-config`。如果操作者传入 `-config config/server.json` 或 `-config config/client.json`，安装命令将该参数写入服务命令行。

默认服务名：

- server：`goginx-server`
- client：`goginx-client`

默认显示名：

- server：`go-ginx server`
- client：`go-ginx client`

服务启动类型默认 `auto`，可通过参数改为 `manual`。

原因：

- 与当前 configless Release 路径一致。
- 显式配置仍作为高级覆盖路径。
- 避免依赖服务工作目录或当前 shell 环境变量。

### 5. 不把 `GOGINX_JOIN_SERVICE_HOST` 作为服务安装的主要推荐路径

Windows Service 不可靠继承安装时 PowerShell 的临时环境变量。文档推荐远程 server 部署使用 `config/server.json` 中的 `join_service_host`，或在生成 token 时显式传入地址参数。

原因：

- 减少服务环境和 CLI 环境不一致导致的错误 token。
- 保持 server 启动日志、admin CLI 默认值解析和 UI token 生成的地址一致。

### 6. 客户端服务安装前校验受管状态

`goginx-client service install` 在默认 managed 模式下必须确认 `data/client-state.json` 存在；如果操作者提供 `-config`，则校验该配置文件存在并可加载。失败时提示先执行 `goginx-client join <token>`。

原因：

- 避免服务安装后立刻循环失败。
- 把已知部署顺序错误变成安装时错误。

### 7. 首版不支持自定义服务账户

服务安装命令首版不提供自定义服务账户参数，使用 Windows Service Control Manager 默认账户行为。需要自定义账户的操作者可以在服务安装后通过 Windows 原生命令或管理控制台调整；产品内置账户参数留给后续增强。

原因：

- 避免首版处理账户密码、权限授予、凭据保存和错误回滚等复杂问题。
- 当前项目的数据目录、端口、防火墙和 join 前置条件已经是首版部署的主要风险。
- 不阻塞高级操作者使用 Windows 现有服务管理工具调整账户。

### 8. 提供 PowerShell 辅助脚本

Windows 发布包增加 PowerShell 辅助脚本，用于封装常见服务操作，例如安装 server 服务、安装 client 服务、启动、停止、重启、卸载和状态检查。脚本内部调用二进制内置 `service` 子命令，不绕过原生命令实现。

原因：

- 降低 Windows 操作者记忆长命令和路径参数的成本。
- 能把 `join_service_host`、客户端 join 前置检查和管理员权限提示放在更显眼的位置。
- 保持真正的服务能力仍在二进制内，不把脚本变成第二套服务实现。

### 9. 继续使用文件日志

Windows Service 首版不接入 Windows Event Log，继续使用部署根目录下的 `logs/server.log` 和 `logs/client.log` 作为受支持日志路径。

原因：

- 当前控制台模式和发布包文档已经围绕文件日志构建。
- 文件日志跨平台一致，便于排查 server/client 行为。
- Event Log 集成涉及 source 注册、权限和日志格式设计，留给后续观测增强。

## Risks / Trade-offs

- [Risk] Windows Service API 只能在 Windows 上编译和运行 → 使用 `*_windows.go` 和 `*_other.go` build tags，非 Windows 命令返回清晰错误。
- [Risk] 服务运行账户没有端口绑定、文件读写或网络权限 → 首版不内置账户配置，文档列出管理员权限、防火墙、数据目录 ACL 和低端口占用排查步骤，并说明可用 Windows 原生工具调整账户。
- [Risk] 服务 stop 超时导致 SCM 认为停止失败 → handler 在 stop 时先报告 `StopPending`，触发 cancel/close，并尽量在合理时间内退出；长耗时清理保持后续改进空间。
- [Risk] 安装时 shell 环境变量与服务运行环境不同 → 文档推荐 JSON 配置或显式 token 参数，不把临时环境变量作为 Windows 服务主路径。
- [Risk] Windows CI 无法真实操作 SCM → 单元测试覆盖命令构造和 handler 逻辑；真实服务安装/启动作为可选 Windows 手动验证或带管理员权限的 CI 作业。

## Migration Plan

1. 保持现有前台运行方式可用，Windows 用户可以继续直接运行 `.exe` 或使用 WinSW/NSSM。
2. 新 Windows 发布包提供原生命令，操作者可在管理员 PowerShell 中安装服务。
3. server 远程部署建议先配置 `config/server.json` 的 `join_service_host`，再安装并启动服务。
4. client 必须先执行 `join` 写入受管状态，再安装并启动服务。
5. 回滚时停止并卸载 Windows Service，然后恢复为直接运行 `.exe` 或第三方 wrapper。
