## ADDED Requirements

### Requirement: Cross-platform runtime log operations
系统 MUST 为 server/client 本地运行日志轮换提供跨平台一致的默认部署行为和文档说明。

#### Scenario: Default deployment enables bounded file logs
- **WHEN** 操作者使用 configless 或默认部署包方式启动 `goginx-server` 或 `goginx-client`
- **THEN** 系统使用默认日志轮换配置写入部署根目录下的 `logs/server.log` 或 `logs/client.log`，并限制单个当前日志文件无限增长

#### Scenario: JSON configuration can tune log rotation
- **WHEN** 操作者通过 server 或 client JSON 配置提供日志轮换大小、保留数量、保留天数或压缩设置
- **THEN** 系统使用显式配置覆盖默认日志轮换行为，并在配置无效时以明确错误拒绝启动

#### Scenario: Linux service documentation preserves stderr capture
- **WHEN** 文档描述 Linux systemd 部署下的日志行为
- **THEN** 文档 MUST 说明守护进程继续向 stderr 输出日志，systemd/journald 可以捕获服务日志，同时应用内文件轮换保护 `logs/` 目录中的本地日志文件

#### Scenario: Windows documentation relies on application rotation
- **WHEN** 文档描述 Windows 部署下的日志行为
- **THEN** 文档 MUST 说明默认依赖应用内日志轮换处理打开中的日志文件，而不是要求操作者使用外部 rename 型 logrotate 工具

#### Scenario: Container documentation prefers runtime log capture
- **WHEN** 文档描述 Docker 或 Kubernetes 部署下的日志行为
- **THEN** 文档 MUST 说明容器环境优先依赖 stdout/stderr 与容器运行时日志轮换，文件日志轮换作为显式部署选择或排障辅助

#### Scenario: Troubleshooting includes oversized logs
- **WHEN** 操作者遇到磁盘占用过高、归档日志未清理、压缩失败或日志文件异常增长
- **THEN** 当前文档提供检查日志轮换配置、保留策略、部署根 `logs/` 目录和服务权限的故障排查指导
