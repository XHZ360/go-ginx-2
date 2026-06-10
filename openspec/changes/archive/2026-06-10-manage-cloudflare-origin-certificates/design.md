## Context

当前 HTTPS 证书生命周期已经完成一次重要收敛：HTTPS proxy 必须持有有效静态证书或托管 active certificate material，运行时基于证书/私钥文件健康检查决定是否终止 TLS；无证书或证书失效时代理进入需要配置状态，且不再 passthrough。

现有托管证书实现主要围绕 ACME DNS-01：

```text
certmanager.Service
  ├─ Issuer: ACME order + DNS-01
  ├─ DNSProvider: Cloudflare DNS challenge
  ├─ Storage: certificate_dir/managed/<host>/active.*
  └─ Store: managed_certificates lifecycle metadata
```

Cloudflare Origin CA 证书与 ACME 的差异不在运行时 TLS 使用方式，而在生命周期 provider：

- Origin CA 证书只适合作为 Cloudflare 到 origin 的证书，公网浏览器直连 origin 不具备普通 WebPKI 信任。
- Origin CA 签发不需要 DNS-01 challenge；服务端本地生成私钥和 CSR，Cloudflare 根据账号/zone 权限签发证书。
- Origin CA 有很长有效期，当前设计按长期证书维护和主动轮换处理，而不是 ACME 风格的短周期自动续期。
- Cloudflare Origin CA API 支持证书 list/create/get/revoke；官方推荐 API Token，Origin CA Service Key 已进入移除路径，因此新能力不应依赖 Service Key。

当前 `managed_certificates.provider` 字段已经存在，但语义更像 ACME DNS provider 名称。这个变更需要把 provider 类型显式化，否则后续会出现 `cloudflare` 既可能表示 ACME DNS provider，也可能表示 Origin CA issuer 的歧义。

## Goals / Non-Goals

**Goals:**

- 把 Cloudflare Origin CA 作为当前阶段 HTTPS 的一等托管证书 provider。
- 支持管理员为 HTTPS proxy 主机签发 Origin CA 证书，并通过现有 HTTPS runtime 执行 TLS 终止。
- 本地生成私钥和 CSR，Cloudflare API 只接收 CSR，私钥不进入 SQLite、管理 API 或普通日志。
- 支持管理员在 Admin UI 中维护 Cloudflare API Token credential，并通过写入式表单完成创建、更新、验证、禁用和删除。
- 复用 active/previous 文件布局、健康检查、fingerprint、热加载、失败保留 active 证书和 `needs_config` 语义。
- 记录并展示 Origin CA provider 元数据，包括 Cloudflare 证书 ID、hostnames、request type、requested validity、provider 状态和最近同步时间。
- 支持手动轮换、调度轮换候选、远端同步和强确认撤销。
- 明确 Cloudflare 使用前提：DNS 应为 proxied，SSL mode 应为 Full (strict) 或等价严格校验路径。

**Non-Goals:**

- 不在本阶段实现共享 wildcard 证书池；第一版仍以 proxy/host 绑定 active material。
- 不自动修改 Cloudflare DNS 记录、proxied 状态或 SSL mode。
- 不支持 Origin CA Service Key；新能力只设计 API Token 路径。
- 不把 Cloudflare API Token 明文保存到 SQLite，且不在管理 API 查询中回显 token 明文。
- 不实现通用自定义 CA 或任意 trust root 管理。
- 不把 Origin CA 证书声明为浏览器直连公网证书。
- 不自动撤销旧证书；撤销必须是显式管理动作并带强确认。

## Decisions

### Decision: Origin CA 是 managed certificate provider，不是 runtime 分支

HTTPS runtime 只关心证书 active material 是否可服务：证书/key 文件存在、可读、匹配、覆盖 SNI host、未过期，并且没有已确认的 provider-side 禁用状态。Cloudflare Origin CA 证书在 TLS 终止时仍是普通 `tls.Certificate`，不需要 runtime 专用分支。

```text
HTTPS SNI host
  │
  ▼
CertificateResolver
  │
  ├─ static_file
  └─ managed_certificate
       ├─ provider_type: acme_dns01 | cloudflare_origin_ca
       ├─ active.crt / active.key
       ├─ serving_status
       └─ provider_status
```

备选方案是新增 `origin_certificate` 独立表和独立 runtime resolver。这个方案隔离性强，但会复制 active/previous、健康检查、fingerprint、热加载和管理查询逻辑。Origin CA 与 ACME 的差异在签发/轮换 provider，不在 runtime 材料使用，因此复用现有托管证书边界更稳。

