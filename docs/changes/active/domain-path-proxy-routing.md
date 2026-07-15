# Domain + Path 到 Proxy 路由模型改造

## 元信息

| 项 | 值 |
| --- | --- |
| 状态 | `active` |
| 最后更新 | `2026-07-15` |
| 相关需求 | [../../requirements/proxy-runtime.md](../../requirements/proxy-runtime.md) |
| 相关架构 | [../../architecture/reverse-proxy.md](../../architecture/reverse-proxy.md)、[../../architecture/certificate-management.md](../../architecture/certificate-management.md) |
| 相关决策 | [../../decisions/domain-path-proxy-routing.md](../../decisions/domain-path-proxy-routing.md) |
| 实现提交 | 未完成 |

> 本文描述已采纳但尚未完成的目标模型。当前代码仍使用“一个 Domain 对应一个父 Proxy + ProxyRoute 子路由”的旧模型。

## 背景

已完成的 HTTP 路径路由实现把 Domain、证书、默认 target 和访问认证放在父 Proxy 上，再用 `ProxyRoute` 保存覆盖路径。该模型可以把不同路径转到不同 Client/target，但无法让多个独立 Proxy 共享同一 Domain，也导致 TLS 选证必须先选出一个父 Proxy。

目标模型是：

```text
Domain + PathPrefix => Proxy
```

Domain 独立管理公网主机身份、HTTP/HTTPS entry 和证书；Proxy 管理路径、Client、target、改写和访问认证。

## 当前实现

当前代码证据：

- `domain.Proxy` 同时持有 `EntryHost`、Web entry、target、`CertificateID` 和访问认证字段。
- `domain.ProxyRoute` 通过 `ProxyID` 挂在父 Proxy 下，并重复保存 Client、target 与路径改写字段。
- `ProxyRepository.ByHTTPRoute` / `ByHTTPSRoute` 只按 listener + Host/SNI 返回一个 Proxy，之后运行时才查询该 Proxy 的子路由。
- SQLite 使用 `(entry_bind_host, entry_port, entry_host)` 唯一索引，禁止同一 listener 上多个 Proxy 共享 Domain。
- `proxy_routes` 使用 `unique(proxy_id, path_prefix)`，路径唯一性被限制在父 Proxy 内。
- HTTPS 在读取请求 Path 前已经根据 SNI 选定 Proxy 和证书。
- Admin API/UI 在 Proxy 详情中管理 routes，证书也绑定 Proxy。

当前实现可运行，但模型已被 [Domain + Path 到 Proxy 的路由决策](../../decisions/domain-path-proxy-routing.md) 替代。

## 问题

- Domain 不能作为独立资源复用或管理。
- 同一 Domain 无法由多个 Proxy 分别承载不同路径。
- HTTPS 证书生命周期与某个后端 Proxy 耦合。
- 父 Proxy 的 `/` target 与子路由形成两套后端表达。
- 统计、访问认证和生命周期操作难以自然归属到最终命中的 Proxy。
- HTTP 与 HTTPS 的相同 Domain/Path 可能形成两套不一致配置。

## 目标

- 引入独立 Domain 和 Domain entry 资源。
- 使用 `(domain_id, path_prefix)` 唯一映射到独立 Web Proxy。
- HTTP 与 HTTPS 复用同一套路径映射。
- HTTPS 按 SNI 选择 Domain/证书并完成 TLS，再按 Path 选择 Proxy。
- 证书与 Domain 可选一对一绑定。
- 访问激活继续绑定 Proxy，并在路径选择后执行。
- 将现有父 Proxy 与 ProxyRoute 数据迁移到新模型。
- Admin API/UI 改为先管理 Domain，再为 Proxy 选择 Domain + PathPrefix。

## 非目标

- 不改变 TCP/UDP Proxy 模型。
- 不增加正则、Header、Method、权重或负载均衡路由。
- 不实现多用户共享同一 Domain。
- 不实现一个证书绑定多个 Domain。
- 不在本 Change 中实现逐设备访问撤销。
- 不顺带实现 HTTP/2 或完整 HTTPS Keep-Alive 重构。

## 核心不变量

