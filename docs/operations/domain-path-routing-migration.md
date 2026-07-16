# Domain + Path 路由迁移与回滚

## 范围

将旧模型「父 Proxy + ProxyRoute」迁移为：

```text
Domain + DomainEntry + (Domain, PathPrefix) => Web Proxy
```

迁移在 SQLite 打开时自动执行（`migrateDomainPathRouting`），幂等。

## 升级前

1. 停止 daemon/admin 写入流量。
2. 备份 SQLite 与证书目录：

```bash
cp -a data/go-ginx.db data/go-ginx.db.pre-domain-path
cp -a data/certs data/certs.pre-domain-path
```

3. 记录当前 HTTP/HTTPS Proxy 与路径配置（可选导出 Admin GraphQL）。

## 升级过程

1. 部署包含 Domain 模型的版本。
2. 启动服务；打开数据库时自动：
   - 创建 `domains` / `domain_entries`
   - 为 `proxies` 增加 `domain_id` / `path_prefix` 等列
   - 将 HTTP/HTTPS Proxy 与 `proxy_routes` 转换为 Domain + Web Proxy
   - 证书绑定迁到 Domain
   - 访问认证版本递增并撤销旧 Token/Cookie
3. 校验：

```bash
CGO_ENABLED=0 go test ./internal/store/sqlite ./internal/proxy/http ./internal/proxy/https ./e2e -count=1
```

4. 在 Admin UI 检查：
   - Domains 列表有期望主机
   - Domain 详情有 HTTP/HTTPS entry 与路径 Proxy
   - Certificates 显示 Bound domain

## 迁移映射

| 旧数据 | 新数据 |
| --- | --- |
| HTTP/HTTPS `EntryHost` | `domains.host` |
| HTTP/HTTPS listener | `domain_entries` |
| 父 Proxy 默认 `/` | Web Proxy `path_prefix=/`（保留原 Proxy ID） |
| `proxy_routes` 行 | 独立 Web Proxy（常用原 route ID） |
| `proxies.certificate_id` | `domains.certificate_id` |
| 父 Proxy 历史统计 | `/` Proxy 上 `stats_legacy_aggregate=1` |

## 冲突处理

以下情况迁移停止并返回错误（不静默选赢家）：

- 同一 host 归属不同用户
- 同一 Domain+Path 映射到不同 Client/target/改写
- 同一 Domain 多个不同证书绑定

处理：人工拆分/删除冲突配置后，用备份库重试。

## 访问认证与统计语义

- 迁移后旧 Cookie/Token 全部失效，需重新生成激活链接。
- `/` Proxy 历史统计可能含旧全部路径流量；UI 应标注 legacy aggregate。
- 新路径 Proxy 从零计数。

## 回滚

1. 停止新版本。
2. 恢复备份：

```bash
cp -a data/go-ginx.db.pre-domain-path data/go-ginx.db
cp -a data/certs.pre-domain-path data/certs
```

3. 启动旧版本二进制。

注意：新版本写入的 Domain/Web Proxy 数据不会反向合并到旧 `proxy_routes` 模型；回滚只能回到备份点。

## 清理阶段（确认稳定后）

已完成：

- 删除空的 `proxy_routes` 表（迁移 flag 完成后 `DROP TABLE`）与 `ProxyRouteRepository`
- Web Proxy 更新路径不再写入 `entry_host` / `certificate_id`（权威在 Domain）
- 管理面已移除 route mutations / ProxyRoute GraphQL

仍可选：

- 物理删除 `proxies` 上 Web 遗留列（需 SQLite rebuild）
- 文档与 Admin UI 中移除 legacy HTTP/HTTPS 类型提示

## 相关文档

- Change：`docs/changes/active/domain-path-proxy-routing.md`
- 决策：`docs/decisions/domain-path-proxy-routing.md`
- 架构：`docs/architecture/reverse-proxy.md`
