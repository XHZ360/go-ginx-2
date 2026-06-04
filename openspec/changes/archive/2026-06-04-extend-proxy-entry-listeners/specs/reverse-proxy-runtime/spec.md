## ADDED Requirements

### Requirement: Per-proxy entry listener configuration
系统 MUST 为 TCP、UDP、HTTP 和 HTTPS 反向代理支持按代理配置有效入口监听地址与端口，并为 HTTP/HTTPS 区分监听地址和路由域名。

#### Scenario: TCP proxy binds configured host and port
- **WHEN** 已启用 TCP 代理配置了入口监听地址和入口端口
- **THEN** 服务端在该监听地址和端口上启动 TCP 代理 listener，并把流量转发到该代理的客户端本地目标

#### Scenario: UDP proxy binds configured host and port
- **WHEN** 已启用 UDP 代理配置了入口监听地址和入口端口
- **THEN** 服务端在该监听地址和端口上启动 UDP 代理 listener，并把 UDP 流量转发到该代理的客户端本地目标

#### Scenario: HTTP proxy routes by domain on configured listener
- **WHEN** 已启用 HTTP 代理配置了入口监听地址、入口端口和路由域名
- **THEN** 服务端在该监听地址和端口上的 HTTP listener 按请求 `Host` 匹配该域名，并把请求转发到该代理的客户端本地 HTTP 目标

#### Scenario: HTTPS proxy routes by SNI on configured listener
- **WHEN** 已启用 HTTPS 代理配置了入口监听地址、入口端口和 SNI 域名
- **THEN** 服务端在该监听地址和端口上的 HTTPS listener 按 TLS ClientHello SNI 匹配该域名，并执行透传或 TLS 终止转发

#### Scenario: Existing proxy records use default listeners
- **WHEN** 已启用代理记录缺少新的监听地址或 HTTP/HTTPS 入口端口字段
- **THEN** 服务端使用当前 server 配置中的默认监听地址和端口解释该代理，而不是要求操作者迁移后才能继续服务

### Requirement: Runtime listener reconciliation
服务端运行时 MUST 在代理入口有效监听配置变化后及时协调实际运行的 listener 服务，使运行状态与已启用代理集合保持一致。

#### Scenario: Create or enable starts required listener
- **WHEN** 管理员创建或启用代理，且该代理的有效入口监听地址尚未运行对应协议 listener
- **THEN** 服务端在管理操作完成前启动所需 listener，使该代理无需重启服务端即可接收外部流量

#### Scenario: Update moves listener
- **WHEN** 管理员更新已启用代理的有效入口监听地址、端口或 HTTP/HTTPS 路由域名
- **THEN** 服务端及时让新有效入口可用，并在旧入口不再被任何已启用代理使用时关闭旧 listener

#### Scenario: Disable or delete closes unused listener
- **WHEN** 管理员禁用或删除代理，且该代理原有效入口 listener 不再被任何已启用代理使用
- **THEN** 服务端及时关闭该 listener，而不是让不再需要的服务继续监听

#### Scenario: Shared HTTP listener remains active
- **WHEN** 多个 HTTP 代理共享同一监听地址和端口，且其中一个代理被禁用或删除
- **THEN** 服务端保持该 HTTP listener 运行，以继续服务同监听地址上的其他已启用 HTTP 代理

#### Scenario: Reconcile failure is visible
- **WHEN** 代理管理操作要求启动新的 listener 但实际绑定失败
- **THEN** 管理操作返回可消费的失败语义，并且系统不得静默声明代理入口已经可用