### Decision: 拆分 provider_type 与 provider_name

托管证书记录需要区分：

- `provider_type`: `acme_dns01` 或 `cloudflare_origin_ca`
- `provider_name`: `cloudflare` 等实际外部服务名称

现有 `provider` 字段可以迁移为 `provider_name` 或保留兼容读取，但新逻辑不得再用单个 `provider=cloudflare` 推断证书生命周期类型。旧 ACME 记录迁移时可设置 `provider_type=acme_dns01`、`provider_name=cloudflare`。

### Decision: 本地生成私钥和 CSR

Origin CA 签发流程使用本地私钥生成 CSR：

```text
generate private key
        │
        ▼
generate CSR(hostnames)
        │
        ▼
Cloudflare Origin CA create(csr, hostnames, request_type, requested_validity)
        │
        ▼
validate returned certificate with local key
        │
        ▼
activate active.crt / active.key
```

这样 Cloudflare 不接收私钥，符合现有私钥边界。第一版建议支持 `origin-ecc` 作为默认 request type，同时允许配置 `origin-rsa` 以兼容环境。`requested_validity` 默认使用长期值，并在 UI/CLI 中明确过期提醒仍由本系统维护。

### Decision: API Token 由 Admin UI 维护为 provider credential

Cloudflare Origin CA API Token 必须能在 Admin UI 中维护。系统新增 provider credential 边界：

```text
Admin UI
  │  create/update token (write-only)
  ▼
Admin API
  │
  ├─ SQLite: credential metadata
  │     ├─ credential_id
  │     ├─ provider_type=cloudflare_origin_ca
  │     ├─ name / zone_id / account_id / status
  │     ├─ token_fingerprint
  │     ├─ last_verified_at / last_error
  │     └─ secret_ref
  │
  └─ Secret store outside SQLite
        └─ encrypted or ACL-protected token material
```

Admin UI 的 token 字段是 write-only：创建和更新时可以提交明文 token，查询时只能看到 credential 名称、状态、作用域、token 指纹、最近验证时间和脱敏错误。管理 API、GraphQL、审计日志和普通日志都不得返回 token 明文。

SQLite 只保存 credential metadata 和 `secret_ref`。token material 必须放在 SQLite 之外的 secret store 中；第一版可以使用 `data/secrets/provider-credentials/<credential_id>` 这类受管文件路径，并设置严格文件权限。后续可以替换为 OS credential store 或 KMS，但对上层只暴露 `credential_id`。

不引入 Origin CA Service Key。Cloudflare 已将 Service Key 放入移除路径，新能力如果依赖 Service Key，会在短期内产生迁移债务。

不保留环境变量 token 路径。Cloudflare Origin CA provider 不从 `CLOUDFLARE_*` 或其他环境变量读取 API Token；所有 Origin CA token 都必须通过 Admin UI credential 写入 SQLite 外 secret store。HTTPS proxy 或托管证书记录通过 `credential_id` 引用 Cloudflare credential；如果没有显式选择，系统可以使用唯一默认 credential。

第一阶段支持一个或多个 Cloudflare Origin CA credential，但不实现复杂 RBAC 和租户隔离。凭据的可见性仍限定为管理员管理面。

### Decision: Origin CA provider metadata 独立于 active material health

active material health 证明本地文件能否用于 TLS 终止；provider metadata 证明 Cloudflare 侧是否仍认可该 Origin CA 证书。两者需要组合展示：

- `serving_status`: `usable` / `expiring_soon` / `expired` / `missing` / `invalid`
- `operation_status`: `idle` / `issuing` / `renewing` / `issue_failed` / `renewal_failed`
- `provider_status`: `active` / `revoked` / `missing_remote` / `unknown`

当同步确认 active certificate 对应的 Cloudflare 证书已撤销或远端缺失时，系统必须把该状态展示为不可用于 Cloudflare Origin CA 服务，并让 HTTPS proxy 进入需要配置状态。同步失败不能直接禁用本地仍有效的 active material，但必须记录 `provider_status=unknown` 或等价状态和脱敏错误。

### Decision: 轮换语义复用 renew 操作状态，但实现为重新签发

Origin CA 没有 ACME renewal order；轮换实质是生成新 key/CSR 并创建新 Origin CA 证书。为减少管理面复杂度，外部 lifecycle 可以继续使用 “renew/rotate” 语义：

- 管理 API 可提供 `rotateCloudflareOriginCertificate` 或在通用 `renewManagedCertificate` 中传入 provider action。
- `operation_status=renewing` 可以用于轮换中状态。
- 成功轮换后更新 active material、保留 previous material、记录新 Cloudflare certificate ID。
- 失败时保留当前 active material。

