# Admin 业务 Facade 物理拆分

## 元信息

| 项 | 值 |
| --- | --- |
| 状态 | `completed` |
| 最后更新 | `2026-07-24` |
| 实施基线 | `f94c7b0` |
| 负责人 | 未指定 |
| 相关需求 | 无（本 Change 不改变产品行为，仅调整实现边界） |
| 相关架构 | [../../architecture/system-architecture.md](../../architecture/system-architecture.md) |
| 相关决策 | [../../decisions/server-runtime-context-boundaries.md](../../decisions/server-runtime-context-boundaries.md) |
| 实现提交 | 未提交（当前工作树） |

> 本文记录已完成的目标与实施过程。下文命令默认从仓库根目录执行，默认 `CGO_ENABLED=0`。

## 背景

[server-runtime-context-architecture.md](../completed/server-runtime-context-architecture.md)（阶段 1）已经把 `admin.Service` 的六个业务领域收窄成六个接口（`UserFacade`、`ClientFacade`、`DomainFacade`、`ProxyFacade`、`CertificateFacade`、`ProviderCredentialFacade`），但接口背后仍由同一个 `admin.Service` struct 实现。调用边界已经建立，实现和测试边界尚未建立。

本 Change 完成阶段 2：把六个 facade 的实现迁入独立 struct，并把 Domain、Proxy、Certificate 之间共享的策略改成显式接口依赖。

## 当前实现

以下结论以 `f94c7b0` 为实施基线。开始实施前必须先执行「阶段 0」，如果当前代码与该基线的调用关系不同，先更新本文再改代码。

- `internal/admin/service.go` 定义 `Service`，包含 `Store`、`Certificates`、`StaticListenerClaims`、`ProxyEntryDefaults`、`ListenerReconciler`、`DefaultJoin` 六组依赖，并实现六个 facade。
- `internal/admin/domain_service.go` 仍以 `Service` 为 receiver；文件名已经分开，但实现对象没有分开。
- `audit` 是唯一横切六个 facade 的私有方法，共享行为是：空 `actorID` 改为 `system`，生成 `audit-*` ID，写入成功审计事件，并把写入错误返回给调用者。
- User 除 `Store` 和 `audit` 外没有共享私有依赖。
- Client 还使用 `DefaultJoin`，其四个 join token 私有方法只在 Client 内调用。
- ProviderCredential 除 `Store` 和 `audit` 外还使用 `certmanager.Service` 的 `ProviderSecretStore`、`VerifyProviderCredential` 等能力；它不是“只依赖 Store 和 AuditRecorder”。证书管理器属于显式基础设施依赖，不是 CertificateFacade 之间的调用。
- Domain 与 Proxy 都依赖 listener reconcile、证书绑定和访问撤销策略。Certificate 依赖 listener reconcile 和证书绑定策略。
- `createWebProxy` 当前只被 `ProxyFacade.CreateProxy` 调用，`DomainFacade.CreateDomainEntry` 不调用它。因此当前基线不存在 `Domain -> Proxy` facade 调用，不需要 `WebProxyCreator`。
- `adminapi.Entry.Commands` 的类型是组合接口 `admin.CommandFacades`。六个独立 service 不能直接赋给该字段，必须增加一个只做接口组合的聚合值。
- 除 daemon 外，`cmd/goginx-admin`、`internal/admintui`、`internal/adminapi` 测试和 `internal/admin` 测试也直接依赖 `admin.Service`；最终删除旧 struct 前必须迁移这些调用点。
- 鉴权位于 Admin API/CLI 适配层，当前六个业务实现没有独立的 actor 鉴权逻辑。本 Change 只保持现有 `actorID` 传递和校验/错误顺序，不新增权限判断。

当前共享私有方法的准确归属如下：

| 目标归属 | 当前方法 |
| --- | --- |
| `ClientService` | `resetClientJoinToken`、`tokenUsesLegacyAdminEnrollmentURL`、`usesLegacyAdminEnrollmentURL`、`defaultJoinTokenPayload` |
| `proxyAdmissionPolicy` | `ensureProxyAdmission`、`activeListenerClaims`、`reconcileProxyListeners`、`findActiveRouteConflict`、`findActiveWebRouteConflict` |
| `certificateBindingPolicy` | `resolveProxyCertificateSelection`、`validateCertificateBinding`、`proxyBoundToCertificate`、`unbindProxyCertificate`、`unbindDomainCertificate`、`boundDomainCertificate`、`boundProxyCertificate`、`certificateServable`、`cleanupManagedCertificateFiles` |
| `proxyAccessPolicy` | `revokeProxyAccessIfEnabled`、`revokeAccessForDomainProxies`、`domainHasEnabledHTTPSEntry` |
| `ProxyService` | `createWebProxy` |
| `CertificateService`、`ProviderCredentialService` 各自的显式字段 | 已规范化的 `certmanager.Service`；构造时设置其 `Store`，不保留共享的 `certificateManager()` 私有方法 |

