# 证书管理

## 证书来源

控制通道首次 configless 启动时生成并复用 CA、服务端证书和私钥；这些材料只用于控制通道，不是公网 HTTPS 证书。公网 HTTPS 使用 Domain 绑定的证书资源：file-backed，或托管证书 provider（ACME Cloudflare DNS-01、Cloudflare Origin CA）。

权威绑定在 Domain（`Domain.CertificateID`）。一张证书可服务多个 Domain（1:n，例如 `*.example.com`）；每个 Domain 最多一张证书。HTTPS 在读取请求 Path 前按 SNI 选择 Domain 与证书并完成 TLS，再按 Path 选择 Web Proxy。

SQLite 只保存证书路径、host/hostnames、指纹、有效期、provider、状态和脱敏错误。私钥及 provider token 保存在 SQLite 之外，token 通过环境变量或受保护 secret store 提供。

Admin GraphQL 通过 `certificateProviderReadiness` 返回只读的 provider readiness。ACME 检查 `acme_enabled`、账户邮箱、条款和配置的 DNS Token 环境变量是否非空；响应只能返回环境变量名称、稳定缺失项和脱敏 guidance。创建/签发会在任何 CA、DNS 或证书记录操作前重复检查；未就绪时返回 `PROVIDER_NOT_READY`，其中 `fields.missingRequirements` 是逗号分隔的稳定枚举。

## 生命周期

证书状态分为两组：`serving_status` 表示 active material 是否可服务（`usable`、`expiring_soon`、`expired`、`missing`、`invalid`），`operation_status` 表示最近签发、续期、轮换或同步结果。健康 active material 即使最近续期失败仍继续服务；新材料只有通过证书、私钥、有效期和主机覆盖检查后才替换 active 文件，并保留上一组材料。

ACME 使用 DNS-01，完成后尝试清理 challenge 记录；失败使用退避和 `next_attempt_at`，不会覆盖旧的可用材料。Origin CA 的 credential、Cloudflare certificate ID、hostnames、request type 和 requested validity 只在 Origin CA provider 路径中使用。撤销是显式高风险操作，不会因轮换自动发生。

## 管理与排查

使用 Admin UI Certificates 页或 `goginx-admin` 的 issue/renew/status 命令操作证书。检查 `certificate_dir/managed/<host>/`、provider 凭据、主机委派、证书覆盖范围和 `serving_status`；不要把私钥、token 或完整敏感响应写入日志、API 或测试快照。

实现入口为 `internal/certmanager/` 和 `internal/proxy/https/`；验证使用 `go test ./internal/certmanager ./internal/admin -count=1`。

平台域名证书范围、自定义域名所有权验证和完整手动上传生命周期仍是已知缺口。
