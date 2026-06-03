## ADDED Requirements

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

## MODIFIED Requirements

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
