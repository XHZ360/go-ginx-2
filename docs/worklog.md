# 工作日志

记录当前状态与下一步。详细历史批次见 [changes/archive/milestone-one-continuation.md](changes/archive/milestone-one-continuation.md)。

## 元信息

| 项 | 值 |
| --- | --- |
| 最后更新 | 2026-07-23 |
| 基线提交 | `2b15535`（证书 1:n + Domain GraphQL 修复） |
| 验证状态 | Domain Path Change 已完成并部署验收 |

## 当前状态

- 里程碑一运行时与首个部署基线已具备：控制通道、反向代理、Admin API/UI、证书管理、SQLite、Linux systemd / Windows 服务发布包。
- 文档按信息类型收敛到 `docs/`（project / requirements / architecture / operations / changes / references）。
- completed Change [domain-path-proxy-routing.md](changes/completed/domain-path-proxy-routing.md)：`Domain + PathPrefix => Proxy` 已落地并部署；证书与 Domain 为 1:n；`proxy_routes` 已清理。
- completed Change [acme-certificate-readiness-ux.md](changes/completed/acme-certificate-readiness-ux.md)：已补齐 ACME DNS-01/Origin CA provider readiness、`PROVIDER_NOT_READY` 契约和 Certificates 页面前置诊断；完整包测仅受既有目录权限测试失败阻断。
- completed Change [server-runtime-context-architecture.md](changes/completed/server-runtime-context-architecture.md)：管理 facade、只读 session 查询端口与本机代理预留端口已实现；接口收窄后的 `ProxyEntryDefaults` 已改为独立装配配置。
- active Change [admin-facade-physical-split.md](changes/active/admin-facade-physical-split.md)：runtime context 阶段 2，计划把 `admin.Service` 按耦合程度分三批物理拆分（User/ProviderCredential → Client → Domain/Proxy/Certificate），尚未开始实施。

## 已实现能力（摘要）

- QUIC 与 TCP+TLS 控制通道、认证、心跳、代理快照、最新会话路由。
- TCP / UDP / Web（Domain + Path）反向代理与 listener 协调。
- Domain 优先 Admin UI；HTTPS 按 Domain 选证，路径命中后访问激活（Cookie）。
- 托管证书（ACME Cloudflare DNS-01、Origin CA）、健康状态与失败保留；一证可服务多 Domain。
- Admin JWT 会话、GraphQL、同源 Admin UI。
- 可复现部署包与 daemon 运行文档。

## 已知缺口

- 配额、限速、普通用户自助、备份恢复、完整指标/告警。
- 正向代理；HTTPS 访问激活不支持逐设备撤销。
- 原生安装器与包管理器分发。
- 详见 [requirements/limits.md](requirements/limits.md)。

## 技术债

- `TestReconcileHTTPProxyCustomListenerWithoutRestart` 与 `TestReconcileHTTPSProxyCustomListenerWithoutRestart` 是预置的 flaky 测试：未修改的 HEAD 独立运行三次出现一次通过、两次 502 失败。该问题与 server runtime context Change 无关，单独跟踪，不阻塞本 Change 收尾。

## 下一步

1. 实施 [admin-facade-physical-split.md](changes/active/admin-facade-physical-split.md) 第一批（User + ProviderCredential 拆分），验证 `AuditRecorder` 共享依赖模式后再推进后续两批。
2. 生产运维：备份恢复、容量校验。
3. 部署含 Access activation 身份变更撤销、`proxies` 遗留列 DROP、`web` 流类型修复的版本。
4. 有代码变更时同步更新 requirements/architecture，并回写本日志验证结果。

## 最近验证

| 日期 | 范围 | 结果 | 说明 |
| --- | --- | --- | --- |
| 2026-07-16 | Domain Path 部署验收 | 通过 | Domain 证书展示/绑定与 1:n 行为可用 |
| 2026-07-16 | `go test` admin/store/adminapi/adminquery + UI cert 测试 | 通过 | GraphQL 嵌入字段、1:n 绑定 |
| 2026-07-15 | 文档链接与路径 | 通过 | 本地相对链接检查无失效路径 |
| 2026-07-23 | runtime context 架构变更 | 已完成 | `go build ./...`、相关包测和 `go test ./e2e -count=1` 通过；受控全量包测仅遇到 daemon 自定义 Web listener 的预置 flaky，已单独记录为技术债 |

建议验证入口：

- 单元/包测：`CGO_ENABLED=0 go test ./...`
- 跨进程：`go test ./e2e -count=1`
- 说明：[operations/milestone-one-e2e.md](operations/milestone-one-e2e.md)
