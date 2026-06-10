## Context

当前证书管理已经具备三个基础能力：

- 控制通道 TLS 材料在 configless 启动时自动生成，并由服务端启动时加载。
- HTTPS 代理可通过静态 `cert_file` / `key_file` 做 TLS 终止。
- 托管 HTTPS 证书可通过 ACME DNS-01 签发、存储到 `certificate_dir/managed/<host>/`，并在续期成功后通过文件 mtime 缓存热加载。

本变更同时调整 HTTPS proxy 的运行时策略：HTTPS proxy 不再代表“可能透传的 TLS TCP 代理”，而是代表“服务端必须持有证书并终止 TLS 的 HTTPS 入口”。需要纯 TLS passthrough 的场景后续应通过独立代理类型或显式高级模式表达，而不是复用 `https` proxy 的默认行为。

主要缺口不在“能不能签证书”，而在生命周期语义。`managed_certificates.status` 现在既表示证书是否有效，又表示最近一次签发/续期操作是否失败。运行时又只认 `valid` / `expiring_soon` 作为可用托管证书，这会让状态和服务能力互相牵连：

```text
当前模型

managed_certificates.status
        │
        ├─ 用于管理端展示
        ├─ 用于续期筛选
        └─ 用于运行时判断能否 TLS 终止

问题：一次续期失败可能覆盖 status，但 active.crt/active.key 仍然有效。
```

这次设计把“当前可服务材料”和“最近一次操作结果”拆开。这里有一个重要分层：`active material` 是 source-neutral 的，静态文件证书和托管 ACME 证书都要经过同一套健康检查；`operation` 和 `schedule` 只适用于托管 ACME 生命周期。

```text
目标模型

Proxy HTTPS Host
      │
      ▼
Certificate Material View
      │
      ├─ Source: static_file | managed_acme
      │
      ├─ Active material: cert_file/key_file/not_after/fingerprint
      │       │
      │       └─ serving_status: usable | expiring_soon | expired | missing | invalid
      │
      ├─ Proxy runtime status: online/offline | needs_config(certificate)
      │
      └─ Managed ACME extension
              ├─ operation_state: idle | issuing | renewing | issue_failed | renewal_failed
              └─ schedule_state: last_checked_at / next_attempt_at / failure_count
```

## Goals / Non-Goals

**Goals:**

- 让 HTTPS 运行时只使用通过实时校验或最近健康检查确认可用的 active material。
- 移除 HTTPS SNI passthrough 行为，要求所有已启用 HTTPS proxy 都必须通过有效证书执行 TLS 终止。
- 在 HTTPS proxy 没有证书或证书失效时，将代理运行时状态标记为证书失效/需要配置，并拒绝对应公网连接。
- 让续期失败、签发失败、证书文件损坏、证书过期等状态在管理端可见且可排查。
- 让失败续期不会禁用仍有效的 active 证书。
- 让续期调度具备下一次尝试时间、失败次数、退避和同一 proxy 单飞能力。
- 保持 SQLite 只保存元数据和路径，不保存私钥或 DNS provider token。

**Non-Goals:**

- 不实现完整证书上传 UI 或 PEM 上传 API。
- 不实现域名所有权验证。
- 不增加除 Cloudflare 之外的 DNS provider。
- 不实现 Cloudflare Origin CA 或自定义 CA 信任。
- 不实现控制通道 CA 信任根轮换；控制通道 server cert 轮换可作为后续独立变更讨论。

## Decisions

### Decision: 拆分 serving status 与 operation status

证书材料保留 active 文件路径和生命周期字段，同时新增或派生两组状态：

- `serving_status`：由 active cert/key 文件健康检查得到，决定运行时是否可以用于 TLS 终止。
- `operation_status`：记录最近一次 issue/renew 的过程和结果，仅适用于托管 ACME 证书，用于管理端、审计和重试调度。

备选方案是继续复用 `status` 并增加更多状态枚举，例如 `renewal_failed_but_serving`。这个方案短期改动少，但状态组合会越来越难维护：文件健康、操作结果、调度状态会挤在同一个字段里。拆分状态更清楚，也更适合后续接入手动上传和域名验证。

为降低迁移风险，本变更可以保留 `managed_certificates.status` 作为兼容字段，但它不得再作为运行时选择证书的依据。实现应新增明确的 serving 状态字段，或在查询/运行时从 active material 健康检查派生 serving 状态；管理端展示时必须优先展示 serving 状态和 operation 状态的组合。

### Decision: HTTPS proxy 必须使用有效证书

证书 resolver 在选择托管证书前必须确认 active material 可用：文件存在且可读、证书和 key 匹配、证书适配 SNI host、证书未过期。即将过期仍可服务，但会进入续期窗口。

如果最近一次 renew 失败，但 `active.crt` / `active.key` 仍然可用，运行时继续使用该证书。管理员看到的是“续期失败，但当前证书仍在服务”的状态。

如果没有完整静态证书，也没有可服务的托管 active 证书，HTTPS proxy 进入证书失效/需要配置状态，运行时拒绝该 SNI 对应连接，并记录可诊断错误。显式静态证书配置无效、托管 active material 缺失或健康检查失败时同样失败关闭，不自动降级 passthrough。

备选方案是保留 passthrough 作为无证书回退。该方案兼容旧部署，但会让 `https` proxy 同时承担“TLS TCP 透传”和“HTTPS 终止”两种语义，不利于证书状态管理。新的策略更严格，也更符合证书生命周期管理目标。

