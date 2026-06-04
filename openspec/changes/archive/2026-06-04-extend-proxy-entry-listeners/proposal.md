## Why

当前代理入口配置把 HTTP/HTTPS 固定在全局 `http_entry_listen` / `https_entry_listen`，TCP/UDP 也只能使用全局 `tcp_entry_host` 加每个代理的端口。管理员无法按代理选择暴露网络、端口和域名组合，且管理面更新代理后不能保证相关监听服务立即与有效配置保持一致。

本变更让四类反向代理都可以表达完整入口监听意图，并让运行时在代理创建、更新、启用、禁用或删除后及时启动或关闭对应服务，减少需要重启守护进程才能生效的运维成本。

## What Changes

- HTTP/HTTPS 代理入口支持独立的监听地址、监听端口和路由域名；当入口不同于全局配置默认监听时，运行时按需要启动额外 HTTP/HTTPS listener。
- TCP/UDP 代理支持配置监听地址，用于控制代理暴露在所有网络、本机回环或指定本机地址上。
- 监听器准入从仅按协议和端口判断，扩展为按协议、监听地址和端口判断，并正确处理 `0.0.0.0` / `::` 这类 wildcard 地址与具体地址之间的冲突。
- 管理 API/UI 暴露可选择的监听地址选项，代理表单使用选项选择监听地址，而不是要求管理员手动输入。
- 代理入口有效配置发生变化后，运行时必须及时 reconcile listener：新增需要的服务、关闭不再被任何启用代理使用的服务，并保持已有服务可继续路由同监听地址上的其他代理。
- 修复代理编辑与启用/禁用的错误反馈，使入口冲突和无效配置在管理 UI 内明确展示。
- 增强服务端和客户端连接生命周期日志，覆盖客户端连接建立、替换、断开、过期以及代理 listener 启停等关键事件。
- 更新文档与测试，验证 HTTP/HTTPS 自定义监听、TCP/UDP 监听地址、运行时热协调和日志基线。

## Capabilities

### New Capabilities

- 无。

### Modified Capabilities

- `reverse-proxy-runtime`: 扩展四类代理的入口监听语义，新增按有效配置热启动/热关闭 listener 的运行时要求。
- `admin-resource-management`: 扩展代理 CRUD/API/UI 合同，支持监听地址选项、HTTP/HTTPS 端口与域名配置，以及入口冲突错误反馈。
- `deployment-operations`: 更新配置、文档和运维诊断要求，说明默认监听和每代理自定义监听的组合行为。
- `observability-and-audit`: 增加连接生命周期和 listener 生命周期日志基线。

## Impact

- 领域模型和持久化：代理入口配置需要区分监听地址、监听端口和 HTTP Host / HTTPS SNI 域名；SQLite 索引与迁移需要适配新唯一性规则。
- 后端服务：admin command/query、GraphQL schema、CLI 参数语义和 listener admission 规则需要更新。
- 运行时：daemon 需要维护 TCP、UDP、HTTP、HTTPS listener registry，并在代理变更后执行增量 reconcile。
- 前端：代理创建/编辑表单、列表/详情展示、错误反馈和监听地址选项查询需要更新。
- 测试：需要覆盖持久化迁移、准入冲突、HTTP/HTTPS 多 listener、TCP/UDP bind host、UI 表单和端到端代理流量。
