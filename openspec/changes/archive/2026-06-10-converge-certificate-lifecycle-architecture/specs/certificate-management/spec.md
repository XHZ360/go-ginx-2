## ADDED Requirements

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
