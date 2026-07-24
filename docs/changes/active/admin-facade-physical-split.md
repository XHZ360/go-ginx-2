# Admin 业务 Facade 物理拆分

## 元信息

| 项 | 值 |
| --- | --- |
| 状态 | `active` |
| 最后更新 | `2026-07-23` |
| 负责人 | 未指定 |
| 相关需求 | 无（本 Change 不改变产品行为，仅调整实现边界） |
| 相关架构 | [../../architecture/system-architecture.md](../../architecture/system-architecture.md) |
| 相关决策 | [../../decisions/server-runtime-context-boundaries.md](../../decisions/server-runtime-context-boundaries.md) |
| 实现提交 | 未完成 |

> 本文描述尚未完成的目标与实施过程，不代表当前代码已经具备目标行为。

## 背景

[server-runtime-context-architecture.md](../completed/server-runtime-context-architecture.md)（阶段 1）已经把 `admin.Service` 的六个业务领域收窄成六个接口（`UserFacade`/`ClientFacade`/`DomainFacade`/`ProxyFacade`/`CertificateFacade`/`ProviderCredentialFacade`），但接口背后仍是同一个 `admin.Service` struct 实现——单测隔离、变更影响面、包体量都没有真正改善，只是把调用方和实现之间加了一层接口。

阶段 1 的完成记录明确把物理拆分列为「本 Change 之外的可选项」，原因是 `admin.Service` 内部存在大量跨域私有方法，直接拆包会导致重复实现或循环依赖。本 Change 是该后续工作的落地计划。

## 当前实现

对 `internal/admin/service.go` + `internal/admin/domain_service.go` 全部私有（小写）方法做了完整调用图梳理（而非假设），耦合关系如下：

- `audit`（`internal/admin/service.go:1864`）：被全部六个 facade 的近 40 处 mutation 方法调用，是唯一一个真正横切全部领域的助手。
- 监听器与准入策略——`reconcileProxyListeners`（`:1811`）、`ensureProxyAdmission`（`:1734`）、`activeListenerClaims`（`:1787`）、`findActiveRouteConflict`（`:1821`）、`findActiveWebRouteConflict`（`:1844`）：只被 **Proxy** 和 **Domain**（`domain_service.go` 中的 `createWebProxy`/`CreateDomainEntry`/`UpdateDomainEntry` 等）调用，互相耦合但不涉及 User/Client/Certificate/ProviderCredential。
- 证书绑定策略——`resolveProxyCertificateSelection`（`:1314`）、`validateCertificateBinding`（`:1358`）、`proxyBoundToCertificate`（`:1378`）、`unbindProxyCertificate`（`:1397`）、`unbindDomainCertificate`（`:1411`）、`boundDomainCertificate`（`:300`）、`boundProxyCertificate`（`:1434`）、`certificateServable`（`:1457`）、`cleanupManagedCertificateFiles`（`:1483`）、`certificateManager`（`:1710`）：被 **Proxy**、**Domain**、**Certificate** 三个 facade 交叉调用，是耦合最深的一组。
- 代理访问策略——`revokeProxyAccessIfEnabled`（`:253`）、`revokeAccessForDomainProxies`（`:270`）、`domainHasEnabledHTTPSEntry`（`:287`）：被 **Proxy** 和 **Domain** 调用。
- `createWebProxy`（`domain_service.go:331`）：由 **Domain** 的 `CreateDomainEntry`（`domain_service.go:375`）直接调用 **Proxy** 创建逻辑——这是唯一一处 facade 之间直接互相创建对方领域资源的耦合，说明 Domain 和 Proxy 在实现层面不能完全独立。
- Client 专属私有方法——`resetClientJoinToken`（`:644`）、`tokenUsesLegacyAdminEnrollmentURL`（`:701`）、`usesLegacyAdminEnrollmentURL`（`:709`）、`defaultJoinTokenPayload`（`:715`）：只被 Client 自身的方法调用，无跨域依赖。
- User、ProviderCredential 两个领域没有除 `audit` 之外的私有助手，是全部六个领域里耦合最低的两个。

结论：**User、ProviderCredential、Client 三个领域可以独立拆包，只需要共享一个 `AuditRecorder`；Domain、Proxy、Certificate 三个领域深度纠缠，必须一起设计共享依赖，不能分别独立拆分。**

