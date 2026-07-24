# Daemon Runtime Deployment

This guide covers the implemented milestone-one daemon runtime plus the first supported deployment baselines: a reproducible bundle with `systemd` service templates for single-node Linux-style deployment, and native Windows Service commands for Windows hosts. It does not describe native installers, package-manager distribution, backup/restore tooling, or other features that are still out of scope.

## Implemented Features

1. cgo-free SQLite persistence through repository interfaces.
2. QUIC and TCP+TLS control authentication with TLS and CA verification.
3. Proxy snapshots sent to clients after successful authentication.
4. Heartbeat and session tracking with latest-session replacement.
5. TCP reverse proxy over QUIC streams or framed TCP+TLS substreams.
6. UDP reverse proxy over QUIC streams or framed TCP+TLS substreams with per-source sessions and idle cleanup.
7. HTTP reverse proxy over QUIC streams or framed TCP+TLS substreams, routed by `Host`.
8. HTTPS reverse proxy TLS termination with a required static or managed ACME DNS-01 certificate selected by SNI over QUIC streams or framed TCP+TLS substreams.
9. Admin CLI commands for milestone-one users, clients, proxy records, and managed certificate issue/renew/status operations.
10. Daemon server and client runtime commands.
11. External process smoke tests for real server and client binaries.
12. SQLite-backed cumulative stats persistence for TCP, UDP, and HTTP traffic.
13. Reproducible deployment bundle generation for server, client, and admin binaries.
14. Checked-in `systemd` service templates for supervised Linux server and client execution.
15. Native Windows Service commands and PowerShell helper scripts for supervised Windows server and client execution.
15. Configless server startup with managed `data/` state and generated control-channel TLS material.
16. One-time client join tokens that write managed client state for later no-`-config` startup.
17. Optional administrator-only management listener with 8-hour JWT login, session bootstrap, logout, GraphQL operations, client enrollment, and same-origin dedicated frontend delivery from the runtime `admin-ui/` directory or an explicit frontend directory override.

## Seed SQLite

Seed the SQLite database before starting the daemon pair. The server reads users, client credentials, and enabled proxies from the configured `sqlite_path`.

Use the admin CLI flow in [admin-seed-sqlite.md](examples/admin-seed-sqlite.md) to create a user, client credential, TCP proxy, UDP proxy, HTTP proxy, and HTTPS termination proxy.

## Configless Server Startup

The default server path does not require a hand-authored `server.json`:

```powershell
./.tmp/goginx-server.exe
```

On first start the server creates managed runtime state beneath the working directory:

1. `data/go-ginx.db` for SQLite persistence.
2. `data/certs/control-ca.crt` for client trust distribution.
3. `data/certs/control.crt` and `data/certs/control.key` for QUIC and TCP+TLS control listeners.
4. `data/certs/managed/` for managed HTTPS proxy certificates.
5. `data/admin-jwt.key` for administrator JWT signing.

Initialize the first administrator locally:

```powershell
./.tmp/goginx-admin.exe init-admin -id admin-1 -username admin -password "<password>"
```

Generate a client join token:

You can generate it from the admin UI Clients page with `Create join token`, or use the CLI:

```powershell
./.tmp/goginx-admin.exe create-client-join -id client-1 -user admin-1 -name home
```

During server startup, the server confirms a default join service host from `join_service_host`, the control listener host, a local interface address, or a loopback fallback. The startup log prints the confirmed host, source, default control addresses, enrollment listener, and default enrollment URL. Admin API, `goginx-admin create-client-join`, `goginx-admin client-join-command`, and `goginx-admin tui` use the same default join resolution when join fields are not explicitly provided. Set `GOGINX_JOIN_SERVICE_HOST` or `join_service_host` when clients must use a public DNS name or load-balancer address instead of a local fallback.

For explicit server config deployments, pass the same config to admin join commands:

```powershell
./.tmp/goginx-admin.exe create-client-join -server-config config/server.json -id client-1 -user admin-1 -name home
```

