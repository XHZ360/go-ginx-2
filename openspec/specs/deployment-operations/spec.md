## Purpose

定义部署与运维契约，覆盖本地守护进程配置、故障排查、可复现部署包、受监督服务生命周期、部署验证、备份/恢复、容量验证、低资源运行和运维文档；同时区分已实现的本地与首个受支持生产部署指导，以及仍未完成的运维缺口。
## Requirements
### Requirement: Local daemon deployment baseline
系统 MUST 在当前文档证据支持的范围内，提供里程碑一守护进程的本地构建、运行、初始化和可选覆盖指导；基础路径 MUST NOT 要求操作者手写 server 或 client JSON 配置文件。

#### Scenario: Build local daemon binaries
- **WHEN** 操作者遵循当前本地守护进程文档
- **THEN** 可以为本地里程碑一用途构建 server、client 和 admin 命令二进制文件

#### Scenario: Start local server without operator-authored config
- **WHEN** 操作者在干净工作目录中运行 `goginx-server` 且未提供 `-config`
- **THEN** 服务端使用内置默认值创建所需运行目录、SQLite 数据库和控制通道 TLS 材料，并以前台守护进程方式启动

#### Scenario: Initialize local administrator without credentials file
- **WHEN** 操作者在 configless 服务端部署中执行文档化的首次管理员初始化流程
- **THEN** 系统把管理员密码校验材料持久化到 SQLite，而不是要求操作者创建独立管理员凭据配置文件

#### Scenario: Join local client without operator-authored config
- **WHEN** 操作者使用文档化的 join/enrollment 流程启动客户端
- **THEN** 客户端获得并保存运行所需的服务端地址、信任材料、客户端身份和凭据，并可在后续无 `-config` 启动时连接服务端

#### Scenario: Optional JSON configuration remains supported
- **WHEN** 操作者需要覆盖默认监听、路径、协议、ACME 或其他高级运行时字段
- **THEN** 可以继续使用已文档化的 server 和 client JSON 配置文件，并保持未知字段拒绝和字段校验行为

#### Scenario: Run local daemon pair
- **WHEN** SQLite 资源、控制通道 TLS 材料和客户端受管状态已由系统生成或由显式配置提供
- **THEN** 操作者可以运行本地 server/client 守护进程对，覆盖已支持的里程碑一行为

### Requirement: Local troubleshooting baseline
系统 MUST 为当前里程碑一守护进程配置和代理运行提供本地故障排查指导。

#### Scenario: Troubleshoot local daemon setup
- **WHEN** 操作者遇到已知本地配置问题，例如未知配置字段、缺少 TLS 文件、CA/SNI 不匹配、认证拒绝、缺少监听器、Host 不匹配、目标不可达、UDP 响应问题或统计刷盘时机
- **THEN** 当前文档提供该问题类别的故障排查指导

### Requirement: Server service address confirmation
系统 MUST 在服务端配置加载、守护进程启动和本地 admin join 材料生成阶段确认当前服务可供客户端 join 使用的默认域名或 IP，并把该确认结果、控制通道地址和客户端 enrollment listener 默认值作为运行时状态或本地解析结果提供给 join/enrollment 生成路径。

#### Scenario: Explicit configured service address is confirmed
- **WHEN** 操作者通过受支持的配置、命令参数或环境覆盖提供服务域名或 IP
- **THEN** 服务端启动或 admin join 默认值解析时确认该显式值为默认 join 服务地址来源，并优先于自动推断结果

#### Scenario: Configless startup infers service address
- **WHEN** 服务端以 configless 模式启动且操作者未显式提供服务域名或 IP
- **THEN** 服务端根据已配置或默认控制通道监听地址、本机可用地址和本地开发兜底规则确认一个默认 join 服务地址来源

#### Scenario: Admin CLI resolves join defaults from server configuration
- **WHEN** 操作者运行 `goginx-admin create-client-join`、`goginx-admin client-join-command` 或 `goginx-admin tui`，且未显式覆盖 join 参数
- **THEN** admin CLI 根据显式 server 配置路径、部署根默认 server 配置、受支持环境变量或 managed 默认配置解析默认 join 服务地址和客户端 enrollment listener，并组合出默认 enrollment URL、控制通道地址、TLS 地址、server name 和 CA 文件

#### Scenario: Confirmed address is operator-visible
- **WHEN** 服务端启动完成或管理员查看用于生成 join token 的默认连接信息
- **THEN** 系统提供可诊断的默认服务域名或 IP、客户端 enrollment 端口及其来源，使操作者能够发现需要显式覆盖的 NAT、容器或负载均衡场景

