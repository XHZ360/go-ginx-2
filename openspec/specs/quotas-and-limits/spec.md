## Purpose

定义配额、限制和限速契约，覆盖用户/代理资源控制、配额周期、执行点和可观测拒绝原因；同时明确跟踪所有当前未实现的执行行为，因为现有实现只有基础流量计数和管理端生命周期校验。

## Requirements

### Requirement: User resource limit contract
系统 MUST 支持用户级资源限制，包括代理数量、并发连接、允许端口范围、总流量配额和带宽上限。当前实现证据 MUST 把这些行为视为缺口，直到存在执行能力。

#### Scenario: User resource limit remains a gap
- **WHEN** 产品或设计文档提到用户级代理数量、并发连接、端口范围、流量配额或带宽行为
- **THEN** 在存在实现证据前，该行为 MUST 作为未来缺口跟踪

#### Scenario: Future user limit enforcement
- **WHEN** 未来实现用户级资源限制执行
- **THEN** 在声明该行为已实现前，MUST 用有实现证据的场景更新本规格

### Requirement: Proxy resource limit contract
系统 MUST 支持代理级流量配额、带宽上限、并发连接限制，以及代理专属拒绝或暂停行为。当前实现证据 MUST 把这些行为视为缺口，直到存在执行能力。

#### Scenario: Proxy resource limit remains a gap
- **WHEN** 产品或设计文档提到代理级配额、带宽、并发、拒绝或暂停行为
- **THEN** 在存在实现证据前，该行为 MUST 作为未来缺口跟踪

#### Scenario: Future proxy limit enforcement
- **WHEN** 未来实现代理级资源限制执行
- **THEN** 在声明该行为已实现前，MUST 用有实现证据的场景更新本规格

### Requirement: Quota period contract
系统 MUST 至少支持月度和年度流量配额窗口。当前实现证据 MUST 把周期配额行为视为缺口，直到存在存储、滚动结算和执行能力。

#### Scenario: Periodic quota remains a gap
- **WHEN** 产品或设计文档提到月度或年度流量配额行为
- **THEN** 在存在实现证据前，该行为 MUST 作为未来缺口跟踪

#### Scenario: Future quota period implementation
- **WHEN** 未来实现月度或年度配额窗口
- **THEN** 在声明该行为已实现前，MUST 用覆盖配额周期计算和滚动结算的有证据场景更新本规格

### Requirement: Proxy enablement enforcement contract
系统 MUST 在启用或接受代理配置前校验适用的用户、客户端、代理、端口、域名、配额和限制约束。当前实现证据 MUST 把 TCP/UDP 监听器冲突准入视为已在管理资源规格中覆盖，把配额和限制检查继续视为缺口，直到存在执行能力。

#### Scenario: Enablement quota check remains a gap
- **WHEN** 创建、启用或更新代理时，产品或设计文档要求执行配额或限制校验
- **THEN** 在存在实现证据前，配额或限制校验行为 MUST 保持为缺口

#### Scenario: Future enablement rejection
- **WHEN** 未来因为超过限制而拒绝代理创建、启用或更新操作
- **THEN** MUST 用被拒绝操作和拒绝原因的有证据场景更新本规格

### Requirement: Runtime enforcement contract
系统 MUST 在 TCP 连接、UDP 包/会话、HTTP 请求、HTTPS 代理以及已实现协议的正向代理运行时流量处理中执行适用配额和限制。当前实现证据 MUST 把运行时配额和限速行为视为缺口，直到存在执行能力。

#### Scenario: Runtime enforcement remains a gap
- **WHEN** 产品或设计文档提到运行时配额、并发、带宽或限速执行
- **THEN** 在存在实现证据前，该行为 MUST 作为未来缺口跟踪

#### Scenario: Future runtime denial or throttling
- **WHEN** 未来因为运行时配额或限制超限而拒绝、暂停、终止或限速流量
- **THEN** MUST 用受影响协议和可观测结果的有证据场景更新本规格

### Requirement: Observable denial reason contract
系统 MUST 使用可观测错误分类暴露配额、限制和限速拒绝，使限制耗尽能够与无关失败区分。当前实现证据 MUST 把配额拒绝可观测性视为缺口，直到分类能力存在。

#### Scenario: Denial classification remains a gap
- **WHEN** 产品或设计文档提到配额拒绝、带宽限速、权限拒绝或限制耗尽行为
- **THEN** 在存在证据支持的分类和报告前，该行为 MUST 保持为缺口

#### Scenario: Future denial classification
- **WHEN** 未来记录或返回配额、限制或限速拒绝
- **THEN** MUST 用分类和资源上下文的有证据场景更新本规格

### Requirement: Basic statistics exclusion
配额和限制基线 MUST NOT 仅凭基础流量计数声明配额执行能力。

#### Scenario: Counters do not imply enforcement
- **WHEN** 当前实现证据显示 TCP、UDP 或 HTTP 流量计数
- **THEN** 这些计数 MUST NOT 被视为配额、带宽、并发或限速执行
