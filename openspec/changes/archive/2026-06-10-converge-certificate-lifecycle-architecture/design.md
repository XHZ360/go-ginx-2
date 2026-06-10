## Context

上一次 Cloudflare Origin CA 变更已经把 ACME DNS-01 与 Origin CA 都接入托管 HTTPS 证书生命周期。当前实现中已经存在 `provider_type`、Origin CA metadata、provider credential、secret store、daemon 轮换 controller 和管理端证书页面。

剩余问题集中在边界形状：

- `certmanager.Service` 仍通过多个方法分支区分 ACME 与 Origin CA，provider 专属字段和默认值在服务方法中分散处理。
- `Renew`、`SyncOriginCA`、`RevokeOriginCA`、`recordFailure`、`ensureCertificateRecord` 等路径多次通过 `ByProxyID` 重新读取同一证书。
- `resolveCredentialID` 通过 `ProviderCredentials().List` 后在内存中筛选默认 Origin CA credential，证书数量和 credential 数量增长后会产生不必要读放大。
- daemon controller 先用最大窗口查询候选，再在内存里按 provider-specific window 二次过滤；窗口计算在 controller、service 和 admin query 中各有局部实现。
- 管理查询已经批量构建证书摘要，但详情和证书动作仍缺少更明确的定点加载合同。

这次变更不改变管理员可见产品语义，而是把 provider、调度和查询边界整理成可测试合同。

## Goals / Non-Goals

**Goals:**

- 建立托管证书 provider contract，使 ACME DNS-01 与 Cloudflare Origin CA 的 issue、renew/rotate、sync、revoke、validate 和 defaulting 都由 provider 策略承接。
- 让 Origin CA provider 专属字段校验位于 domain/provider 边界，避免 service、controller、admin command 和 GraphQL resolver 重复判断。
- 将生命周期窗口计算收敛为一个调度策略，包括 ACME renewal window、Origin CA rotation window、失败退避、紧急过期重试和 `expiring_soon` 判定。
- 为生命周期操作提供可复用已加载 `domain.ManagedCertificate` 的服务入口，减少 controller 与手动动作中的重复 `ByProxyID` 查询。
- 为 provider credential 增加 provider/status 定点查询能力，避免默认 credential 解析读取全部 credential 后内存过滤。
- 保持 secret-safe 输出和现有管理 API/UI 字段兼容。

**Non-Goals:**

- 不新增新的证书 provider。
- 不改变 Cloudflare Origin CA 的运行时 TLS 处理方式。
- 不在成功轮换后自动撤销 previous Origin CA 证书。
- 不引入完整 KMS、多租户 secret ownership 或 RBAC 重设计。
- 不为了去重而抽出跨层通用框架；重复实现只在相关代码被实际触碰时收敛。
- 不把 controller 动态 sleep 作为 correctness 要求；本次可以保留固定扫描间隔，只要求候选和窗口计算收敛。

## Decisions

### Decision: 引入 provider strategy registry

`certmanager.Service` 保留作为外部入口，但内部通过 provider strategy 处理 provider-specific 行为：

```text
certmanager.Service
  ├─ lifecycleScheduler
  ├─ certificateStore / credentialStore
  └─ providerRegistry
       ├─ acmeDNS01Provider
       └─ cloudflareOriginCAProvider
```

strategy 的职责包括：

- 校验请求和已有证书记录是否满足 provider 合同。
- 从 settings/request/certificate 中计算默认字段。
- 执行 issue 或 renew/rotate。
- 执行 provider sync 或 revoke，若 provider 不支持则返回明确错误。
- 返回成功/失败结果所需 metadata，不直接绕过通用 active/previous material 与 failure recording 语义。

备选方案是继续在 `Service` 方法中扩大 `switch provider_type`。这个方案改动小，但每次新增 provider 或字段都会继续扩散到 controller、admin command 和测试中，正是本次要解决的问题。

### Decision: domain/provider 边界负责 provider 字段校验

`domain.ManagedCertificate.Validate` 可以继续做通用结构校验，但 Origin CA active 成功结果还需要 provider-level validation，例如 credential ID、Cloudflare certificate ID、hostnames、request type、requested validity 与 provider status 的一致性。

建议新增轻量函数或方法，例如：

```text
ValidateManagedCertificateForProvider(certificate)
ValidateProviderSuccess(providerType, result)
NormalizeManagedCertificateRequest(providerType, request, settings, existing)
```

这些函数可以位于 `internal/domain` 或 `internal/certmanager` 的 provider 边界；关键是 controller、admin service 和 GraphQL resolver 不再复制字段规则。

### Decision: 用统一 scheduler 计算窗口和候选时间

新增或抽取 `lifecycleScheduler`，统一处理：

