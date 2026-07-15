# 部署与运维

部署、打包、验证与本地开发操作说明。

## 文档列表

| 文档 | 说明 |
| --- | --- |
| [daemon-runtime.md](daemon-runtime.md) | 守护进程运行、部署基线与故障排查 |
| [deploy-release.md](deploy-release.md) | Release 包 Linux/Windows 部署流程 |
| [runtime-configuration.md](runtime-configuration.md) | 托管路径、环境变量与显式配置 |
| [certificate-operations.md](certificate-operations.md) | ACME / Origin CA 签发、续期与撤销操作 |
| [admin-cli.md](admin-cli.md) | 管理 CLI / TUI 与代理种子命令 |
| [admin-api.md](admin-api.md) | 管理 API、会话与前端构建 |
| [docker-development.md](docker-development.md) | Docker Compose 本地开发环境 |
| [milestone-one-e2e.md](milestone-one-e2e.md) | 里程碑一可执行验证路径 |
| [domain-path-routing-migration.md](domain-path-routing-migration.md) | Domain + Path 路由数据迁移与回滚 |
| [examples/admin-seed-sqlite.md](examples/admin-seed-sqlite.md) | Admin CLI SQLite 种子示例 |

## 相关设计

- 证书与运行时失败语义：[../architecture/certificate-management.md](../architecture/certificate-management.md)
- 工程质量护栏：[../architecture/engineering-quality-guardrails.md](../architecture/engineering-quality-guardrails.md)
- 产品需求：[../requirements/README.md](../requirements/README.md)
