## ADDED Requirements

### Requirement: Provider state participates in managed certificate availability
系统 SHALL 在托管证书状态中维护 provider-side 状态，并在已确认 provider-side 不可用时阻止该证书被声明为可服务。

#### Scenario: Confirmed provider revocation blocks managed certificate serving
- **WHEN** 托管证书的 active material 本地健康检查通过，但 provider sync 确认该 active 证书已被远端撤销或不可用
- **THEN** 系统把该证书标记为 provider-side 不可服务，并把对应 HTTPS proxy 标记为证书失效或需要配置状态

#### Scenario: Unknown provider state does not erase local health
- **WHEN** provider sync 失败或 provider 状态未知，但 active material 仍存在、未过期且通过代理主机校验
- **THEN** 系统保留本地 serving status，同时记录 provider 状态未知和脱敏同步错误供管理员检查

## MODIFIED Requirements

### Requirement: Lifecycle scheduling metadata
系统 SHALL 为托管证书维护 provider-aware 续期或轮换调度元数据，以支持失败退避、下一次尝试时间、最近检查和最近同步可见性。

#### Scenario: Renewal or rotation failure schedules retry
- **WHEN** 托管证书续期或 provider-specific 轮换失败
- **THEN** 系统增加失败计数，记录最近尝试时间和脱敏错误，并计算 `next_attempt_at` 用于后续退避重试

#### Scenario: Renewal or rotation success resets schedule
- **WHEN** 托管证书续期或 provider-specific 轮换成功并激活新的 active material
- **THEN** 系统清空退避状态，重置失败计数，并根据新的过期时间和 provider 调度策略计算后续续期或轮换候选时间

#### Scenario: Health check records inspection time
- **WHEN** 系统完成托管证书 active material 健康检查
- **THEN** 系统记录 `last_checked_at` 或等价检查时间，供管理端展示和后续调度判断使用

#### Scenario: Provider sync records synchronization time
- **WHEN** 系统完成托管证书 provider sync
- **THEN** 系统记录 `last_synced_at` 或等价同步时间，供管理端展示和 provider 状态判断使用

### Requirement: Secret-safe lifecycle visibility
系统 MUST 通过管理查询暴露托管证书生命周期健康状态、provider 状态和 credential metadata，同时不得暴露私钥字节、DNS provider token 或 Cloudflare API Token 明文。

#### Scenario: Admin status includes actionable lifecycle fields
- **WHEN** 管理员查看托管证书列表或代理详情
- **THEN** 响应包含代理 ID、主机、provider 类型、credential ID、服务状态、操作状态、provider 状态、过期时间、最近签发或续期/轮换时间、最近检查时间、最近同步时间、失败次数、下一次尝试时间和脱敏错误

#### Scenario: Admin status does not expose secret values
- **WHEN** 管理端返回托管证书状态、操作结果、provider 结果或健康错误
- **THEN** 响应 MUST NOT 包含私钥字节、DNS provider token 值、Cloudflare API Token 明文或未脱敏的敏感错误上下文
