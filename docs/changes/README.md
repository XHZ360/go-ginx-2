# 变更记录

变更过程用单文件记录，不再拆成 OpenSpec 的 proposal/design/tasks/specs。

完整生命周期、状态门槛和事实同步规则见 [../project/change-workflow.md](../project/change-workflow.md)。新建 Change 时使用 [change-template.md](change-template.md)。

## 目录约定

```text
changes/
  active/      进行中的变更（按需创建）
  completed/   已完成、仍可能参考的变更
  archive/     历史批次与过期续作笔记
```

## 当前记录

| 文档 | 说明 |
| --- | --- |
| [completed/http-path-routing-and-https-access-activation.md](completed/http-path-routing-and-https-access-activation.md) | HTTP 路径路由与 HTTPS 访问激活实施记录 |
| [archive/milestone-one-continuation.md](archive/milestone-one-continuation.md) | 里程碑一实施批次与续作笔记（历史） |

## 写法

单文件包含当前实现、目标、非目标、核心不变量、设计、迁移、实施步骤、验收条件、验证记录与结果。日常进度摘要写入 [../worklog.md](../worklog.md)。

关键规则：

- `active` 描述目标，不代表已实现。
- 完成前必须同步 requirements / architecture / operations 等长期文档。
- 被否决或被替代的 Change 移入 `archive/`，并链接后继文档。
- completed 文档发现模型错误时，新建 active Change 纠正，不改写历史掩盖偏差。
