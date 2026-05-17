## ADDED Requirements

### Requirement: Client enrollment without operator-authored config
系统 SHALL 支持客户端通过一次性 join/enrollment 流程获得安全控制通道所需配置，使基础部署不要求操作者手写 `client.json`。

#### Scenario: Generate client join material
- **WHEN** 已授权管理员为某个客户端生成 join/enrollment 材料
- **THEN** 系统生成包含或可换取服务端地址、TLS 信任材料、TLS 服务端名称、客户端 ID、客户端凭据和协议默认值的一次性 join 材料

#### Scenario: Join material is time-bounded or single-use
- **WHEN** join/enrollment 材料已被消费、过期或被撤销
- **THEN** 后续使用该材料的客户端入网请求 MUST 被拒绝

#### Scenario: Client join writes managed local state
- **WHEN** 操作者在客户端主机执行文档化的 join 流程
- **THEN** 客户端把控制通道连接和认证所需信息写入本地受管状态，使后续无 `-config` 启动可以连接服务端

#### Scenario: Joined client authenticates over secure transport
- **WHEN** 客户端使用 join 流程写入的本地受管状态启动
- **THEN** 客户端通过已验证的控制通道服务端身份和有效客户端凭据完成认证，并接收代理快照

#### Scenario: Join does not log reusable secrets
- **WHEN** 生成、消费或拒绝 join/enrollment 材料
- **THEN** 普通日志和审计事件 MUST NOT 明文记录可重放的客户端 credential 或完整 join secret

## MODIFIED Requirements

### Requirement: Server certificate verification
客户端 MUST 校验控制通道服务端证书链和服务端名称，控制通道基线 MUST NOT 依赖跳过证书校验的非安全路径；信任来源可以来自显式配置的 CA 文件，也可以来自 join/enrollment 流程写入的受管信任材料。

#### Scenario: Trusted server certificate from explicit CA file
- **WHEN** 服务端提供的证书受配置的客户端 CA 文件信任，并且匹配配置的服务端名称
- **THEN** 客户端可以继续控制通道握手

#### Scenario: Trusted server certificate from enrolled trust material
- **WHEN** 服务端提供的证书受 join/enrollment 流程写入的 CA 或固定信任材料信任，并且匹配写入的服务端名称
- **THEN** 客户端可以继续控制通道握手

#### Scenario: Untrusted server certificate
- **WHEN** 服务端证书不受信任、已过期、名称不匹配或因其他原因无效
- **THEN** 客户端 MUST 拒绝控制通道连接

#### Scenario: No insecure join fallback
- **WHEN** 客户端通过 join/enrollment 流程获得控制通道配置
- **THEN** 客户端 MUST NOT 使用跳过证书校验的 TLS 配置作为连接成功的回退路径

### Requirement: Client reconnect recovery baseline
客户端 MUST 在临时拨号或运行时失败后按照配置或受管状态中的重连退避恢复控制通道，并且服务端关闭时 MUST 使活跃控制连接可被客户端及时感知。

#### Scenario: Transient startup or listener failure retries
- **WHEN** 客户端启动时控制监听器暂不可用，或运行中控制监听器重启
- **THEN** `goginx-client` 按配置文件或客户端受管状态中的 `reconnect.initial_delay` 和 `reconnect.max_delay` 重试，并在控制面恢复后重新认证和恢复代理快照

#### Scenario: Authentication rejection is not retried forever
- **WHEN** 服务端因客户端凭据无效而拒绝认证
- **THEN** 客户端立即退出该运行流程，而不是把永久认证失败当成临时网络失败持续重试

#### Scenario: Server shutdown closes active control sessions
- **WHEN** 服务端控制监听器或守护进程关闭
- **THEN** 活跃 QUIC 和 TCP+TLS 控制连接被关闭，使客户端能够检测故障并进入重连退避流程