#### Scenario: Invalid explicit service address fails clearly
- **WHEN** 操作者显式配置的服务域名、IP 或客户端 enrollment 监听地址无法通过格式校验或无法组合为受支持的 join 连接地址
- **THEN** 服务端启动、admin 默认值解析或配置校验失败并返回明确错误，而不是静默回退到自动推断地址

### Requirement: Packaged deployment bundle baseline
系统 MUST 为首个受支持的单节点部署模型生成可复现部署包，并且基础启动路径 MUST NOT 依赖操作者编辑或携带额外配置文件；部署包 MUST 包含默认管理前端运行所需的 `admin-ui/` 构建产物目录。

#### Scenario: Bundle contains required runtime artifacts
- **WHEN** 操作者为受支持的生产模型构建部署包
- **THEN** 输出包含 `goginx-server`、`goginx-client` 和 `goginx-admin` 二进制文件、默认 `admin-ui/` 前端构建产物目录、服务单元模板、文档化的可选配置覆盖位置，以及数据、证书和日志的预期运行时目录布局

#### Scenario: Bundle requires frontend build output
- **WHEN** 操作者构建部署包但仓库中没有可复制的管理前端构建产物
- **THEN** 打包流程失败并提示先构建管理前端，而不是生成缺少默认 `admin-ui/` 运行时目录的部署包

#### Scenario: Bundle marks sample config as optional
- **WHEN** 部署包包含 server 或 client JSON 示例
- **THEN** 这些文件使用 `server.example.json` 或 `client.example.json` 命名，并被文档化为高级覆盖或迁移参考，而不是基础部署启动的必需输入

#### Scenario: Bundle layout is stable across builds
- **WHEN** 操作者或自动化流程消费部署包
- **THEN** 工件路径和目录结构足够稳定，使已文档化的安装和升级步骤无需人工发现即可定位目标

### Requirement: Service lifecycle baseline
系统 MUST 通过在外部服务管理器下运行现有前台二进制文件，为首个受支持部署模型提供受监督的启动、停止和重启行为；默认服务单元 MUST 使用 configless 启动路径。

#### Scenario: Supervised server lifecycle
- **WHEN** 操作者安装并启动受支持的 server 服务单元
- **THEN** 服务管理器以前台方式启动 `goginx-server`，使用配置的工作目录作为受管状态根目录，并可通过正常服务关闭行为停止它

#### Scenario: Supervised client lifecycle
- **WHEN** 操作者安装并启动受支持的 client 服务单元，且客户端本地受管状态已通过 join/enrollment 流程创建
- **THEN** 服务管理器以前台方式启动 `goginx-client`，使用本地受管状态连接服务端，并可按文档化策略在临时失败后重启它

#### Scenario: Explicit config path remains available for supervised services
- **WHEN** 操作者选择高级配置文件部署模型
- **THEN** 服务单元或覆盖片段可以显式传入配置路径，而不改变默认 configless 服务单元合同

#### Scenario: Graceful shutdown preserves runtime guarantees
- **WHEN** 服务管理器停止受监督的守护进程
- **THEN** 守护进程通过正常关闭路径退出，使监听器干净关闭，并在退出前刷写累计代理统计等持久化运行状态

### Requirement: Deployment validation baseline
系统 MUST 为 configless 打包部署、可选配置覆盖和受监督重启模型提供有证据支持的验证。

#### Scenario: Packaged runtime starts without config files
- **WHEN** 自动化验证针对部署包运行，并且没有提供 `server.json`、`client.json`、管理员凭据文件或 `admin_frontend_dir` 配置，但保留部署包根目录默认 `admin-ui/` 目录，且进程工作目录可以不同于部署根目录
- **THEN** 它证明打包后的 server 可以使用内置默认值、受管状态和部署根目录默认 `admin-ui/` 前端目录成功启动，并能服务管理前端入口

#### Scenario: Joined client starts from managed state
- **WHEN** 自动化验证完成客户端 join/enrollment 流程
- **THEN** 它证明打包后的 client 把受管 `data/client-state.json`、显式配置 `config/client.json` 和 `data/certs/server-ca.crt` 写入由 `goginx-client` 二进制位置推导出的部署根目录，并且可以在后续无 `-config`、进程工作目录不同于部署根目录时通过控制通道认证并接收代理快照

#### Scenario: Packaged runtime supports explicit override layout
- **WHEN** 自动化验证针对显式配置覆盖路径运行
- **THEN** 它证明打包后的 server 和 client 二进制文件仍可以使用文档化配置路径成功启动

#### Scenario: Supervised restart recovery is validated
- **WHEN** 自动化验证模拟受支持监督模型下的守护进程重启
- **THEN** 它证明运行时可以干净关闭，并使用文档化重启流程恢复客户端连接

