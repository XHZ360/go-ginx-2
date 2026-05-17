## Purpose

定义客户端/服务端控制通道契约，覆盖安全传输、客户端认证、服务端证书校验、代理快照下发、心跳/会话存活、最新会话路由、TCP+TLS 回退代理子流、客户端重连退避与监听器重启恢复，并显式跟踪事件重放和配置版本协调等剩余恢复缺口。

## Requirements

### Requirement: Secure control transport baseline
系统 MUST 通过 QUIC 和 TCP+TLS 提供已认证、加密的客户端/服务端控制通道。TCP+TLS MUST 支持控制认证、代理快照下发、心跳，以及在回退连接上的分帧代理子流。

#### Scenario: QUIC control connection succeeds
- **WHEN** 客户端使用受信任的服务端 TLS 身份和有效凭据连接到配置的 QUIC 控制监听器
- **THEN** 服务端接受控制连接并创建已认证会话

#### Scenario: TCP+TLS control connection succeeds
- **WHEN** 客户端使用受信任的服务端 TLS 身份和有效凭据连接到配置的 TCP+TLS 控制监听器
- **THEN** 服务端接受控制连接并创建已认证 TCP+TLS 会话

#### Scenario: TCP+TLS proxy stream routing succeeds
- **WHEN** QUIC 不可用且客户端已通过 TCP+TLS 完成认证
- **THEN** 服务端可以在 TCP+TLS 连接上打开分帧代理子流，并把代理流量路由到客户端

#### Scenario: TCP+TLS head-of-line limitation documented
- **WHEN** 文档描述 TCP+TLS 回退代理流
- **THEN** 文档 MUST 说明多个复用流共享一条 TCP 连接，可能受到 TCP 队头阻塞影响

### Requirement: Server certificate verification
客户端 MUST 校验控制通道服务端证书链和服务端名称，控制通道基线 MUST NOT 依赖跳过证书校验的非安全路径。

#### Scenario: Trusted server certificate
- **WHEN** 服务端提供的证书受配置的客户端 CA 文件信任，并且匹配配置的服务端名称
- **THEN** 客户端可以继续控制通道握手

#### Scenario: Untrusted server certificate
- **WHEN** 服务端证书不受信任、已过期、名称不匹配或因其他原因无效
- **THEN** 客户端 MUST 拒绝控制通道连接

### Requirement: Client authentication
服务端 MUST 在注册活跃控制通道会话或向客户端提供代理配置前认证客户端凭据。

#### Scenario: Valid client credential
- **WHEN** 客户端在控制通道握手期间提供已知客户端 ID 和匹配凭据
- **THEN** 服务端为该客户端注册已认证会话

#### Scenario: Invalid client credential
- **WHEN** 客户端提供未知客户端 ID 或错误凭据
- **THEN** 服务端 MUST 拒绝连接，且 MUST NOT 注册活跃会话

### Requirement: Proxy snapshot delivery
服务端 MUST 在控制通道认证成功后，向已认证客户端发送其拥有的代理快照。

#### Scenario: Snapshot after authentication
- **WHEN** 客户端认证成功
- **THEN** 服务端通过控制通道发送该客户端拥有的代理配置

#### Scenario: No snapshot before authentication
- **WHEN** 客户端认证尚未成功
- **THEN** 服务端 MUST NOT 发送客户端拥有的代理配置

### Requirement: Heartbeat and session liveness
客户端 MUST 通过控制通道发送心跳或状态消息，服务端 MUST 根据这些消息更新会话存活状态。

#### Scenario: Heartbeat updates liveness
- **WHEN** 已认证客户端通过控制通道发送心跳
- **THEN** 服务端更新该客户端的会话存活记录

#### Scenario: Heartbeat timeout remains a gap
- **WHEN** 文档描述超出当前实现证据的心跳超时、软离线、硬离线或恢复行为
- **THEN** 在存在实现证据前，该行为 MUST 保持为缺口

### Requirement: Latest authenticated session routing
服务端 MUST 把新的代理子通道路由到客户端最新的有效已认证会话。

#### Scenario: Latest session selected
- **WHEN** 同一客户端曾存在多个会话
- **THEN** 新的代理子通道路由到最新的有效已认证会话

#### Scenario: Duplicate-session grace remains a gap
- **WHEN** 文档描述重复会话代际编号、宽限期或旧会话排空行为
- **THEN** 在存在实现证据前，该行为 MUST 保持为缺口

### Requirement: Client reconnect recovery baseline
客户端 MUST 在临时拨号或运行时失败后按照配置的重连退避恢复控制通道，并且服务端关闭时 MUST 使活跃控制连接可被客户端及时感知。

#### Scenario: Transient startup or listener failure retries
- **WHEN** 客户端启动时控制监听器暂不可用，或运行中控制监听器重启
- **THEN** `goginx-client` 按配置的 `reconnect.initial_delay` 和 `reconnect.max_delay` 重试，并在控制面恢复后重新认证和恢复代理快照

#### Scenario: Authentication rejection is not retried forever
- **WHEN** 服务端因客户端凭据无效而拒绝认证
- **THEN** 客户端立即退出该运行流程，而不是把永久认证失败当成临时网络失败持续重试

#### Scenario: Server shutdown closes active control sessions
- **WHEN** 服务端控制监听器或守护进程关闭
- **THEN** 活跃 QUIC 和 TCP+TLS 控制连接被关闭，使客户端能够检测故障并进入重连退避流程

### Requirement: Advanced recovery gap tracking
控制通道规格 MUST 把事件重放、配置版本协调、更细的代理恢复语义和重复会话排空作为尚未完整实现的需求/设计行为继续跟踪。

#### Scenario: Advanced recovery behavior planned but not implemented
- **WHEN** 产品或设计文档提到事件重放、配置版本协调、代理恢复语义或重复会话排空
- **THEN** 本规格 MUST 标识该行为是已有证据支持，还是仍为未来缺口

#### Scenario: Future recovery implementation
- **WHEN** 未来实现事件重放、配置版本协调或更完整的代理恢复语义
- **THEN** 在声明该行为已实现前，MUST 用有实现证据的场景更新本规格
