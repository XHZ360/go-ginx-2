## ADDED Requirements

### Requirement: Per-proxy listener operations documentation
系统 MUST 为每代理入口监听配置提供部署和故障排查说明，帮助操作者理解默认监听、额外监听和端口绑定行为。

#### Scenario: Documentation explains default and per-proxy listeners
- **WHEN** 文档描述 HTTP、HTTPS、TCP 或 UDP 代理入口
- **THEN** 文档说明全局默认监听配置如何作为旧记录和空配置的 fallback，以及每代理监听地址和端口如何创建额外 listener

#### Scenario: Documentation explains hot listener reconciliation
- **WHEN** 文档描述管理员创建、更新、启用、禁用或删除代理
- **THEN** 文档说明运行时会及时启动或关闭对应 listener，并指出 listener 启动失败会作为管理操作错误暴露

#### Scenario: Troubleshooting includes listener bind conflicts
- **WHEN** 操作者遇到端口占用、wildcard 地址冲突、低端口权限或 HTTP/HTTPS 域名路由失败
- **THEN** 当前文档提供故障排查指导，帮助确认实际监听地址、端口、域名和代理状态

#### Scenario: Startup diagnostics include dynamic proxy listeners
- **WHEN** 服务端启动完成
- **THEN** 启动诊断展示已启动的 TCP、UDP、HTTP 和 HTTPS proxy listener 数量，使操作者能够确认自定义入口已参与运行
