# 开发与验证流程

## 日常开发

1. 以代码、测试和 `docs/` 为事实来源；不再维护 OpenSpec proposal/design/spec/tasks 模板。
2. 改动前先定位相关主题文档（`requirements/` 或 `architecture/`），确认当前行为边界。
3. 实现后更新受影响的文档；普通进度写入 [../worklog.md](../worklog.md)。
4. 影响长期方向的选择写入 `decisions/` 单文件。
5. 有明确变更过程时在 `changes/` 记录，完成后归档。

## 验证

按改动范围选择最小验证：

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

可执行验证路径见 [../operations/milestone-one-e2e.md](../operations/milestone-one-e2e.md)。

## 本地开发环境

Docker Compose 开发环境见 [../operations/docker-development.md](../operations/docker-development.md)。

## 提交与边界

- 不提交令牌、凭据、私钥、日志、本地数据或生成目录。
- 重点排除：`.tmp/`、`dist/`、`data/`、`admin-ui/node_modules/`、`admin-ui/dist/`、`*.log`、`*.local*`、`*.private*`。
- 证书首次创建失败且无可用材料时，不得让坏状态进入运行时；续期/轮换失败时保留旧可用材料。
- 敏感数据不得进入 API、日志或测试快照。
- 未经明确要求，不操作远端服务或生产数据。

## 文档检查

文档与代码一并变更时：

1. 检索旧路径与失效链接。
2. 确认一个事实只有一个主来源。
3. 入口地图 [../README.md](../README.md) 仍能定位到目标文档。
