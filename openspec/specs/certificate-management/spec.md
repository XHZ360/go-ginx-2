## Purpose

定义证书与域名生命周期契约，覆盖平台/自定义域名、文件型 HTTPS 代理证书、ACME DNS-01 自动化、私钥保护、续期、热加载、回滚，以及 Origin CA/自定义 CA 边界；同时区分已实现的控制通道 TLS 校验、configless 控制通道 TLS bootstrap 与仍待实现的代理证书生命周期能力。
## Requirements
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

### Requirement: Domain ownership and certificate binding contract
系统 MUST 支持平台域名边界、用户自定义域名所有权校验，以及代理域名的证书绑定规则。当前实现证据 MUST 把这些能力视为缺口，直到对应实现存在。

#### Scenario: Platform certificate scope remains a gap
- **WHEN** 产品或设计文档提到平台代理域名证书范围
- **THEN** 在存在实现证据前，该行为 MUST 作为未来缺口跟踪

#### Scenario: Custom domain ownership remains a gap
- **WHEN** 产品或设计文档提到自定义域名所有权校验或绑定行为
- **THEN** 在存在实现证据前，该行为 MUST 作为未来缺口跟踪

#### Scenario: Custom domain certificate isolation remains a gap
- **WHEN** 产品需求声明自定义域名 MUST NOT 复用平台通配证书
- **THEN** 在存在实现证据前，该行为 MUST 保持为缺口

### Requirement: File-backed HTTPS proxy certificate selection
系统 MUST 支持为 HTTPS 代理 TLS 终止配置文件型证书和私钥路径，并按代理主机 SNI 选择证书。私钥 MUST 保留在 SQLite 之外。

#### Scenario: HTTPS proxy certificate selected by host
- **WHEN** HTTPS 代理配置了证书和私钥文件，且公网 TLS 流量带有匹配的 SNI
- **THEN** 服务端使用该证书和私钥为该代理终止 TLS

#### Scenario: Private key path only
- **WHEN** HTTPS 代理证书元数据被持久化
- **THEN** SQLite 仅存储文件路径，且 MUST NOT 存储私钥材料

### Requirement: Manual certificate lifecycle contract
系统 MUST 支持托管代理域名的证书上传、校验、替换、禁用和状态可见性。当前实现证据 MUST 把上传/UI 生命周期能力视为缺口，直到对应实现存在。

#### Scenario: Manual upload remains a gap
- **WHEN** 产品或设计文档提到证书上传、替换、禁用或状态可见性
- **THEN** 在存在实现证据前，该行为 MUST 作为未来缺口跟踪

#### Scenario: Future manual certificate implementation
- **WHEN** 未来实现手动证书生命周期行为
- **THEN** 在声明该行为已实现前，MUST 用有实现证据的场景更新本规格

### Requirement: ACME DNS-01 automation contract
系统 MUST 支持对符合条件的代理域名使用 ACME DNS-01 签发和续期证书，DNS 提供商凭据以最小权限方式在 SQLite 之外提供。

#### Scenario: ACME automation issues managed certificate
- **WHEN** 对符合条件的 HTTPS 代理主机请求 ACME DNS-01 签发，且提供商校验成功
- **THEN** 系统获取、校验、存储并激活该主机的托管证书

#### Scenario: ACME automation preserves private-key boundary
- **WHEN** ACME DNS-01 签发或续期存储证书元数据
- **THEN** SQLite 仅存储生命周期元数据和文件路径，且 MUST NOT 存储私钥材料或 DNS 提供商令牌值

#### Scenario: ACME challenge cleanup is required
- **WHEN** 创建 DNS 挑战记录后，ACME DNS-01 校验完成或失败
- **THEN** 系统尝试清理挑战记录，并在不暴露提供商凭据的情况下记录清理失败

### Requirement: Private-key protection contract
系统 MUST 保护证书私钥，避免写入 SQLite、在管理 UI 中明文展示或进入普通日志。当前实现证据 MUST 把私钥文件路径边界视为已实现，把私钥上传/展示生命周期行为视为缺口，直到对应实现存在。

#### Scenario: Private-key handling remains a gap
- **WHEN** 产品或设计文档提到私钥材料存储、上传处理、日志脱敏或 UI 展示行为
- **THEN** 在存在实现证据前，该行为 MUST 作为未来缺口跟踪

