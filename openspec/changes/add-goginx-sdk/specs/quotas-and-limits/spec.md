## ADDED Requirements

### Requirement: SDK bridged stream resource contract
系统 MUST 把 consumer SDK 桥接流纳入服务端资源防护模型。当前首批实现 MUST 至少限制 provider 子流打开等待时间，并 MUST 为全局、用户、连接和速率限制保留明确执行点，直到完整执行能力存在。

#### Scenario: Provider open timeout is enforced
- **WHEN** consumer SDK 桥接流需要服务端向 provider 打开子流
- **THEN** 服务端 MUST 使用有限超时等待 provider 子流打开
- **AND** 超时后服务端 MUST 拒绝该次桥接并释放处理资源

#### Scenario: Global bridged stream limit remains a planned hook
- **WHEN** 产品或设计文档提到 SDK 桥接流全局并发硬顶
- **THEN** 在存在执行证据前，该行为 MUST 作为未来缺口跟踪
- **AND** 实现 MUST 保留可接入该全局限制的桥接流准入位置

#### Scenario: Per user bridged stream limit remains a planned hook
- **WHEN** 产品或设计文档提到 SDK 桥接流每 user 或每 consumer 连接并发上限
- **THEN** 在存在执行证据前，该行为 MUST 作为未来缺口跟踪
- **AND** 实现 MUST 保留可接入该用户或连接限制的桥接流准入位置

#### Scenario: Stream open rate limit remains a planned hook
- **WHEN** 产品或设计文档提到 SDK consumer 流打开速率限制
- **THEN** 在存在执行证据前，该行为 MUST 作为未来缺口跟踪
- **AND** 实现 MUST 保留可接入速率限制的 consumer 流接受或拒绝位置

### Requirement: SDK resource denial observability
系统 MUST 在 SDK 桥接流因权限、状态、provider 不可用、超时或未来资源限制被拒绝时，保留可观测且不泄露 secret 的失败分类边界。首批实现可以通过关闭或重置流表达拒绝，但不得记录 credential、私钥、完整敏感请求或响应体。

#### Scenario: Unauthorized SDK stream denial is secret safe
- **WHEN** consumer SDK 请求访问不存在、未授权或已禁用的 proxy
- **THEN** 服务端拒绝该流
- **AND** 日志或审计信息 MUST NOT 明文记录 consumer credential、provider credential、私钥或完整敏感 payload

#### Scenario: Resource denial classification remains a gap until implemented
- **WHEN** 未来因为全局并发、用户并发、连接并发或速率限制拒绝 SDK 桥接流
- **THEN** 系统 MUST 使用可观测分类区分资源耗尽与认证、授权、provider 离线或普通网络失败
- **AND** 在存在执行证据前，该分类行为 MUST 作为未来缺口跟踪
