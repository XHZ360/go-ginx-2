## ADDED Requirements

### Requirement: Native Windows service lifecycle
系统 MUST 在 Windows 发布包中为 `goginx-server.exe` 和 `goginx-client.exe` 提供原生 Windows Service 生命周期命令，使操作者不依赖第三方 wrapper 即可安装、启动、停止、重启、查询状态、卸载并由 SCM 托管运行服务。

#### Scenario: Install server service
- **WHEN** Windows 操作者以管理员权限运行 `goginx-server.exe service install`
- **THEN** 系统注册一个指向当前 server 二进制 `service run` 模式的 Windows Service，并保留默认 configless/managed 部署路径

#### Scenario: Install client service
- **WHEN** Windows 操作者以管理员权限运行 `goginx-client.exe service install`，且客户端受管状态或显式配置已经存在
- **THEN** 系统注册一个指向当前 client 二进制 `service run` 模式的 Windows Service

#### Scenario: Control installed services
- **WHEN** Windows 操作者运行受支持的 `service start`、`service stop`、`service restart`、`service status` 或 `service uninstall` 命令
- **THEN** 系统通过 Windows Service Control Manager 对目标服务执行对应生命周期操作，并在失败时返回可诊断错误

#### Scenario: Non-Windows service commands fail clearly
- **WHEN** 操作者在非 Windows 平台运行原生 Windows Service 管理命令
- **THEN** 系统返回明确的不支持错误，而不是尝试注册服务或静默忽略命令

### Requirement: Windows service graceful shutdown
系统 MUST 在 Windows Service 停止或系统关闭事件中触发现有守护进程关闭路径，使 server/client 与控制台模式共享配置加载、运行和清理行为。

#### Scenario: Stop server service gracefully
- **WHEN** Windows Service Control Manager 向 `goginx-server` 服务发送 stop 或 shutdown 事件
- **THEN** server 取消运行 context，关闭监听器和后台循环，并在退出前执行已有 runtime close 行为

#### Scenario: Stop client service gracefully
- **WHEN** Windows Service Control Manager 向 `goginx-client` 服务发送 stop 或 shutdown 事件
- **THEN** client 取消运行 context，停止控制通道连接和代理流处理，并按现有客户端退出路径结束进程

#### Scenario: Console mode remains unchanged
- **WHEN** 操作者不使用 `service` 子命令直接运行 `goginx-server.exe` 或 `goginx-client.exe`
- **THEN** 程序继续以前台控制台模式运行，并响应现有控制台终止信号

### Requirement: Windows service deployment prerequisites
系统 MUST 在 Windows 服务安装和文档化流程中保护已知部署前置条件，避免服务启动后因缺少 join 地址或客户端受管状态而进入不可恢复的错误状态。

#### Scenario: Server join address is configured before remote deployment
- **WHEN** Windows 操作者准备让远程客户端加入 server
- **THEN** 文档化部署流程要求在启动服务和生成 join token 前配置客户端可访问的 `join_service_host`，或在生成 token 时显式提供 enrollment/control 地址

#### Scenario: Client managed state is required before service install
- **WHEN** Windows 操作者在默认 managed 模式下安装 `goginx-client` 服务
- **THEN** 安装命令校验 `data/client-state.json` 已存在；如果不存在，命令失败并提示先执行 `goginx-client join <token>`

#### Scenario: Explicit client config can satisfy install prerequisite
- **WHEN** Windows 操作者安装 `goginx-client` 服务并显式传入 client 配置路径
- **THEN** 安装命令校验该配置可加载，并把该配置参数写入服务运行命令

### Requirement: Windows service packaging and documentation
系统 MUST 扩展 Windows 发布包文档和 PowerShell 辅助脚本，说明原生 Windows Service 安装、升级、回滚、文件日志、权限、防火墙和故障排查流程，同时保持 Windows bundle 的现有目录布局稳定。

#### Scenario: Windows bundle documents native service commands
- **WHEN** 操作者阅读 Windows 发布包部署说明
- **THEN** 文档提供 server/client 服务安装、启动、停止、重启、状态查询和卸载命令示例

#### Scenario: Windows bundle includes PowerShell helper scripts
- **WHEN** 操作者解压 Windows 发布包
- **THEN** 发布包包含调用内置 `service` 子命令的 PowerShell 辅助脚本，用于执行常见 server/client 服务安装和生命周期操作

#### Scenario: Windows service logs use deployment log directory
- **WHEN** Windows Service 模式启动 server 或 client
- **THEN** 程序继续把运行日志写入部署根目录下的 `logs/server.log` 或 `logs/client.log`

#### Scenario: Windows service does not require Event Log
- **WHEN** Windows Service 模式启动 server 或 client
- **THEN** 系统不要求注册 Windows Event Log source，也不把 Event Log 作为首版受支持日志路径

#### Scenario: Custom service account is out of scope
- **WHEN** 操作者使用内置 service 安装命令或 PowerShell 辅助脚本安装 Windows 服务
- **THEN** 首版不提供自定义服务账户参数；需要自定义账户时，文档说明可在安装后使用 Windows 原生服务管理工具调整

#### Scenario: Windows service upgrade remains file-based
- **WHEN** 操作者升级 Windows 发布包中的 server 或 client 二进制
- **THEN** 文档说明先停止服务、替换发布包文件、保留 `data/` 和必要配置、再启动服务的流程

### Requirement: Windows service validation
系统 MUST 为原生 Windows Service 支持提供有证据的验证，覆盖命令解析、服务注册参数、关闭路径和文档化部署前置条件。

#### Scenario: Service command behavior is tested
- **WHEN** 自动化测试运行 Windows Service 命令相关单元测试
- **THEN** 测试验证服务名、显示名、binPath、配置参数、启动类型和错误信息符合文档化行为

#### Scenario: Service handler cancellation is tested
- **WHEN** 自动化测试向 service handler 模拟 stop 或 shutdown 请求
- **THEN** 测试验证 handler 会取消运行 context，并让 server/client runner 进入退出路径

#### Scenario: Windows manual validation is documented
- **WHEN** 真实 Windows SCM 启停无法在默认 CI 环境中执行
- **THEN** 文档提供管理员 PowerShell 下的手动验证步骤，覆盖安装、启动、状态查询、停止、重启和卸载
