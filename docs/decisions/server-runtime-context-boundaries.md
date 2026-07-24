# 服务端运行时上下文边界

## 状态

已采纳，已实现（2026-07-23）。

## 背景

`admin.Service`、`adminapi.Server` 和 `control` 曾同时承接业务命令、协议适配和运行时状态，导致新能力容易绕过业务边界，且让 server-local virtual client 无法复用稳定端口。管理查询也曾直接依赖可变的 `session.Manager`。

## 决策

- 管理适配器只通过按领域划分的 `admin.CommandFacades` 执行业务命令；不得依赖 `admin.Service` 的实现字段。适配器需要的非命令入口选项（当前为 `ProxyEntryDefaults`）必须作为独立装配配置注入。
- `adminquery` 只能通过只读 session 快照接口读取运行时状态；session 的注册、关闭和流打开必须经由运行时端口。
- `control` 只负责协议和连接生命周期。认证和连接关闭可直连 store 同步 `ClientStatus`，这是唯一已登记的业务 facade 例外；任何新增例外必须先更新本决策和系统架构。
- 系统 client 和本机代理能力通过明确端口接入。`SystemClientFacade` 是该身份边界的唯一导出名；`ListenerReconciler` 是 listener 生命周期端口的规范名称，`ProxyListenerReconciler` 仅作为兼容别名保留。
- `daemon` 只负责这些依赖的装配和生命周期，不承载管理业务规则。

## 后果

- 现有 `admin.Service` 曾是阶段 1 的实现载体，调用方已只见六组命令接口；物理拆分已按 [Admin facade 物理拆分 Change](../changes/completed/admin-facade-physical-split.md) 完成，并采用显式共享依赖设计。
- API/GraphQL、control wire protocol 和数据库 schema 均不因此改变。
- 新增 session、listener、网络连接或本机代理能力时，必须首先复用或扩展既有端口，而非向 adapter、daemon 或 control 堆叠业务特例。

## 相关文档

- 当前架构：[../architecture/system-architecture.md](../architecture/system-architecture.md)
- 已完成 Change：[../changes/completed/server-runtime-context-architecture.md](../changes/completed/server-runtime-context-architecture.md)
- 后续 feature：[../changes/completed/server-local-virtual-client.md](../changes/completed/server-local-virtual-client.md)