### Requirement: Production packaging gap tracking
部署运维规格 MUST 把可复现的单节点部署包视为已实现基线，同时继续把更完整的打包和安装行为作为未来工作跟踪。

#### Scenario: Supported packaging baseline exists
- **WHEN** 操作者遵循首个受支持生产模型的文档化部署打包流程
- **THEN** 可以生成包含所需二进制、配置布局和服务模板的可复现部署包

#### Scenario: Advanced packaging remains a gap
- **WHEN** 产品或设计文档提到原生安装器、包管理器分发、签名发布工件或多平台打包行为
- **THEN** 在存在实现证据前，该行为 MUST 保持为未来缺口

### Requirement: Service supervision gap tracking
部署运维规格 MUST 把首个受支持部署模型的外部服务管理器监督视为已实现基线，同时继续把更完整的生命周期管理作为未来工作跟踪。

#### Scenario: Supported supervision baseline exists
- **WHEN** 操作者遵循首个受支持部署模型的文档化服务安装和生命周期步骤
- **THEN** 可以使用打包工件在受支持服务管理器下启动、停止并重启 server 和 client

#### Scenario: Advanced supervision remains a gap
- **WHEN** 产品或设计文档提到就绪信号、多服务编排、高级健康管理、watchdog 集成或不受支持的服务管理器
- **THEN** 在存在实现证据前，该行为 MUST 保持为未来缺口

### Requirement: Backup and restore gap tracking
部署运维规格 MUST 把备份与恢复行为作为当前基线未实现的需求/设计行为跟踪。

#### Scenario: Backup and restore remain gaps
- **WHEN** 产品或设计文档提到 SQLite 备份、配置备份、证书元数据备份、受私钥保护的备份、恢复或恢复后重载行为
- **THEN** 在存在实现证据前，该行为 MUST 作为未来缺口跟踪

#### Scenario: Future backup or restore implementation
- **WHEN** 未来实现备份或恢复行为
- **THEN** 在声明该行为已实现前，MUST 用有实现证据的场景更新本规格

### Requirement: Capacity and low-resource operations gap tracking
部署运维规格 MUST 把 1C1G 和 800+ 并发连接目标作为当前基线尚未验证的需求/设计行为跟踪。

#### Scenario: Capacity target remains a gap
- **WHEN** 产品或设计文档提到 1C1G 运行、低空闲开销、800+ 并发连接、文件描述符限制、内存限制或容量策略行为
- **THEN** 在存在证据支持的验证前，该行为 MUST 作为未来缺口跟踪

#### Scenario: Future capacity validation
- **WHEN** 未来验证容量或低资源行为
- **THEN** 在声明该行为已实现前，MUST 用有证据支持的场景更新本规格

### Requirement: Operations documentation gap tracking
部署运维规格 MUST 把首个受支持部署模型的 configless 打包安装、可选配置覆盖和受监督生命周期指导视为已实现文档基线，同时继续把更完整的生产运维文档作为未来工作跟踪。

#### Scenario: Supported operations documentation exists
- **WHEN** 操作者遵循当前首个受支持生产模型的部署运维文档
- **THEN** 可以构建部署包、无额外配置文件启动基础服务端、初始化管理员、完成客户端 join、安装服务单元、运行启动/停止/重启流程，并排查已文档化的失败类别

#### Scenario: Optional configuration documentation exists
- **WHEN** 操作者需要覆盖默认监听、路径、TLS、ACME 或协议行为
- **THEN** 当前文档说明如何使用显式 JSON、环境变量或命令参数作为高级覆盖路径

#### Scenario: Broader production operations documentation remains a gap
- **WHEN** 产品或设计文档提到备份/恢复运行手册、事故响应手册、安全加固指南或多环境运维过程
- **THEN** 在存在证据支持的文档前，该行为 MUST 保持为未来缺口

### Requirement: Client enrollment listener deployment defaults
系统 MUST 为客户端 enrollment listener 提供可配置的部署默认值，使 configless 服务端在不暴露 admin-ui 的情况下提供客户端 join token 兑换入口。

#### Scenario: Configless enrollment listener defaults to public port
- **WHEN** 服务端以 configless 模式启动且操作者未显式配置客户端 enrollment 监听地址
- **THEN** 服务端默认监听所有地址的 `:8081`，并在该 listener 上服务 `/api/client/enroll`

#### Scenario: Enrollment listener can be overridden by environment
- **WHEN** 操作者设置 `GOGINX_CLIENT_ENROLLMENT_LISTEN`
- **THEN** configless 服务端和本地 admin join 默认值解析使用该地址作为客户端 enrollment listener，并用其端口生成默认 enrollment URL

