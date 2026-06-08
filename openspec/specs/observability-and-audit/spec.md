## Purpose

定义可观测性与审计契约，覆盖基础代理统计、累计统计持久化边界、完整指标、日志、审计记录、错误分类、告警、留存/查询/导出和敏感数据脱敏；同时区分当前已实现的基础 TCP/UDP/HTTP 统计和轻量管理审计能力，与完整可观测性缺口。
## Requirements
### Requirement: Basic proxy statistics baseline
系统 MUST 在当前实现证据支持的范围内记录基础累计 TCP、UDP 和 HTTP 代理统计。

#### Scenario: TCP statistics baseline
- **WHEN** TCP 代理流量经过当前运行时
- **THEN** 运行时记录该代理的基础 TCP 连接和字节计数

#### Scenario: UDP statistics baseline
- **WHEN** UDP 代理流量经过当前运行时
- **THEN** 运行时记录该代理的基础 UDP 包和字节计数

#### Scenario: HTTP statistics baseline
- **WHEN** HTTP 代理流量经过当前运行时
- **THEN** 运行时记录该代理的基础 HTTP 请求、状态码、字节和错误计数

### Requirement: Statistics persistence boundary
系统 MUST 区分 SQLite 支持的累计统计和易失的运行时活跃计数。

#### Scenario: Cumulative stats survive clean shutdown
- **WHEN** 服务端运行时在 TCP、UDP 或 HTTP 流量之后干净关闭
- **THEN** 在当前实现证据支持的范围内，累计代理统计会刷写到 SQLite

#### Scenario: Active counts reset after restart
- **WHEN** 运行时重启
- **THEN** 活跃连接数或会话数 MUST 被视为会在重启后重置的运行时状态，除非未来实现证据证明其具备持久恢复能力

### Requirement: Minimal audit timeline baseline
系统 MUST 在当前管理员管理面范围内记录并暴露轻量控制面审计时间线。

#### Scenario: Admin create and lifecycle actions record audit events
- **WHEN** 管理 CLI 或管理员管理 API 成功执行当前已支持的创建、生命周期或证书操作，并且对应实现路径记录审计事件
- **THEN** 审计事件会持久化资源上下文、动作、结果、行为者语义和时间戳

#### Scenario: Recent audit list is available to the admin surface
- **WHEN** 已认证管理员请求当前管理端审计视图
- **THEN** 系统返回按时间倒序排列的近期控制面事件列表，而不是完整日志搜索系统

### Requirement: Full metrics gap tracking
可观测性与审计规格 MUST 把完整指标聚合作为当前基线未完整实现的需求/设计行为跟踪。

#### Scenario: Metrics aggregation remains a gap
- **WHEN** 产品或设计文档提到全局、用户、客户端、代理、时间窗口、响应时间、错误、配额拒绝、目标不可达、证书或 GraphQL 指标
- **THEN** 在存在实现证据前，该行为 MUST 作为未来缺口跟踪

#### Scenario: Future metrics implementation
- **WHEN** 未来实现完整指标聚合
- **THEN** 在声明该行为已实现前，MUST 用有实现证据的场景更新本规格

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

### Requirement: Advanced audit gap tracking
可观测性与审计规格 MUST 把超出当前轻量近期事件列表的审计覆盖和审计查询行为作为未来工作跟踪。

#### Scenario: Advanced audit query remains a gap
- **WHEN** 产品或设计文档提到高级审计过滤、导出、日志关联、长周期留存、配额修改审计、系统设置审计或高风险操作工作流
- **THEN** 在存在实现证据前，该行为 MUST 作为未来缺口跟踪

#### Scenario: Future audit implementation
- **WHEN** 未来实现更完整的审计记录或查询行为
- **THEN** 在声明该行为已实现前，MUST 用有实现证据的场景更新本规格

### Requirement: Error classification gap tracking
可观测性与审计规格 MUST 把完整错误分类作为当前基线未完整实现的需求/设计行为跟踪。

#### Scenario: Error taxonomy remains a gap
- **WHEN** 产品或设计文档提到认证失败、权限拒绝、无效证书、未验证域名、入口冲突、客户端离线、目标不可达、超时、配额拒绝、带宽限速、代理禁用、凭据吊销、重复会话、协议协商失败、DNS 校验失败或证书续期失败分类
- **THEN** 在存在实现证据前，该行为 MUST 作为未来缺口跟踪

#### Scenario: Future error taxonomy implementation
- **WHEN** 未来实现错误分类
- **THEN** 在声明该行为已实现前，MUST 用包含分类和资源上下文的有证据场景更新本规格

### Requirement: Alert gap tracking
可观测性与审计规格 MUST 把管理端告警行为作为当前基线未实现的需求/设计行为跟踪。

#### Scenario: Alert state remains a gap
- **WHEN** 产品或设计文档提到客户端频繁离线、代理错误率、配额临近、证书过期、证书续期失败、认证激增、入口冲突、日志积压或资源容量告警
- **THEN** 在存在实现证据前，该行为 MUST 作为未来缺口跟踪

#### Scenario: Future alert implementation
- **WHEN** 未来实现告警状态或告警展示行为
- **THEN** 在声明该行为已实现前，MUST 用有实现证据的场景更新本规格

### Requirement: Sensitive-data redaction gap tracking
可观测性与审计规格 MUST 把日志和审计记录中的敏感数据脱敏作为当前基线未完整实现的需求/设计行为跟踪。

#### Scenario: Redaction remains a gap
- **WHEN** 产品或设计文档提到 Authorization、Cookie、密码、令牌、私钥、分享令牌、访问密码或其他敏感值的日志行为
- **THEN** 在存在实现证据前，脱敏行为 MUST 作为未来缺口跟踪

#### Scenario: Future redaction implementation
- **WHEN** 未来实现日志或审计脱敏
- **THEN** 在声明该行为已实现前，MUST 用有实现证据的场景更新本规格

### Requirement: Connection and listener lifecycle log baseline
系统 MUST 在当前本地日志能力范围内记录关键客户端连接生命周期和代理 listener 生命周期事件，同时避免记录敏感数据。

#### Scenario: Server logs client session lifecycle
- **WHEN** 客户端控制会话认证成功、替换旧会话、正常断开或因心跳超时过期
- **THEN** 服务端日志记录客户端 ID、会话 ID、协议和事件结果，不记录客户端凭据或令牌

#### Scenario: Client logs control session lifecycle
- **WHEN** 客户端控制会话建立、正常关闭、认证永久失败或因心跳/代理流错误进入重连
- **THEN** 客户端日志记录客户端 ID、协议、会话 ID 或错误摘要，不记录凭据或令牌

#### Scenario: Server logs proxy listener lifecycle
- **WHEN** 服务端启动或关闭 TCP、UDP、HTTP 或 HTTPS proxy listener
- **THEN** 服务端日志记录协议、监听地址、端口和相关代理数量，使操作者能够确认监听服务是否已按有效配置运行

#### Scenario: Server logs HTTP and HTTPS routing failures
- **WHEN** HTTP Host 或 HTTPS SNI 没有匹配已启用代理、匹配代理的客户端离线，或打开代理流失败
- **THEN** 服务端日志记录代理类型、监听地址、域名和错误类别，不记录请求头、Cookie、请求体、证书私钥或其他敏感数据

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

