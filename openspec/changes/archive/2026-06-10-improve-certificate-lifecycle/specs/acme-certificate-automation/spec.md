## ADDED Requirements

### Requirement: Retry-aware renewal controller
系统 MUST 使用可观察、可退避的续期控制器调度托管 ACME 证书续期，而不是把失败续期无限制地立即重试。

#### Scenario: Due certificate renews once per controller pass
- **WHEN** 托管证书处于续期窗口内、未处于退避期且没有同一代理主机的续期正在运行
- **THEN** 守护进程对该证书发起一次续期尝试，并记录最近尝试时间

#### Scenario: Backoff skips premature retry
- **WHEN** 托管证书存在失败退避并且当前时间早于 `next_attempt_at`
- **THEN** 续期控制器在该轮次跳过该证书，且 MUST NOT 创建新的 ACME order

#### Scenario: Concurrent renewals are coalesced
- **WHEN** 管理员手动续期与 daemon 自动续期同时针对同一代理主机触发
- **THEN** 系统只允许一个续期操作执行，另一个操作等待、复用结果或返回可消费的忙碌状态

### Requirement: ACME operation failure preserves serving material
系统 MUST 在 ACME 签发或续期失败时保留当前 active certificate material，并把失败记录为操作结果和调度状态。

#### Scenario: Renewal failure does not replace active files
- **WHEN** ACME 订单、DNS-01 校验、证书获取、证书校验或文件替换失败
- **THEN** 系统 MUST NOT 替换当前 active 证书/私钥文件，并记录续期失败、失败次数和下一次尝试时间

#### Scenario: Missing provider credential records configuration failure
- **WHEN** ACME 自动化启用但 Cloudflare token 环境变量缺失或为空
- **THEN** 系统记录 provider 配置错误和下一次尝试时间，且 MUST NOT 修改现有 active 证书/私钥文件

### Requirement: ACME status exposes health and schedule
系统 MUST 在托管证书状态中暴露 ACME 生命周期、健康检查和续期调度信息，同时保持凭据 secret-safe。

#### Scenario: Status includes retry context
- **WHEN** 管理员查看 ACME 托管证书状态
- **THEN** 输出包含服务状态、操作状态、过期时间、最近续期尝试时间、失败次数、下一次尝试时间和最近错误

#### Scenario: Status hides provider token
- **WHEN** ACME provider 配置或请求失败
- **THEN** 管理端状态和日志 MUST NOT 包含 Cloudflare token 值