On the client host:

```powershell
./.tmp/goginx-client.exe join "<join-token>"
./.tmp/goginx-client.exe
```

The join command redeems the token through the dedicated client enrollment listener at `/api/client/enroll`, writes `data/client-state.json`, writes `data/certs/server-ca.crt`, and subsequent client runs use that managed state. The admin listener no longer serves `/api/client/enroll`; old tokens that point at the admin listener must be regenerated. By default these paths are under the deployment root derived from the `goginx-client` binary location; when the binary is under `bin/`, the deployment root is the parent of `bin/`.

## Server 本机代理运维

server 启动时会在 SQLite 中幂等确保 `server-local-system` 用户、`server-local` client 和 `local_target_allowlist` 默认记录，并注册常驻 virtual session。默认白名单只包含 `127.0.0.1/32` 与 `::1/128`，两条记录默认允许全部端口；上线前应在 Admin UI 的 `Server Local` 客户端详情中按实际服务端口收紧。当前管理面只支持 TCP/UDP 本机代理，target 必须是 IP，不能填写 hostname。

白名单替换使用事务持久化和原子运行时快照。无效/空白名单、保留身份冲突或初始化失败会阻止 server 注册可用 virtual session；更新写入失败时旧策略继续生效。收紧后只拒绝新连接，不主动中断既有连接。所有本机代理管理操作和 forbidden 尝试进入审计，但日志与审计错误摘要不保存凭据或完整底层拨号错误。

回滚旧版本前，先在 Admin UI 禁用所有本机代理并确认对应 TCP/UDP listener 已关闭，再停止 server、备份 SQLite 并替换二进制/UI。保留系统 user/client/proxy 和 `local_target_allowlist` 数据，不要手工删除；不了解这些表的旧版本会忽略它们，重新升级后可恢复。若仅需紧急阻断流量，禁用本机代理比删除数据更可恢复。

Managed startup accepts environment overrides for file-free deployments that need non-default ports, paths, secrets, or join defaults, including `GOGINX_ADMIN_LISTEN`, `GOGINX_ADMIN_JWT_SECRET_FILE`, `GOGINX_CLIENT_ENROLLMENT_LISTEN`, `GOGINX_CONTROL_QUIC_LISTEN`, `GOGINX_CONTROL_TLS_LISTEN`, `GOGINX_JOIN_SERVICE_HOST`, `GOGINX_HTTP_ENTRY_LISTEN`, `GOGINX_HTTPS_ENTRY_LISTEN`, `GOGINX_SQLITE_PATH`, `GOGINX_DATA_DIR`, and `GOGINX_CERTIFICATE_DIR`. Treat `127.0.0.1` as a local development or last-resort fallback; cross-host joins should use a reachable DNS name or IP through `GOGINX_JOIN_SERVICE_HOST`, `join_service_host`, `-server-config`, or explicit join command flags. Configless defaults use `:8081` for client enrollment, `:80` for HTTP entry traffic, and `:443` for HTTPS entry traffic; binding 80/443 can require root, `CAP_NET_BIND_SERVICE`, service-manager privileges, or explicit non-privileged overrides.

## Optional Server Config

The configless server command automatically loads `config/server.json` from the deployment root when that file exists, then applies supported `GOGINX_*` environment overrides. Explicit `-config` remains available when a different path is required. Create `server.json` with fields from `internal/config/config.go` when defaults and environment overrides are not enough:

