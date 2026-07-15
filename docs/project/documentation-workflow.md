# 文档工作流

## 目标

移除 OpenSpec 后，长期信息按信息类型归档到 `docs/`，而不是按工具模板拆分。产品行为以代码、测试和普通 Markdown 为准。

## 目录规则

| 目录 | 写入内容 | 不写入内容 |
| --- | --- | --- |
| `project/` | 项目目标、协作流程、文档规则 | 单次变更细节 |
| `requirements/` | 产品能力、业务流程、验收口径、UI 设计 | 实现细节与部署步骤 |
| `architecture/` | 运行时、协议、数据、集成设计 | 操作手册式命令清单 |
| `decisions/` | 影响长期方向的决策（一事一文） | 临时讨论与日常进度 |
| `changes/` | 进行中/已完成变更过程 | 永久产品规格 |
| `operations/` | 部署、打包、验证、运维 | 产品需求正文 |
| `references/` | 历史迁移、兼容说明、外部资料 | 当前唯一事实来源 |
| `worklog.md` | 当前状态、下一步、最近验证 | 完整设计正文 |

## 命名

- 使用 kebab-case：`system-architecture.md`、`daemon-runtime.md`。
- 避免 `misc.md`、`note.md`、`temp.md`、`wip.md`。
- 决策文件建议：`YYYYMMDD-short-title.md` 或稳定主题名。

## 单一事实来源

- 同一行为只在一个主文档维护；其他位置用相对链接引用。
- README 只保留面向使用者的摘要，并链接到 `docs/` 主文档。
- UI 页面设计在 `requirements/admin-ui/`；运行时语义在 `architecture/`；操作步骤在 `operations/`。

## 变更记录

- 进行中的变更：`changes/active/<change-name>.md`（按需建立）。
- 完成后：移到 `changes/completed/` 或摘要后归档到 `changes/archive/`。
- 单文件记录背景、范围、结果和验证；不再拆 proposal/design/tasks/specs。

## 迁移与兼容资料

- OpenSpec 迁移说明：[../references/openspec-migration.md](../references/openspec-migration.md)
- 证书绑定迁移：[../references/certificate-binding-migration.md](../references/certificate-binding-migration.md)
- Git 历史是旧方案和实施记录的存档，不必再复制进 `docs/`。

## 检查清单

文档变更合并前：

- [ ] 路径落在正确目录职责内
- [ ] 无失效相对链接
- [ ] 无旧路径残留（如已废弃的扁平 `docs/*.md` 引用）
- [ ] 入口地图仍可定位
- [ ] 与代码/测试表达的行为一致
