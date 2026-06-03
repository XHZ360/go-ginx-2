## ADDED Requirements

### Requirement: Dedicated client enrollment endpoint
系统 MUST 为客户端 join/enrollment token 兑换提供与 admin-ui/admin API 分离的客户端 enrollment 入口，并且该入口 MUST 只暴露客户端兑换所需的最小 HTTP 行为。

#### Scenario: Dedicated enrollment endpoint redeems valid token
- **WHEN** 客户端向专用 enrollment listener 的 `/api/client/enroll` 提交未使用、未过期且校验通过的 join token
- **THEN** 系统兑换该 token 并返回控制通道连接、服务端身份、信任材料、客户端 ID、客户端凭据、协议默认值和重连配置

#### Scenario: Dedicated enrollment endpoint rejects invalid token
- **WHEN** 客户端向专用 enrollment listener 提交已使用、过期、被篡改或 hash 不匹配的 join token
- **THEN** 系统拒绝兑换，并且 MUST NOT 返回客户端 credential 或控制通道受管配置

#### Scenario: Dedicated enrollment endpoint excludes admin resources
- **WHEN** 请求访问专用 enrollment listener 上的 admin-ui 路由、`/api/admin/*` 或其他非 `/api/client/enroll` 路径
- **THEN** 系统拒绝或返回未找到，而不是暴露管理前端、管理员会话或 GraphQL 管理 API

#### Scenario: Token pointing at admin listener is not redeemed
- **WHEN** 客户端 join token 中的 `enrollment_url` 指向 admin listener 上的 `/api/client/enroll`
- **THEN** 客户端 join 不能通过 admin listener 完成兑换，操作者必须重新生成指向专用 enrollment listener 或显式公开 enrollment URL 的 token

## MODIFIED Requirements

### Requirement: Join material default service address
系统 MUST 在生成客户端 join/enrollment 材料时，默认使用服务端配置、环境覆盖和启动阶段确认的服务域名或 IP 作为客户端连接服务端的地址来源，并且默认 `enrollment_url` MUST 使用客户端 enrollment 专用监听器的端口。显式输入 MUST 能覆盖该默认值。该默认行为 MUST 覆盖 Admin API、admin CLI 和 TUI 等所有受支持的 join 材料生成入口。

#### Scenario: Join material uses confirmed service address by default
- **WHEN** 已授权管理员通过任一受支持入口生成客户端 join/enrollment 材料，且请求未显式提供服务端控制通道地址或 enrollment URL
- **THEN** 系统把服务端配置、环境覆盖或启动时确认的默认服务域名或 IP 组合为 join 材料中的默认 `serverAddress` 和相关 TLS 地址，并把同一默认服务域名或 IP 与 `client_enrollment_listen` 端口组合为默认 `enrollment_url`

#### Scenario: Admin CLI and TUI use the same default source
- **WHEN** 操作者通过 `goginx-admin create-client-join`、`goginx-admin client-join-command` 或 `goginx-admin tui` 生成或查看 join 材料，且未显式覆盖 join 参数
- **THEN** 系统使用与 server 配置加载兼容的默认 join 参数解析结果，并使用该解析结果中的客户端 enrollment 专用监听端口生成默认 enrollment URL

#### Scenario: Explicit join address overrides confirmed default
- **WHEN** 已授权管理员生成客户端 join/enrollment 材料，并显式提供服务端地址、TLS 地址、服务端名称或 enrollment URL
- **THEN** 系统使用显式输入填充对应 join 材料字段，而不是强制使用启动时确认的默认值或客户端 enrollment 专用监听端口

#### Scenario: Join address default does not expose reusable secrets
- **WHEN** 系统记录或展示 join/enrollment 材料的默认服务地址来源、客户端 enrollment 监听地址或默认 enrollment URL
- **THEN** 日志、审计事件和非 secret UI 文案可以包含服务域名、IP、端口或 URL，但 MUST NOT 明文记录完整 join token、客户端 credential 或可重放 join secret