- Domain 的规范化 Host 全局唯一，且只有一个用户所有者。
- Domain 下所有 Proxy 的 `UserID` 必须等于 Domain 的 `UserID`。
- `(domain_id, normalized_path_prefix)` 唯一并按最长路径段前缀匹配。
- Domain 可以有多个 HTTP/HTTPS entry；entry 只决定 listener，不改变路径映射。
- HTTPS entry 启用时 Domain 必须绑定可服务证书；TLS 失败不得回退到明文或 passthrough。
- 访问认证归 Proxy；认证失败或存储不可用时 fail closed。
- Proxy 的 Domain/Path 变化必须统一撤销其历史激活 Token 和访问凭据。
- 未匹配任何 Proxy 时返回 `404`。

## 目标设计

### 数据模型

建议目标模型：

```text
Domain
  ID
  UserID
  Host
  CertificateID      # nullable；HTTPS entry 启用时必填
  Status
  CreatedAt
  UpdatedAt

DomainEntry
  ID
  DomainID
  Protocol           # http | https
  BindHost
  Port
  Status
  CreatedAt
  UpdatedAt

Proxy
  ID
  UserID
  ClientID
  DomainID           # Web Proxy 必填；TCP/UDP 为空
  PathPrefix         # Web Proxy 必填
  StripPrefix
  UpstreamPathPrefix
  TargetHost
  TargetPort
  AccessAuthEnabled
  AccessAuthVersion
  Status
  ...
```

约束：

- `unique(lower(domains.host))`。
- `unique(domains.certificate_id)`，空值除外。
- `unique(domain_id, protocol, lower(bind_host), port)`。
- `unique(domain_id, normalized_path_prefix)`。
- Domain 删除前必须处理 Proxy、entry、证书和访问凭据依赖，不能产生孤儿运行时状态。

Web Proxy 应使用协议无关的类型（例如 `web`）；HTTP/HTTPS 由 DomainEntry 决定。TCP/UDP 保留现有类型与 entry 字段。

### HTTP 运行时

1. 根据 listener + Host 找到 Domain。
2. 校验 Domain 可用。
3. 校验请求 Path。
4. 查询 Domain 下启用的 Web Proxy，按最长路径段前缀选择一个 Proxy。
5. 按 Proxy 配置改写 Path。
6. 选择 Proxy 的最新 Provider 会话并打开 target 流。
7. 未命中 Proxy 返回 `404`；Client 离线返回 `503`；上游失败返回 `502`。

### HTTPS 运行时

1. 根据 listener + SNI 找到 Domain。
2. 根据 Domain 的 `CertificateID` 解析可服务证书。
3. 完成 TLS 握手并读取 HTTP 请求。
4. 激活端点优先处理；Token 直接定位目标 Proxy。
5. 根据 Domain + Path 最长前缀选择 Proxy。
6. 如果 Proxy 启用访问认证，校验该 Proxy 的 Cookie；失败返回 `401`。
7. 移除 go-ginx 认证 Cookie，按 Proxy 配置改写并转发。

### 访问激活

- Token 与 Credential 继续关联 `ProxyID`。
- Cookie 名包含 Proxy 稳定摘要，Cookie `Path` 使用该 Proxy 的规范化 `PathPrefix`。
- 激活 URL 使用 Domain 的 HTTPS entry 和系统保留路径，Token 用于定位 Proxy。
- Proxy 迁移 Domain、修改 PathPrefix、关闭认证或删除时，递增认证版本并撤销未使用 Token 与历史 Credential。
- 路径重叠时先完成最长前缀选择，再校验命中 Proxy 的认证策略。
- HTTP 与 HTTPS 共享 Proxy；如果命中 Proxy 启用了访问认证，HTTP 不能绕过 HTTPS 认证。具体采用 HTTPS 重定向还是直接拒绝，实施前必须关闭下方阻塞问题。

### 证书

- `Domain.CertificateID` 是权威绑定；`Proxy.CertificateID` 与证书侧遗留 `ProxyID` 迁移后删除。
- HTTPS entry 启用时要求证书覆盖 Domain Host 且材料可服务。
- 静态文件证书也注册为证书资源，不再直接保存到 Proxy。
- 证书签发、续期、轮换、同步和撤销以 Domain/Certificate 为身份，不再以 Proxy 为身份。
- Domain 删除或改名不得自动撤销远端证书；沿用显式高风险操作与失败保留规则。

### listener 协调

