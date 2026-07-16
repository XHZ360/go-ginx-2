# AGENTS.md

适用于整个仓库。

## 项目

- Go 模块：`github.com/simp-frp/go-ginx-2`，Go `1.26.0`，默认 `CGO_ENABLED=0`。
- 后端入口在 `cmd/`，核心代码在 `internal/`，端到端测试在 `e2e/`。
- `admin-ui/` 使用 React、Vite、TypeScript、Ant Design 和 `pnpm@10.33.2`。
- 当前文档在 `docs/`（入口 [docs/README.md](docs/README.md)）；产品行为以代码、测试和普通 Markdown 文档为准。
- 文档默认使用简体中文，代码和技术术语可保留英文。
- 文档按信息类型归档：`project/`、`requirements/`、`architecture/`、`operations/`、`changes/`、`references/`；协作流程见 `docs/project/workflow.md`，Change 生命周期见 `docs/project/change-workflow.md`。
- 当前线上环境信息见 [docs/operations/production-environment.local.md](docs/operations/production-environment.local.md)；该本地文档可能包含敏感信息，不得提交。

## 验证

如果需要验证，按改动范围选择最小验证：

```powershell
$env:CGO_ENABLED="0"
go test ./internal/<相关包>
go test ./...                 # 跨模块改动
go test ./e2e -count=1        # 跨进程行为

cd admin-ui
pnpm test
pnpm build                    # 构建链路或类型改动
pnpm graphql:refresh          # GraphQL schema/operation 改动
```

## 项目边界

- 不提交令牌、凭据、私钥、日志、本地数据或生成目录；重点排除 `.tmp/`、`dist/`、`data/`、`admin-ui/node_modules/`、`admin-ui/dist/`、`*.log`、`*.local*`、`*.private*`。
- 证书首次创建失败且无可用材料时，不得让坏状态进入运行时；续期或轮换失败时保留旧的可用材料。敏感数据不得进入 API、日志或测试快照。
- 未经明确要求，不操作远端服务或生产数据；清理生产数据前必须备份。
- 涉及证书、ACME、Cloudflare 或远端资源生命周期时，参考 `docs/architecture/engineering-quality-guardrails.md` 和 `docs/operations/daemon-runtime.md`。