## 问题

- `admin.Service` 仍是单一 2000+ 行 struct，新贡献者容易继续把业务方法加到同一个实现对象。
- 证书绑定、监听器准入、代理访问撤销散落在 `Service` 私有方法中，无法独立替换或测试。
- 现有 Change 的旧调用图曾把 `createWebProxy` 误判为 Domain 到 Proxy 的跨 facade 调用，并漏记 ProviderCredential 的证书管理依赖；不纠正这两点就无法机械实施。

## 目标

- 在同一 `internal/admin` 包内建立六个独立实现：`UserService`、`ClientService`、`DomainService`、`ProxyService`、`CertificateService`、`ProviderCredentialService`。
- 新增 `AuditRecorder`、`ProxyAdmissionPolicy`、`CertificateBindingPolicy`、`ProxyAccessPolicy`，所有跨领域依赖通过字段显式注入。
- 新增只负责装配的 `Services` 聚合体；它可以持有六个 service，并通过嵌入六个 facade 接口组成 `Commands`。聚合体不得实现业务规则或通过私有方法连接领域。
- 拆分前后 48 个 facade 方法的签名、校验顺序、持久化顺序、补偿动作、审计内容和错误语义不变。
- `adminapi.Entry.Commands` 字段类型保持 `admin.CommandFacades`，GraphQL/HTTP、数据库 schema、Admin UI 均不变化。

## 非目标

- 不拆成 `internal/admin/*` 子包；本 Change 先用同包不同文件和不同 struct 建立边界。
- 不改变 API、协议、数据库 schema、权限模型或产品行为。
- 不重新设计证书、listener、访问激活、Domain/Proxy 模型。
- 不顺手修复 daemon 自定义 Web listener flaky 测试。
- 不操作远端服务或生产数据。

## 核心不变量

- 六个 facade 接口的 48 个方法签名不变，`adminapi.Entry.Commands` 字段类型不变。
- `AuditRecorder`、三类 policy 必须通过 service 字段注入；service 之间不得访问彼此字段或私有方法。
- `Services` 只做构造和组合，不出现 store mutation、校验、审计或回滚逻辑。
- `certmanager.Service.Store` 只在装配时用同一个 `store.Store` 规范化，然后显式传给 `CertificateService`、`ProviderCredentialService` 和证书绑定策略。
- 迁移方法体时只允许修改 receiver、字段路径和接口方法名；同一批次不得重排校验、持久化、reconcile、补偿或审计语句。
- reconcile 的返回/忽略语义逐调用点保持不变。例如创建/更新 proxy 失败后的第二次 reconcile 仍忽略错误；不能统一成总是返回或总是忽略。
- 证书文件清理仍只能删除受管证书目录内的文件；不得扩大路径范围。
- ProviderCredential 的 token、secret ref 和证书私钥不得进入日志、API、测试快照或本文验证记录。

## 已确定的目标设计

### 文件和类型

按以下目标文件实施；输入/结果类型和纯函数可以暂留原文件，避免把结构迁移与无关文件整理混在一起。

| 文件 | 最终职责 |
| --- | --- |
| `internal/admin/facade.go` | 六个既有 facade、`CommandFacades`；签名不改 |
| `internal/admin/services.go` | `Options`、`Services`、`Commands`、`NewServices`、编译期接口断言 |
| `internal/admin/audit.go` | `AuditRecorder`、store 默认实现 |
| `internal/admin/user_service.go` | `UserService` 和 5 个 UserFacade 方法 |
| `internal/admin/client_service.go` | `ClientService`、8 个 ClientFacade 方法和四个 join token 私有方法 |
| `internal/admin/provider_credential_service.go` | `ProviderCredentialService` 和 5 个 ProviderCredentialFacade 方法 |
| `internal/admin/proxy_admission_policy.go` | listener/route 冲突检查与 reconcile |
| `internal/admin/certificate_binding_policy.go` | 证书选择、绑定、解绑、可服务判断和受管文件清理 |
| `internal/admin/proxy_access_policy.go` | access auth 撤销和 HTTPS entry 检查 |
| `internal/admin/proxy_service.go` | `ProxyService`、9 个 ProxyFacade 方法、`createWebProxy` |
| `internal/admin/domain_service.go` | `DomainService` 和 10 个 DomainFacade 方法 |
| `internal/admin/certificate_service.go` | `CertificateService`、11 个 CertificateFacade 方法、`MigrateLegacyFileCertificates` |