#### Scenario: Enrollment listener can be overridden by JSON config
- **WHEN** 操作者在 server JSON 中配置 `client_enrollment_listen`
- **THEN** 服务端启动和 admin join 默认值解析使用该显式地址，而不是内置 `:8081` 默认值

#### Scenario: Configless default ports avoid listener conflict
- **WHEN** 服务端使用内置 configless 默认值启动
- **THEN** `client_enrollment_listen`、`admin_listen`、`control_quic_listen`、`control_tls_listen`、`http_entry_listen` 和 `https_entry_listen` 的默认端口不会互相冲突

#### Scenario: Configless reverse proxy entries use standard web ports
- **WHEN** 服务端以 configless 模式启动且操作者未显式配置 HTTP 或 HTTPS 反向代理入口监听地址
- **THEN** `http_entry_listen` 默认监听所有地址的 `:80`，`https_entry_listen` 默认监听所有地址的 `:443`

#### Scenario: Low-port binding requirement is documented
- **WHEN** 文档描述 configless 默认 `http_entry_listen` 或 `https_entry_listen`
- **THEN** 文档 MUST 说明在需要权限的操作系统上绑定 80/443 可能需要 root、低端口绑定 capability、服务管理器授权或显式改用非特权端口

#### Scenario: Startup diagnostics include enrollment listener
- **WHEN** 服务端启动完成或管理员查看用于生成 join token 的默认连接信息
- **THEN** 系统展示客户端 enrollment 监听地址和默认 enrollment URL，使操作者能够确认远程客户端 join 会访问的端口

### Requirement: Per-proxy listener operations documentation
系统 MUST 为每代理入口监听配置提供部署和故障排查说明，帮助操作者理解默认监听、额外监听和端口绑定行为。

#### Scenario: Documentation explains default and per-proxy listeners
- **WHEN** 文档描述 HTTP、HTTPS、TCP 或 UDP 代理入口
- **THEN** 文档说明全局默认监听配置如何作为旧记录和空配置的 fallback，以及每代理监听地址和端口如何创建额外 listener

#### Scenario: Documentation explains hot listener reconciliation
- **WHEN** 文档描述管理员创建、更新、启用、禁用或删除代理
- **THEN** 文档说明运行时会及时启动或关闭对应 listener，并指出 listener 启动失败会作为管理操作错误暴露

#### Scenario: Troubleshooting includes listener bind conflicts
- **WHEN** 操作者遇到端口占用、wildcard 地址冲突、低端口权限或 HTTP/HTTPS 域名路由失败
- **THEN** 当前文档提供故障排查指导，帮助确认实际监听地址、端口、域名和代理状态

#### Scenario: Startup diagnostics include dynamic proxy listeners
- **WHEN** 服务端启动完成
- **THEN** 启动诊断展示已启动的 TCP、UDP、HTTP 和 HTTPS proxy listener 数量，使操作者能够确认自定义入口已参与运行

### Requirement: Admin JWT signing key deployment
系统 MUST 为管理员 JWT 登录态提供稳定、受保护且可配置的签名密钥来源，使 configless 和显式配置部署都能在服务端重启后继续验证未过期的管理员 JWT。

#### Scenario: Managed startup creates admin JWT signing key
- **WHEN** 服务端以 configless 或 managed 默认路径启动，且默认 admin JWT 签名密钥文件尚不存在
- **THEN** 系统在受管数据目录中生成新的随机签名密钥文件，并使用仅服务账号可读写的文件权限保存

#### Scenario: Managed startup reuses admin JWT signing key
- **WHEN** 服务端以 configless 或 managed 默认路径重启，且默认 admin JWT 签名密钥文件已存在且有效
- **THEN** 系统复用该密钥验证重启前签发的未过期管理员 JWT，而不是生成新密钥使会话失效

#### Scenario: Explicit config overrides admin JWT signing key path
- **WHEN** 操作者通过 server JSON 或受支持环境变量配置 admin JWT 签名密钥文件路径
- **THEN** 服务端使用该显式路径加载签名密钥，并在路径无效、文件不可读、密钥格式无效或密钥强度不足时拒绝启动 admin listener

#### Scenario: Admin JWT signing key remains secret
- **WHEN** 服务端启动、登录、验证 JWT、记录日志、返回 API 错误或展示前端页面
- **THEN** 系统不得在日志、审计事件、HTTP 响应、GraphQL 错误或前端可见文本中暴露 admin JWT 签名密钥明文

#### Scenario: Admin JWT signing key is part of managed state
- **WHEN** 文档描述部署备份、恢复、升级或回滚注意事项
- **THEN** 文档 MUST 把 admin JWT 签名密钥文件列为影响管理员重启后登录态连续性的受管状态，并说明丢失或轮换该文件会让既有管理员 JWT 失效