```json
{
  "admin_listen": "127.0.0.1:8080",
  "admin_frontend_dir": "web/admin",
  "admin_jwt_secret_file": "data/admin-jwt.key",
  "client_enrollment_listen": "0.0.0.0:8081",
  "control_quic_listen": "127.0.0.1:8443",
  "control_tls_listen": "127.0.0.1:9443",
  "join_service_host": "control.example.com",
  "control_tls_cert_file": "data/certs/control.crt",
  "control_tls_key_file": "data/certs/control.key",
  "tcp_entry_host": "127.0.0.1",
  "http_entry_listen": "0.0.0.0:80",
  "https_entry_listen": "0.0.0.0:443",
  "sqlite_path": "data/go-ginx.db",
  "data_dir": "data",
  "certificate_dir": "data/certs",
  "acme_enabled": true,
  "acme_directory_url": "https://acme-v02.api.letsencrypt.org/directory",
  "acme_account_email": "ops@example.com",
  "acme_terms_accepted": true,
  "acme_renewal_window": 2592000000000000,
  "acme_cloudflare_token_env": "CF_DNS_API_TOKEN",
  "heartbeat_timeout": 45000000000,
  "log_max_size_mb": 50,
  "log_max_backups": 10,
  "log_retention_days": 7,
  "log_compress": true
}
```

`control_quic_listen` is the primary control listener. `control_tls_listen` enables TCP+TLS fallback for authentication, proxy snapshots, heartbeats, and framed proxy substreams. TCP+TLS fallback is reliable but uses one TCP connection, so slow streams can cause normal TCP head-of-line effects.

When `admin_frontend_dir` is empty, the server loads the dedicated admin frontend from `admin-ui/` under the deployment root. If the server binary is under `bin/`, the deployment root is the parent of `bin/`; otherwise it is the binary directory. `admin_frontend_dir` is optional and, when set, must point to another directory containing the dedicated admin frontend build output, including `index.html`. The admin listener serves those browser routes and assets on the same origin as the administrator API while keeping `/api/admin/*` reserved for backend endpoints.

`admin_jwt_secret_file` points to the stable HMAC signing key used for administrator browser JWTs. The managed default is `data/admin-jwt.key`; explicit deployments can override it through JSON or `GOGINX_ADMIN_JWT_SECRET_FILE`. The key file should be readable only by the service account, included in backup and restore plans, and never logged or exposed to clients.

The control certificate must be valid for the client `server_name`, and the client must trust the CA that signed it.

## Optional Client Config

The default client path is `goginx-client join <token>`. Explicit `client.json` remains supported for advanced deployments:

```json
{
  "server_address": "127.0.0.1:8443",
  "server_tls_address": "127.0.0.1:9443",
  "server_name": "localhost",
  "server_ca_file": "data/certs/server-ca.crt",
  "client_id": "client-1",
  "credential": "secret",
  "allowed_protocols": ["quic", "tcp_tls"],
  "reconnect": {
    "initial_delay": 1000000000,
    "max_delay": 30000000000
  },
  "log_max_size_mb": 50,
  "log_max_backups": 10,
  "log_retention_days": 7,
  "log_compress": true
}
```

Duration fields are JSON numbers in nanoseconds because the config structs use `time.Duration`. `reconnect.initial_delay` and `reconnect.max_delay` control client retries after transient dial or runtime failures. Authentication rejection still returns immediately instead of retrying forever.

## Runtime Log Rotation

Both daemons write local runtime logs to stable files under the deployment root: `logs/server.log` and `logs/client.log`. The same messages are also written to stderr, so service managers, foreground shells, and container runtimes can keep capturing process output. Runtime errors are not split into a separate error file; they remain in the same rotated log stream with error level/category context and without credentials, tokens, cookies, private keys, request bodies, or other sensitive values.

By default, each current log file rotates at 50 MiB, keeps up to 10 archives, removes archives older than 7 days, and compresses rotated archives with gzip. Archive names use timestamps such as `server-20260608-153000.log` or `client-20260608-153000.log.gz`; if several rotations happen within the same second, the archive name gets a numeric suffix.

Linux `systemd` deployments should keep relying on stderr/journald for service capture while the application manages files in `logs/`. Windows deployments should rely on the built-in application rotation instead of external rename-based logrotate tools, because open files cannot always be renamed or removed externally. Docker and Kubernetes deployments should prefer stdout/stderr plus the container runtime's log rotation, with file logs used only when the deployment explicitly wants local files or troubleshooting artifacts.

