# 托管 HTTPS 证书操作

### ACME DNS-01

启用 ACME DNS-01 时，服务端需要 Cloudflare API token 环境变量和额外配置：

```json
{
  "acme_enabled": true,
  "acme_directory_url": "https://acme-v02.api.letsencrypt.org/directory",
  "acme_account_email": "ops@example.com",
  "acme_terms_accepted": true,
  "acme_renewal_window": 2592000000000000,
  "acme_cloudflare_token_env": "CF_DNS_API_TOKEN"
}
```

证书管理命令：

```bash
export CF_DNS_API_TOKEN="<cloudflare-token>"
./bin/goginx-admin issue-managed-certificate -proxy secure-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
./bin/goginx-admin renew-managed-certificate -proxy secure-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
./bin/goginx-admin managed-certificate-status -proxy secure-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
```

托管证书文件保存在 `certificate_dir/managed/<host>/`。SQLite 只保存证书生命周期元数据和文件路径，不保存私钥字节或 Cloudflare token。

### Cloudflare Origin CA

Cloudflare Origin CA 是另一类托管证书 provider。它不使用 DNS-01 challenge；服务端本地生成私钥和 CSR，只把 CSR、hostnames、request type 和 requested validity 发送给 Cloudflare。私钥仍只写入 `certificate_dir/managed/<host>/`，Cloudflare API Token 明文只写入 SQLite 外的 `origin_ca_secret_store_path`。

托管默认启动会默认启用 Cloudflare Origin CA，并使用 `data/secrets/provider-credentials` 保存 Admin UI 写入的 credential secret。只有显式配置需要覆盖路径或关闭该能力时，才需要手写下面的配置项。

```json
{
  "origin_ca_enabled": true,
  "origin_ca_secret_store_path": "data/secrets/provider-credentials",
  "origin_ca_default_request_type": "origin-ecc",
  "origin_ca_requested_validity": 5475,
  "origin_ca_rotation_window": 2592000000000000
}
```

在 Admin UI 的 Certificates 页面创建、更新并验证 Cloudflare Origin CA API Token credential；token 字段是 write-only，查询响应、审计事件和 SQLite 只包含 credential metadata、token 指纹和 secret 引用。不要使用 Origin CA Service Key，配置中的 `origin_ca_service_key_path` 会被拒绝。

创建或轮换 Origin CA 证书时，HTTPS proxy 的 DNS 记录应在 Cloudflare 中保持 proxied，SSL/TLS mode 应使用 Full (strict) 或等价的严格 origin 校验路径。Origin CA 证书只适合 Cloudflare 到 origin 的 TLS 连接，公网浏览器直连 origin 不会按普通 WebPKI 信任该证书。

CLI 可以消费已经由 Admin UI/API 写入的 credential ID；如果未显式提供 credential，系统只会在 Cloudflare Origin CA 的可用 credential 中按 provider/status scoped 查询唯一默认项，多于一个可用 credential 时会要求显式选择：

```bash
./bin/goginx-admin issue-managed-certificate \
  -proxy secure-1 \
  -provider cloudflare_origin_ca \
  -credential cfcred_123 \
  -certificate-dir data/certs \
  -origin-ca-secret-store data/secrets/provider-credentials

./bin/goginx-admin sync-origin-ca-certificate \
  -proxy secure-1 \
  -certificate-dir data/certs \
  -origin-ca-secret-store data/secrets/provider-credentials
```

Origin CA 的调度窗口由 `origin_ca_rotation_window` 控制；ACME renewal window、Origin CA rotation window、`expiring_soon` 状态和失败 `next_attempt_at` 都来自统一生命周期调度规则。进入窗口后 daemon 会把证书纳入 provider-specific rotation 候选，并沿用失败退避和 active material 保留语义。撤销是高风险动作，不会在轮换后自动执行；只有显式提供 proxy ID、host 和 Cloudflare certificate ID 时才会调用 revoke。撤销当前 active 证书会让 Cloudflare 到 origin 的 Full (strict) 连接失败，通常应先轮换并确认新证书已部署。

```bash
./bin/goginx-admin revoke-origin-ca-certificate \
  -proxy secure-1 \
  -host secure.example.com \
  -cloudflare-certificate-id <cloudflare-origin-ca-certificate-id> \
  -certificate-dir data/certs \
  -origin-ca-secret-store data/secrets/provider-credentials
```

托管证书状态拆分为两类：`serving_status` 描述当前 active 证书材料是否能服务 TLS，取值包括 `usable`、`expiring_soon`、`expired`、`missing` 和 `invalid`；`operation_status` 描述最近一次签发或续期操作，取值包括 `idle`、`issue_failed` 和 `renewal_failed` 等。续期失败只会记录操作失败、失败次数和 `next_attempt_at` 退避时间；只要上一组 active 证书仍健康，新握手会继续使用它。续期成功后会更新 active 文件、证书 SHA-256 指纹、过期时间，并重置失败次数和退避状态。

Cloudflare Origin CA 还会展示 `provider_status`、`credential_id`、Cloudflare certificate ID、hostnames、request type、requested validity 和 `last_synced_at`。当 provider sync 确认 active Origin CA 证书已 revoked 或 missing_remote 时，HTTPS runtime 不会继续声明该托管证书可服务，并会把对应 HTTPS proxy 映射为需要证书配置。
