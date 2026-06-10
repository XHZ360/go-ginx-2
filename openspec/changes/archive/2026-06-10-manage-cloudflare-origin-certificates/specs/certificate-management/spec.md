## ADDED Requirements

### Requirement: Cloudflare Origin CA managed certificate contract
系统 MUST 支持 Cloudflare Origin CA 作为受管 HTTPS 证书来源，同时继续保持证书校验、私钥边界和 provider 凭据边界。

#### Scenario: Origin CA is a supported managed certificate source
- **WHEN** 管理员为 HTTPS proxy 请求 Cloudflare Origin CA 托管证书
- **THEN** 系统将该证书记录为 Cloudflare Origin CA provider 管理的 active certificate material，并通过通用 HTTPS 证书健康检查决定是否可用于 TLS 终止

#### Scenario: Origin CA does not bypass certificate verification
- **WHEN** Cloudflare Origin CA 证书被签发、轮换、同步或用于 HTTPS 运行时选择
- **THEN** 系统 MUST 校验证书/私钥匹配、证书覆盖代理主机、证书未过期和 provider-side 已确认可用状态，且 MUST NOT 依赖跳过证书校验的非安全路径

#### Scenario: Custom CA remains outside current support
- **WHEN** 设计文档或产品文档提到非 Cloudflare 的自定义 CA 信任根管理
- **THEN** 在存在实现证据前，该行为 MUST 保持为未来缺口

## MODIFIED Requirements

### Requirement: Renewal, hot reload, and rollback contract
系统 MUST 支持证书续期或 provider-specific 轮换、经过校验的热加载、保留旧证书以便回滚，以及不削弱证书校验的失败处理。

#### Scenario: Renewal or rotation hot reloads valid replacement
- **WHEN** 托管证书续期或轮换成功，且替换证书/私钥对通过配置代理主机的校验
- **THEN** 新的 HTTPS 终止握手无需重启 HTTPS 监听器即可使用替换证书

#### Scenario: Renewal or rotation failure preserves active certificate
- **WHEN** 续期、轮换、校验、文件写入或热加载失败
- **THEN** 系统继续提供上一组生效的有效证书，并记录失败状态供检查

#### Scenario: Rollback material is retained
- **WHEN** 托管证书替换件成为生效证书
- **THEN** 上一组有效证书和私钥会被保留用于回滚，直到后续成功生命周期操作替换它们

### Requirement: Certificate lifecycle metadata remains secret-safe
系统 MUST 在扩展证书健康、操作、provider、credential 和调度元数据时继续保持私钥、DNS provider 凭据和 Cloudflare API Token 明文位于 SQLite 之外。

#### Scenario: Lifecycle fields store metadata only
- **WHEN** 系统持久化证书健康状态、操作状态、provider 类型、credential ID、Cloudflare certificate ID、失败次数、下一次尝试时间、证书指纹或错误摘要
- **THEN** SQLite 只保存元数据、secret 引用、文件路径和脱敏错误，且 MUST NOT 保存私钥字节、DNS provider token 值或 Cloudflare API Token 明文

#### Scenario: Health errors are sanitized
- **WHEN** 证书健康检查或生命周期操作失败
- **THEN** 系统记录可诊断的错误摘要，但 MUST NOT 把私钥内容、DNS provider token、Cloudflare API Token 明文或完整敏感响应写入普通日志、SQLite 或管理 API 响应

## REMOVED Requirements

### Requirement: Origin CA advanced mode contract
**Reason**: Cloudflare Origin CA 从未来高级模式缺口提升为当前阶段 HTTPS 的主要托管证书来源；继续把它标记为缺口会和新的 provider 合同冲突。
**Migration**: 使用新增的 `Cloudflare Origin CA managed certificate contract` 和 `cloudflare-origin-certificates` capability 表达 Origin CA 行为；非 Cloudflare 的自定义 CA 信任根仍作为未来缺口保留。