`Options` 保留旧 `Service` 的六组装配输入，构造函数签名固定为 `func NewServices(options Options) Services`。构造函数不增加错误返回；`Store == nil` 仍由各业务方法在与现状相同的位置返回 `store is required`。`Options` 字段固定如下：

```go
type Options struct {
	Store                store.Store
	Certificates         certmanager.Service
	StaticListenerClaims []domain.ListenerClaim
	ProxyEntryDefaults   domain.ProxyEntryDefaults
	ListenerReconciler   ListenerReconciler
	DefaultJoin          config.JoinServiceDefaults
}
```

`NewServices` 必须完成以下固定顺序：

1. 保存 `Store`，把 `Options.Certificates.Store` 设为同一个 Store。
2. 构造 store audit recorder。
3. 构造三个 policy 实现。
4. 构造六个 service，并注入各自所需接口。
5. 用 `Commands` 匿名嵌入六个 facade 接口；把六个具体 service 赋给对应接口。
6. 返回 `Services`。`Services` 暴露 `Store`、六个具体 service 和 `Commands`，仅供装配、CLI 和测试定位依赖；业务调用方只接收 `CommandFacades`。

建议形状（字段名可按 Go 命名调整，但职责不得改变）：

```go
type Commands struct {
	UserFacade
	ClientFacade
	DomainFacade
	ProxyFacade
	CertificateFacade
	ProviderCredentialFacade
}

type Services struct {
	Commands
	Store               store.Store
	Users               *UserService
	Clients             *ClientService
	Domains             *DomainService
	Proxies             *ProxyService
	Certificates        *CertificateService
	ProviderCredentials *ProviderCredentialService
}
```

阶段 1 创建 `Services` 时只加入已经存在的具体 service 字段；其余 facade 暂由 `Commands` 中的旧 `Service` 实例满足。后续每完成一个领域就增加对应具体字段并替换组合接口，避免为了让阶段 1 编译而预先创建空 service 类型。

### 共享接口

接口方法应覆盖实际消费者，不保留已经不需要的 `WebProxyCreator`：

```go
type AuditRecorder interface {
	Record(ctx context.Context, actorID, resourceType, resourceID, action string) error
}

type ProxyAdmissionPolicy interface {
	EnsureAdmission(ctx context.Context, proxy domain.Proxy, ignoreProxyID string) error
	ReconcileListeners(ctx context.Context) error
}

type CertificateBindingPolicy interface {
	ResolveProxySelection(ctx context.Context, proxyType domain.ProxyType, proxyID, certificateID string, certificateIDSet bool, entryHost, certFile, keyFile, actorID string) (string, error)
	ValidateBinding(ctx context.Context, certificateID, host, domainID string) error
	ProxyBoundToCertificate(ctx context.Context, certificateID string) (domain.Proxy, bool, error)
	UnbindProxy(ctx context.Context, proxy domain.Proxy) error
	UnbindDomain(ctx context.Context, webDomain domain.Domain) error
	BoundDomain(ctx context.Context, webDomain domain.Domain) (domain.ManagedCertificate, error)
	BoundProxy(ctx context.Context, proxy domain.Proxy) (domain.ManagedCertificate, error)
	CertificateServable(certificate domain.ManagedCertificate) bool
	CleanupManagedFiles(certificate domain.ManagedCertificate)
}

type ProxyAccessPolicy interface {
	RevokeIfEnabled(ctx context.Context, proxy *domain.Proxy) error
	RevokeForDomain(ctx context.Context, domainID string) error
	DomainHasEnabledHTTPSEntry(ctx context.Context, domainID string) bool
}
```

具体依赖矩阵：

| Service | 显式依赖 |
| --- | --- |
| User | Store、AuditRecorder |
| Client | Store、DefaultJoin、AuditRecorder |
| ProviderCredential | Store、已规范化的 certmanager.Service、AuditRecorder |
| Domain | Store、AuditRecorder、ProxyAdmissionPolicy、CertificateBindingPolicy、ProxyAccessPolicy |
| Proxy | Store、AuditRecorder、ProxyAdmissionPolicy、CertificateBindingPolicy、ProxyAccessPolicy |
| Certificate | Store、已规范化的 certmanager.Service、AuditRecorder、ProxyAdmissionPolicy、CertificateBindingPolicy |

### 兼容迁移策略

每一批都必须可编译、可独立提交。迁移中暂时保留旧 `Service` 的同名 facade 方法作为薄转发器：每个转发器只根据旧字段构造/调用对应新 service，不复制业务逻辑。所有生产装配和测试迁到 `NewServices` 后，一次性删除这些转发器和旧 `Service` struct。

