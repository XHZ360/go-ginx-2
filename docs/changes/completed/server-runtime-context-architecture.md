# Server Runtime Context 架构重整

## 元信息

| 项 | 值 |
| --- | --- |
| 状态 | `completed` |
| 最后更新 | `2026-07-23` |
| 完成日期 | `2026-07-23` |
| 负责人 | 未指定 |
| 相关需求 | 无（本 Change 不改变产品行为，仅调整实现边界） |
| 相关架构 | [../../architecture/system-architecture.md](../../architecture/system-architecture.md)、[../../architecture/control-channel.md](../../architecture/control-channel.md)、[../../architecture/admin-and-observability.md](../../architecture/admin-and-observability.md) |
| 相关决策 | [../../decisions/server-runtime-context-boundaries.md](../../decisions/server-runtime-context-boundaries.md) |
| 实现提交 | 未提交 |

> 本 Change 已完成。当前产品行为以 `requirements/`、`architecture/`、`operations/` 与代码为准。

## 背景

`internal/admin/service.go`、`internal/adminapi/server.go`、`internal/control/transport.go` 三个文件持续承接新业务，边界逐渐模糊：业务规则、协议适配、运行时状态写入混在同一个包甚至同一个函数里。这直接阻塞了 [server-local-virtual-client.md](server-local-virtual-client.md) 的实施——该 feature 需要一个稳定的 `SystemClientFacade`/`LocalProxyFacade`/`LocalTargetPolicy` 边界才能接入，而不是继续向 `daemon/server.go`、`admin/service.go`、`control/transport.go` 堆叠特例。

本 Change 先把边界和依赖方向定下来，再逐步迁移现有代码，不新增本机代理能力。

## 当前实现

现状证据（用于确定迁移范围，避免凭空描述）：

- `internal/admin/service.go` 共 2099 行，单一 `Service` 结构体承接用户、客户端、代理、证书、Provider 凭据、join token、访问激活、审计等全部业务命令（如 `CreateUser`、`CreateProxy`、`IssueManagedCertificate`、`CreateProviderCredential` 等均为该结构体方法）。它已经使用一个端口做依赖注入：`ProxyListenerReconciler` 接口（`internal/admin/service.go:33-35`），由 `daemon.ServerRuntime` 实现并在 `internal/daemon/server.go:135` 注入——这是目前唯一可复用的端口先例。
- `internal/adminapi/server.go` 共 2467 行，`buildSchema`（706–1829 行，约 1100 行）单函数构建全部 GraphQL resolver；同一文件还承担管理员登录/会话 cookie、CSRF 校验和 Admin 前端静态文件服务（`loginHandler`、`sessionHandler`、`serveFrontendFile` 等）。
- `internal/control/transport.go:379-399` 的 `setClientRuntimeStatus` 在控制通道认证成功/连接关闭时，直接调用 `store.Store.Clients().SetStatus`，跳过 `admin.Service`/facade，是当前对「所有业务 mutation 经过 facade」不变量的一个已知例外（心跳与上下线驱动，高频、无审计要求）。
- `internal/adminquery/service.go:20` 的只读查询 `Service` 直接持有 `*session.Manager` 字段，`latestSessions()`（659 行）等方法直接读取运行时 session 快照，用于把在线状态拼进客户端/代理列表。这是 API 层读取 session 的现状，尚未被本文档的不变量覆盖（不变量目前只写「禁止 API 层直接操作 session」，未区分读/写）。
- `internal/store/store.go` 已经是纯 repository 接口集合（`UserRepository`、`ClientRepository`、`ProxyRepository` 等），不包含权限判断，符合目标模型对持久化层的要求，无需大改。
- `internal/daemon/server.go` 共 261 行，目前只做装配（打开 store、启动 session manager、启动 control/admin/enrollment listener、组装 `admin.Service`），没有承接业务逻辑，符合目标模型对 `daemon` 的定位。

## 问题

