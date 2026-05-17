## MODIFIED Requirements

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

### Requirement: Packaged deployment bundle baseline
系统 MUST 为首个受支持的单节点部署模型生成可复现部署包，并且基础启动路径 MUST NOT 依赖操作者编辑或携带额外配置文件。

#### Scenario: Bundle contains required runtime artifacts
- **WHEN** 操作者为受支持的生产模型构建部署包
- **THEN** 输出包含 `goginx-server`、`goginx-client` 和 `goginx-admin` 二进制文件、服务单元模板、文档化的可选配置覆盖位置，以及数据、证书和日志的预期运行时目录布局

#### Scenario: Bundle marks sample config as optional
- **WHEN** 部署包包含 server 或 client JSON 示例
- **THEN** 这些文件被文档化为高级覆盖或迁移参考，而不是基础部署启动的必需输入

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
- **WHEN** 自动化验证针对部署包运行，并且没有提供 `server.json`、`client.json`、管理员凭据文件或 admin 前端目录配置
- **THEN** 它证明打包后的 server 可以使用内置默认值和受管状态成功启动

#### Scenario: Joined client starts from managed state
- **WHEN** 自动化验证完成客户端 join/enrollment 流程
- **THEN** 它证明打包后的 client 可以在后续无 `-config` 启动时通过控制通道认证并接收代理快照

#### Scenario: Packaged runtime supports explicit override layout
- **WHEN** 自动化验证针对显式配置覆盖路径运行
- **THEN** 它证明打包后的 server 和 client 二进制文件仍可以使用文档化配置路径成功启动

#### Scenario: Supervised restart recovery is validated
- **WHEN** 自动化验证模拟受支持监督模型下的守护进程重启
- **THEN** 它证明运行时可以干净关闭，并使用文档化重启流程恢复客户端连接

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