#### Scenario: Future private-key implementation
- **WHEN** 未来实现代理私钥存储或上传行为
- **THEN** MUST 更新本规格，证明密钥不存入 SQLite、不以明文展示、且不写入普通日志后，才能声明该行为已实现

### Requirement: Renewal, hot reload, and rollback contract
系统 MUST 支持证书续期、经过校验的热加载、保留旧证书以便回滚，以及不削弱证书校验的失败处理。

#### Scenario: Renewal hot reloads valid replacement
- **WHEN** 托管证书续期成功，且替换证书/私钥对通过配置代理主机的校验
- **THEN** 新的 HTTPS 终止握手无需重启 HTTPS 监听器即可使用替换证书

#### Scenario: Renewal failure preserves active certificate
- **WHEN** 续期、校验、文件写入或热加载失败
- **THEN** 系统继续提供上一组生效的有效证书，并记录失败状态供检查

#### Scenario: Rollback material is retained
- **WHEN** 托管证书替换件成为生效证书
- **THEN** 上一组有效证书和私钥会被保留用于回滚，直到后续成功生命周期操作替换它们

### Requirement: Origin CA advanced mode contract
系统 MUST 把 Cloudflare Origin CA 或自定义 CA 信任视为显式高级模式；该模式需要配置的信任根，并且 MUST NOT 引入跳过证书校验的非安全路径。

#### Scenario: Origin CA remains a gap
- **WHEN** 设计文档提到 Origin CA 或自定义 CA 信任行为
- **THEN** 在存在实现证据前，该行为 MUST 作为未来缺口跟踪

#### Scenario: No insecure certificate skip
- **WHEN** 未来实现 Origin CA 或自定义 CA 信任行为
- **THEN** MUST 保持证书校验，且 MUST NOT 依赖跳过证书校验

### Requirement: Managed certificate active material boundary
系统 MUST 把托管 HTTPS 证书的 active material 作为独立的服务边界维护，且最近一次生命周期操作失败不得直接禁用仍有效的 active 证书。

#### Scenario: Failed renewal keeps valid active certificate
- **WHEN** 托管证书续期失败，但上一组 active 证书和私钥仍存在、未过期且通过代理主机校验
- **THEN** 系统继续把上一组 active 证书作为当前可服务材料，并记录续期失败供管理员检查

#### Scenario: Invalid active certificate is reported
- **WHEN** active 证书或私钥文件无法通过健康检查
- **THEN** 系统记录不可服务状态和脱敏错误，并且 MUST NOT 声明该证书材料可用于 TLS 终止

### Requirement: HTTPS proxy certificate requirement
系统 MUST 要求 HTTPS proxy 具备可服务证书；没有证书或证书失效的 HTTPS proxy MUST 被标记为失效或需要配置，并且不得以 passthrough 方式服务。

#### Scenario: HTTPS proxy without certificate is invalid
- **WHEN** 管理员创建、更新、启用或 daemon 启动加载 HTTPS proxy，且该 proxy 没有完整静态证书，也没有可服务托管证书
- **THEN** 系统把该 proxy 标记为证书失效或需要配置状态，并且该 proxy 不得对外提供 HTTPS 服务

#### Scenario: HTTPS proxy with invalid certificate is invalid
- **WHEN** HTTPS proxy 的静态证书或托管 active certificate material 过期、缺失、不可读、域名不匹配或 key 不匹配
- **THEN** 系统把该 proxy 标记为证书失效或需要配置状态，并记录脱敏错误供管理员修复

#### Scenario: Valid certificate restores HTTPS proxy
- **WHEN** 管理员配置有效静态证书，或托管证书签发/续期成功并激活可服务 active material
- **THEN** 系统允许该 HTTPS proxy 进入可服务状态，并允许 HTTPS 入口使用该证书执行 TLS 终止

### Requirement: Certificate lifecycle metadata remains secret-safe
系统 MUST 在扩展证书健康、操作和调度元数据时继续保持私钥和 DNS provider 凭据位于 SQLite 之外。

