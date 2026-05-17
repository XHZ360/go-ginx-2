## Purpose

定义部署与运维契约，覆盖本地守护进程配置、故障排查、可复现部署包、受监督服务生命周期、部署验证、备份/恢复、容量验证、低资源运行和运维文档；同时区分已实现的本地与首个受支持生产部署指导，以及仍未完成的运维缺口。

## Requirements

### Requirement: Local daemon deployment baseline
系统 MUST 在当前文档证据支持的范围内，提供里程碑一守护进程的本地构建、运行和配置指导。

#### Scenario: Build local daemon binaries
- **WHEN** 操作者遵循当前本地守护进程文档
- **THEN** 可以为本地里程碑一用途构建 server、client 和 admin 命令二进制文件

#### Scenario: Configure local server and client
- **WHEN** 操作者遵循当前本地守护进程文档
- **THEN** 可以使用已文档化的运行时字段创建 server 和 client JSON 配置文件

#### Scenario: Run local daemon pair
- **WHEN** SQLite 资源和 TLS 文件已按当前文档准备
- **THEN** 操作者可以运行本地 server/client 守护进程对，覆盖已支持的里程碑一行为

### Requirement: Local troubleshooting baseline
系统 MUST 为当前里程碑一守护进程配置和代理运行提供本地故障排查指导。

#### Scenario: Troubleshoot local daemon setup
- **WHEN** 操作者遇到已知本地配置问题，例如未知配置字段、缺少 TLS 文件、CA/SNI 不匹配、认证拒绝、缺少监听器、Host 不匹配、目标不可达、UDP 响应问题或统计刷盘时机
- **THEN** 当前文档提供该问题类别的故障排查指导

### Requirement: Packaged deployment bundle baseline
系统 MUST 为首个受支持的单节点部署模型生成可复现部署包。

#### Scenario: Bundle contains required runtime artifacts
- **WHEN** 操作者为受支持的生产模型构建部署包
- **THEN** 输出包含 `goginx-server`、`goginx-client` 和 `goginx-admin` 二进制文件、示例或已文档化的配置位置、服务单元模板，以及配置、数据、证书和日志的预期运行时目录布局

#### Scenario: Bundle layout is stable across builds
- **WHEN** 操作者或自动化流程消费部署包
- **THEN** 工件路径和目录结构足够稳定，使已文档化的安装和升级步骤无需人工发现即可定位目标

### Requirement: Service lifecycle baseline
系统 MUST 通过在外部服务管理器下运行现有前台二进制文件，为首个受支持部署模型提供受监督的启动、停止和重启行为。

#### Scenario: Supervised server lifecycle
- **WHEN** 操作者安装并启动受支持的 server 服务单元
- **THEN** 服务管理器以前台方式启动 `goginx-server`，使用配置的工作目录和配置路径，并可通过正常服务关闭行为停止它

#### Scenario: Supervised client lifecycle
- **WHEN** 操作者安装并启动受支持的 client 服务单元
- **THEN** 服务管理器以前台方式启动 `goginx-client`，使用配置的工作目录和配置路径，并可按文档化策略在临时失败后重启它

#### Scenario: Graceful shutdown preserves runtime guarantees
- **WHEN** 服务管理器停止受监督的守护进程
- **THEN** 守护进程通过正常关闭路径退出，使监听器干净关闭，并在退出前刷写累计代理统计等持久化运行状态

### Requirement: Deployment validation baseline
系统 MUST 为打包部署和受监督重启模型提供有证据支持的验证。

#### Scenario: Packaged runtime starts from bundle layout
- **WHEN** 自动化验证针对部署包运行
- **THEN** 它证明打包后的 server 和 client 二进制文件可以使用文档化的包布局和配置路径成功启动

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
部署运维规格 MUST 把首个受支持部署模型的打包安装和受监督生命周期指导视为已实现文档基线，同时继续把更完整的生产运维文档作为未来工作跟踪。

#### Scenario: Supported operations documentation exists
- **WHEN** 操作者遵循当前首个受支持生产模型的部署运维文档
- **THEN** 可以构建部署包、安装服务单元、运行启动/停止/重启流程，并排查已文档化的失败类别

#### Scenario: Broader production operations documentation remains a gap
- **WHEN** 产品或设计文档提到备份/恢复运行手册、事故响应手册、安全加固指南或多环境运维过程
- **THEN** 在存在证据支持的文档前，该行为 MUST 保持为未来缺口