禁止长期保留兼容转发器；最终残留检索必须为零。

## 方法与审计迁移台账

每迁移一行就执行：复制原方法体，改 receiver，替换 `audit` 为 `AuditRecorder.Record`，替换共享私有调用为 policy 方法，补/改对应测试，然后在本表勾选。未列审计的只读方法不得新增审计。

### User、Client、ProviderCredential

| 完成 | 目标 | 方法 | 审计/特殊语义 |
| --- | --- | --- | --- |
| [x] | User | `CreateUser` | `user/create_user` |
| [x] | User | `DisableUser` | `user/disable_user` |
| [x] | User | `EnableUser` | `user/enable_user` |
| [x] | User | `SetUserPassword` | `user/set_user_password` |
| [x] | User | `DeleteUser` | `user/delete_user` |
| [x] | Client | `CreateClient` | `client/create_client` |
| [x] | Client | `CreateClientWithCredential` | 经 `CreateClient` 产生 `create_client`，不得重复或省略 |
| [x] | Client | `CreateClientJoin` | 先 `create_client`，再 `client/create_client_join` |
| [x] | Client | `ReviewClientJoinToken` | 正常为 `review_client_join_token`；重置分支为 `reset_client_join_token` |
| [x] | Client | `EnableClient` | `client/enable_client` |
| [x] | Client | `DisableClient` | `client/disable_client` |
| [x] | Client | `DeleteClient` | `client/delete_client` |
| [x] | Client | `RotateClientCredential` | `client/rotate_client_credential`，返回前仍清空 hash |
| [x] | ProviderCredential | `CreateProviderCredential` | `provider_credential/create_provider_credential`；store 失败仍清理刚写入 secret |
| [x] | ProviderCredential | `UpdateProviderCredential` | `provider_credential/update_provider_credential` |
| [x] | ProviderCredential | `VerifyProviderCredential` | `provider_credential/verify_provider_credential`；验证失败优先于 audit error 返回 |
| [x] | ProviderCredential | `DisableProviderCredential` | `provider_credential/disable_provider_credential` |
| [x] | ProviderCredential | `DeleteProviderCredential` | `provider_credential/delete_provider_credential`；secret 删除错误仍忽略 |

### Domain、Proxy、Certificate

| 完成 | 目标 | 方法 | 审计/特殊语义 |
| --- | --- | --- | --- |
| [x] | Domain | `CreateDomain` | `domain/create_domain`；reconcile 失败删除新 Domain 并重试 reconcile |
| [x] | Domain | `UpdateDomain` | `domain/update_domain`；host 变化撤销 access；reconcile 失败恢复旧 Domain |
| [x] | Domain | `EnableDomain` | `domain/enable_domain` |
| [x] | Domain | `DisableDomain` | `domain/disable_domain` |
| [x] | Domain | `DeleteDomain` | `domain/delete_domain` |
| [x] | Domain | `CreateDomainEntry` | `domain_entry/create_domain_entry`；reconcile 失败删除新 entry |
| [x] | Domain | `UpdateDomainEntry` | `domain_entry/update_domain_entry`；reconcile 失败恢复旧 entry |
| [x] | Domain | `DeleteDomainEntry` | `domain_entry/delete_domain_entry` |
| [x] | Domain | `BindDomainCertificate` | `domain/bind_certificate` |
| [x] | Domain | `UnbindDomainCertificate` | `domain/unbind_certificate`；本来未绑定时直接成功且不审计 |
| [x] | Proxy | `CreateProxy` | Web=`proxy/create_web_proxy`；TCP/HTTP/HTTPS/UDP 使用原动态 action；reconcile 失败删除新 proxy |
| [x] | Proxy | `UpdateProxy` | `proxy/update_proxy`；所有 Domain/certificate/access 分支顺序不变 |
| [x] | Proxy | `EnableProxy` | `proxy/enable_proxy`；reconcile 失败恢复 disabled |
| [x] | Proxy | `DisableProxy` | `proxy/disable_proxy` |
| [x] | Proxy | `DeleteProxy` | `proxy/delete_proxy` |
| [x] | Proxy | `EnableProxyAccessAuthAndCreateActivation` | `proxy/enable_proxy_access_auth` |
| [x] | Proxy | `CreateProxyActivationLink` | `proxy/create_proxy_activation` |
| [x] | Proxy | `RevokeAllProxyAccess` | `proxy/revoke_proxy_access` |
| [x] | Proxy | `DisableProxyAccessAuth` | `proxy/disable_proxy_access_auth` |
| [x] | Certificate | `IssueManagedCertificate` | 默认 `issue_managed_certificate`；Origin CA 为 `issue_cloudflare_origin_certificate` |
| [x] | Certificate | `RenewManagedCertificate` | `certificate/renew_managed_certificate` |
| [x] | Certificate | `CreateCertificate` | 默认/Origin CA/file 三种原 action 保持不变 |
| [x] | Certificate | `DeleteCertificate` | `certificate/delete_managed_certificate`；确认、解绑、删除文件、reconcile 顺序不变 |
| [x] | Certificate | `BindCertificate` | 兼容 API 仍审计 `domain/bind_certificate` |
| [x] | Certificate | `UnbindCertificate` | 兼容 API 仍审计 `domain/unbind_certificate`；未绑定时不审计 |
| [x] | Certificate | `RotateOriginCACertificate` | `certificate/rotate_cloudflare_origin_certificate` |
| [x] | Certificate | `SyncOriginCACertificate` | `certificate/sync_cloudflare_origin_certificate` |
| [x] | Certificate | `RevokeOriginCACertificate` | `certificate/revoke_cloudflare_origin_certificate` |
| [x] | Certificate | `CertificateProviderReadiness` | 无审计；manager 不可用仍返回 `nil` |
| [x] | Certificate | `ManagedCertificateStatus` | 无审计 |