- `admin.Service` 是事实上的上帝对象：新业务默认加进这一个结构体，缺乏子领域边界，单测和变更影响面随体量线性变差。
- `adminapi/server.go` 把协议适配（GraphQL/HTTP/cookie）、认证会话管理和静态文件服务混在一个包里，新增字段/mutation 时容易直接绕过 `admin.Service` 读写 `store`。
- `control/transport.go` 中存在业务层可判断的状态写入（`setClientRuntimeStatus`）直连 store，缺少「哪些运行时状态写入允许跳过 facade」的书面规则，导致后续贡献者难以判断新增的类似写入是否合规。
- `adminquery.Service` 直接依赖 `*session.Manager`，若之后 session 内部结构调整，查询层会被动跟着改，且当前无接口隔离。
- 缺少可执行的迁移清单：仅有目标状态描述，没有从当前代码到目标状态的具体步骤，导致「实施顺序」一节缺乏抓手。
- 未按项目 [change-template.md](../change-template.md) 的结构撰写，缺少元信息、当前实现、验收条件、验证记录、文档同步等必需章节，不满足 [change-workflow.md](../../project/change-workflow.md) 的 Ready for Implementation 门槛。

## 目标

- 固定四个上下文（管理、运行时、通信、持久化）之间的依赖方向，且用现有代码可验证：`go list` 依赖图中不出现禁止的反向依赖。
- 把 `admin.Service` 按子领域拆分为可独立测试的业务 facade（用户/客户端/代理/证书/凭据等），不再新增字段到单一大结构体。
- 明确 `control` 包中运行时状态写入（心跳、上下线）与业务 mutation 的边界，写清允许直连 store 的例外范围。
- 明确 `adminquery` 对 session 只读访问的合法边界，通过接口而非具体类型注入。
- 为 [server-local-virtual-client.md](server-local-virtual-client.md) 提供 `SystemClientFacade`、`LocalProxyFacade`、`LocalTargetPolicy`、`VirtualSession`、`LocalDialer` 等新端口的接口定义和放置位置，但不实现其业务逻辑。

## 非目标

- 不新增本机代理、系统 client 或白名单能力（由 [server-local-virtual-client.md](server-local-virtual-client.md) 承接）。
- 不改变现有 API/GraphQL 契约、数据库 schema 或对外行为。
- 不要求一次性完成全部迁移；允许分阶段合并，只要每个阶段保持行为不变且通过现有测试。
- 不解决 `adminapi/server.go` 中前端静态文件服务的归属问题（记录为待决策，不在本 Change 内处理）。

## 核心不变量

- 管理上下文不持有长生命周期网络连接。
- 运行时上下文不持有管理员 JWT 或用户密码。
- 所有外部连接都经由明确的 opener/dialer 端口。
- 除下列已登记例外，所有业务 mutation 经过 facade；store 不提供安全策略绕过入口：
  - `control` 包中由认证结果或连接生命周期直接驱动的 `ClientStatus` 写入（当前即 `setClientRuntimeStatus`）视为运行时状态同步，允许直连 `store.Clients().SetStatus`，不要求审计事件；新增此类例外前必须先在本文档登记范围和理由。
- 只读查询可以通过明确的只读接口访问运行时状态（例如 session 快照），但不能借助只读入口执行 mutation；`adminquery` 对 `session.Manager` 的依赖应收敛为一个只读接口（如 `SessionSnapshotSource`），不直接依赖具体类型的可变方法。
- 远端 client 和未来 system client 可以共享 session/stream 接口，但不共享错误的远端连接假设。

## 目标设计

### 上下文边界

```text
管理上下文：Admin API/UI/CLI → 业务 Facade
运行时上下文：Proxy Listener → Session Registry → StreamOpener
通信上下文：Remote Client ↔ Control Listener ↔ Session
持久化上下文：Domain Service → Repository → SQLite
```

- 管理上下文负责身份、权限、配置、审计和业务命令。
- 运行时上下文负责 listener、session、stream 生命周期和连接转发。
- 通信上下文负责 control wire protocol 和远端 client 握手；允许按上文登记的例外直连 store 同步运行时可见状态。
- 持久化上下文只负责数据存取、事务和迁移，不承载管理员权限。

上下文之间通过接口、端口和不可变数据结构通信；禁止 API 层直接执行 session mutation，禁止 runtime 层解析 JWT，禁止 control 层承担业务配置权限。

### 外观与端口

分两组，避免把「现有对象拆分」和「新 feature 预留端口」混为一谈。

**A. 现有 `admin.Service` 拆分目标**（本 Change 直接负责迁移）：

