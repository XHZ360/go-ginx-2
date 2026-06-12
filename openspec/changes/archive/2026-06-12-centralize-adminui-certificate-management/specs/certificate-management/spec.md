## MODIFIED Requirements

### Requirement: Domain ownership and certificate binding contract
系统 MUST 支持平台域名边界、用户自定义域名所有权校验，以及 HTTPS proxy SNI 域名的显式证书绑定规则。当前实现证据仍 MUST 把平台证书范围和自定义域名所有权能力视为缺口，直到对应实现存在。

#### Scenario: Platform certificate scope remains a gap
- **WHEN** 产品或设计文档提到平台代理域名证书范围
- **THEN** 在存在实现证据前，该行为 MUST 作为未来缺口跟踪

#### Scenario: Custom domain ownership remains a gap
- **WHEN** 产品或设计文档提到自定义域名所有权校验或绑定行为
- **THEN** 在存在实现证据前，该行为 MUST 作为未来缺口跟踪

#### Scenario: Custom domain certificate isolation remains a gap
- **WHEN** 产品需求声明自定义域名 MUST NOT 复用平台通配证书
- **THEN** 在存在实现证据前，该行为 MUST 保持为缺口

#### Scenario: HTTPS proxy selects compatible certificate
- **WHEN** 管理员为 HTTPS proxy 选择证书资源
- **THEN** 系统校验证书 host 或 hostnames 覆盖该 proxy 的 SNI 域名，并拒绝不兼容的绑定

#### Scenario: Certificate binding is explicit
- **WHEN** HTTPS proxy 被创建、更新、启用或 daemon 启动加载
- **THEN** 系统通过该 proxy 的显式证书绑定解析 TLS active material，而不是依赖管理员在 proxy 表单中直接维护证书文件和私钥文件路径

#### Scenario: Certificate binding remains one-to-one for this change
- **WHEN** 管理员尝试把已绑定到其他 HTTPS proxy 的证书绑定到新的 HTTPS proxy
- **THEN** 系统拒绝该绑定并返回可消费冲突，除非后续规格显式引入多 proxy 共享证书语义

### Requirement: File-backed HTTPS proxy certificate selection
系统 MUST 支持把文件型证书和私钥路径登记为可选择的 HTTPS 证书资源，并按 HTTPS proxy 的显式证书绑定和 SNI 选择证书。私钥 MUST 保留在 SQLite 之外。

#### Scenario: HTTPS proxy certificate selected by binding and host
- **WHEN** HTTPS proxy 绑定了 file-backed 证书资源，且公网 TLS 流量带有匹配的 SNI
- **THEN** 服务端使用该证书资源记录的证书和私钥文件为该 proxy 终止 TLS

#### Scenario: Private key path only
- **WHEN** file-backed HTTPS 证书元数据被持久化
- **THEN** SQLite 仅存储文件路径、状态、指纹、有效期和脱敏错误，且 MUST NOT 存储私钥材料

#### Scenario: Legacy proxy file paths are migrated or adapted
- **WHEN** 系统读取迁移前直接保存在 HTTPS proxy 上的完整证书文件和私钥文件路径
- **THEN** 系统把它们迁移、适配或呈现为 file-backed 证书资源，并保持该 proxy 可继续使用该证书
- **AND** 新的 AdminUI 主流程 MUST 使用证书选择而不是直接编辑 proxy 文件路径

### Requirement: HTTPS proxy certificate requirement
系统 MUST 要求 HTTPS proxy 具备可服务证书；没有显式绑定可服务证书或证书失效的 HTTPS proxy MUST 被标记为失效或需要配置，并且不得以 passthrough 方式服务。

#### Scenario: HTTPS proxy without certificate is invalid
- **WHEN** 管理员创建、更新、启用或 daemon 启动加载 HTTPS proxy，且该 proxy 没有绑定可服务证书
- **THEN** 系统把该 proxy 标记为证书失效或需要配置状态，并且该 proxy 不得对外提供 HTTPS 服务

#### Scenario: HTTPS proxy with invalid certificate is invalid
- **WHEN** HTTPS proxy 绑定的证书 active material 过期、缺失、不可读、域名不匹配、key 不匹配或 provider status 阻止服务
- **THEN** 系统把该 proxy 标记为证书失效或需要配置状态，并记录脱敏错误供管理员修复

#### Scenario: Valid certificate restores HTTPS proxy
- **WHEN** 管理员绑定有效 file-backed 证书，或托管证书签发/续期成功并激活可服务 active material
- **THEN** 系统允许该 HTTPS proxy 进入可服务状态，并允许 HTTPS 入口使用该证书执行 TLS 终止

## ADDED Requirements

### Requirement: Certificate resources can exist before proxy binding
系统 MUST 支持管理员在绑定 HTTPS proxy 之前创建或签发证书资源，并在后续 proxy 创建/编辑流程中选择该证书。

#### Scenario: Create unbound certificate for a host
- **WHEN** 管理员从 Certificates 页或 proxy 表单跳转创建证书，并提供目标 host/provider 配置
- **THEN** 系统创建未绑定或待绑定的证书资源，并维护其生命周期状态

#### Scenario: Bind existing certificate to HTTPS proxy
- **WHEN** 管理员创建或更新 HTTPS proxy 并选择未绑定且 hostnames 兼容的证书资源
- **THEN** 系统把该证书绑定到 proxy，并在运行时使用该绑定解析 TLS active material

#### Scenario: Unbind certificate by changing proxy selection
- **WHEN** 管理员把 HTTPS proxy 的证书选择改为其他兼容证书或清空选择
- **THEN** 系统更新绑定关系，并让原证书保留为未绑定资源，除非管理员在 Certificates 页显式删除它

### Requirement: Certificate deletion uses risk-based confirmation
系统 MUST 允许管理员删除 certificate 资源，并在删除前按引用状态和可服务状态决定是否需要强确认。

#### Scenario: Serving referenced certificate delete requires strong confirmation
- **WHEN** 管理员请求删除仍被 HTTPS proxy 绑定且当前可服务的证书
- **THEN** 系统要求管理员提供 certificate ID、host 或等价强确认材料
- **AND** 删除成功后系统解除该 proxy 的证书绑定，并把受影响 HTTPS proxy 标记为证书失效或需要配置

#### Scenario: Unreferenced certificate can be deleted without secondary confirmation
- **WHEN** 管理员请求删除未被任何 HTTPS proxy 绑定的证书
- **THEN** 系统删除证书元数据，并按 provider/storage 策略安全清理可删除的受管 active/previous material
- **AND** 系统 MUST NOT 删除不属于受管证书目录或无法安全归属的任意外部文件路径
- **AND** 系统 MUST NOT 要求二次确认或输入式强确认

#### Scenario: Invalid or expired certificate can be deleted without secondary confirmation
- **WHEN** 管理员请求删除无效、失效/过期、missing active material 或 provider status 阻止服务的证书
- **THEN** 系统删除证书元数据，解除存在的 HTTPS proxy 绑定，并把受影响 proxy 标记为证书失效或需要配置
- **AND** 系统 MUST NOT 要求二次确认或输入式强确认

#### Scenario: Delete result remains secret-safe
- **WHEN** 证书删除成功或失败
- **THEN** 系统返回证书 ID、删除状态、受影响 proxy、强确认需求或脱敏错误，且 MUST NOT 返回私钥内容或 provider token
