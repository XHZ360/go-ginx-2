## ADDED Requirements

### Requirement: Join material default service address
系统 MUST 在生成客户端 join/enrollment 材料时，默认使用服务端配置和启动阶段确认的服务域名或 IP 作为客户端连接服务端的地址来源，并且显式输入 MUST 能覆盖该默认值。

#### Scenario: Join material uses confirmed service address by default
- **WHEN** 已授权管理员生成客户端 join/enrollment 材料，且请求未显式提供服务端控制通道地址
- **THEN** 系统把服务端启动时确认的默认服务域名或 IP 组合为 join 材料中的默认 `serverAddress`、相关 TLS 地址和 enrollment URL 地址来源

#### Scenario: Explicit join address overrides confirmed default
- **WHEN** 已授权管理员生成客户端 join/enrollment 材料，并显式提供服务端地址、TLS 地址、服务端名称或 enrollment URL
- **THEN** 系统使用显式输入填充对应 join 材料字段，而不是强制使用启动时确认的默认值

#### Scenario: Join address default does not expose reusable secrets
- **WHEN** 系统记录或展示 join/enrollment 材料的默认服务地址来源
- **THEN** 日志、审计事件和非 secret UI 文案可以包含服务域名或 IP，但 MUST NOT 明文记录完整 join token、客户端 credential 或可重放 join secret