代理状态映射使用现有 `needs_config` 作为第一版外部状态，证书 summary 中的 `serving_status` 和脱敏错误给出具体原因，例如 missing、expired 或 invalid。实现如果新增更细的 `certificate_invalid` 状态，必须同步更新管理 API、UI 和文档；否则不得只用泛化的 `error` 隐藏可修复配置问题。

HTTPS listener 的生命周期不由单个 proxy 的证书健康直接决定。共享 listener 上若有其他可服务 HTTPS proxy，listener 必须继续运行；当 SNI 命中证书失效 proxy 时，该连接被拒绝或关闭。没有任何可服务证书的 listener 可以继续监听并按 SNI 返回证书缺失错误，也可以在不影响其他 proxy 的前提下不启动，具体实现以 listener reconciliation 的共享安全为准。

### Decision: 续期 controller 管理调度、单飞和退避

daemon 内的简单 ticker 将被抽象为托管证书 controller。controller 负责：

- 启动时扫描一次需要检查或续期的证书。
- 按固定基础间隔加 jitter 周期检查。
- 只对 `serving_status` 为 `usable` / `expiring_soon` 且 `not_after` 落入续期窗口的证书发起续期。
- 根据 `next_attempt_at` 跳过还在退避中的记录。
- 对同一 proxy/host 保证单飞，避免并发 ACME order 或并发替换文件。
- 成功后重置失败次数和下一次尝试时间；失败后记录错误、增加失败次数，并计算下一次尝试时间。

备选方案是只在现有 ticker 上加失败次数。这样可以少改代码，但调度语义仍分散在 daemon、store 和 certmanager 里，不利于测试。controller 边界更清楚。

### Decision: 文件替换继续使用 active/previous 对

成功签发或续期仍先写临时 cert/key，校验通过后再替换 active 文件，并把上一组 active 移到 previous。替换成功后，新连接通过 resolver 缓存 mtime 自动热加载。

失败路径不移动 active 文件，不覆盖 previous 文件。存储层只在完整替换成功后更新 active path、not_after 和 fingerprint。

### Decision: 持久化 active leaf certificate fingerprint

本变更第一版持久化 active leaf certificate 的 SHA-256 指纹，使用规范化的小写十六进制字符串。fingerprint 在健康检查或证书替换成功时从 leaf certificate 派生，并随 `not_after`、`serving_status` 一起更新。

fingerprint 的用途是帮助管理员和审计记录识别当前服务的证书是否已经替换成功，也用于比较 active/previous 材料时提供稳定标识。fingerprint 不包含私钥材料，不替代证书/key 校验，也不得作为唯一的运行时安全判断依据。

### Decision: 管理 API 输出 secret-safe 的健康视图

GraphQL 和 UI 不暴露私钥字节，也不暴露 DNS token。可以暴露文件路径、证书指纹、过期时间、健康状态、操作状态、失败次数、下一次尝试时间和脱敏错误。

文件路径已经属于现有管理面可见信息，本变更不扩大到私钥内容展示。

### Decision: TLS passthrough 后续作为独立能力

本变更取消 HTTPS proxy 的 passthrough 语义后，不在 `https` proxy 上保留显式高级模式开关。需要按 SNI 透传 TLS 字节的场景后续应作为独立能力设计，例如新的 TLS/SNI passthrough proxy 类型或独立 capability。

这样可以让 `https` proxy 始终表示“服务端终止公网 TLS 并转发 HTTP 请求”，而透传能力独立表达“服务端只读取 ClientHello SNI 并转发加密字节”。二者在目标协议、证书要求、运行时错误、统计和管理 UI 上都不同，拆开后更容易维护。

## Risks / Trade-offs

- [Risk] 新增状态字段后，旧记录缺少调度和健康数据。→ Migration: 迁移时为新增字段写入安全默认值，首次 daemon 启动或首次查询时执行健康检查补齐派生状态。
- [Risk] 依赖旧 passthrough 行为的 HTTPS proxy 会在升级后不可服务。→ Mitigation: 在文档和管理端明确标记 BREAKING 行为，升级前要求管理员为 HTTPS proxy 签发或配置证书；纯 TLS 透传场景后续迁移到独立能力。
- [Risk] 健康检查在每次握手都读文件会增加开销。→ Mitigation: 延续现有 mtime 缓存；只有文件 mtime 变化、缓存缺失或健康检查过期时重新读取证书。
- [Risk] ACME 失败退避过长可能错过临近过期证书。→ Mitigation: 退避设置上限，并在证书进入更紧急窗口时缩短下一次尝试间隔。
- [Risk] 续期 controller 和管理员手动 renew 并发。→ Mitigation: 对 proxy/host 使用单飞锁，手动操作可以复用同一生命周期服务。
- [Risk] 单一 Cloudflare token 配置仍是全局边界。→ Mitigation: 本变更只改善生命周期可靠性，多 provider 和 per-domain 凭据后续独立设计。

## Migration Plan

1. SQLite 迁移新增健康、操作和调度字段，保留现有 `status` 字段用于兼容或迁移映射。
2. 启动 daemon 时对现有 HTTPS proxy 和托管证书执行一次健康检查，填充 `serving_status`、`last_checked_at` 和调度字段；没有有效证书的 HTTPS proxy 标记为证书失效/需要配置。
3. 管理查询先兼容旧记录：缺少新字段时按 active 文件实时派生健康状态，并把无证书 HTTPS proxy 展示为不可服务。
4. 续期 controller 上线后替代现有简单 renewal loop。
5. 回滚到旧版本时，旧版本忽略新增字段；active/previous 文件布局保持不变，但旧版本可能恢复 passthrough 行为。

## Open Questions

无。