## Build And Run

Build the runtime commands into a temporary output directory:

```powershell
$env:CGO_ENABLED="0"
New-Item -ItemType Directory -Force .tmp
go build -o ./.tmp/goginx-server.exe ./cmd/goginx-server
go build -o ./.tmp/goginx-client.exe ./cmd/goginx-client
go build -o ./.tmp/goginx-admin.exe ./cmd/goginx-admin
```

Build the first supported deployment bundle for a Linux `systemd` installation:

```powershell
Set-Location admin-ui
corepack enable
pnpm install --frozen-lockfile
pnpm build
Set-Location ..
$env:CGO_ENABLED="0"
go run ./cmd/goginx-admin build-deploy-bundle -output ./.tmp/linux-systemd-bundle -goos linux -goarch amd64 -install-root /opt/go-ginx
```

The core bundle layout is stable and contains:

1. `bin/` with `goginx-server`, `goginx-client`, and `goginx-admin`.
2. `admin-ui/` with the management frontend build output used by default at runtime.
3. `config/` with optional sample `server.example.json`, `client.example.json`, and environment examples.
4. `data/` with SQLite, certificate directories, and the administrator JWT signing key.
5. `logs/` for application-rotated runtime log files.
6. `systemd/` with rendered `goginx-server.service` and `goginx-client.service` units.
7. `config/admin-credentials.json.example` for the optional administrator management surface.

Deployments use the install-root `admin-ui/` directory by default. Rebuild the frontend before packaging; `build-deploy-bundle` fails if `admin-ui/dist` is missing. Deployments that need a custom frontend may include built assets in a different directory inside the install root, such as `web/admin/`, and set `admin_frontend_dir` through explicit config.

Build the Windows release bundle for direct execution on Windows hosts:

```powershell
Set-Location admin-ui
corepack enable
pnpm install --frozen-lockfile
pnpm build
Set-Location ..
$env:CGO_ENABLED="0"
go run ./cmd/goginx-admin build-deploy-bundle -output ./.tmp/windows-amd64-bundle -goos windows -goarch amd64
```

The Windows bundle keeps `bin/`, `config/`, `data/`, `logs/`, `admin-ui/`, and `scripts/`, but does not include `systemd/`. Run the generated `.exe` files from the unpacked bundle root, or install server/client as native Windows Services using the built-in `service` subcommands or the PowerShell helpers under `scripts/`.

Run the server from the desired state directory:

```powershell
./.tmp/goginx-server.exe
```

After `goginx-client join <token>`, run the client:

```powershell
./.tmp/goginx-client.exe
```

If the client exits with a missing `data/client-state.json` error, it was started before the join flow wrote managed state, or a custom path was used inconsistently. Run `goginx-client join <new-token>`, confirm `data/client-state.json` exists under the deployment root, then start the client service.

For the supported native Windows Service model, run commands from an Administrator PowerShell. For remote clients, prefer a persistent `config/server.json` with `join_service_host` set before installing and starting the server service; do not rely on a temporary PowerShell environment variable for service-mode join defaults.

Server service using helper script:

```powershell
Set-Location C:\go-ginx
Copy-Item .\config\server.example.json .\config\server.json
# Edit config\server.json and set join_service_host to a client-reachable host.
.\scripts\goginx-server-service.ps1 -Action install -Config config\server.json
.\scripts\goginx-server-service.ps1 -Action start
.\scripts\goginx-server-service.ps1 -Action status
```

Equivalent built-in commands:

```powershell
.\bin\goginx-server.exe service install -config config\server.json
.\bin\goginx-server.exe service start
.\bin\goginx-server.exe service status
```

Generate join tokens with the same server config, or pass explicit enrollment/control addresses:

```powershell
.\bin\goginx-admin.exe init-admin -id admin-1 -username admin -password "<password>"
$token = .\bin\goginx-admin.exe create-client-join -server-config config\server.json -id client-1 -user admin-1 -name home
```