- listener 注册表继续按 `(protocol, bind_host, port)` 合并。
- DomainEntry 决定需要哪些 listener；Proxy 的启停不直接创建新的 listener。
- 同一 listener 可以服务多个 Domain；同一 Domain 可以配置多个 entry，且共享路径映射和 HTTPS 证书。

### API 与 GraphQL

- 新增 Domain 的列表、详情、创建、更新、启停、删除和 entry 管理。
- 证书绑定/解绑操作从 Proxy 改为 Domain。
- Proxy 输入增加 `domainId`、`pathPrefix`、`stripPrefix`、`upstreamPathPrefix`。
- 删除 `routes` 字段和 create/update/delete ProxyRoute mutation。
- 访问激活 mutation 保留 ProxyID 语义。
- API 必须返回 Domain/Path 冲突、跨用户 Domain、证书不可用和迁移冲突的结构化错误。

### Admin UI

- 新增 Domain 列表与详情管理入口。
- Domain 页面管理 Host、HTTP/HTTPS entries、证书绑定和其下 Proxy 列表。
- Proxy 创建/编辑选择 Domain 和 PathPrefix，不再编辑 Domain、证书或子路由列表。
- Proxy 详情保留 Client/target、路径改写、统计与访问激活操作。
- Certificates 页面展示绑定 Domain，不再展示绑定 Proxy。
- Domain/Path 冲突、证书缺失和跨用户选择必须有明确字段级错误。

### 安全与失败处理

- Domain 所有权、Proxy 用户、Client 用户必须一致。
- TLS 证书解析只使用 Domain 权威绑定，迁移结束后删除按 Proxy/Host 猜测的回退路径。
- 访问 Token、Cookie secret、私钥和 provider credential 不进入日志、API 查询或快照。
- Domain、证书或路径迁移失败时不得部分切换运行时；旧模型在迁移事务成功前保持可用。
- 迁移检测到无法自动合并的数据时停止并输出脱敏冲突，不静默选择记录。

## 兼容与迁移

### 数据映射

1. 按规范化 `EntryHost` 聚合现有 HTTP/HTTPS Proxy，创建 Domain。
2. 从现有 HTTP/HTTPS listener 配置创建 DomainEntry；HTTP 与 HTTPS entry 可以并存。
3. 每个父 Proxy 转成对应 Domain 下的 `/` Web Proxy。
4. 每条 `ProxyRoute` 转成独立 Web Proxy，继承父 Proxy 的 UserID/DomainID，保留自己的 Client/target/path 改写。
5. 把 HTTPS Proxy 的证书绑定迁移到 Domain。
6. 确认新运行时可读取后，再删除 `proxy_routes` 和 Proxy 上被替代字段。

### 必须阻止自动迁移的冲突

- 同一 Domain + Path 在现有 HTTP/HTTPS 配置中指向不同 Client/target/改写规则。
- 同一 Domain 存在多个不同证书绑定或静态证书材料。
- 同一 Domain 下的现有 Proxy 属于不同用户。
- 规范化后 Host 或 PathPrefix 冲突。
- 证书不覆盖 Domain Host，或绑定证书当前不可服务。

冲突必须生成脱敏报告并要求管理员处理；不得按创建时间或 ID 自动选择赢家。

### 访问认证迁移

旧父 Proxy 的访问认证覆盖全部子路由；迁移为多个独立 Proxy 后无法安全复用同一 Cookie/secret：

- 父 Proxy ID 保留给 `/` Proxy。
- 子路由生成的新 Proxy 继承“是否启用认证”，但使用新的认证版本。
- 迁移时撤销相关未使用 Token 和历史 Credential，要求管理员为各 Proxy 重新生成激活链接。
- 迁移完成前保持旧运行时可用；切换后旧 Cookie 必须全部失效。

### 统计迁移

- 原父 Proxy 的历史累计统计包含所有旧子路由流量，不能伪装为 `/` Proxy 的精确统计。
- 实施前选择“保留为 legacy aggregate”或“归档后新模型从零计数”，并在 Admin UI 明确展示语义。
- 新模型完成后统计按最终命中的 Proxy 聚合。

### 回滚

