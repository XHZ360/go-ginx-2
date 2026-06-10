## ADDED Requirements

### Requirement: Managed certificate active material boundary
系统 MUST 把托管 HTTPS 证书的 active material 作为独立的服务边界维护，且最近一次生命周期操作失败不得直接禁用仍有效的 active 证书。

#### Scenario: Failed renewal keeps valid active certificate
- **WHEN** 托管证书续期失败，但上一组 active 证书和私钥仍存在、未过期且通过代理主机校验
- **THEN** 系统继续把上一组 active 证书作为当前可服务材料，并记录续期失败供管理员检查

#### Scenario: Invalid active certificate is reported
- **WHEN** active 证书或私钥文件无法通过健康检查
- **THEN** 系统记录不可服务状态和脱敏错误，并且 MUST NOT 声明该证书材料可用于 TLS 终止

### Requirement: HTTPS proxy certificate requirement
系统 MUST 要求 HTTPS proxy 具备可服务证书；没有证书或证书失效的 HTTPS proxy MUST 被标记为失效或需要配置，并且不得以 passthrough 方式服务。

#### Scenario: HTTPS proxy without certificate is invalid
- **WHEN** 管理员创建、更新、启用或 daemon 启动加载 HTTPS proxy，且该 proxy 没有完整静态证书，也没有可服务托管证书
- **THEN** 系统把该 proxy 标记为证书失效或需要配置状态，并且该 proxy 不得对外提供 HTTPS 服务

#### Scenario: HTTPS proxy with invalid certificate is invalid
- **WHEN** HTTPS proxy 的静态证书或托管 active certificate material 过期、缺失、不可读、域名不匹配或 key 不匹配
- **THEN** 系统把该 proxy 标记为证书失效或需要配置状态，并记录脱敏错误供管理员修复

#### Scenario: Valid certificate restores HTTPS proxy
- **WHEN** 管理员配置有效静态证书，或托管证书签发/续期成功并激活可服务 active material
- **THEN** 系统允许该 HTTPS proxy 进入可服务状态，并允许 HTTPS 入口使用该证书执行 TLS 终止

### Requirement: Certificate lifecycle metadata remains secret-safe
系统 MUST 在扩展证书健康、操作和调度元数据时继续保持私钥和 DNS provider 凭据位于 SQLite 之外。

#### Scenario: Lifecycle fields store metadata only
- **WHEN** 系统持久化证书健康状态、操作状态、失败次数、下一次尝试时间、证书指纹或错误摘要
- **THEN** SQLite 只保存元数据、文件路径和脱敏错误，且 MUST NOT 保存私钥字节或 DNS provider token 值

#### Scenario: Health errors are sanitized
- **WHEN** 证书健康检查或生命周期操作失败
- **THEN** 系统记录可诊断的错误摘要，但 MUST NOT 把私钥内容、DNS provider token 或完整敏感响应写入普通日志、SQLite 或管理 API 响应
