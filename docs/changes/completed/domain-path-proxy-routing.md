# Domain + Path 到 Proxy 路由模型改造

## 元信息

| 项 | 值 |
| --- | --- |
| 状态 | `completed` |
| 最后更新 | `2026-07-16` |
| 完成日期 | `2026-07-16` |
| 相关需求 | [../../requirements/proxy-runtime.md](../../requirements/proxy-runtime.md) |
| 相关架构 | [../../architecture/reverse-proxy.md](../../architecture/reverse-proxy.md)、[../../architecture/certificate-management.md](../../architecture/certificate-management.md) |
| 相关决策 | [../../decisions/domain-path-proxy-routing.md](../../decisions/domain-path-proxy-routing.md) |
| 实现提交 | `95afd70` … `2b15535`（含 Domain 模型、Admin UI、ProxyRoute 清理、证书 1:n、Domain GraphQL 修复） |

> 本 Change 已完成。当前产品行为以 `requirements/`、`architecture/`、`operations/` 与代码为准。

## 背景

旧实现把 Domain、证书、默认 target 和访问认证放在父 Proxy 上，再用 `ProxyRoute` 保存覆盖路径。该模型无法让多个独立 Proxy 共享同一 Domain，也导致 TLS 选证必须先选出一个父 Proxy。

目标模型：

```text
Domain + PathPrefix => Proxy
```

Domain 管理公网主机身份、HTTP/HTTPS entry 和证书；Proxy 管理路径、Client、target、改写和访问认证。

## 改造前问题

- Domain 不能作为独立资源复用或管理。
- 同一 Domain 无法由多个 Proxy 分别承载不同路径。
- HTTPS 证书生命周期与某个后端 Proxy 耦合。
- 父 Proxy 的 `/` target 与子路由形成两套后端表达。
- 统计、访问认证难以自然归属到最终命中的 Proxy。

## 目标（已达成）

- 引入独立 Domain 和 DomainEntry 资源。
- 使用 `(domain_id, path_prefix)` 唯一映射到独立 Web Proxy。
- HTTP 与 HTTPS 复用同一套路径映射。
- HTTPS 按 SNI 选择 Domain/证书并完成 TLS，再按 Path 选择 Proxy。
- 证书与 Domain 可选 **1:n** 绑定（一证多 Domain；每 Domain 最多一证）。
- 访问激活继续绑定 Proxy，并在路径选择后执行。
- 将现有父 Proxy 与 ProxyRoute 数据迁移到新模型。
- Admin API/UI 改为先管理 Domain，再为 Proxy 选择 Domain + PathPrefix。

## 非目标

- 不改变 TCP/UDP Proxy 模型。
- 不增加正则、Header、Method、权重或负载均衡路由。
- 不实现多用户共享同一 Domain。
- 不实现一个 Domain 绑定多张证书。
- 不在本 Change 中实现逐设备访问撤销。
- 不顺带实现 HTTP/2 或完整 HTTPS Keep-Alive 重构。
- 不在本 Change 物理 DROP `proxies` 上 Web 遗留列（可另开变更）。

## 核心不变量

- Domain 的规范化 Host 全局唯一，且只有一个用户所有者。
- Domain 下所有 Proxy 的 `UserID` 必须等于 Domain 的 `UserID`。
- `(domain_id, normalized_path_prefix)` 唯一并按最长路径段前缀匹配。
- Domain 可以有多个 HTTP/HTTPS entry；entry 只决定 listener，不改变路径映射。
- HTTPS entry 启用时 Domain 必须绑定可服务证书；TLS 失败不得回退到明文或 passthrough。
- 同一张证书可被多个 Domain 引用；每个 Domain 最多一张证书。
- 访问认证归 Proxy；认证失败或存储不可用时 fail closed。
- Proxy 的 Domain/Path 变化必须统一撤销其历史激活 Token 和访问凭据。
- 未匹配任何 Proxy 时返回 `404`。

## 已落地设计摘要

### 数据模型

- `Domain`：`Host`、`CertificateID`（可空）、`Status`
- `DomainEntry`：`protocol` + `bind_host` + `port`
- Web `Proxy`：`DomainID` + `PathPrefix` + Client/target/改写/访问认证
- TCP/UDP 仍使用 Proxy entry 字段

### 运行时

1. HTTP：listener + Host → Domain → 最长 Path 前缀 → Proxy
2. HTTPS：listener + SNI → Domain/证书 → TLS → Path → Proxy → 访问认证
3. HTTP 认证：有可用 HTTPS entry 则 `308`，否则 `403`

### 迁移

