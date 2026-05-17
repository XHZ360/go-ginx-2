## Purpose

定义托管 ACME DNS-01 证书自动化契约，覆盖 DNS 提供商配置、证书签发、DNS 挑战清理、续期调度、经过校验的热加载与回滚，以及 HTTPS 代理主机在管理端可见的证书状态。

## Requirements

### Requirement: ACME provider configuration
系统 MUST 通过服务端运行时配置和环境变量提供的 DNS 凭据配置 ACME DNS-01 自动化，并且 MUST NOT 把提供商密钥存入 SQLite。

#### Scenario: Cloudflare token loaded from environment
- **WHEN** 启用 ACME Cloudflare DNS-01 自动化，并且配置的令牌环境变量存在
- **THEN** 服务端使用该令牌执行 DNS 挑战操作，且 MUST NOT 把令牌值持久化到 SQLite

#### Scenario: Missing provider credential blocks issuance
- **WHEN** 请求 ACME 签发，但配置的 Cloudflare 令牌环境变量缺失或为空
- **THEN** 签发以凭据配置错误失败，并且现有 HTTPS 证书文件保持不变

### Requirement: Managed certificate issuance
系统 MUST 在显式请求时，为已启用的 HTTPS 代理主机通过 ACME DNS-01 签发托管证书。

#### Scenario: Successful issuance writes managed files
- **WHEN** 已启用的 HTTPS 代理请求托管证书签发，且 DNS-01 校验成功
- **THEN** 服务端把证书和私钥写入 `certificate_dir` 下，校验证书/私钥对，并记录该代理主机的证书元数据

#### Scenario: Issuance failure preserves existing state
- **WHEN** ACME 订单创建、DNS 挑战校验、证书获取、证书校验或文件持久化失败
- **THEN** 服务端记录失败元数据，且 MUST NOT 替换当前生效的证书文件

### Requirement: DNS challenge cleanup
系统 MUST 在 ACME 校验尝试结束后清理 DNS-01 挑战记录。

#### Scenario: Cleanup after validation success
- **WHEN** 托管证书请求的 DNS-01 校验成功
- **THEN** 服务端在标记签发成功前删除临时 DNS 挑战记录

#### Scenario: Cleanup after validation failure
- **WHEN** 创建挑战记录后 DNS-01 校验失败
- **THEN** 服务端尝试删除临时 DNS 挑战记录，并把清理失败与签发失败分开记录

### Requirement: Renewal scheduling
系统 MUST 在证书过期前，按照配置的续期窗口续期托管 HTTPS 证书。

#### Scenario: Certificate enters renewal window
- **WHEN** 托管证书将在配置的续期窗口内过期
- **THEN** 守护进程无需重启 `goginx-server` 即可尝试续期

#### Scenario: Certificate outside renewal window
- **WHEN** 托管证书的过期时间晚于配置的续期窗口
- **THEN** 守护进程在该轮次保持当前生效证书不变，且不尝试续期

### Requirement: Hot reload and rollback
系统 MUST 热加载通过校验的托管证书替换件，并保留上一组有效证书材料用于回滚。

#### Scenario: Successful renewal hot reloads certificate
- **WHEN** 托管证书续期成功，并且替换证书/私钥对通过代理主机校验
- **THEN** HTTPS 入口无需重启监听器，即可在新的 TLS 握手中使用替换证书

#### Scenario: Invalid replacement rolls back
- **WHEN** 签发或文件写入后，替换证书/私钥对校验失败
- **THEN** 服务端继续提供上一组有效证书，并记录续期失败元数据

### Requirement: Managed certificate status
系统 MUST 通过管理操作暴露托管证书状态，同时 MUST NOT 暴露私钥材料或 DNS 提供商密钥。

#### Scenario: Status inspection
- **WHEN** 管理员查看某个代理主机的托管证书状态
- **THEN** 输出包含主机、状态、过期时间、生效证书路径、最近签发或续期结果，且 MUST NOT 包含私钥字节或 DNS 提供商令牌值