- `WindowFor(provider_type)`：ACME 使用 renewal window，Origin CA 使用 rotation window。
- `ServingStatus(not_after, provider_type, now)`：`usable` / `expiring_soon` / `expired` 的判定。
- `DueAt(certificate)` 或 `IsDue(certificate, now)`：候选判断。
- `NextAttemptAt(now, failure_count, not_after)`：失败退避和临近过期重试。
- `MaxLookahead()`：store 批量候选查询需要的最大窗口。

controller、service failure handling、admin summary health 检查都消费这一套函数。这样后续修改 Origin CA rotation window 或紧急重试规则时只有一个来源。

### Decision: 生命周期操作接受已加载证书记录

新增内部入口，例如：

```text
RenewCertificate(ctx, certificate domain.ManagedCertificate)
SyncOriginCACertificate(ctx, certificate domain.ManagedCertificate)
RevokeOriginCACertificate(ctx, certificate domain.ManagedCertificate, request)
```

外部 API 仍可保留 `proxyID` 入口用于 CLI/Admin mutation，它们只负责加载一次目标证书后进入统一路径。controller 从 `ListRenewable` 或新的 provider-aware 候选查询拿到记录后直接调用记录入口。操作成功后为了返回最新状态，可以做一次目标记录刷新；provider 选择、窗口判断和 credential 解析前不应再重复 `ByProxyID`。

### Decision: store 增加定点 credential 和候选查询

`ProviderCredentialRepository` 增加 provider/status 维度查询，例如：

```text
DefaultByProviderType(ctx, providerType)
ListByProviderType(ctx, providerType, includeDisabled)
```

默认 Origin CA credential 解析应通过数据库过滤可用状态；多条候选时仍返回“需要显式 credential ID”的结构化错误。

`CertificateRepository` 可以保留 `ListRenewable(before, now)`，但推荐新增 provider-aware 候选查询或让 scheduler 传入 provider 窗口条件，避免“最大窗口全拉出再按 provider 二次过滤”的读放大。若 SQLite 查询复杂度过高，可以先保留单次批量查询加内存过滤，但必须避免 per-certificate `ByProxyID`。

### Decision: 管理查询维持字段兼容，优化加载路径

Admin GraphQL schema 和 UI 字段不需要破坏性变更。后端查询服务应调整为：

- 列表使用批量证书加载，不为每个 HTTPS proxy 调 `ByProxyID`。
- 详情使用目标 proxy + 目标 certificate 的定点加载，而不是通过全量列表再过滤。
- credential 列表继续分页展示 metadata；生命周期动作和默认 credential 解析使用定点/provider 查询。
- 所有错误摘要继续通过现有脱敏函数处理。

## Risks / Trade-offs

- [Risk] provider strategy 抽象过大，反而让 ACME 与 Origin CA 的简单路径更难读。→ Mitigation: 只抽出生命周期动作真正共享的边界，保留 provider 实现内的直观流程，不做泛型框架。
- [Risk] 统一 scheduler 与现有 admin summary 健康检查窗口不一致会短期改变状态显示。→ Mitigation: 先用现有默认值建立回归测试，迁移后断言同一证书在 controller 与 admin summary 中得到相同 `serving_status`。
- [Risk] 定点查询接口增加后，旧测试里的假 store 需要同步更新。→ Mitigation: 让新增接口保持小而稳定，并优先在 store 层提供默认 helper 或测试 stub。
- [Risk] provider 字段校验下沉可能暴露之前被服务层吞掉的无效记录。→ Mitigation: 对历史记录做宽读取、严写入；只有新写入或 active 成功结果必须满足新校验。
- [Risk] 查询优化容易只改代码不改行为测试。→ Mitigation: 为 fake store 添加调用计数，覆盖 controller 不重复 `ByProxyID`、credential 默认解析不调用全量 `List`、详情查询不走全量证书列表。

## Migration Plan

1. 增加 scheduler、provider strategy 接口和定点 store 查询接口，先保持外部 API 兼容。
2. 将现有 ACME issue/renew 和 Origin CA issue/rotate/sync/revoke 迁入对应 strategy，保留 active/previous、failure recording 和 secret-safe 行为。
3. 将 controller 改为消费统一 scheduler 和已加载证书记录入口。
4. 将 provider credential 默认解析改为 provider/status 定点查询。
5. 将 admin query 详情和证书动作改为目标加载，列表保持批量加载且避免 per-proxy 查询。
6. 收敛被触碰路径上的重复 helper，并补齐回归测试。

回滚策略：这些改动不要求破坏性 schema 迁移。若需要回滚，旧版本仍可读取已有托管证书和 credential metadata；新增的非破坏性索引或查询接口不会改变持久化语义。

## Open Questions

无。动态按下一张证书到期时间 sleep 可以作为后续规模化优化，不作为本 change 的完成条件。
