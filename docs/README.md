# 文档入口地图

本文档是项目文档的检索入口。移除 OpenSpec 后，所有长期信息都收敛到 `docs/`，按信息类型归档，而不是按工具模板归档。

## 快速定位

| 想找什么 | 入口 |
| --- | --- |
| 项目目标、产品形态、技术栈约束 | [project/overview.md](project/overview.md) |
| 日常开发、验证、提交流程 | [project/workflow.md](project/workflow.md) |
| 文档目录规则、迁移流程、记录要求 | [project/documentation-workflow.md](project/documentation-workflow.md) |
| 当前进展、下一步、最近验证结果 | [worklog.md](worklog.md) |
| 产品需求和业务流程 | [requirements/README.md](requirements/README.md) |
| 运行时、协议、数据与集成设计 | [architecture/README.md](architecture/README.md) |
| 重要决策及其影响 | [decisions/](decisions/) |
| 正在做、已完成、历史归档的变更 | [changes/README.md](changes/README.md) |
| 部署、打包、验收和运维说明 | [operations/README.md](operations/README.md) |
| 外部格式、历史兼容和参考资料 | [references/](references/) |

## 目录职责

```text
docs/
  project/       项目级长期上下文和协作流程
  requirements/  产品需求、业务能力、验收口径
  architecture/  技术架构、运行时、数据、集成设计
  decisions/     重要设计决策，一事一文
  changes/       变更记录，按 active/completed/archive 管理
  operations/    部署、打包、验证、运维和验收
  references/    历史资料、外部格式、兼容说明
  worklog.md     当前状态和下一步
```

## 维护原则

- 一个事实只保留一个主来源，其他文档用链接引用。
- 文件名使用 kebab-case，避免 `misc.md`、`note.md`、`temp.md` 这类低信号名称。
- 普通上下文进入 `worklog.md` 或对应主题文档；只有影响长期方向的选择才进入 `decisions/`。
- 变更过程用单文件记录，不再拆成 OpenSpec 的 proposal/design/tasks/specs 多份模板。
- 文档变更和代码变更一样需要经过基本检索检查，确认没有失效链接或残留路径。