拆分前先核对完整方法清单（`internal/admin/service.go` + `internal/admin/domain_service.go`，共 46 个导出方法），按子领域分成六组，覆盖全部现有调用点，不遗留未分组的方法。**接口方法名与 `admin.Service` 现有导出方法名逐一保持一致**（不做 `CreateUser` → `Create` 之类的重命名）：这样 `admin.Service` 天然满足全部六个接口（Go 结构化类型，签名匹配即自动实现），阶段 1 不需要改一行 `admin.Service` 实现代码，也不需要触碰 `adminapi/server.go` 中任何调用点，只需新增接口定义并把 `adminapi.Entry.Commands` 字段类型从 `admin.Service` 换成接口组合：

```go
type UserFacade interface {
    CreateUser(ctx context.Context, input CreateUserInput) (domain.User, error)
    DisableUser(ctx context.Context, userID string, actorID string) error
    EnableUser(ctx context.Context, userID string, actorID string) error
    SetUserPassword(ctx context.Context, userID string, password string, actorID string) error
    DeleteUser(ctx context.Context, userID string, actorID string) error
}

type ClientFacade interface {
    CreateClient(ctx context.Context, input CreateClientInput) (domain.Client, error)
    CreateClientWithCredential(ctx context.Context, input CreateClientInput) (CreateClientResult, error)
    CreateClientJoin(ctx context.Context, input CreateClientJoinInput) (CreateClientJoinResult, error)
    ReviewClientJoinToken(ctx context.Context, clientID string, actorID string) (ReviewClientJoinTokenResult, error)
    EnableClient(ctx context.Context, clientID string, actorID string) error
    DisableClient(ctx context.Context, clientID string, actorID string) error
    DeleteClient(ctx context.Context, clientID string, actorID string) error
    RotateClientCredential(ctx context.Context, input RotateClientCredentialInput) (RotateClientCredentialResult, error)
}

type DomainFacade interface {
    CreateDomain(ctx context.Context, input CreateDomainInput) (domain.Domain, error)
    UpdateDomain(ctx context.Context, input UpdateDomainInput) (domain.Domain, error)
    EnableDomain(ctx context.Context, domainID string, actorID string) error
    DisableDomain(ctx context.Context, domainID string, actorID string) error
    DeleteDomain(ctx context.Context, domainID string, actorID string) error
    CreateDomainEntry(ctx context.Context, input CreateDomainEntryInput) (domain.DomainEntry, error)
    UpdateDomainEntry(ctx context.Context, input UpdateDomainEntryInput) (domain.DomainEntry, error)
    DeleteDomainEntry(ctx context.Context, entryID string, actorID string) error
    BindDomainCertificate(ctx context.Context, domainID string, certificateID string, actorID string) (domain.Domain, error)
    UnbindDomainCertificate(ctx context.Context, domainID string, actorID string) (domain.Domain, error)
}

type ProxyFacade interface {
    CreateProxy(ctx context.Context, input CreateProxyInput) (domain.Proxy, error)
    UpdateProxy(ctx context.Context, input UpdateProxyInput) (domain.Proxy, error)
    EnableProxy(ctx context.Context, proxyID string, actorID string) error
    DisableProxy(ctx context.Context, proxyID string, actorID string) error
    DeleteProxy(ctx context.Context, proxyID string, actorID string) error
    EnableProxyAccessAuthAndCreateActivation(ctx context.Context, proxyID string, actorID string) (ProxyActivationResult, error)
    CreateProxyActivationLink(ctx context.Context, proxyID string, actorID string) (ProxyActivationResult, error)
    RevokeAllProxyAccess(ctx context.Context, proxyID string, actorID string) error
    DisableProxyAccessAuth(ctx context.Context, proxyID string, actorID string) error
}

type CertificateFacade interface {
    IssueManagedCertificate(ctx context.Context, input CertificateInput) (domain.ManagedCertificate, error)
    RenewManagedCertificate(ctx context.Context, input CertificateInput) (domain.ManagedCertificate, error)
    CreateCertificate(ctx context.Context, input CreateCertificateInput) (domain.ManagedCertificate, error)
    DeleteCertificate(ctx context.Context, input DeleteCertificateInput) (DeleteCertificateResult, error)
    BindCertificate(ctx context.Context, input BindCertificateInput) (domain.Proxy, error)
    UnbindCertificate(ctx context.Context, input UnbindCertificateInput) (domain.Proxy, error)
    RotateOriginCACertificate(ctx context.Context, input CertificateInput) (domain.ManagedCertificate, error)
    SyncOriginCACertificate(ctx context.Context, input CertificateInput) (domain.ManagedCertificate, error)
    RevokeOriginCACertificate(ctx context.Context, input RevokeOriginCACertificateInput) (domain.ManagedCertificate, error)
    CertificateProviderReadiness() []certmanager.ProviderReadiness
    ManagedCertificateStatus(ctx context.Context, proxyID string) (certmanager.CertificateStatus, error)
}

type ProviderCredentialFacade interface {
    CreateProviderCredential(ctx context.Context, input ProviderCredentialInput) (domain.ProviderCredential, error)
    UpdateProviderCredential(ctx context.Context, input UpdateProviderCredentialInput) (domain.ProviderCredential, error)
    VerifyProviderCredential(ctx context.Context, credentialID string, actorID string) (domain.ProviderCredential, error)
    DisableProviderCredential(ctx context.Context, credentialID string, actorID string) (domain.ProviderCredential, error)
    DeleteProviderCredential(ctx context.Context, credentialID string, actorID string) error
}
```

