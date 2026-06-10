# certificate-lifecycle-health Specification

## Purpose
TBD - created by archiving change improve-certificate-lifecycle. Update Purpose after archive.
## Requirements
### Requirement: Active material health
系统 SHALL 对 HTTPS 证书 active material 执行健康检查，并用该结果决定 HTTPS proxy 是否可用于公网 TLS 终止。

#### Scenario: Healthy active material is usable
- **WHEN** HTTPS proxy 引用的静态证书或托管 active 证书和私钥文件存在、可读取、证书/key 匹配、证书覆盖该代理主机且尚未过期
- **THEN** 系统把该 active material 标记为可服务，并允许 HTTPS 入口在匹配 SNI 的新 TLS 握手中使用该证书

#### Scenario: Expiring active material remains usable
- **WHEN** active 证书尚未过期但 `not_after` 已进入配置的续期窗口
- **THEN** 系统把该 active material 标记为即将过期且仍可服务，并把该证书纳入续期候选

#### Scenario: Expired active material is not usable
- **WHEN** active 证书的有效期已经结束
- **THEN** 系统把该 active material 标记为已过期，并且 MUST NOT 使用该证书完成新的公网 TLS 终止握手

#### Scenario: Broken active material is not usable
- **WHEN** active 证书或私钥文件缺失、不可读、证书/key 不匹配、证书不覆盖代理主机或证书解析失败
- **THEN** 系统把该 active material 标记为不可服务，并记录脱敏的健康检查错误供管理端检查

#### Scenario: Missing active material invalidates HTTPS proxy
- **WHEN** 已启用 HTTPS proxy 没有完整静态证书，也没有可服务的托管 active 证书
- **THEN** 系统把该 HTTPS proxy 标记为证书失效或需要配置状态，并且 MUST NOT 把公网 TLS 字节透传到客户端目标

### Requirement: Serving state is independent from operation state
系统 MUST 将托管证书当前是否可服务的状态与最近一次签发或续期操作结果分开维护。

#### Scenario: Renewal failure preserves usable active material
- **WHEN** 托管证书续期失败，但当前 active 证书/私钥仍通过健康检查
- **THEN** 系统记录续期失败操作结果，同时继续把 active material 标记为可服务

#### Scenario: Issue failure without active material is unavailable
- **WHEN** 托管证书首次签发失败，且该代理主机没有可服务 active material
- **THEN** 系统记录签发失败操作结果，并把服务状态标记为不可服务，同时把对应 HTTPS proxy 标记为证书失效或需要配置状态

#### Scenario: Successful operation clears operation failure
- **WHEN** 后续签发或续期成功并激活新的 active material
- **THEN** 系统清除最近操作错误，重置失败计数，并把服务状态更新为健康检查结果

### Requirement: Lifecycle scheduling metadata
系统 SHALL 为托管证书维护续期调度元数据，以支持失败退避、下一次尝试时间和最近检查可见性。

#### Scenario: Renewal failure schedules retry
- **WHEN** 托管证书续期失败
- **THEN** 系统增加失败计数，记录最近尝试时间和脱敏错误，并计算 `next_attempt_at` 用于后续退避重试

#### Scenario: Renewal success resets schedule
- **WHEN** 托管证书续期成功并激活新的 active material
- **THEN** 系统清空退避状态，重置失败计数，并根据新的过期时间计算后续续期候选时间

#### Scenario: Health check records inspection time
- **WHEN** 系统完成托管证书 active material 健康检查
- **THEN** 系统记录 `last_checked_at` 或等价检查时间，供管理端展示和后续调度判断使用

### Requirement: Secret-safe lifecycle visibility
系统 MUST 通过管理查询暴露托管证书生命周期健康状态，同时不得暴露私钥字节或 DNS provider token。

#### Scenario: Admin status includes actionable lifecycle fields
- **WHEN** 管理员查看托管证书列表或代理详情
- **THEN** 响应包含代理 ID、主机、服务状态、操作状态、过期时间、最近签发或续期时间、最近检查时间、失败次数、下一次尝试时间和脱敏错误

#### Scenario: Admin status does not expose secret values
- **WHEN** 管理端返回托管证书状态、操作结果或健康错误
- **THEN** 响应 MUST NOT 包含私钥字节、DNS provider token 值或未脱敏的敏感错误上下文