## 问题

- `admin.Service` 仍是单一 2000+ 行 struct，六个 facade 接口的调用方看不出实现内部的领域边界，新贡献者容易继续往同一个 struct 加方法。
- 证书绑定、监听器准入、代理访问撤销三组策略散落在 `admin.Service` 的私有方法里，没有独立命名和边界，无法单独测试或复用（例如 [server-local-virtual-client.md](server-local-virtual-client.md) 未来若需要判断证书绑定，只能依赖整个 `admin.Service`）。
- Domain 直接调用 Proxy 创建逻辑（`createWebProxy`）说明当前模型下 Domain 和 Proxy 不是纯粹的同级领域，拆分设计必须显式面对这层依赖，而不是假装可以完全解耦。

## 目标

- 把 User、ProviderCredential、Client 三个低耦合领域从 `admin.Service` 中物理拆出，独立成 struct（可在同一 `internal/admin` 包内，也可拆子包），只共享一个 `AuditRecorder` 接口。
- 为 Domain、Proxy、Certificate 三个高耦合领域设计显式共享依赖（`ProxyAdmissionPolicy`、`CertificateBindingPolicy`、`ProxyAccessPolicy`、`WebProxyCreator` 或等价命名），替代当前的私有方法直连；三者仍允许在物理实现上放在同一个内部协作单元，但对外通过独立的 `DomainFacade`/`ProxyFacade`/`CertificateFacade` 实现暴露。
- 拆分前后每个 facade 方法的鉴权、审计、错误语义必须逐一比对，不产生行为差异。
- 六个 facade 接口签名（阶段 1 已冻结）保持不变，`adminapi` 侧调用点不需要修改。

## 非目标

- 不改变 GraphQL/HTTP 契约、数据库 schema 或权限模型。
- 不在本 Change 内实现细粒度多角色权限（`Actor` 类型讨论见阶段 1 结论，不重复展开）。
- 不要求把 Domain/Proxy/Certificate 拆成三个完全独立、零共享状态的 struct——三者允许共享内部协作依赖，只要求对外接口边界清晰、可分别测试关键路径。
- 不处理 `adminapi/server.go` 的静态文件服务/cookie/CSRF 归属（阶段 1 已记录为独立议题）。

## 核心不变量

- `AuditRecorder`、`ProxyAdmissionPolicy`、`CertificateBindingPolicy`、`ProxyAccessPolicy` 等共享依赖必须是显式接口注入，不允许通过嵌入同一个 struct 隐式共享私有方法。
- 拆分后的领域实现之间如果存在依赖（例如 Domain 依赖 Proxy 创建），必须通过接口调用，不允许互相访问对方的私有字段或方法。
- 六个 facade 对外方法签名不变；`adminapi.Entry.Commands` 字段类型不变。
- 审计事件的 `resourceType`/`action` 字符串必须与拆分前逐一保持一致（详见「当前实现」中 `audit` 调用点清单），防止审计日志格式漂移。

## 目标设计

### 拆分分组与顺序

按耦合程度分三批，每批独立提交、独立验证：

**第一批：User + ProviderCredential**（耦合最低，先验证拆分模式）
- 拆出 `UserService`、`ProviderCredentialService`（或等价命名），只依赖 `store.Store` 和一个新增的 `AuditRecorder` 接口（`Record(ctx, actorID, resourceType, resourceID, action string) error`，签名对齐现有 `audit` 私有方法）。
- `admin.Service` 保留一个 `AuditRecorder` 默认实现（当前 `audit` 方法原样移动），供 `daemon` 装配时注入给拆分出来的新 struct。

**第二批：Client**
- 拆出 `ClientService`，携带 `resetClientJoinToken`/`tokenUsesLegacyAdminEnrollmentURL`/`usesLegacyAdminEnrollmentURL`/`defaultJoinTokenPayload` 四个私有方法（原样迁移，无需重新设计，因为它们没有跨域调用）。
- 同样依赖 `AuditRecorder`。