保留 `actorID string` 参数，不引入新的 `Actor` 类型：当前只有单一 admin 角色（`internal/adminapi/server.go:297` 登录时判断 `user.Role != domain.RoleAdmin`），没有细粒度权限模型需要承载，提前定义抽象属于过度设计。若未来出现真实的多角色权限需求，应先在 `decisions/` 记录该需求再引入新类型，不在本 Change 内预置。

`admin.Service` 内部存在跨域私有方法（`audit`、`ensureProxyAdmission`、`activeListenerClaims`、`resolveProxyCertificateSelection`、`revokeProxyAccessIfEnabled`、`reconcileProxyListeners` 等），例如 `CreateProxy` 会调用证书绑定校验，`DeleteProxy` 会调用访问撤销。因此拆分分两个阶段：

- **阶段 1（本 Change 范围）**：只做接口收窄，不做物理拆包、不改调用点——`admin.Service` 现有方法名与上述六个接口逐一对齐，天然满足接口，实现代码零改动；所有命令方法均为值接收器，因此 `Service` 和 `*Service` 均满足接口，前者用于默认装配，后者仅用于需要修改测试夹具配置的场景。`internal/adminapi.Entry.Commands` 改为六个接口的组合；`ProxyEntryDefaults` 是 GraphQL 入口选项而非业务命令，改由独立的 `Entry.ProxyEntryDefaults` 注入。`daemon` 装配处传入同一个 `adminService` 实例及同一份入口默认值。`adminapi/server.go` 中的 `server.commands.*` 调用点只剩 facade 方法，不再读取实现字段。这一步零行为风险，靠一次 `go build ./...` 即可验证。
- **阶段 2（本 Change 之外，记录为后续可选项，不列入验收条件）**：把六个接口拆成真正独立的 struct/子包，需要先把 `audit`、`ensureProxyAdmission`、`resolveProxyCertificateSelection` 等跨域助手提炼成显式共享依赖（例如注入一个 `AuditRecorder` 和 `ProxyAdmissionPolicy`），避免拆包后重复实现或产生循环依赖。本 Change 不要求完成阶段 2。

禁止创建聚合以上全部职责的上帝式 `ServerFacade`。

**B. 为 [server-local-virtual-client.md](server-local-virtual-client.md) 预留的新端口**（本 Change 只固定接口和放置目录，不实现业务逻辑，由后续 feature Change 落地）：

```go
type SystemClientFacade interface {
    Ensure(ctx context.Context) (domain.Client, error)
    Get(ctx context.Context) (domain.Client, error)
    IsSystemClient(clientID string) bool
}

type LocalProxyFacade interface {
    Create(ctx context.Context, actorID string, input LocalProxyInput) (domain.Proxy, error)
    Update(ctx context.Context, actorID string, input LocalProxyInput) (domain.Proxy, error)
    Delete(ctx context.Context, actorID string, proxyID string) error
}

type LocalTargetPolicy interface {
    ValidateTarget(ctx context.Context, host string, port int) error
    Snapshot() AllowlistSnapshot
    Replace(ctx context.Context, input AllowlistInput) error
}
```

运行时端口包括 `VirtualSession`、`LocalDialer`、`SessionRegistry` 和 `ListenerReconciler`。`ListenerReconciler` 是本 Change 固定的 B 组端口名，现有 `ProxyListenerReconciler` 保留为其兼容别名，避免改变既有 admin 注入点。Facade 负责业务编排；端口隐藏 session、socket 和数据库实现。