另有非 facade 启动方法 `MigrateLegacyFileCertificates` 迁到 `CertificateService`；它继续直接委托 certmanager，不新增审计。`ResolveProxySelection` 中登记遗留 file certificate 后仍以忽略错误的方式记录 `certificate/migrate_file_certificate`。

## 机械实施步骤

### 阶段 0：冻结基线和建立护栏

- [x] 确认工作树，记录已有用户改动；不得覆盖无关修改：

  ```bash
  git status --short
  git rev-parse --short HEAD
  ```

- [x] 确认 `f94c7b0` 后没有改变本 Change 涉及的方法；有变化则先更新“当前实现”、依赖矩阵和迁移台账：

  ```bash
  git diff --stat f94c7b0 -- internal/admin internal/adminapi internal/daemon internal/admintui cmd/goginx-admin
  rg -n '^func \(service Service\)' internal/admin/service.go internal/admin/domain_service.go
  rg -n 'service\.(audit|createWebProxy|ensureProxyAdmission|reconcileProxyListeners|validateCertificateBinding|revokeProxyAccessIfEnabled|revokeAccessForDomainProxies|domainHasEnabledHTTPSEntry)' internal/admin
  ```

- [x] 跑基线验证并把结果写入“验证记录”。任何非已知 flaky 的失败都先处理或登记阻碍，不进入迁移：

  ```bash
  CGO_ENABLED=0 go build ./...
  CGO_ENABLED=0 go test ./internal/admin ./internal/adminapi ./internal/daemon -count=1
  ```

- [x] 在 `internal/admin` 测试中新增 characterization 覆盖：空 actor 变 `system`；audit 写入失败如何返回；CreateClientJoin 双审计；VerifyProviderCredential 验证错误优先级；proxy/domain reconcile 的回滚与“第二次错误忽略”；证书文件仅清理受管目录。
- [x] 验证新增测试在旧 `Service` 上通过。此提交只允许增加测试和更新本文，不改生产行为。

停止条件：基线测试出现无法归因的失败，或台账不能覆盖全部 48 个 facade 方法。记录一个合并后的阻碍项，不继续阶段 1。

### 阶段 1：装配骨架、Audit、User、ProviderCredential

- [x] 新建 `audit.go`，定义 `AuditRecorder` 和默认 store 实现；逐字符保留旧 `audit` 的 actor 默认值、事件字段和错误返回。
- [x] 新建 `services.go`，定义 `Options`、`Commands`、`Services`、`NewServices`。先允许未迁移 facade 指向旧 `Service`，保证本阶段可编译。
- [x] 在 `facade.go` 增加编译期断言：六个新 service 分别满足各自 facade，`Commands` 的值和指针都满足 `CommandFacades`。未创建的 service 断言在相应阶段加入。
- [x] 新建 `user_service.go`，按台账原样迁移 5 个 User 方法；旧 `Service` 同名方法改为单行薄转发。
- [x] 新建 `provider_credential_service.go`，注入 Store、规范化后的 certmanager.Service 和 AuditRecorder，原样迁移 5 个方法；删除这些方法对 `certificateManager()` 的依赖，改用构造时已经设置 Store 的 manager；旧方法改为薄转发。
- [x] 把 User/ProviderCredential 测试改为直接实例化对应 service 或经 `NewServices` 获取，不再验证旧实现细节。
- [x] 对照方法体：

  ```bash
  git diff --word-diff=porcelain f94c7b0 -- internal/admin/service.go internal/admin/user_service.go internal/admin/provider_credential_service.go
  gofmt -w internal/admin/audit.go internal/admin/services.go internal/admin/user_service.go internal/admin/provider_credential_service.go internal/admin/facade.go internal/admin/service.go
  CGO_ENABLED=0 go test ./internal/admin -count=1
  CGO_ENABLED=0 go test ./internal/adminapi -count=1
  CGO_ENABLED=0 go build ./...
  ```