- Schema 迁移和数据转换必须先建立新表/新列，不立即删除旧数据。
- 切换运行时前完成一致性校验并保留备份。
- 切换失败时继续使用旧表与旧查找路径；不得留下部分 Domain 使用新模型、部分仍使用旧模型的未标记状态。
- 删除旧表、旧 GraphQL 字段和兼容代码应作为迁移确认后的独立清理阶段。

## 阻塞问题

- [ ] 启用访问认证的 Web Proxy 经 HTTP entry 被访问时，是 `308` 重定向到 HTTPS，还是直接 fail closed（`403`/`404`）？建议有可用 HTTPS entry 时 `308`，否则 fail closed。
- [ ] 旧父 Proxy 聚合统计采用 legacy aggregate 保留，还是归档后新模型从零计数？

阻塞问题关闭前，本 Change 不满足 Ready for Implementation。

## 实施步骤

- [ ] 关闭阻塞问题并更新决策/需求
- [ ] 新增 Domain、DomainEntry 领域模型与仓储接口
- [ ] 新增 SQLite schema、约束、冲突检测和可回滚迁移
- [ ] 将 Web Proxy 改为 DomainID + PathPrefix 模型
- [ ] 实现 Domain + Path 最长前缀查询与 HTTP runtime
- [ ] 调整 HTTPS 为 Domain 选证、TLS 后选 Proxy
- [ ] 将证书生命周期身份从 Proxy 迁到 Domain
- [ ] 迁移 Proxy 级访问认证并处理 Cookie/Token 失效
- [ ] 调整 listener reconcile 由 DomainEntry 驱动
- [ ] 更新 GraphQL、Admin service/query 和结构化错误
- [ ] 新增 Domain UI，重构 Proxy/Certificate UI
- [ ] 迁移或移除 ProxyRoute API、生成代码与 UI
- [ ] 增加单元、SQLite、runtime、Admin、前端与 E2E 测试
- [ ] 同步 requirements、architecture、operations 与 worklog
- [ ] 确认迁移稳定后清理旧字段、旧表和兼容回退

## 验收条件

- [ ] 同一 Domain 可以创建 `/`、`/api`、`/api/v2` 等多个独立 Proxy。
- [ ] HTTP 与 HTTPS 对相同 Domain+Path 命中同一个 Proxy。
- [ ] 最长路径段前缀正确，`/api` 不匹配 `/apix`。
- [ ] HTTPS 在读取 Path 前只解析 Domain/证书，不预选 Proxy。
- [ ] Domain 无可服务证书时 HTTPS fail closed。
- [ ] 访问认证按最终命中的 Proxy 生效，未认证请求不转发。
- [ ] Proxy 的 Domain/Path 变化后旧 Token/Cookie 全部失效。
- [ ] 同一 Domain 的 Proxy 不能跨用户。
- [ ] 迁移可转换无冲突旧数据，并明确拒绝冲突数据。
- [ ] listener 更新、Proxy 更新和证书失败不会留下部分运行状态。
- [ ] Admin UI 能独立管理 Domain，并为 Proxy 选择 Domain + Path。
- [ ] 旧 ProxyRoute API/UI 被移除或明确处于兼容期。

## 验证记录

| 日期 | 命令/步骤 | 结果 | 说明 |
| --- | --- | --- | --- |
| 2026-07-15 | 文档模型评审 | 通过 | 已确认 Domain 独立、证书 1:1、认证归 Proxy、HTTP/HTTPS 共享映射 |
| 2026-07-15 | 代码测试 | 未执行 | 当前仅建立 Change，尚未修改代码 |

计划中的最小验证：

```powershell
$env:CGO_ENABLED="0"
go test ./internal/domain ./internal/store/sqlite ./internal/proxy/http ./internal/proxy/https ./internal/admin ./internal/adminapi ./internal/daemon
go test ./e2e -count=1

Set-Location admin-ui
pnpm graphql:refresh
pnpm test
pnpm build
```

## 文档同步

- [ ] requirements 已更新为最终目标与实现状态
- [ ] architecture 已更新为完成后的当前事实
- [ ] certificate 文档已改为 Domain 绑定
- [ ] Admin UI 文档已增加 Domain 页面并移除 ProxyRoute 模型
- [ ] operations 已增加迁移/回滚说明
- [x] worklog 已记录 active Change

## 结果

进行中。当前只完成模型决策与变更计划，代码、Schema、API、UI 和迁移尚未实施。
