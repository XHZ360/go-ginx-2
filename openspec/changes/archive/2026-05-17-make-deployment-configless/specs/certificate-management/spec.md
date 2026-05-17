## ADDED Requirements

### Requirement: Control-channel TLS bootstrap material
系统 SHALL 在 configless 服务端基础部署中生成和管理控制通道 TLS 所需材料，同时 MUST 保持私钥材料位于 SQLite 之外。

#### Scenario: Generate control TLS material on first start
- **WHEN** `goginx-server` 以 configless 模式启动且控制通道 TLS 证书或私钥不存在
- **THEN** 系统生成控制通道 CA、服务端证书和私钥，保存到受管证书目录，并使用该证书启动 QUIC 和 TCP+TLS 控制监听器

#### Scenario: Reuse existing control TLS material
- **WHEN** `goginx-server` 重启且受管控制通道 TLS 材料已经存在
- **THEN** 系统复用现有证书和私钥，而不是每次启动重新生成会破坏已入网客户端信任的材料

#### Scenario: Control private key remains outside SQLite
- **WHEN** 控制通道 TLS 材料被生成、加载或用于 join/enrollment 信任分发
- **THEN** SQLite MUST NOT 存储控制通道私钥材料

#### Scenario: Enrolled trust material supports certificate verification
- **WHEN** 管理员生成客户端 join/enrollment 材料
- **THEN** 系统提供客户端校验控制通道服务端身份所需的 CA、证书指纹或等价信任材料，而不要求客户端操作者手动复制 CA 文件

## MODIFIED Requirements

### Requirement: Control-channel TLS boundary
系统 MUST 区分控制通道 TLS 证书校验、configless 控制通道 TLS bootstrap，以及代理证书生命周期管理。

#### Scenario: Control TLS verification remains current evidence
- **WHEN** 当前实现证据显示 QUIC/TCP+TLS 控制通道已加载服务端证书并由客户端完成校验
- **THEN** 该证据只能用于证明控制通道 TLS 校验能力

#### Scenario: Generated control TLS is limited to control channel
- **WHEN** 服务端在 configless 模式下生成控制通道 CA、证书或私钥
- **THEN** 该材料只用于客户端/服务端控制通道认证和加密，不声明可作为公网代理 HTTPS 证书

#### Scenario: Control TLS trust can be enrolled without CA file copy
- **WHEN** 客户端通过 join/enrollment 流程入网
- **THEN** 客户端可以从受管入网材料获得控制通道信任根或固定信任信息，而不是要求操作者创建 `server_ca_file`

#### Scenario: Control TLS does not imply unrelated proxy certificate lifecycle
- **WHEN** 产品或设计文档提到域名所有权、手动证书生命周期或 Origin CA/自定义 CA 行为
- **THEN** 在存在实现证据前，这些行为 MUST 保持为缺口
