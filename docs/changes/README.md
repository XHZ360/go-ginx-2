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

### Active Changes

| 文档 | 说明 |
| --- | --- |
| [active/admin-facade-physical-split.md](active/admin-facade-physical-split.md) | Admin 业务 facade 物理拆分（Runtime Context 架构 Change 阶段 2） |
| [active/server-local-virtual-client.md](active/server-local-virtual-client.md) | Server 本机虚拟 client、白名单与管理员专属本机代理（依赖架构 Change） |

### Completed Changes

| 文档 | 说明 |
| --- | --- |
| [completed/acme-certificate-readiness-ux.md](completed/acme-certificate-readiness-ux.md) | ACME DNS-01 创建前置条件可见性与失败诊断改进（已完成） |
| [completed/domain-path-proxy-routing.md](completed/domain-path-proxy-routing.md) | Domain + Path => Proxy 路由模型改造（已完成） |
| [completed/http-path-routing-and-https-access-activation.md](completed/http-path-routing-and-https-access-activation.md) | HTTP 路径路由与 HTTPS 访问激活（旧模型实施记录） |
| [completed/server-runtime-context-architecture.md](completed/server-runtime-context-architecture.md) | Server Runtime Context、业务 facade 与依赖边界重整（已完成） |
| [archive/milestone-one-continuation.md](archive/milestone-one-continuation.md) | 里程碑一实施批次与续作笔记（历史） |

## 写法

单文件包含当前实现、目标、非目标、核心不变量、设计、迁移、实施步骤、验收条件、验证记录与结果。日常进度摘要写入 [../worklog.md](../worklog.md)。

关键规则：

- `active` 描述目标，不代表已实现。
- 完成前必须同步 requirements / architecture / operations 等长期文档。
- 被否决或被替代的 Change 移入 `archive/`，并链接后继文档。
- completed 文档发现模型错误时，新建 active Change 纠正，不改写历史掩盖偏差。
