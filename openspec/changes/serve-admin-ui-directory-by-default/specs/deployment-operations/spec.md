## MODIFIED Requirements

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
- **THEN** 这些文件被文档化为高级覆盖或迁移参考，而不是基础部署启动的必需输入

#### Scenario: Bundle layout is stable across builds
- **WHEN** 操作者或自动化流程消费部署包
- **THEN** 工件路径和目录结构足够稳定，使已文档化的安装和升级步骤无需人工发现即可定位目标

### Requirement: Deployment validation baseline
系统 MUST 为 configless 打包部署、可选配置覆盖和受监督重启模型提供有证据支持的验证。

#### Scenario: Packaged runtime starts without config files
- **WHEN** 自动化验证针对部署包运行，并且没有提供 `server.json`、`client.json`、管理员凭据文件或 `admin_frontend_dir` 配置，但保留部署包根目录默认 `admin-ui/` 目录，且进程工作目录可以不同于部署根目录
- **THEN** 它证明打包后的 server 可以使用内置默认值、受管状态和部署根目录默认 `admin-ui/` 前端目录成功启动，并能服务管理前端入口

#### Scenario: Joined client starts from managed state
- **WHEN** 自动化验证完成客户端 join/enrollment 流程
- **THEN** 它证明打包后的 client 把受管 `data/client-state.json` 和 `data/certs/server-ca.crt` 写入由 `goginx-client` 二进制位置推导出的部署根目录，并且可以在后续无 `-config`、进程工作目录不同于部署根目录时通过控制通道认证并接收代理快照

#### Scenario: Packaged runtime supports explicit override layout
- **WHEN** 自动化验证针对显式配置覆盖路径运行
- **THEN** 它证明打包后的 server 和 client 二进制文件仍可以使用文档化配置路径成功启动

#### Scenario: Supervised restart recovery is validated
- **WHEN** 自动化验证模拟受支持监督模型下的守护进程重启
- **THEN** 它证明运行时可以干净关闭，并使用文档化重启流程恢复客户端连接
