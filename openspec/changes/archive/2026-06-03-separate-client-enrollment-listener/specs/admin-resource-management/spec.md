## ADDED Requirements

### Requirement: Admin and enrollment listener separation
系统 MUST 把管理员浏览器/API 入口与客户端 enrollment 入口作为不同的监听职责处理，使公开客户端 join 端口不会同时公开 admin-ui 或管理员 API。

#### Scenario: Admin listener keeps management surface
- **WHEN** admin listener 启用
- **THEN** admin listener 继续服务 admin-ui、`/api/admin/*`、管理员会话和受支持深链，并保持管理访问的受保护传输要求
- **AND** admin listener MUST NOT 服务 `/api/client/enroll`

#### Scenario: Enrollment listener exposes no management surface
- **WHEN** 客户端 enrollment listener 监听所有地址
- **THEN** 该 listener 只服务客户端 token 兑换所需的 `/api/client/enroll`，并且 MUST NOT 服务 admin-ui、`/api/admin/*`、管理员登录、管理员会话或 GraphQL 管理 API

#### Scenario: Join default does not require exposing admin listener
- **WHEN** 管理员生成默认 join token 且 admin listener 绑定在本机回环或受外网访问限制
- **THEN** token 中默认 `enrollment_url` 指向客户端 enrollment listener，而不是要求客户端访问 admin listener

#### Scenario: Old admin enrollment route is removed
- **WHEN** 请求访问 admin listener 上的 `/api/client/enroll`
- **THEN** 系统返回未找到或其他非兑换响应，而不是消费 join token 或返回客户端 enrollment 配置

## MODIFIED Requirements

### Requirement: Proxy listener-admission semantics
系统 MUST 通过共享 ListenerClaim 模型在活跃运行时监听空间上评估 TCP 和 UDP 代理 socket 准入，并把活跃监听冲突作为显式合同行为暴露。

#### Scenario: ListenerClaim conflict rejects create, update, or enable operations
- **WHEN** 已认证管理员创建、更新或启用 TCP/UDP 代理，且请求的活跃监听器与现有活跃 claim 在 V1 `same network + same port` 规则下冲突
- **THEN** 操作以 `ENTRY_CONFLICT` 语义被拒绝，而不是落为通用持久化失败

#### Scenario: Active ListenerClaim set includes configured static listeners
- **WHEN** 为 TCP 或 UDP 代理活动评估监听器准入
- **THEN** 活跃 ListenerClaim 集合包含来自 `control_quic_listen`、`control_tls_listen`、`client_enrollment_listen`、`admin_listen`、`http_entry_listen` 和 `https_entry_listen` 的已配置静态监听器，只要这些监听器参与运行时绑定

#### Scenario: Active ListenerClaim set includes enabled TCP and UDP proxies
- **WHEN** 为 TCP 或 UDP 代理活动评估监听器准入
- **THEN** 活跃 ListenerClaim 集合包含会占用活跃运行时监听器的已启用 TCP 代理和已启用 UDP 代理

#### Scenario: Disabled proxies do not participate in active ListenerClaim admission
- **WHEN** 为 TCP 或 UDP 代理活动评估监听器准入
- **THEN** 禁用代理不参与用于冲突检测的活跃 claim 集合