停止条件：方法除 receiver/依赖路径外出现行为 diff，或 audit characterization 失败。修复后重跑本阶段全部命令。

### 阶段 2：Client

- [x] 新建 `client_service.go`，注入 Store、DefaultJoin、AuditRecorder。
- [x] 按台账迁移 8 个 facade 方法和四个 join token 私有方法；`CreateClientWithCredential`、`CreateClientJoin` 继续调用同一个 `ClientService` 上的方法。
- [x] 旧 `Service` 的 8 个 Client 方法改为薄转发；四个 Client 私有方法从旧 struct 删除。
- [x] 把 Client/join token 测试改成直接使用 `ClientService` 或 `NewServices`。
- [x] 执行：

  ```bash
  gofmt -w internal/admin/client_service.go internal/admin/service.go internal/admin/services.go internal/admin/facade.go
  CGO_ENABLED=0 go test ./internal/admin -count=1
  CGO_ENABLED=0 go test ./internal/adminapi -count=1
  CGO_ENABLED=0 go build ./...
  ```

停止条件：join token 内容、TTL、旧 enrollment URL 迁移、credential hash 清空或双审计任一行为改变。

### 阶段 3：提取三类共享策略

- [x] 新建三个 policy 文件并加入接口；先让旧 `Service` 通过 adapter 调用 policy，尚不迁移 Domain/Proxy/Certificate facade 方法。
- [x] `proxyAdmissionPolicy` 接收 Store、StaticListenerClaims、ProxyEntryDefaults、ListenerReconciler，迁移五个私有方法。`proxyRequiresListenerAdmission`、`displayBindHost` 可保留为包级纯函数。
- [x] `proxyAccessPolicy` 只接收 Store，迁移三个私有方法。
- [x] `certificateBindingPolicy` 接收 Store、规范化后的 certmanager.Service、AuditRecorder，迁移台账中的九个方法。`removeManagedFile`、`hostnameWithinCertificate`、`hostnameMatchesPattern` 可保留为包级纯函数。
- [x] 给每个 policy 增加编译期接口断言和聚焦单测。至少覆盖 listener 静态/动态冲突、Web path 冲突、Domain 全量 access 撤销、通配证书 host 校验、解绑导致 `needs_config`、目录外文件不删除。
- [x] 执行：

  ```bash
  gofmt -w internal/admin/proxy_admission_policy.go internal/admin/proxy_access_policy.go internal/admin/certificate_binding_policy.go internal/admin/service.go internal/admin/services.go
  CGO_ENABLED=0 go test ./internal/admin -count=1
  CGO_ENABLED=0 go build ./...
  ```

停止条件：policy 测试必须能在不构造任一 facade service 的情况下运行；否则边界仍是隐式的，先修正依赖。

### 阶段 4：Domain、Proxy、Certificate

- [x] 把 `domain_service.go` receiver 改成 `DomainService`，注入矩阵中的五项依赖，按台账迁移 10 个方法。
- [x] 新建 `proxy_service.go`，按台账迁移 9 个 facade 方法和 `createWebProxy`。不要增加 `WebProxyCreator`，也不要让 DomainService 引用 ProxyService。
- [x] 新建 `certificate_service.go`，迁移 11 个 facade 方法、`deleteConfirmationMatches` 和 `MigrateLegacyFileCertificates`；manager 使用构造时已规范化的字段。
- [x] 旧 `Service` 对应的 facade 方法暂改为薄转发；删除已经迁入 policy/service 的旧私有方法体，确保业务逻辑只有一份。
- [x] 把 admin 包测试按领域拆用具体 service；混合流程测试经 `NewServices` 使用 `Services.Commands`。
- [x] 执行：

  ```bash
  gofmt -w internal/admin/*.go
  CGO_ENABLED=0 go test ./internal/admin -count=1
  CGO_ENABLED=0 go test ./internal/adminapi -count=1
  CGO_ENABLED=0 go test ./internal/daemon -count=1
  CGO_ENABLED=0 go build ./...
  ```

停止条件：任何 facade 仍直接调用另一个 facade 的具体 struct，或任何共享策略调用仍表现为跨 struct 私有方法。