**第三批：Domain + Proxy + Certificate**（耦合最深，需要先定义共享依赖再拆）
- 新增以下共享依赖接口（放在 `internal/admin` 包内，不下沉到子包，避免过度设计）：
  ```go
  type ProxyAdmissionPolicy interface {
      EnsureAdmission(ctx context.Context, proxy domain.Proxy, ignoreProxyID string) error
      ReconcileListeners(ctx context.Context) error
  }

  type CertificateBindingPolicy interface {
      ResolveProxyCertificateSelection(ctx context.Context, proxyType domain.ProxyType, proxyID string, certificateID string, certificateIDSet bool, entryHost string, certFile string, keyFile string, actorID string) (string, error)
      ValidateBinding(ctx context.Context, certificateID string, host string, domainID string) error
      UnbindProxyCertificate(ctx context.Context, proxy domain.Proxy) error
      UnbindDomainCertificate(ctx context.Context, webDomain domain.Domain) error
      BoundDomainCertificate(ctx context.Context, webDomain domain.Domain) (domain.ManagedCertificate, error)
  }

  type ProxyAccessPolicy interface {
      RevokeIfEnabled(ctx context.Context, proxy *domain.Proxy) error
      RevokeForDomainProxies(ctx context.Context, domainID string) error
      DomainHasEnabledHTTPSEntry(ctx context.Context, domainID string) bool
  }

  type WebProxyCreator interface {
      CreateWebProxy(ctx context.Context, input CreateProxyInput) (domain.Proxy, error)
  }
  ```
- `DomainService`、`ProxyService`、`CertificateService` 三个新 struct 各自持有上述接口中自己需要的子集；`DomainService` 额外持有 `WebProxyCreator`（解决 `createWebProxy` 依赖），实现可以是指向 `ProxyService` 自身的引用。
- 具体实现（`certificateManager`、`activeListenerClaims`、`findActiveRouteConflict`、`findActiveWebRouteConflict` 等原私有方法体）原样迁移进对应策略实现，不重写逻辑，只重新归属包裹它们的 struct。

### 依赖方向

```text
adminapi.Entry.Commands (六个接口组合，不变)
            ↓
UserService / ClientService / ProviderCredentialService  →  AuditRecorder
            ↓
DomainService / ProxyService / CertificateService  →  AuditRecorder
                                                    →  ProxyAdmissionPolicy
                                                    →  CertificateBindingPolicy
                                                    →  ProxyAccessPolicy
                                                    →  WebProxyCreator（Domain → Proxy）
```

`daemon` 装配处负责把这些新 struct 组装成满足六个既有 facade 接口的实例（可以是六个独立字段各自实现自己的接口，也可以用一个轻量聚合 struct 把六个字段包起来再满足 `admin.CommandFacades`——具体选择留给实施时决定，不在本 Change 预先固定，只要求 `adminapi.Entry.Commands` 的注入方式不变）。

### API 与协议

无变化。

### Admin UI

无影响。

### 安全与失败处理

- 拆分过程中，每迁移一个方法，必须同时核对该方法调用的 `audit`/权限校验/错误分类是否原样保留，禁止在物理搬迁时“顺手”简化或合并校验逻辑。
- `AuditRecorder`、各策略接口的实现如果失败，错误处理语义（返回错误 vs. 吞掉继续）必须与原 `admin.Service` 内联调用时的行为完全一致——现状中部分调用点対 `reconcileProxyListeners` 错误做了“失败后重试一次仍失败则忽略”的处理（如 `internal/admin/service.go:784-786`），迁移时必须保留这个具体行为，不能统一改成“总是返回错误”或“总是忽略”。

## 兼容与迁移

- 不涉及数据模型变更。
- 六个 facade 对外签名不变，`adminapi`/`daemon` 侧无需修改调用代码，只需修改 `daemon` 装配处如何构造实现六个接口的实例。
- 任一批次拆分完成后即可独立合并；三批之间没有顺序依赖之外的耦合（第三批依赖前两批验证过的拆分模式，但不依赖前两批的具体代码）。
- 任一步失败（编译或测试回归）回退该次提交即可。

## 实施步骤

