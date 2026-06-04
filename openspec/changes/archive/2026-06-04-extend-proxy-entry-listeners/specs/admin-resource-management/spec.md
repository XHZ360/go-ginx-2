## ADDED Requirements

### Requirement: Administrator proxy entry configuration
系统 MUST 在管理员 API 和专用前端中支持为当前反向代理类型配置入口监听地址、入口端口和适用的路由域名，并提供可操作的错误反馈。

#### Scenario: HTTP create includes listener and domain
- **WHEN** 已认证管理员创建 HTTP 代理
- **THEN** 管理面允许提交监听地址、入口端口、HTTP Host 域名、本地目标主机和本地目标端口

#### Scenario: HTTPS create includes listener and SNI domain
- **WHEN** 已认证管理员创建 HTTPS 代理
- **THEN** 管理面允许提交监听地址、入口端口、SNI 域名、本地目标主机、本地目标端口以及可选证书文件和私钥文件

#### Scenario: TCP and UDP create include listener host
- **WHEN** 已认证管理员创建 TCP 或 UDP 代理
- **THEN** 管理面允许提交监听地址、入口端口、本地目标主机和本地目标端口

#### Scenario: Proxy edit validates type-specific entry fields
- **WHEN** 已认证管理员更新现有代理
- **THEN** 系统按该代理类型校验必填入口字段，并拒绝会导致该代理无法路由或无法监听的配置

#### Scenario: Listener host options are provided
- **WHEN** 专用前端渲染代理创建或编辑表单
- **THEN** 它从管理员 API 获取可选择的监听地址选项，并使用选择器提交所选监听地址

#### Scenario: Proxy lifecycle errors remain visible
- **WHEN** 创建、更新、启用、禁用或删除代理返回入口冲突、监听启动失败、校验失败或资源不存在错误
- **THEN** 专用前端在当前代理列表、详情或表单动作 surface 中展示该错误，并保留当前已加载内容

## MODIFIED Requirements

### Requirement: Proxy listener-admission semantics
系统 MUST 通过共享 ListenerClaim 模型在活跃运行时监听空间上评估 TCP、UDP、HTTP 和 HTTPS 代理 socket 准入，并把活跃监听冲突作为显式合同行为暴露。

#### Scenario: ListenerClaim conflict rejects create, update, or enable operations
- **WHEN** 已认证管理员创建、更新或启用代理，且请求的活跃监听器与现有活跃 claim 在 `same network + conflicting bind host + same port` 规则下冲突
- **THEN** 操作以 `ENTRY_CONFLICT` 或等价可消费冲突语义被拒绝，而不是落为通用持久化失败

#### Scenario: Active ListenerClaim set includes configured static listeners
- **WHEN** 为代理活动评估监听器准入
- **THEN** 活跃 ListenerClaim 集合包含来自 `control_quic_listen`、`control_tls_listen`、`client_enrollment_listen`、`admin_listen`、`http_entry_listen` 和 `https_entry_listen` 的已配置静态监听器，只要这些监听器参与运行时绑定

#### Scenario: Active ListenerClaim set includes enabled proxy listeners
- **WHEN** 为代理活动评估监听器准入
- **THEN** 活跃 ListenerClaim 集合包含会占用活跃运行时监听器的已启用 TCP、UDP、HTTP 和 HTTPS 代理有效监听地址

#### Scenario: Wildcard bind host conflicts with concrete host
- **WHEN** 一个活跃 claim 监听所有地址，另一个请求在同协议同端口监听某个具体地址
- **THEN** 系统把二者视为冲突，避免运行时绑定失败

#### Scenario: Disabled proxies do not participate in active ListenerClaim admission
- **WHEN** 为代理活动评估监听器准入
- **THEN** 禁用代理不参与用于冲突检测的活跃 claim 集合