#### Scenario: Lifecycle fields store metadata only
- **WHEN** 系统持久化证书健康状态、操作状态、失败次数、下一次尝试时间、证书指纹或错误摘要
- **THEN** SQLite 只保存元数据、文件路径和脱敏错误，且 MUST NOT 保存私钥字节或 DNS provider token 值

#### Scenario: Health errors are sanitized
- **WHEN** 证书健康检查或生命周期操作失败
- **THEN** 系统记录可诊断的错误摘要，但 MUST NOT 把私钥内容、DNS provider token 或完整敏感响应写入普通日志、SQLite 或管理 API 响应

### Requirement: Managed certificate provider contract
系统 MUST 通过明确的托管证书 provider contract 执行 provider-specific 生命周期动作，而不是让调用方直接复制 ACME DNS-01 或 Cloudflare Origin CA 分支规则。

#### Scenario: ACME lifecycle uses ACME provider contract
- **WHEN** 系统为 ACME DNS-01 托管证书执行签发或续期
- **THEN** 该动作通过 ACME provider contract 解析 DNS provider、账号配置、续期请求和成功/失败结果
- **AND** 该路径 MUST NOT 要求或消费 Cloudflare Origin CA credential、Cloudflare certificate ID、Origin CA request type 或 requested validity

#### Scenario: Origin CA lifecycle uses Origin CA provider contract
- **WHEN** 系统为 Cloudflare Origin CA 托管证书执行签发、轮换、同步或撤销
- **THEN** 该动作通过 Origin CA provider contract 解析 credential、hostnames、request type、requested validity、provider status 和 Cloudflare certificate ID
- **AND** 该路径继续使用通用 active/previous material、健康检查、失败保留 active 证书和 secret-safe 错误语义

#### Scenario: Unsupported provider is rejected before mutation
- **WHEN** 管理 API、admin CLI 或 controller 请求不受支持的托管证书 provider 类型
- **THEN** 系统拒绝该请求并返回结构化错误
- **AND** 系统 MUST NOT 创建、更新或替换该代理的托管证书 active material

### Requirement: Provider-specific metadata validation boundary
系统 MUST 在 domain 或 provider 边界校验托管证书 provider 专属字段，使 service、controller、admin command 和 GraphQL resolver 不各自复制字段规则。

#### Scenario: Origin CA active result validates provider metadata
- **WHEN** Cloudflare Origin CA 签发或轮换准备把新证书声明为 active material
- **THEN** 系统验证 credential ID、Cloudflare certificate ID、hostnames、request type、requested validity、provider type 和 provider status 满足 Origin CA provider contract
- **AND** 校验失败时系统记录脱敏失败结果，并继续保留上一组可服务 active material

#### Scenario: ACME active result validates only ACME metadata
- **WHEN** ACME DNS-01 签发或续期准备把新证书声明为 active material
- **THEN** 系统验证 ACME provider contract 所需的 provider type、provider name、证书文件、私钥文件、有效期和指纹
- **AND** 系统 MUST NOT 因缺少 Origin CA 专属字段而拒绝 ACME 成功结果

#### Scenario: Historical records are read leniently and written strictly
- **WHEN** 系统读取缺少新 provider metadata 约束的历史托管证书记录
- **THEN** 系统可以用兼容默认值展示或调度该记录
- **AND** 后续写入、签发成功、轮换成功或 provider sync 更新 MUST 满足当前 provider metadata 校验规则

### Requirement: Origin CA rotation keeps manual revoke boundary
系统 MUST 保持 Cloudflare Origin CA 轮换与撤销解耦；成功轮换不得自动撤销 previous Cloudflare Origin CA 证书。

#### Scenario: Successful rotation does not revoke previous certificate
- **WHEN** Cloudflare Origin CA 证书轮换成功并激活新 active material
- **THEN** 系统记录新的 Cloudflare certificate ID，并在可用时保留 previous Cloudflare certificate ID
- **AND** 系统 MUST NOT 自动调用 Cloudflare revoke 删除 previous 证书

#### Scenario: Previous certificate revoke remains explicit
- **WHEN** 管理员需要撤销 previous Cloudflare Origin CA 证书
- **THEN** 管理员必须通过显式撤销动作提供 proxy ID、host 和目标 Cloudflare certificate ID 的强确认
- **AND** 撤销失败 MUST NOT 修改当前 active certificate material