- 打开 SQLite 时幂等执行 `migrateDomainPathRouting`
- 父 Proxy → Domain 下 `/` Web Proxy；`proxy_routes` → 独立 Web Proxy
- 证书绑定迁到 Domain；访问认证版本递增并撤销旧 Token/Cookie
- 迁移 flag 完成后 `DROP TABLE proxy_routes`；升级路径仍可读旧表
- 详见 [../../operations/domain-path-routing-migration.md](../../operations/domain-path-routing-migration.md)

## 阻塞问题

- [x] HTTP 访问认证：有可用 HTTPS entry 则 `308`，否则 `403`
- [x] 历史统计：`/` Proxy 保留 legacy aggregate；子路径 Proxy 从零计数

## 实施步骤

- [x] 关闭阻塞问题并更新决策/需求
- [x] 新增 Domain、DomainEntry 领域模型与仓储接口
- [x] 新增 SQLite schema、约束、冲突检测和可回滚迁移
- [x] 将 Web Proxy 改为 DomainID + PathPrefix 模型
- [x] 实现 Domain + Path 最长前缀查询与 HTTP/HTTPS runtime
- [x] 将证书生命周期身份从 Proxy 迁到 Domain（权威绑定为 Domain.CertificateID）
- [x] 迁移 Proxy 级访问认证并处理 Cookie/Token 失效
- [x] 调整 listener reconcile 由 DomainEntry 驱动
- [x] 更新 GraphQL、Admin service/query 和结构化错误
- [x] 新增 Domain UI，重构 Proxy/Certificate UI
- [x] 移除 ProxyRoute API、生成代码与 UI；迁移后 DROP `proxy_routes`
- [x] 证书→Domain 改为 1:n（去掉 unique 索引与互斥校验）
- [x] 修复 Domain 详情 GraphQL 嵌入字段解析（`certificateId`/`host`）
- [x] 增加单元、SQLite、runtime、Admin、前端与 E2E 测试
- [x] 同步 requirements、architecture、operations、decisions 与 worklog

## 验收条件

- [x] 同一 Domain 可以创建 `/`、`/api`、`/api/v2` 等多个独立 Proxy
- [x] HTTP 与 HTTPS 对相同 Domain+Path 命中同一个 Proxy
- [x] 最长路径段前缀正确，`/api` 不匹配 `/apix`
- [x] HTTPS 在读取 Path 前只解析 Domain/证书，不预选 Proxy
- [x] Domain 无可服务证书时 HTTPS fail closed
- [x] 访问认证按最终命中的 Proxy 生效，未认证请求不转发
- [x] Proxy 的 Domain/Path 变化后旧 Token/Cookie 全部失效
- [x] 同一 Domain 的 Proxy 不能跨用户
- [x] 迁移可转换无冲突旧数据，并明确拒绝冲突数据
- [x] listener 更新、Proxy 更新和证书失败不会留下部分运行状态
- [x] Admin UI 能独立管理 Domain，并为 Proxy 选择 Domain + Path
- [x] 旧 ProxyRoute API/UI 被移除；`proxy_routes` 表迁移后 DROP
- [x] 同一证书可绑定多个 Domain（hostnames 覆盖时）；生产部署已确认

## 验证记录

| 日期 | 命令/步骤 | 结果 | 说明 |
| --- | --- | --- | --- |
| 2026-07-15 | 文档模型评审 | 通过 | Domain 独立、证书绑定、认证归 Proxy |
| 2026-07-15 | `go test` domain/store/proxy/control/admin/adminapi/adminquery/daemon/e2e | 通过 | 核心运行时、迁移、Admin、e2e |
| 2026-07-16 | `go test` admin/store/adminapi/adminquery + admin-ui certificate 相关测试 | 通过 | GraphQL 嵌入字段、1:n 证书、UI 选证 |
| 2026-07-16 | 生产部署验收 | 通过 | Domain 详情证书展示与绑定可用；一证多 Domain |

## 文档同步

- [x] requirements 已更新为最终目标与实现状态
- [x] architecture 已更新为完成后的当前事实
- [x] certificate 文档改为 Domain 绑定（1:n）
- [x] Admin UI 文档已增加 Domain 页面并移除 ProxyRoute 模型
- [x] operations 已增加迁移/回滚说明
- [x] decisions 状态更新为已实现
- [x] worklog 已更新

## 结果

Domain + Path 路由模型已完成并部署验证。权威绑定为 `Domain.CertificateID`；证书与 Domain 为 1:n。后续可选工作：`proxies` Web 遗留列物理 DROP、legacy HTTP/HTTPS 类型提示清理。

## 相关文档

- 决策：[../../decisions/domain-path-proxy-routing.md](../../decisions/domain-path-proxy-routing.md)
- 迁移：[../../operations/domain-path-routing-migration.md](../../operations/domain-path-routing-migration.md)
- 被替代实施记录：[../completed/http-path-routing-and-https-access-activation.md](../completed/http-path-routing-and-https-access-activation.md)