Client service installation must happen after join writes managed state:

```powershell
Set-Location C:\go-ginx
.\bin\goginx-client.exe join "$token"
Test-Path .\data\client-state.json
.\scripts\goginx-client-service.ps1 -Action install
.\scripts\goginx-client-service.ps1 -Action start
.\scripts\goginx-client-service.ps1 -Action status
```

The equivalent built-in commands are `.\bin\goginx-client.exe service install`, `service start`, and `service status`. Stop, restart, and uninstall use the same verb shape. Windows service mode continues using `logs/server.log` and `logs/client.log`; the first native service release does not register Windows Event Log sources and does not expose custom service-account parameters. If a custom account is required, install the service first and adjust the account using Windows service management tools.

For Windows upgrades or rollback, stop services first, replace release files such as `bin/`, `admin-ui/`, `scripts/`, and examples, preserve `data/` plus existing `config/server.json` and `config/client.json`, then start services again.

For the supported `systemd` deployment model:

1. Copy the generated bundle to the rendered install root, such as `/opt/go-ginx`.
2. Start the server service and initialize the first administrator.
3. Install `systemd/goginx-server.service` and `systemd/goginx-client.service` into `/etc/systemd/system/`.
4. Run `systemctl daemon-reload`.
5. Start the units with `systemctl enable --now goginx-server goginx-client`.
6. Restart after config or binary updates with `systemctl restart goginx-server goginx-client`, preserving `data/admin-jwt.key` when you want unexpired administrator JWTs to survive the restart.

The server starts SQLite, the QUIC control listener, the optional TCP+TLS fallback listener, the default HTTP entry listener, the optional default HTTPS entry listener, and any extra per-proxy listeners required by enabled proxies. Each proxy has an effective entry listener:

- TCP/UDP use `entry_bind_host` plus `entry_port`; an empty bind host falls back to `tcp_entry_host`.
- HTTP uses `entry_bind_host`, `entry_port`, and `entry_host`; an empty bind host or zero entry port falls back to `http_entry_listen`, while `entry_host` remains the HTTP Host route domain.
- HTTPS uses `entry_bind_host`, `entry_port`, and `entry_host`; an empty bind host or zero entry port falls back to `https_entry_listen`, while `entry_host` remains the SNI route domain.

When administrators create, update, enable, disable, or delete proxies, the daemon reconciles listeners before the management operation returns. Missing listeners are started immediately, custom listeners no longer referenced by enabled proxies are closed, and shared HTTP/HTTPS listeners stay active for other domains on the same address and port. A bind failure is returned as a management error rather than being hidden behind an enabled proxy record. The HTTPS entry uses SNI to choose the proxy. If that proxy has `cert_file` and `key_file`, the server health-checks the pair, terminates TLS, and forwards the decrypted HTTP request to the configured local HTTP target. Otherwise it checks for an active managed certificate for that HTTPS host and uses it only when the active certificate and key pass health checks. If neither static nor managed certificate material is usable, the proxy is reported as `needs_config`, the matching TLS connection is rejected or closed, and encrypted TLS bytes are not forwarded to the client target. The client authenticates, receives the proxy snapshot, sends heartbeats, serves proxy streams to configured local targets over QUIC or framed TCP+TLS fallback, and retries transient control-plane failures using the configured reconnect backoff.

When configless server startup is used, the server starts an administrator-only management listener on `admin_listen` and authenticates SQLite administrator users initialized by `init-admin`. `admin_credentials_file` remains a compatibility override. The `/api/admin/*` namespace remains reserved for management API behavior and exposes:

1. `POST /api/admin/login` for administrator JWT creation.
2. `GET /api/admin/session` for browser session bootstrap.
3. `POST /api/admin/logout` for browser cookie clearing.
4. `POST /api/admin/graphql` for administrator GraphQL queries and mutations.