`LocalProxyFacade` 沿用 A 组的 `actorID string`，不引入新的 `Actor` 类型。未来出现真实多角色权限模型时，应先通过 decision 记录需求，再演进该端口。

### 依赖方向

```text
adminapi / CLI / UI adapter
            ↓
       business facades
            ↓
 domain policies + repository interfaces + runtime ports
            ↓
 store/sqlite, session, proxy listeners, net.Dialer
```

`daemon` 只负责装配和生命周期（现状已符合，见「当前实现」）；`adminapi` 只负责认证和协议映射；`control` 只负责远端通信（含上文登记的运行时状态同步例外）；`store` 只实现 repository。新业务不得继续堆叠到 `daemon/server.go`、`admin/service.go` 或 `control/transport.go`。

### 运行时流程

无新增运行时流程；现有 control 认证、心跳、session 注册、proxy listener 转发流程保持不变，仅调整代码归属和调用路径（例如 `adminquery` 改为通过只读接口而非具体类型访问 session）。

### API 与协议

无变化。GraphQL schema、HTTP 路由、control wire protocol 字段均不改变；`buildSchema` 内部实现允许拆分为更小的 resolver 分组文件，但对外行为不变。

### Admin UI

无影响。

### 安全与失败处理

- 拆分 facade 不改变现有权限判断逻辑，仅改变调用路径；拆分前后每个方法的鉴权行为必须逐一比对，防止拆分过程中遗漏审计或校验调用。
- `control` 包登记的运行时状态写入例外仅限 `ClientStatus` 同步，不得扩大到其他字段；如需扩大范围，必须先更新本文档的核心不变量。

## 兼容与迁移

- 本 Change 不涉及数据模型变更，无需数据迁移。
- 接口方法名与 `admin.Service` 现有导出方法名逐一对齐（见「外观与端口」A 组），`adminapi/server.go` 中现有调用点不需要改写，避免无意义的大范围替换。
- 迁移过程中任何一步失败（编译失败或测试回归），回退该次提交即可，不涉及运行时数据回滚。

## 实施步骤

- [x] 在 `internal/admin` 包内新增 `UserFacade`、`ClientFacade`、`DomainFacade`、`ProxyFacade`、`CertificateFacade`、`ProviderCredentialFacade` 六个接口类型，方法名与 `admin.Service` 现有导出方法逐一对齐；添加六个值类型断言及 `CommandFacades` 的值/指针断言，确保两种注入形式都匹配。
- [x] 把 `internal/adminapi.Entry.Commands` 字段类型从 `admin.Service` 改为六个接口的组合，`internal/daemon/server.go` 装配处传入同一个 `adminService` 实例；入口默认值 `ProxyEntryDefaults` 作为非命令配置独立注入，审查 `server.commands.*` 确认其余调用均为 facade 方法。
- [x] 执行 `go build ./...` 及 `go test ./internal/admin/... ./internal/adminapi/... ./internal/adminquery/... ./internal/control/...`，确认接口切换零行为回归。
- [x] 把 `internal/adminquery/service.go` 对 `*session.Manager` 的依赖收敛为只读 `SessionSnapshotSource`，更新 `internal/daemon/server.go` 中的装配；确认查询服务仅使用 `SnapshotLatest`。
- [x] 在 `internal/control/transport.go` 中为 `setClientRuntimeStatus` 补充核心不变量登记的例外注释；审查生产代码后仅保留该 `ClientStatus` 直连 store 写入。
- [x] 固定 B 组端口（`SystemClientFacade`、`LocalProxyFacade`、`LocalTargetPolicy`、`VirtualSession`、`LocalDialer`、`SessionRegistry`、`ListenerReconciler`）接口定义和目录位置，供 [server-local-virtual-client.md](server-local-virtual-client.md) 直接实现，本 Change 内不写实现；`SystemClientFacade` 是唯一导出名。
- [x] 通过普通 client、四类 proxy 和 admin 回归测试；daemon 自定义 HTTP/HTTPS listener 两项预置 flaky 测试单独记录，不阻塞本 Change。

按实际范围删除不适用项，不要保留无意义任务。阶段 2（把六个接口拆成物理独立的 struct/子包）不列入本 Change 范围，见「外观与端口」说明。

