## Why

当前 server/client 守护进程把日志持续追加到固定的 `logs/server.log` 和 `logs/client.log`，长时间运行后单个文件可能无限增长，给磁盘容量、故障排查和 Windows 部署带来风险。现有配置已经暴露 `log_retention_days`，但运行时尚未把日志轮换和留存作为可验证行为落实。

## What Changes

- 为 `goginx-server` 和 `goginx-client` 的本地文件日志增加应用内轮换机制，避免依赖平台外部工具处理打开中的日志文件。
- 当前写入文件继续保持为 `logs/server.log` 和 `logs/client.log`，轮换后的归档文件使用可诊断的时间戳命名。
- 增加可配置的轮换大小、保留数量、保留天数和可选压缩行为，并提供保守默认值。
- 将现有 `log_retention_days` 接入实际清理逻辑，过期归档日志在启动或轮换后被清理。
- 保持 stderr 输出，兼容 systemd/journald、容器运行时和命令行前台运行。
- 文档说明 Linux、Windows、macOS、systemd 和容器部署下的推荐日志处理方式。
- 不引入完整日志查询、访问日志平台、告警或集中式日志收集能力。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `observability-and-audit`: 增加 server/client 本地运行日志文件轮换、归档保留和敏感数据边界要求，并把完整日志查询/收集仍然保留为未来缺口。
- `deployment-operations`: 增加跨平台日志轮换部署契约、默认配置说明和平台推荐处理方式。

## Impact

- 影响 `cmd/goginx-server` 和 `cmd/goginx-client` 的日志初始化路径，预期抽取共享日志输出实现以消除重复。
- 影响 `internal/config` 的 server/client 日志配置结构、默认值、JSON 解析和校验。
- 可能新增轻量日志轮换实现或引入成熟 Go 轮换库；需评估 Windows 打开文件 rename 行为、压缩时机和测试可控性。
- 影响 README、`docs/daemon-runtime.md`、部署包示例配置和相关测试。
