# 工作日志

记录当前状态与下一步。详细历史批次见 [changes/archive/milestone-one-continuation.md](changes/archive/milestone-one-continuation.md)。

## 元信息

| 项 | 值 |
| --- | --- |
| 最后更新 | 2026-07-15 |
| 基线提交 | `02d9aed`（Merge branch `opencode/silent-comet`） |
| 验证状态 | 本次文档整改未执行全量测试 |

## 当前状态

- 里程碑一运行时与首个部署基线已具备：控制通道、反向代理、Admin API/UI、证书管理、SQLite、Linux systemd / Windows 服务发布包。
- 文档按信息类型收敛到 `docs/`（project / requirements / architecture / operations / changes / references）。
- OpenSpec 已移除；有效内容已并入普通 Markdown。
- 文档整改已落地：收敛单一事实来源、补齐 requirements 层、收缩根 README 长文。
- active Change [domain-path-proxy-routing.md](changes/active/domain-path-proxy-routing.md)：Domain 优先 Admin UI 已落地（列表/详情、Proxy 选 Domain+Path、证书展示 Domain）；后端 Domain GraphQL/API 已通。剩余：移除 ProxyRoute GraphQL 兼容层、operations 迁移文档、旧字段清理。

## 已实现能力（摘要）

- QUIC 与 TCP+TLS 控制通道、认证、心跳、代理快照、最新会话路由。
- TCP / UDP / HTTP / HTTPS 反向代理与 listener 协调。
- HTTP/HTTPS 路径前缀路由与 HTTPS 访问激活（Cookie）。
- 托管证书（ACME Cloudflare DNS-01、Origin CA）、健康状态与失败保留。
- Admin JWT 会话、GraphQL、同源 Admin UI。
- 可复现部署包与 daemon 运行文档。

## 已知缺口

- 配额、限速、普通用户自助、备份恢复、完整指标/告警。
- 正向代理；HTTPS 访问激活不支持逐设备撤销。
- 原生安装器与包管理器分发。
- 详见 [requirements/limits.md](requirements/limits.md)。

## 下一步

1. 移除 ProxyRoute GraphQL/API 兼容层，并补 operations 迁移/回滚说明。
2. 生产运维：备份恢复、容量校验。
3. 有代码变更时同步更新 requirements/architecture，并回写本日志验证结果。

## 阻塞问题关闭记录（2026-07-15）

- HTTP 认证：有可用 HTTPS entry 时 `308`，否则 `403`。
- 历史统计：`/` Proxy 保留 legacy aggregate 并 UI 标注；子路径 Proxy 从零计数。

## 最近验证

| 日期 | 范围 | 结果 | 说明 |
| --- | --- | --- | --- |
| 2026-07-15 | 文档链接与路径 | 通过 | 本地相对链接检查无失效路径 |
| 2026-07-15 | `go test ./...` / e2e | 未执行 | 本次仅文档整改 |

建议验证入口：

- 单元/包测：`CGO_ENABLED=0 go test ./...`
- 跨进程：`go test ./e2e -count=1`
- 说明：[operations/milestone-one-e2e.md](operations/milestone-one-e2e.md)