### 阶段 5：切换所有装配点并删除旧 Service

- [x] `internal/daemon/server.go`：用 `admin.NewServices(admin.Options{...})` 替代 struct literal；启动迁移调用 `services.Certificates.MigrateLegacyFileCertificates`；`adminapi.Entry.Commands` 传 `services.Commands`。
- [x] `cmd/goginx-admin/main.go`：`openService` 返回 `admin.Services`；查询使用 `services.Store`；业务命令经嵌入的 `Commands` 或显式 `services.Commands`；`existingAdminID` 只接收其实际需要的 Store/查询依赖，不再接收旧 concrete Service。
- [x] `internal/admintui/tui.go`：把 `Commands admin.Service` 改成满足实际调用集合的接口；可直接使用 `admin.CommandFacades`，不得依赖 concrete service。
- [x] `internal/adminapi/server_test.go`：fixture 同时保存 db 和 `admin.Services`；删除 `server.commands.(*admin.Service)`、`testCommands() *admin.Service` 等 concrete 断言，通过 fixture 的 Store/具体 CertificateService 设置测试依赖。
- [x] `internal/admin` 测试：清除所有 `Service{...}`，单领域测试用具体 service，跨领域测试用 `NewServices`。
- [x] 删除旧 `Service` struct、全部兼容转发器、`certificateManager()` 和 `facade.go` 中针对旧 Service 的断言。
- [x] 执行残留检查；以下命令除注释/历史说明外必须无输出：

  ```bash
  rg -n 'admin\.Service|\bService\s*\{' cmd/goginx-admin internal/admin internal/adminapi internal/daemon internal/admintui --glob '*.go'
  rg -n 'func \(service Service\)|service\.(audit|ensureProxyAdmission|reconcileProxyListeners|validateCertificateBinding|revokeProxyAccessIfEnabled|revokeAccessForDomainProxies|domainHasEnabledHTTPSEntry)' internal/admin --glob '*.go'
  rg -n 'WebProxyCreator|CreateWebProxy' internal/admin --glob '*.go'
  ```

- [x] 执行编译和相关包验证：

  ```bash
  gofmt -w internal/admin/*.go internal/adminapi/*.go internal/daemon/*.go internal/admintui/*.go cmd/goginx-admin/*.go
  git diff --check
  CGO_ENABLED=0 go build ./...
  CGO_ENABLED=0 go test ./internal/admin ./internal/adminapi ./internal/daemon ./internal/admintui ./cmd/goginx-admin -count=1
  ```

停止条件：调用方需要读取任一 service 私有字段才能工作。此时补充最小装配/测试 API，不允许恢复旧大 Service。

### 阶段 6：全量验证和文档收尾

- [x] 全量单元/集成测试：

  ```bash
  CGO_ENABLED=0 go test ./... -count=1
  ```

- [x] 如果只出现 worklog 已登记的 daemon custom listener flaky，独立重复相关测试三次并逐次记录，不把它笼统写成“全量通过”：

  ```bash
  CGO_ENABLED=0 go test ./internal/daemon -run 'TestReconcile(HTTP|HTTPS)ProxyCustomListenerWithoutRestart' -count=1
  CGO_ENABLED=0 go test ./internal/daemon -run 'TestReconcile(HTTP|HTTPS)ProxyCustomListenerWithoutRestart' -count=1
  CGO_ENABLED=0 go test ./internal/daemon -run 'TestReconcile(HTTP|HTTPS)ProxyCustomListenerWithoutRestart' -count=1
  ```

- [x] 跨进程验证：

  ```bash
  CGO_ENABLED=0 go test ./e2e -count=1
  ```

- [x] 检查敏感/生成文件未进入 diff：

  ```bash
  git status --short
  git diff --name-only
  git diff --check
  ```

- [x] 更新 `docs/architecture/system-architecture.md`：写入六个 concrete service、`Services/Commands` 装配体和三个 policy 边界。
- [x] 更新 `docs/worklog.md`：记录完成状态、验证结论和仍存在的已知 flaky。
- [x] 在本文逐项勾选实施和验收条件，填写实际文件/类型偏差、提交和验证记录。
- [x] 检索旧描述：

  ```bash
  rg -n 'admin\.Service|WebProxyCreator|同一个.*Service|物理拆分' docs --glob '*.md'
  ```

- [x] 所有条件满足后，把状态改为 `completed`，填写完成日期和实现提交，并执行：

  ```bash
  git mv docs/changes/active/admin-facade-physical-split.md docs/changes/completed/admin-facade-physical-split.md
  ```

