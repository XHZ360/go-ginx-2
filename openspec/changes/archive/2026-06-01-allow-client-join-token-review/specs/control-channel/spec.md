## MODIFIED Requirements

### Requirement: Client enrollment without operator-authored config
系统 SHALL 支持客户端通过一次性 join/enrollment 流程获得安全控制通道所需配置，使基础部署不要求操作者手写 `client.json`。

#### Scenario: Generate client join material
- **WHEN** 已授权管理员为某个客户端生成 join/enrollment 材料
- **THEN** 系统生成包含或可换取服务端地址、TLS 信任材料、TLS 服务端名称、客户端 ID、客户端凭据和协议默认值的 join 材料
- **AND** 管理员可以在该材料未使用且未过期期间重复查看完整 token

#### Scenario: Reuse, expiry, and revocation are rejected
- **WHEN** join/enrollment 材料已被客户端消费、过期或被撤销
- **THEN** 后续客户端 join 尝试 MUST 被拒绝，并且管理员侧不再返回该 token 明文

#### Scenario: Join secrets remain out of logs
- **WHEN** 生成、查看、消费或拒绝 join/enrollment 材料
- **THEN** 普通日志和审计事件 MUST NOT 明文记录可重放的客户端 credential 或完整 join token