调度窗口不应直接复用 ACME 默认值。Origin CA 有效期通常很长，默认轮换窗口建议独立配置，例如 `origin_ca_rotation_window`，同时保留紧急过期窗口和失败退避。

### Decision: 撤销必须显式强确认

Cloudflare Origin CA revoke 是危险动作：撤销当前 active certificate 会让 Cloudflare 到 origin 的 Full (strict) 连接失败。第一版不在成功轮换后自动撤销 previous 证书，只展示 previous/remote 证书信息并允许管理员显式撤销。

强确认至少需要匹配 proxy ID、host 和 Cloudflare certificate ID。撤销失败不得影响 active 文件；撤销成功后如果目标是当前 active certificate，系统必须重新同步并将代理标记为需要配置或证书不可服务。

### Decision: Cloudflare 环境检查先提示，不自动修改

Origin CA 证书只有在 Cloudflare 代理到 origin 的链路中才是正确选择。系统应在签发或状态页提示：

- DNS record 应为 proxied。
- SSL mode 应为 Full (strict) 或等价严格校验。
- 直连 origin 的浏览器不会信任 Origin CA 证书。

第一版可以通过 Cloudflare API 做只读检查并展示警告，但不自动修改 DNS 记录或 SSL mode。自动修改 Cloudflare zone 配置属于更高风险的运维动作，应后置。

## Risks / Trade-offs

- [Risk] `provider` 字段语义迁移不完整会让 ACME 与 Origin CA 记录混淆。→ Mitigation: 新增明确 `provider_type`，旧记录迁移为 `acme_dns01`，查询和 runtime 不再根据旧 `provider` 推断类型。
- [Risk] Cloudflare token 通过 Admin UI 写入后成为服务端持久 secret。→ Mitigation: token 明文只进入 SQLite 外 secret store，SQLite 只保存 secret 引用和指纹；查询、审计和日志不得回显 token。
- [Risk] Cloudflare token 权限过大。→ Mitigation: 文档要求使用最小权限 API Token，Admin UI 展示作用域建议和验证状态，错误输出脱敏。
- [Risk] 本地证书健康正常，但 Cloudflare 远端证书已撤销。→ Mitigation: provider sync 记录远端状态；确认撤销或缺失后将证书标记为不可服务，状态页显示最近同步时间。
- [Risk] Origin CA 长有效期导致过期被忽略。→ Mitigation: 本地维护 `not_after`、rotation window、过期提醒和列表过滤，不依赖 Cloudflare 邮件提醒。
- [Risk] 自动撤销 previous 证书可能破坏仍在使用的 Cloudflare 配置。→ Mitigation: 默认不自动撤销，所有 revoke 必须强确认。
- [Risk] first-stage credential 管理不等于完整多租户 secret manager。→ Mitigation: 当前阶段限定管理员维护 provider credential；RBAC、KMS 和 per-user secret ownership 后续独立设计。

## Migration Plan

1. 扩展托管证书 schema，新增 `provider_type`、`provider_name`、`credential_id`、Cloudflare Origin CA 非敏感 metadata 和 `last_synced_at` 等字段。
2. 迁移现有 ACME 记录：`provider_type=acme_dns01`，`provider_name` 从旧 `provider` 或配置派生。
3. 新增 provider credential metadata 存储和 SQLite 外 secret store；Admin API/UI 支持创建、更新、验证、禁用和删除 Cloudflare API Token credential。
4. 新增 Origin CA provider issuer，并接入现有 certmanager lifecycle service 或抽象后的 provider registry。
5. 管理 API/UI 增加 provider/credential 选择、Origin CA issue/rotate/sync/revoke 动作和 provider metadata 展示。
6. daemon controller 扩展为 provider-aware：ACME 使用 renewal window，Origin CA 使用 rotation window；两者共享退避和单飞。
7. 文档补充 Cloudflare API Token credential 维护、Full (strict)、proxied DNS、直连限制和撤销风险。

回滚到旧版本时，旧版本会忽略新增 metadata，但不能理解 `provider_type=cloudflare_origin_ca` 的证书生命周期操作。active/previous 文件仍是普通证书文件；若旧版本只按 active material 使用托管证书，可能继续服务已写入的 Origin CA 证书，但无法维护远端状态。

## Open Questions

无。第一阶段决策为：API Token-only、Admin UI-only provider credential、不保留环境变量 token、SQLite 外 secret store、每 proxy/host 独立 active material、默认长期有效期、手动强确认撤销、只读 Cloudflare 环境提示。
