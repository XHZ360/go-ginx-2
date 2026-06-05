## ADDED Requirements

### Requirement: Default HTTP target header rewrite
系统 MUST 在 HTTP 类反向代理转发到客户端本地 HTTP 目标前，默认把目标可见的请求 `Host` 改写为配置的目标地址，并在安全可解析时改写已有 `Origin`。该行为 MUST 适用于 HTTP 反向代理和 HTTPS TLS 终止后的 HTTP 转发；HTTPS SNI 透传作为加密 TCP 字节流 MUST NOT 声明或尝试改写 HTTP 请求头。

#### Scenario: HTTP proxy rewrites Host to target authority
- **WHEN** HTTP 代理按公网请求 `Host` 匹配已启用代理，并把请求转发到配置的本地 HTTP 目标
- **THEN** 本地目标接收到的 HTTP `Host` MUST 是该代理的 `targetHost:targetPort`

#### Scenario: HTTP proxy rewrites parseable Origin
- **WHEN** HTTP 代理请求包含可解析的 HTTP 或 HTTPS `Origin`
- **THEN** 本地目标接收到的 `Origin` MUST 使用目标 HTTP origin `http://targetHost:targetPort`

#### Scenario: HTTP proxy preserves missing or special Origin
- **WHEN** HTTP 代理请求没有 `Origin`，或 `Origin` 为 `null`、空值、非 HTTP scheme、不可解析值
- **THEN** 运行时 MUST NOT 新增 `Origin`，且 MUST 保留特殊或不可解析的原始 `Origin` 值

#### Scenario: HTTPS termination uses HTTP header rewrite
- **WHEN** HTTPS 代理选择证书并终止公网 TLS，然后把解密后的 HTTP 请求转发到客户端本地 HTTP 目标
- **THEN** 本地目标接收到的 `Host` 和可解析 `Origin` MUST 按默认 HTTP target header rewrite 规则改写

#### Scenario: HTTPS passthrough does not rewrite HTTP headers
- **WHEN** HTTPS 代理没有生效证书并按 SNI 透传加密 TLS 字节到客户端本地 HTTPS 目标
- **THEN** 运行时 MUST 保持透传字节流，不得声明或尝试修改 TLS 内部的 HTTP `Host` 或 `Origin`