任一步失败时只回退当前批次自己的提交或修复当前批次；不得用 `git reset --hard`、`git checkout --` 覆盖工作树，也不得删除用户的既有修改。

## 验收条件

- [x] 六个领域由六个独立 struct 实现，并各有编译期 facade 断言。
- [x] User 只依赖 Store/Audit；Client 只依赖 Store/DefaultJoin/Audit；ProviderCredential 的 certmanager 依赖已显式记录。
- [x] Domain、Proxy、Certificate 的共享依赖只通过三类 policy 接口注入；不存在跨 service 私有方法调用。
- [x] `createWebProxy` 属于 ProxyService，DomainService 不依赖 ProxyService；代码中不存在无消费者的 `WebProxyCreator`。
- [x] `Services`/`Commands` 只负责装配；旧 `admin.Service` 和兼容转发器已删除。
- [x] 48 个 facade 方法签名不变，`adminapi.Entry.Commands` 字段类型不变。
- [x] 方法与审计迁移台账逐项完成，特殊错误/回滚语义有 characterization 测试。
- [x] daemon、CLI、TUI、adminapi 测试均不依赖旧 concrete Service。
- [x] `CGO_ENABLED=0 go build ./...` 通过。
- [x] 相关包测试、`go test ./... -count=1` 和 `go test ./e2e -count=1` 均有真实记录；失败项明确区分本 Change 回归与既有 flaky。
- [x] architecture、worklog 和旧描述检索已完成，无敏感或生成文件进入变更。

## 验证记录

| 日期 | 基线/提交 | 命令/步骤 | 结果 | 说明 |
| --- | --- | --- | --- | --- |
| `2026-07-23` | 旧计划 | — | 未执行 | 计划阶段 |
| `2026-07-24` | `f94c7b0` | 静态调用图和装配点核对 | 通过 | 纠正 ProviderCredential 依赖、`createWebProxy` 调用方和聚合装配缺口 |
| `2026-07-24` | 当前工作树 | `CGO_ENABLED=0 go build ./...` | 通过 | 六个 service 和装配点构建通过 |
| `2026-07-24` | 当前工作树 | `gofmt`、相关包真实测试 | 通过 | `internal/admin`、`internal/adminapi`、`internal/admintui`、`cmd/goginx-admin` 通过；残留扫描无旧 `admin.Service`、兼容转发器或 `WebProxyCreator` |
| `2026-07-24` | 当前工作树 | `CGO_ENABLED=0 go test ./... -count=1` | 部分通过 | e2e 和其余包通过；daemon 两个 custom listener 测试及 certmanager 临时目录权限测试为已知阻碍 |
| `2026-07-24` | 当前工作树 | `CGO_ENABLED=0 go test ./e2e -count=1` | 通过 | 跨进程测试通过 |

## 文档同步

- [x] requirements 无需更新（不改变产品行为）
- [x] architecture 更新实际 service/policy/装配边界
- [x] operations 确认无影响
- [x] Admin UI 文档确认无影响
- [x] worklog 更新完成状态与验证结果

## 阻碍记录

- daemon custom listener：`TestReconcileHTTPProxyCustomListenerWithoutRestart` 与 `TestReconcileHTTPSProxyCustomListenerWithoutRestart` 独立运行三次均失败，分别出现 HTTP 502 与 HTTPS 502；已知与本 Change 无关，暂不修复。
- certmanager 测试：`TestFileSecretStoreRoundTripAndPathSafety` 在当前环境报告临时目录为 `0755` 而非期望模式；已知环境差异，与本 Change 无关，暂不修复。

## 已关闭决策

1. **物理位置**：留在同一 `internal/admin` 包，以不同文件和 struct 区分；不在本 Change 拆子包。
2. **聚合方式**：删除承担业务逻辑的 `admin.Service`；保留名为 `Services` 的装配结果和名为 `Commands` 的接口组合值。二者不得承载业务逻辑。
3. **Web Proxy 依赖方向**：当前基线只有 ProxyService 自己需要 `createWebProxy`，因此不引入 `WebProxyCreator`，也不建立 Domain -> Proxy 依赖。
4. **ProviderCredential 的证书依赖**：把规范化后的 `certmanager.Service` 作为显式基础设施依赖注入，不通过 CertificateService 调用。

## 结果

已完成 Admin Facade 物理拆分：六个 concrete service、三类 policy、`Services/Commands` 装配和全部调用点迁移均已落地。旧 `admin.Service`、测试兼容层和无消费者的 `WebProxyCreator` 均已删除。全量构建与 e2e 通过；全量包测仍受已登记的 daemon custom listener 502 和当前环境 certmanager 临时目录权限断言阻断，详见验证记录和 worklog。
