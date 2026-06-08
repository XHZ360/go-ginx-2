## Why

Windows 发布包目前只能以前台进程运行，操作者需要借助 WinSW、NSSM 或任务计划程序才能获得开机自启、停止、重启和故障恢复能力。为降低 Windows 部署门槛，并让 server/client 在 Windows 上拥有与 Linux `systemd` 基线相近的服务生命周期，应提供不依赖第三方 wrapper 的原生 Windows Service 支持。

## What Changes

- 为 `goginx-server.exe` 和 `goginx-client.exe` 增加 Windows Service 生命周期子命令：安装、卸载、启动、停止、重启、状态查询和 SCM 托管运行。
- 在 Windows 上让服务停止和系统关闭事件进入现有 `context.Context` 取消路径，保持监听器关闭、客户端退出和运行状态刷写的既有保证。
- 为服务安装提供可配置的服务名、显示名、描述、启动类型和配置路径，默认仍使用部署根目录推导的 configless/managed 路径。
- 为 Windows 发布包提供 PowerShell 辅助脚本，封装常见 server/client 服务安装、启动、停止、重启和卸载流程。
- 扩展 Windows 发布包和运维文档，说明服务安装顺序、客户端 join 前置要求、`join_service_host` 配置、文件日志位置和故障排查。
- 增加 Windows 专属单元测试或可在 Windows CI 上运行的服务命令测试，覆盖命令解析、服务注册参数、非 Windows 行为和受管状态前置校验。

## Capabilities

### New Capabilities
- 无。

### Modified Capabilities
- `deployment-operations`：把 Windows 原生服务安装、生命周期管理和文档化部署流程从未来缺口推进为受支持部署能力。

## Impact

- 影响代码：`cmd/goginx-server`、`cmd/goginx-client` 入口，新增 Windows Service 管理包，可能新增 Windows build-tag 文件。
- 影响依赖：使用已有间接依赖 `golang.org/x/sys/windows/svc` 和 `golang.org/x/sys/windows/svc/mgr`，通常不需要新增第三方运行时 wrapper。
- 影响发布包：Windows bundle 保持当前目录布局，并增加 PowerShell 辅助脚本和文档说明原生服务安装流程。
- 影响运维：Windows 操作者可以用内置命令安装、启动、停止和卸载服务；客户端服务仍必须在 `join` 写入受管状态后启动。
- 不涉及数据库 schema、控制协议、GraphQL API 或管理前端功能变更。
