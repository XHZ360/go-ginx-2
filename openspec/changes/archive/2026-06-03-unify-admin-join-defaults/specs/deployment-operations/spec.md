## MODIFIED Requirements

### Requirement: Server service address confirmation
系统 MUST 在服务端配置加载、守护进程启动和本地 admin join 材料生成阶段确认当前服务可供客户端 join 使用的默认域名或 IP，并把该确认结果作为运行时状态或本地解析结果提供给 join/enrollment 生成路径。

#### Scenario: Explicit configured service address is confirmed
- **WHEN** 操作者通过受支持的配置、命令参数或环境覆盖提供服务域名或 IP
- **THEN** 服务端启动或 admin join 默认值解析时确认该显式值为默认 join 服务地址来源，并优先于自动推断结果

#### Scenario: Configless startup infers service address
- **WHEN** 服务端以 configless 模式启动且操作者未显式提供服务域名或 IP
- **THEN** 服务端根据已配置或默认控制通道监听地址、本机可用地址和本地开发兜底规则确认一个默认 join 服务地址来源

#### Scenario: Admin CLI resolves join defaults from server configuration
- **WHEN** 操作者运行 `goginx-admin create-client-join`、`goginx-admin client-join-command` 或 `goginx-admin tui`，且未显式覆盖 join 参数
- **THEN** admin CLI 根据显式 server 配置路径、部署根默认 server 配置、受支持环境变量或 managed 默认配置解析默认 join 服务地址，并组合出默认 enrollment URL、控制通道地址、TLS 地址、server name 和 CA 文件

#### Scenario: Confirmed address is operator-visible
- **WHEN** 服务端启动完成或管理员查看用于生成 join token 的默认连接信息
- **THEN** 系统提供可诊断的默认服务域名或 IP 及其来源，使操作者能够发现需要显式覆盖的 NAT、容器或负载均衡场景

#### Scenario: Invalid explicit service address fails clearly
- **WHEN** 操作者显式配置的服务域名或 IP 无法通过格式校验或无法组合为受支持的 join 连接地址
- **THEN** 服务端启动、admin 默认值解析或配置校验失败并返回明确错误，而不是静默回退到自动推断地址