Successful login signs an administrator JWT with an 8-hour absolute lifetime and writes it to the existing HttpOnly same-origin cookie. The server no longer keeps an in-process session map for administrator login state and does not apply an idle timeout. `GET /api/admin/session` validates the JWT cookie and returns the administrator context plus the CSRF token stored in JWT claims. GraphQL queries require only a valid JWT; GraphQL mutations and authenticated logout still require `X-GoGinx-CSRF-Token` to match the JWT claim.

If the server restarts with the same `admin_jwt_secret_file` contents, unexpired administrator JWT cookies remain valid. Deleting, corrupting, or rotating `data/admin-jwt.key` invalidates existing administrator JWTs and requires administrators to log in again. Logout only clears the browser cookie; pure stateless JWT mode does not provide server-side revocation for an unexpired token that was copied outside the browser. During upgrade from older in-memory sessions, existing session cookies cannot be migrated and administrators should log in once after the first restart. Rolling back to an older build makes JWT cookies look like unknown session ids, so administrators will also need to log in again.

The same listener serves the dedicated admin frontend on non-API `GET` and `HEAD` routes. Browser routes such as `/`, `/login`, `/users`, `/clients`, `/proxies`, `/certificates`, and `/audit` resolve through the frontend shell, deep links serve `index.html`, and real asset files are served directly from the selected frontend directory. Missing asset-like paths such as `/assets/missing.js` still return `404 Not Found`. `admin_frontend_dir` is an override for development or custom deployments rather than a baseline requirement.

The first JWT-authenticated management slice is intentionally narrow: it excludes ordinary-user self-service, quota editing, log search, domain lifecycle management, advanced alerts, and realtime subscriptions. The legacy server-rendered admin UI and browser-facing `/graphql` route are not served in this slice.

When `acme_enabled` is true, the server reads the Cloudflare token from `acme_cloudflare_token_env`, stores managed certificates under `certificate_dir/managed/<host>/`, renews expiring managed certificates inside `acme_renewal_window`, hot reloads future TLS handshakes, and keeps the previous active certificate pair for rollback. SQLite stores only lifecycle metadata and file paths. The daemon renewal controller records `last_attempted_at`, `next_attempt_at`, `failure_count`, active certificate SHA-256 fingerprint, and health inspection time. Failed renewals increase the backoff and keep serving the previous active certificate when `serving_status` remains `usable` or `expiring_soon`.

When `origin_ca_enabled` is true, Cloudflare Origin CA is available as a managed certificate provider. Administrators maintain Cloudflare API Token credentials in the Admin UI; token material is written to `origin_ca_secret_store_path`, while SQLite stores only credential metadata, token fingerprints, and secret refs. Origin CA Service Key paths are rejected. If an issue or rotate action omits `-credential`, the server resolves a default credential with a provider/status-scoped query and requires an explicit credential ID when more than one usable Cloudflare Origin CA credential exists. The server generates the private key and CSR locally, sends only the CSR and non-sensitive request metadata to Cloudflare, stores active/previous material in the same `certificate_dir/managed/<host>/` layout, and rotates certificates inside `origin_ca_rotation_window`. ACME renewal windows, Origin CA rotation windows, `expiring_soon`, and `next_attempt_at` are calculated by the shared lifecycle scheduler. Origin CA deployments must use Cloudflare proxied DNS plus Full (strict) or an equivalent strict origin verification path. Direct browser connections to the origin are not a WebPKI trust target for these certificates. Provider sync records `provider_status` and `last_synced_at`; confirmed `revoked` or `missing_remote` active certificates are treated as not serviceable by HTTPS runtime.

Managed certificate admin commands:

```powershell
$env:CF_DNS_API_TOKEN="<cloudflare-token>"
./.tmp/goginx-admin.exe issue-managed-certificate -db data/go-ginx.db -proxy https-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
./.tmp/goginx-admin.exe renew-managed-certificate -db data/go-ginx.db -proxy https-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
./.tmp/goginx-admin.exe managed-certificate-status -db data/go-ginx.db -proxy https-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
./.tmp/goginx-admin.exe issue-managed-certificate -db data/go-ginx.db -proxy https-1 -provider cloudflare_origin_ca -credential cfcred_123 -certificate-dir data/certs -origin-ca-secret-store data/secrets/provider-credentials
./.tmp/goginx-admin.exe sync-origin-ca-certificate -db data/go-ginx.db -proxy https-1 -certificate-dir data/certs -origin-ca-secret-store data/secrets/provider-credentials
./.tmp/goginx-admin.exe revoke-origin-ca-certificate -db data/go-ginx.db -proxy https-1 -host secure.example.com -cloudflare-certificate-id <cloudflare-origin-ca-certificate-id> -certificate-dir data/certs -origin-ca-secret-store data/secrets/provider-credentials
```

Origin CA revoke is intentionally explicit and high risk. It requires matching proxy ID, host, and Cloudflare certificate ID. Revoking the active certificate marks the provider side unavailable and can break Cloudflare-to-origin Full (strict) traffic until a replacement active certificate is available.

## Troubleshooting

