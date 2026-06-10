## ADDED Requirements

### Requirement: Unified lifecycle scheduling source
系统 SHALL 使用单一生命周期调度来源计算托管证书的 provider-specific 窗口、服务状态、候选时间和失败重试时间。

#### Scenario: Scheduler selects provider-specific window
- **WHEN** 系统评估 ACME DNS-01 托管证书是否进入生命周期候选窗口
- **THEN** 调度来源使用 ACME renewal window
- **AND** 当系统评估 Cloudflare Origin CA 托管证书是否进入生命周期候选窗口时，调度来源使用 Origin CA rotation window

#### Scenario: Serving status uses the same window everywhere
- **WHEN** controller、certmanager service 或 admin query 根据 `not_after` 计算 `expiring_soon`
- **THEN** 它们 MUST 使用同一调度来源返回的 provider-specific window
- **AND** 同一证书在同一时间点不得因调用路径不同得到不同的 `serving_status`

#### Scenario: Retry time uses the same backoff rule
- **WHEN** 托管证书签发、续期、轮换或 provider sync 失败
- **THEN** 系统通过同一调度来源计算 `next_attempt_at`
- **AND** 临近过期的证书继续使用更短紧急重试间隔，且该规则不在 controller 和 service 中重复实现

#### Scenario: Candidate query uses scheduler lookahead
- **WHEN** daemon controller 查询需要续期或轮换的托管证书候选
- **THEN** store 查询使用调度来源提供的最大 lookahead 或等价 provider-aware 条件
- **AND** controller MUST NOT 因使用单个过大的固定窗口而长期加载明显不可能到期的 provider 候选

### Requirement: Lifecycle actions reuse loaded certificate records
系统 MUST 支持使用已加载的托管证书记录执行生命周期评估和 provider 选择，避免在同一操作链路中按 proxy ID 重复读取目标证书。

#### Scenario: Controller renews from loaded candidate
- **WHEN** daemon controller 从 store 获得一个需要续期或轮换的托管证书候选记录
- **THEN** controller 使用该记录完成 provider 选择、窗口判断和操作锁 key 计算
- **AND** controller MUST NOT 在调用生命周期动作前再次通过 proxy ID 读取同一证书记录

#### Scenario: Service accepts loaded certificate for lifecycle action
- **WHEN** certmanager service 已经收到完整的托管证书记录
- **THEN** service 可以直接基于该记录执行 ACME 续期或 Origin CA 轮换
- **AND** 成功或失败后允许只刷新一次目标记录用于返回最新状态

#### Scenario: Manual lifecycle action loads target once
- **WHEN** admin CLI 或管理 API 通过 proxy ID 触发证书续期、轮换、同步或撤销
- **THEN** 应用层只加载一次目标托管证书记录后进入统一生命周期路径
- **AND** provider 选择、credential 解析和 active material 健康检查 MUST 复用该记录携带的 provider metadata
