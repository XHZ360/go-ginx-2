## ADDED Requirements

### Requirement: Runtime file log rotation baseline
系统 MUST 为 server 和 client 的本地运行日志文件提供应用内轮换与归档保留能力，防止当前日志文件无限增长。

#### Scenario: Server log rotates by configured size
- **WHEN** `goginx-server` 写入 `logs/server.log` 且当前文件达到配置的轮换大小阈值
- **THEN** 系统关闭当前文件、把旧内容归档为带时间戳的 server 日志文件，并继续向新的 `logs/server.log` 写入后续日志

#### Scenario: Client log rotates by configured size
- **WHEN** `goginx-client` 写入 `logs/client.log` 且当前文件达到配置的轮换大小阈值
- **THEN** 系统关闭当前文件、把旧内容归档为带时间戳的 client 日志文件，并继续向新的 `logs/client.log` 写入后续日志

#### Scenario: Rotated logs are retained within configured limits
- **WHEN** server 或 client 启动、完成一次日志轮换，或执行归档清理检查
- **THEN** 系统按配置的保留天数和保留数量删除过期归档日志，并保留当前正在写入的日志文件

#### Scenario: Optional archive compression
- **WHEN** 日志压缩配置启用且日志文件完成轮换
- **THEN** 系统把归档日志压缩为可诊断的压缩文件，并确保压缩失败不会阻止后续日志继续写入当前日志文件

#### Scenario: Stderr output remains available
- **WHEN** server 或 client 使用文件日志轮换输出运行日志
- **THEN** 同一日志消息仍写入 stderr，使服务管理器、容器运行时或前台命令行可以继续捕获日志

#### Scenario: Log rotation preserves sensitive-data boundary
- **WHEN** server 或 client 记录连接生命周期、listener 生命周期、路由失败或日志轮换事件
- **THEN** 日志轮换机制不得新增凭据、令牌、Cookie、私钥、请求体或其他敏感数据输出

## MODIFIED Requirements

### Requirement: Log collection and query gap tracking
可观测性与审计规格 MUST 把集中式日志收集、代理访问日志、管理操作日志、证书任务日志、查询和导出行为作为当前基线未实现的需求/设计行为跟踪；已由本地运行日志覆盖的 server/client 文件轮换和归档保留不再属于该缺口。

#### Scenario: Log query remains a gap
- **WHEN** 产品或设计文档提到日志过滤、时间范围查询、导出、访问日志、集中式收集或跨节点日志关联行为
- **THEN** 在存在实现证据前，该行为 MUST 作为未来缺口跟踪

#### Scenario: Runtime file log retention is no longer a gap
- **WHEN** 产品或设计文档提到 server/client 本地运行日志文件的大小轮换、归档保留或压缩
- **THEN** 系统 MUST 按本地运行日志轮换基线处理该行为，而不是把它归入完整日志查询缺口

#### Scenario: Future log implementation
- **WHEN** 未来实现集中式日志收集、日志查询、访问日志或导出行为
- **THEN** 在声明该行为已实现前，MUST 用有实现证据的场景更新本规格