1. Unknown config fields: Config loading rejects unknown JSON fields. Remove fields that are not defined in `internal/config/config.go`.
2. Missing TLS files: `control_tls_cert_file`, `control_tls_key_file`, and `server_ca_file` must point to readable files from the process working directory, unless absolute paths are used.
3. CA or SNI mismatch: The client verifies the server certificate with `server_ca_file` and matches it against `server_name`. Use a CA file that signed the control certificate and a `server_name` present in the certificate SANs.
4. Auth rejected: Confirm `client_id` exists in SQLite and `credential` matches the value seeded by `goginx-admin create-client`.
5. No TCP or UDP listener: The server only starts TCP and UDP listeners for enabled proxies stored in SQLite. Check the proxy `client_id`, `entry_bind_host`, entry port, status, and the server `tcp_entry_host` fallback.
6. HTTP Host mismatch: HTTP routing uses the request `Host` plus the listener bind host and port. Send requests to the configured listener address with the host seeded for the HTTP proxy, such as `app.example.com`.
7. Target unreachable: The client connects to each proxy target from the client machine. Confirm the target host and port are reachable there, not only from the server.
8. UDP response missing: UDP routing keeps one QUIC stream per external source address until idle cleanup. Make sure replies return to the same source address and that the local UDP target responds before the session idles out.
9. HTTPS SNI mismatch: HTTPS routing uses the TLS ClientHello SNI value plus the listener bind host and port. Connect to the configured listener address with a server name matching the seeded HTTPS proxy host, such as `secure.example.com`.
10. HTTPS termination certificate errors: `cert_file` and `key_file` must point to a certificate/key pair valid for the HTTPS proxy host. If either field is omitted, the proxy uses a healthy active managed certificate when present; otherwise the proxy is `needs_config` and matching TLS connections are closed. HTTPS passthrough is no longer a fallback behavior for `https` proxies.
11. Stats flush and shutdown: TCP, UDP, and HTTP cumulative totals are flushed to SQLite during daemon shutdown. Stop the server cleanly with `SIGINT` or `SIGTERM` when you need the latest totals persisted. Active connection counts are runtime state and reset after restart.
12. Managed certificate issuance or renewal records an operation failure and `next_attempt_at` if the Cloudflare token environment variable is missing, the proxy host is not delegated to Cloudflare, or the process cannot reach the ACME directory or Cloudflare API. The status output keeps provider tokens and private key bytes out of SQLite, logs, and API responses.
13. Cloudflare Origin CA failures: create the API Token credential in Admin UI and verify it before issue/rotate/sync/revoke. The token field is write-only; use the displayed credential ID in CLI actions. Confirm the Cloudflare DNS record is proxied and SSL/TLS mode is Full (strict). Browser direct-to-origin failures are expected for Origin CA certificates. If sync reports `revoked` or `missing_remote`, HTTPS runtime will fail closed until a valid active certificate is issued or rotated. Treat active certificate revoke as a production-impacting action.
14. Client reconnect loops: transient dial or runtime failures now retry using the configured reconnect backoff, including control-listener outages or daemon restarts. Authentication rejection still exits immediately, so re-check the stored credential instead of waiting for automatic recovery.
15. `systemd` install paths: the generated service files assume the rendered `install_root` passed to `build-deploy-bundle`. Rebuild the bundle or edit the unit files if you deploy somewhere other than that path.
16. Upgrade and rollback: replace the bundle contents in the install root, including `admin-ui/`, then restart the units. Preserve `data/admin-jwt.key` across upgrades when you want unexpired administrator JWTs to remain valid after restart. The first upgrade from older in-memory sessions requires administrators to log in again once; rollback to an older build also clears JWT cookies as unknown sessions.
17. Administrator management credentials: `admin_credentials_file` must point to a readable JSON file containing administrator usernames and bcrypt password hashes. The file is separate from SQLite users and should be readable only by the service account.
18. Administrator JWT signing key: `admin_jwt_secret_file` must point to a readable base64url key with at least 32 decoded bytes. If `data/admin-jwt.key` is missing or corrupted, restore it from backup to keep unexpired JWTs valid; deleting or rotating it requires administrators to log in again. Do not copy the key into logs, bug reports, frontend assets, or client-visible config.
19. Management transport protection: the admin listener uses JWT-authenticated same-origin API routes and is expected to run behind TLS. Local loopback access is accepted for development and automated tests; loopback testing may issue non-`Secure` cookies because browsers and HTTP clients do not send `Secure` cookies over plain `http://127.0.0.1` development traffic.
20. Configless port conflicts: set `GOGINX_ADMIN_LISTEN`, `GOGINX_CLIENT_ENROLLMENT_LISTEN`, `GOGINX_CONTROL_QUIC_LISTEN`, `GOGINX_CONTROL_TLS_LISTEN`, `GOGINX_HTTP_ENTRY_LISTEN`, or `GOGINX_HTTPS_ENTRY_LISTEN` to free static addresses, or switch to explicit JSON config. Per-proxy `entry_bind_host` and `entry_port` must also avoid active static listeners and active proxy listeners. Wildcard addresses such as `0.0.0.0`, `::`, or an empty host conflict with concrete addresses on the same protocol and port. On systems that restrict low ports, either grant permission for the server to bind 80/443 or override the HTTP/HTTPS entry listeners.
21. Managed control TLS state: if `data/certs/control-ca.crt`, `control.crt`, or `control.key` is missing or corrupted, stop the server and restore the set from backup; deleting the set forces regeneration and breaks existing joined clients until they join again.
22. Missing administrator bootstrap: configless management login has no default password. Run `goginx-admin init-admin` before logging in. By default the CLI writes to `data/go-ginx.db` under the deployment root derived from the `goginx-admin` binary location; use `-db` when targeting a custom server SQLite path.
23. Join token failures: expired, already-used, tampered, revoked, or admin-listener-era join tokens are rejected by `/api/client/enroll`; generate a new token with `goginx-admin create-client-join`.
24. Client managed state damage: if `data/client-state.json` or `data/certs/server-ca.crt` is missing on the client host, run `goginx-client join <new-token>` again.
25. Runtime log growth: check `log_max_size_mb`, `log_max_backups`, `log_retention_days`, `log_compress`, the deployment-root `logs/` directory, and service-account permissions when current or archived logs grow beyond the expected bounds. Compression failures are diagnostic only; current logging should continue.

## Not Implemented

1. Forward proxying.
2. Quotas and rate limits.
3. Wildcard/platform-domain ownership verification.
4. Native installers, package-manager distribution, and broader cross-platform service orchestration.
5. Ordinary-user self-service, quota/settings UI, log search, advanced alerts, backup/restore, and broader production hardening.
