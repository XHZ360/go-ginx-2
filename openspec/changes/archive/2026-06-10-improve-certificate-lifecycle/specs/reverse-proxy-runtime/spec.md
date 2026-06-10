## ADDED Requirements

### Requirement: Validated managed certificate selection
HTTPS 反向代理运行时 MUST 只使用通过 active material 健康检查的托管证书完成公网 TLS 终止。

#### Scenario: Usable managed certificate terminates TLS
- **WHEN** TLS ClientHello SNI 主机匹配已启用 HTTPS 代理，且该代理主机存在通过健康检查的托管 active certificate material
- **THEN** 服务端使用该 active certificate material 终止公网 TLS，并把解密后的 HTTP 请求转发到客户端本地 HTTP 目标

#### Scenario: Failed renewal still serves valid active certificate
- **WHEN** TLS ClientHello SNI 主机匹配已启用 HTTPS 代理，该代理最近一次托管证书续期失败，但 active certificate material 仍通过健康检查
- **THEN** 服务端继续使用该 active certificate material 终止公网 TLS

#### Scenario: Invalid managed material is not used
- **WHEN** TLS ClientHello SNI 主机匹配已启用 HTTPS 代理，但该代理主机的托管 active certificate material 已过期、缺失、不可读、域名不匹配或 key 不匹配
- **THEN** 服务端 MUST NOT 使用该托管证书完成公网 TLS 终止，并把该代理标记为证书失效或需要配置状态

#### Scenario: HTTPS proxy without certificate is rejected
- **WHEN** TLS ClientHello SNI 主机匹配已启用 HTTPS 代理，但该代理没有完整静态证书，也没有可服务托管 active certificate material
- **THEN** 服务端 MUST NOT 透传该 TLS 连接，并拒绝该连接或关闭该连接，同时记录可诊断的证书缺失错误

## MODIFIED Requirements

### Requirement: HTTPS TLS termination baseline
系统 MUST 要求已启用 HTTPS 代理具备生效的文件型证书/私钥路径或可热加载托管证书，并使用该证书执行 TLS 终止。服务端 MUST 按 SNI 选择代理和证书，完成公网 TLS 握手，并把解密后的 HTTP 请求转发到客户端本地 HTTP 目标。没有可用证书或证书失效的 HTTPS 代理 MUST 被标记为证书失效或需要配置状态，且 MUST NOT 以 passthrough 方式服务流量。

#### Scenario: HTTPS termination reaches local HTTP target
- **WHEN** TLS ClientHello SNI 主机存在已启用 HTTPS 代理，代理有生效的静态或托管证书和私钥，且其客户端在线
- **THEN** 运行时终止公网 TLS，并通过客户端把解密后的 HTTP 请求转发到配置的本地 HTTP 目标

#### Scenario: HTTPS certificate selected by SNI
- **WHEN** HTTPS 终止流量到达已配置的 HTTPS 代理主机
- **THEN** 服务端使用该代理主机对应的生效静态或托管证书及私钥

#### Scenario: Managed certificate hot reload applies to new handshakes
- **WHEN** HTTPS 代理主机的托管证书替换件被激活
- **THEN** 该 SNI 主机的新 TLS 握手无需重启 HTTPS 监听器即可使用替换证书

#### Scenario: HTTPS proxy without certificate is unavailable
- **WHEN** 已启用 HTTPS 代理没有配置生效的静态或托管证书和私钥
- **THEN** 运行时把该代理标记为证书失效或需要配置状态，并且 MUST NOT 透传加密 TLS 字节到客户端目标

## REMOVED Requirements

### Requirement: HTTPS reverse proxy passthrough baseline
**Reason**: HTTPS proxy 现在必须代表服务端持证书并终止 TLS 的 HTTPS 入口；无证书 passthrough 会让证书生命周期状态和运行时服务语义不一致。

**Migration**: 升级前为 HTTPS proxy 配置有效静态证书或签发托管证书。需要纯 TLS/SNI 透传的部署后续应迁移到独立代理类型或显式高级能力，而不是依赖 HTTPS proxy 的默认行为。
