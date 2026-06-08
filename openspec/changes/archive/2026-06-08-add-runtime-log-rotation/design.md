## Context

当前 `goginx-server` 与 `goginx-client` 在进程启动时分别打开部署根目录下的 `logs/server.log` 和 `logs/client.log`，并通过标准库 `log` 写入 stderr 与文件。该实现简单可靠，但长时间运行会让单个日志文件持续增长；Windows 下外部工具无法稳定重命名仍被进程打开的文件，Linux 下依赖 `logrotate` 也需要额外 reopen 机制，否则容易出现日志丢失或仍写入旧 inode 的问题。

已有 server 配置包含 `log_retention_days`，文档也声明部署包包含 `logs/` 目录，但当前没有应用内轮换、归档清理或跨平台处理说明。本变更把运行日志文件轮换作为守护进程基础运维能力，而不是完整日志平台。

## Goals / Non-Goals

**Goals:**

- server/client 默认启用应用内日志轮换，避免 `server.log` 和 `client.log` 无限增长。
- 保持当前活动日志文件名稳定，便于操作者持续 tail 或排查。
- 支持按大小轮换、按天数和数量清理归档日志，并可选择压缩归档。
- 保持 stderr 输出，兼容 systemd/journald、容器日志和本地命令行运行。
- 在 Linux、Windows、macOS、systemd 和容器部署文档中说明推荐日志处理方式。
- 为日志轮换提供可测试的配置解析、默认值、校验和文件行为。

**Non-Goals:**

- 不实现日志搜索、过滤、导出、集中收集、访问日志平台或告警中心。
- 不改造全项目日志 API 为结构化日志框架。
- 不承诺多进程同时写入同一个日志文件的安全轮换。
- 不让外部 `logrotate` 成为默认或必需路径。

## Decisions

1. 应用内负责轮换，平台日志系统作为辅助输出。
   - 选择：server/client 进程自己在写入文件达到阈值时 close、归档、重新打开当前日志文件。
   - 原因：这是 Windows、Linux、macOS 都可控的共同路径，也避免 Linux 外部轮换需要进程 reopen 的复杂度。
   - 替代方案：只依赖 systemd/journald 或 `logrotate`。这会让 Windows 部署缺少默认保护，也让打包部署行为依赖平台配置。

2. 当前文件名稳定，归档文件带时间戳。
   - 选择：当前写入文件继续是 `logs/server.log` / `logs/client.log`，归档类似 `server-20260608-153000.log`，压缩后为 `.gz`。
   - 原因：稳定文件名便于文档和运维习惯，归档时间戳便于诊断和清理。
   - 替代方案：直接写入日期文件。按日期不一定能限制单文件大小，且 tail 当前文件需要额外发现逻辑。

3. 配置保守默认，server/client 共享结构。
   - 选择：新增共享日志轮换配置，例如 `log_max_size_mb`、`log_max_backups`、`log_retention_days`、`log_compress`；server 继续使用现有 `log_retention_days`，client 也获得一致默认。
   - 原因：保留现有字段语义，避免只给服务端轮换而客户端仍可能无限增长。
   - 替代方案：只做硬编码默认值。短期更小，但无法满足不同磁盘容量和运维策略。

4. 用项目内 `internal/logging` 隔离实现细节。
   - 选择：抽取共享日志初始化代码，入口只传入部署根、日志名和配置，返回 close 函数。
   - 原因：当前 server/client 有重复实现，抽取后能统一测试 Windows rename、清理、压缩和 stderr 旁路行为。
   - 替代方案：在两个入口分别实现轮换。改动看似小，但后续容易出现默认值、错误处理和测试差异。

5. 底层轮换优先使用成熟 Go 轮换实现，外面保留适配层。
   - 选择：实现阶段评估引入成熟库或自研极小轮换器；无论底层选择如何，对外只暴露 `internal/logging` 的项目内 API。
   - 原因：成熟库能减少 Windows 文件处理、压缩和保留策略缺陷；适配层让未来替换底层不会影响入口与配置合同。
   - 替代方案：完全自研。依赖更少，但需要自己覆盖更多边界条件。

## Risks / Trade-offs

- [Risk] 多进程同时写入同一个日志文件时轮换可能互相踩踏。 -> Mitigation: 文档声明每个部署根内每种守护进程日志文件由单个进程拥有；受监督服务不要启动重复实例指向同一部署根。
- [Risk] 压缩归档在轮换瞬间增加 CPU 和 I/O。 -> Mitigation: 默认值保守，压缩可配置关闭；测试覆盖禁用压缩路径。
- [Risk] 容器部署中同时写文件和 stdout/stderr 可能产生重复日志流。 -> Mitigation: 文档推荐容器优先依赖 stdout/stderr，文件轮换作为显式部署选择。
- [Risk] 过小的轮换大小会导致频繁归档，影响排查连续性。 -> Mitigation: 配置校验设置合理下限，并在文档中给出推荐值。
- [Risk] 启动阶段日志配置加载之前发生错误时只能写到默认 stderr 或默认日志路径。 -> Mitigation: 保持早期 stderr 输出；配置加载失败时不因日志轮换失败掩盖真正启动错误。

## Migration Plan

1. 引入共享日志配置默认值和校验；未配置时使用兼容默认值自动启用轮换。
2. 替换 server/client 入口的重复 `setupLogOutput`，让两者都使用共享日志初始化。
3. 更新示例配置和部署文档，说明新增字段和跨平台推荐。
4. 回滚时删除或忽略新增配置字段即可回到旧行为；已有归档日志文件保留在 `logs/` 下，不影响旧版本继续写当前日志文件。

## Open Questions

- 实现阶段最终选择成熟轮换库还是项目内极小实现，需要以依赖体积、维护状态和 Windows 行为测试结果为准。
- 是否提供显式关闭文件日志的选项，留给容器-only 部署只写 stdout/stderr，可以作为后续小变更处理。