## 验收条件

- [x] 现有普通 client 和四类 proxy 的 e2e 回归通过。
- [x] `internal/admin` 已定义 `UserFacade`/`ClientFacade`/`DomainFacade`/`ProxyFacade`/`CertificateFacade`/`ProviderCredentialFacade` 六个接口，`admin.Service` 满足全部六个接口（编译期断言通过），`internal/adminapi.Entry.Commands` 字段类型已改为接口组合。
- [x] `adminquery` 不再直接依赖 `*session.Manager` 的可变方法，只通过只读接口访问。
- [x] `control` 包中直连 store 的写入只剩下已登记的 `ClientStatus` 例外，其余生产业务 mutation 均经 facade。
- [x] B 组端口接口已定义并可编译通过，但不承载业务逻辑。
- [x] `adminapi`、`daemon`、`control` 之间不存在新的反向依赖（`go list -deps` 审查）。
- [x] 全量包测结果已结论化：`umask 077; CGO_ENABLED=0 go test ./...` 中除 daemon 自定义 HTTP/HTTPS listener 的预置 flaky 外其余包均通过；默认 `umask` 下另有既有的证书目录权限断言环境敏感失败。两类已知问题均已独立记录，不构成本 Change 回归或收尾阻塞。

## 验证记录

| 日期 | 命令/步骤 | 结果 | 说明 |
| --- | --- | --- | --- |
| `2026-07-23` | `CGO_ENABLED=0 go test ./internal/admin/... ./internal/adminapi/... ./internal/adminquery/... ./internal/control/...` | 通过 | facade、查询端口和状态同步例外 |
| `2026-07-23` | `CGO_ENABLED=0 go build ./...` | 通过 | 全模块编译 |
| `2026-07-23` | `CGO_ENABLED=0 go test ./e2e -count=1` | 通过 | 普通 client 与代理跨进程回归 |
| `2026-07-23` | `CGO_ENABLED=0 go test ./...` | 已记录既有环境敏感失败 | 默认 `umask` 下 `internal/certmanager` 的目录权限断言失败 |
| `2026-07-23` | `umask 077; CGO_ENABLED=0 go test ./...` | 已记录预置 flaky | 除 `internal/daemon.TestReconcileHTTPSProxyCustomListenerWithoutRestart` 的 502 间歇失败外，其余包通过；该类 custom Web listener 测试在未修改 HEAD 上三次独立运行中一次通过、两次失败，与本 Change 无关 |

## 文档同步

- [x] requirements 无需更新（本 Change 不改变产品行为）
- [x] architecture 已更新（`system-architecture.md` 记录 facade、只读 session 和独立入口默认值装配边界）
- [x] operations 已确认无影响
- [x] Admin UI 文档已确认无影响
- [x] worklog 已更新

## 待决策与风险

1. **长期架构决定已落文**：四层上下文的依赖方向、管理命令 facade、只读 session 查询口和 `ClientStatus` 例外已记录到 [../../decisions/server-runtime-context-boundaries.md](../../decisions/server-runtime-context-boundaries.md)。
2. **`adminapi/server.go` 的静态文件服务与 cookie/CSRF 会话管理继续保留在 adapter 内**：本 Change 不新增 `adminsession` 包；只有出现可独立演进的认证会话需求时，才以新的 Change 评估拆分，不能继续借此文件增加业务规则。
3. **`control` 运行时状态写入例外保持收窄**：目前仅允许连接生命周期同步 `ClientStatus`。未来出现高频、无需审计的运行时写入时，必须先更新架构决策和长期架构文档，不能照抄新增直连 store 调用。

## 结果

阶段 1 接口收窄与运行时端口冻结已完成，且没有改变 API、协议或运行时行为；物理拆分 facade 的阶段 2 留给后续 Change。`SystemClientFacade` 已收敛为唯一导出名，`ListenerReconciler` 成为规范端口名并保留 `ProxyListenerReconciler` 兼容别名；`ProxyEntryDefaults` 作为接口收窄后必须独立装配的非命令配置传入 `adminapi`。全量包测中的 custom Web listener 502 是已确认的预置 flaky，已转入技术债，不阻塞本 Change。

阶段 2（物理拆分 `admin.Service`）的具体实施记录见 [../completed/admin-facade-physical-split.md](../completed/admin-facade-physical-split.md)。
