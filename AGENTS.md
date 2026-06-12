# AGENTS.md

本文件给进入本仓库的自动化 agent 使用。适用范围为仓库根目录及其所有子目录，除非更深层目录另有 `AGENTS.md` 覆盖。

## 项目概览

- 项目：`go-ginx-2`，Go 模块为 `github.com/simp-frp/go-ginx-2`。
- 后端：Go `1.25.0`，默认使用 cgo-free SQLite。
- 前端：`admin-ui/`，React + Vite + TypeScript，包管理器为 `pnpm@10.33.2`。
- 主要运行时：`goginx-server`、`goginx-client`、`goginx-admin`。
- 主要能力：控制面、TCP/UDP/HTTP/HTTPS 反向代理、Admin API/UI、证书管理、SQLite 持久化、部署包。

## 仓库结构

- `cmd/`：服务端、客户端、管理 CLI 和辅助命令入口。
- `internal/`：核心后端实现，包括 admin、adminapi、certmanager、control、daemon、proxy、store 等。
- `admin-ui/`：管理后台前端源码、GraphQL 生成代码和前端测试。
- `deploy/`：部署模板。
- `docs/`：运行、测试、Admin UI、工程质量和示例文档。
- `e2e/`：跨进程或部署形态的端到端测试。
- `openspec/`：规格和变更记录。

## 常用验证命令

后端完整验证：

```powershell
$env:CGO_ENABLED="0"
go test ./...
```

按模块验证常用命令：

```powershell
$env:CGO_ENABLED="0"
go test ./internal/admin ./internal/adminquery ./internal/adminapi
go test ./internal/certmanager
go test ./internal/daemon
go test ./internal/store/sqlite
go test ./e2e -count=1
```

前端验证：

```powershell
cd admin-ui
corepack enable
pnpm test
pnpm build
```

GraphQL schema 或 operation 改动后：

```powershell
cd admin-ui
pnpm graphql:refresh
```

## 开发约定

- 优先遵循现有包结构、错误处理风格、测试风格和文档语言。
- 搜索文件或内容优先使用 `rg` 或 `rg --files`。
- 不要回滚或覆盖与当前任务无关的脏工作区改动。
- 手工编辑文件时使用补丁方式，保持变更聚焦。
- 文档主要使用中文；已有英文文档可保持原语言风格。
- 不要把本地部署产物、日志、数据库、令牌、私钥或临时文件加入提交。
- `.tmp/`、`dist/`、`data/`、`admin-ui/node_modules/`、`admin-ui/dist/`、`*.log`、`*.local*`、`*.private*` 默认视为非源码产物或敏感文件。

## Admin UI 与 GraphQL

- 后端 GraphQL schema 改动后，应刷新 `admin-ui/src/graphql/schema.json` 和生成类型。
- 前端页面改动应补充或更新 `admin-ui/src/test/` 中的行为测试。
- UI 应展示可操作的错误和生命周期状态，不要只暴露泛化的服务器错误。
- 管理后台使用 Ant Design，新增控件应尽量沿用现有页面布局和交互模式。

## 外部服务与生命周期类改动

涉及 Cloudflare、ACME、证书、密钥、远端资源创建/删除/轮换等逻辑时，必须参考：

- `docs/engineering-quality-guardrails.md`
- `docs/daemon-runtime.md`
- `docs/milestone-one-e2e.md`

特别注意：

- 外部提供方错误应映射成调用方可消费的错误，而不是默认变成 `internal server error`。
- 首次创建失败和已有材料续期、轮换失败要分开处理和测试。
- 首次创建失败且没有可用材料时，应清理空占位，或确保坏状态不会被运行时选中。
- 已有可用证书续期或轮换失败时，应保留旧材料继续服务，并记录脱敏后的 `lastError`。
- 私钥、令牌、完整敏感请求/响应体不得进入 API 响应、日志或测试快照。

## 部署与远端环境

- 不要在未明确要求时改动远端部署目录或运行中服务。
- 远端修复应先说明构建、上传、替换、重启和 smoke check 步骤。
- 部署后至少确认服务状态、最近日志、Admin API/UI 基本可用性和本次修复的关键路径。
- 清理生产数据前必须先备份，并记录备份路径。

## 提交前检查清单

- 相关 Go 测试是否已运行，或已说明为什么未运行。
- 涉及前端时，`pnpm test` 或 `pnpm build` 是否已运行，或已说明原因。
- 涉及 GraphQL 时，schema 和 generated 文件是否同步。
- 涉及外部服务时，成功、失败、补偿、错误映射和敏感信息保护是否都有测试或说明。
- README 或 `docs/` 是否需要更新。