- [ ] 第一批：新增 `AuditRecorder` 接口和默认实现（迁移现有 `audit` 方法体）；拆出 `UserService`、`ProviderCredentialService`，各自实现 `UserFacade`、`ProviderCredentialFacade`；更新 `daemon` 装配处；跑 `go build ./... && go test ./internal/admin/... ./internal/adminapi/...`确认零回归。
- [ ] 第二批：拆出 `ClientService`（携带四个 join token 私有方法），实现 `ClientFacade`；更新装配；跑同一组测试。
- [ ] 第三批第 1 步：定义 `ProxyAdmissionPolicy`、`CertificateBindingPolicy`、`ProxyAccessPolicy`、`WebProxyCreator` 四个接口，逐一核对本文档「当前实现」列出的每个私有方法应归入哪个接口，不遗漏。
- [ ] 第三批第 2 步：拆出 `DomainService`、`ProxyService`、`CertificateService`，把对应私有方法体原样迁移进策略实现；`DomainService` 注入 `WebProxyCreator`（指向 `ProxyService`）解决 `createWebProxy` 依赖。
- [ ] 第三批第 3 步：更新 `daemon` 装配处，组装六个新 struct 满足 `admin.CommandFacades`；跑 `go build ./... && go test ./internal/admin/... ./internal/adminapi/... ./internal/daemon/...`。
- [ ] 全部三批完成后，删除 `admin.Service` 中已迁移的方法和字段；确认 `internal/admin` 包内不再存在跨域私有方法。
- [ ] 通过普通 client、四类 proxy 和 admin 回归测试，含 `go test ./e2e -count=1`。

## 验收条件

- [ ] User、ProviderCredential、Client 三个领域已拆成独立 struct，只依赖 `AuditRecorder`。
- [ ] Domain、Proxy、Certificate 三个领域已拆成独立 struct，依赖关系收敛到 `ProxyAdmissionPolicy`/`CertificateBindingPolicy`/`ProxyAccessPolicy`/`WebProxyCreator` 四个显式接口，不存在跨 struct 的私有方法调用。
- [ ] `admin.Service` 作为聚合 struct 被移除，或收缩为仅供测试/装配使用的组合体（视实施时的装配方式而定，实施完成后在本文档「结果」中记录实际选择）。
- [ ] 六个 facade 对外签名不变，`adminapi`/`daemon` 调用点不需要修改（除装配代码）。
- [ ] 审计事件的 `resourceType`/`action` 字符串逐一比对拆分前后一致。
- [ ] `go test ./...`（除已知与本 Change 无关的 daemon custom listener flaky 和证书目录权限断言外）通过；`go test ./e2e -count=1` 通过。

## 验证记录

| 日期 | 命令/步骤 | 结果 | 说明 |
| --- | --- | --- | --- |
| `2026-07-23` | — | 未执行 | 计划阶段，尚未开始实施 |

## 文档同步

- [ ] requirements 无需更新（本 Change 不改变产品行为）
- [ ] architecture 需要更新：完成后在 `system-architecture.md` 补充实际拆分后的包/struct 边界（当前只记录了接口层面的边界）
- [ ] operations 确认无影响
- [ ] Admin UI 文档确认无影响
- [ ] worklog 需要更新

## 待决策与风险

1. **拆分后的物理位置**：六个新 struct 是否需要下沉到独立子包（如 `internal/admin/userdomain`），还是留在 `internal/admin` 包内以不同文件区分。倾向于先留在同一包内（降低本次改动幅度），如后续包体量或循环依赖风险增加再拆子包，避免提前过度设计。
2. **`admin.Service` 是否完全删除**：若 `daemon` 装配处选择用一个聚合 struct 满足 `admin.CommandFacades`，`admin.Service` 这个名字可能继续以聚合体形式存在，只是内部字段从直接实现变成组合六个新 struct。实施时需要决定是否保留这个名字或换新名字，避免与阶段 1 的历史命名冲突造成混淆。
3. **`WebProxyCreator` 的方向是否应该反过来**：当前 `createWebProxy` 由 Domain 调用 Proxy 逻辑，但语义上"Domain 决定要不要创建默认 web proxy"更像是 Domain 的业务规则，只是复用了 Proxy 的创建能力。实施时需要确认这层依赖方向本身是否符合长期设计意图，而不是照抄现状了事；如果认为方向不对，应该先在本文档记录新的决策再改，不要在拆分过程中顺手改变语义。

## 结果

未开始，计划阶段。
