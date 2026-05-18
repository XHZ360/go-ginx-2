# Daemon Runtime Deployment

This guide covers the implemented milestone-one daemon runtime plus the first supported deployment baseline: a reproducible bundle with `systemd` service templates for single-node Linux-style deployment. It does not describe native installers, non-`systemd` supervisors, backup/restore tooling, or other features that are still out of scope.

## Implemented Features

1. cgo-free SQLite persistence through repository interfaces.
2. QUIC and TCP+TLS control authentication with TLS and CA verification.
3. Proxy snapshots sent to clients after successful authentication.
4. Heartbeat and session tracking with latest-session replacement.
5. TCP reverse proxy over QUIC streams or framed TCP+TLS substreams.
6. UDP reverse proxy over QUIC streams or framed TCP+TLS substreams with per-source sessions and idle cleanup.
7. HTTP reverse proxy over QUIC streams or framed TCP+TLS substreams, routed by `Host`.
8. HTTPS reverse proxy SNI passthrough, file-backed TLS termination, or managed ACME DNS-01 TLS termination with SNI certificate selection over QUIC streams or framed TCP+TLS substreams.
9. Admin CLI commands for milestone-one users, clients, proxy records, and managed certificate issue/renew/status operations.
10. Daemon server and client runtime commands.
11. External process smoke tests for real server and client binaries.
12. SQLite-backed cumulative stats persistence for TCP, UDP, and HTTP traffic.
13. Reproducible deployment bundle generation for server, client, and admin binaries.
14. Checked-in `systemd` service templates for supervised server and client execution.
15. Configless server startup with managed `data/` state and generated control-channel TLS material.
16. One-time client join tokens that write managed client state for later no-`-config` startup.
17. Optional administrator-only management listener with session login, session bootstrap, logout, GraphQL operations, client enrollment, and same-origin dedicated frontend delivery from the runtime `admin-ui/` directory or an explicit frontend directory override.

## Seed SQLite

Seed the SQLite database before starting the daemon pair. The server reads users, client credentials, and enabled proxies from the configured `sqlite_path`.

Use the admin CLI flow in [admin-seed-sqlite.md](examples/admin-seed-sqlite.md) to create a user, client credential, TCP proxy, UDP proxy, HTTP proxy, HTTPS passthrough proxy, and HTTPS termination proxy.

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

Initialize the first administrator locally:

```powershell
./.tmp/goginx-admin.exe init-admin -id admin-1 -username admin -password "<password>"
```

Generate a client join token:

You can generate it from the admin UI Clients page with `Create join token`, or use the CLI:

```powershell
./.tmp/goginx-admin.exe create-client-join -id client-1 -user admin-1 -name home
```

During server startup, the server confirms a default join service host from `join_service_host`, the control listener host, a local interface address, or a loopback fallback. The startup log prints the confirmed host, source, and default control addresses. Set `GOGINX_JOIN_SERVICE_HOST` or `join_service_host` when clients must use a public DNS name or load-balancer address instead of the inferred host.

On the client host:

```powershell
./.tmp/goginx-client.exe join "<join-token>"
./.tmp/goginx-client.exe
```

The join command redeems the token through `/api/client/enroll`, writes `data/client-state.json`, writes `data/certs/server-ca.crt`, and subsequent client runs use that managed state. By default these paths are under the deployment root derived from the `goginx-client` binary location; when the binary is under `bin/`, the deployment root is the parent of `bin/`.

Managed startup accepts environment overrides for file-free deployments that need non-default ports, paths, or join defaults, including `GOGINX_ADMIN_LISTEN`, `GOGINX_CONTROL_QUIC_LISTEN`, `GOGINX_CONTROL_TLS_LISTEN`, `GOGINX_JOIN_SERVICE_HOST`, `GOGINX_HTTP_ENTRY_LISTEN`, `GOGINX_SQLITE_PATH`, `GOGINX_DATA_DIR`, and `GOGINX_CERTIFICATE_DIR`.

## Optional Server Config

Explicit JSON config remains supported for advanced deployments. Create `server.json` with fields from `internal/config/config.go` when defaults and environment overrides are not enough:

```json
{
  "admin_listen": "127.0.0.1:8080",
  "admin_frontend_dir": "web/admin",
  "control_quic_listen": "127.0.0.1:8443",
  "control_tls_listen": "127.0.0.1:9443",
  "join_service_host": "control.example.com",
  "control_tls_cert_file": "data/certs/control.crt",
  "control_tls_key_file": "data/certs/control.key",
  "tcp_entry_host": "127.0.0.1",
  "http_entry_listen": "127.0.0.1:8081",
  "https_entry_listen": "127.0.0.1:8444",
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
  "log_retention_days": 7
}
```

`control_quic_listen` is the primary control listener. `control_tls_listen` enables TCP+TLS fallback for authentication, proxy snapshots, heartbeats, and framed proxy substreams. TCP+TLS fallback is reliable but uses one TCP connection, so slow streams can cause normal TCP head-of-line effects.

When `admin_frontend_dir` is empty, the server loads the dedicated admin frontend from `admin-ui/` under the deployment root. If the server binary is under `bin/`, the deployment root is the parent of `bin/`; otherwise it is the binary directory. `admin_frontend_dir` is optional and, when set, must point to another directory containing the dedicated admin frontend build output, including `index.html`. The admin listener serves those browser routes and assets on the same origin as the administrator API while keeping `/api/admin/*` reserved for backend endpoints.

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
  }
}
```

Duration fields are JSON numbers in nanoseconds because the config structs use `time.Duration`. `reconnect.initial_delay` and `reconnect.max_delay` control client retries after transient dial or runtime failures. Authentication rejection still returns immediately instead of retrying forever.

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
npm ci
npm run build
Set-Location ..
$env:CGO_ENABLED="0"
go run ./cmd/goginx-admin build-deploy-bundle -output ./.tmp/linux-systemd-bundle -goos linux -goarch amd64 -install-root /opt/go-ginx
```

The core bundle layout is stable and contains:

1. `bin/` with `goginx-server`, `goginx-client`, and `goginx-admin`.
2. `admin-ui/` with the management frontend build output used by default at runtime.
3. `config/` with optional sample `server.json`, `client.json`, and environment examples.
4. `data/` with SQLite and certificate directories.
5. `logs/` for operator-managed log files.
6. `systemd/` with rendered `goginx-server.service` and `goginx-client.service` units.
7. `config/admin-credentials.json.example` for the optional administrator management surface.

Deployments use the install-root `admin-ui/` directory by default. Rebuild the frontend before packaging; `build-deploy-bundle` fails if `admin-ui/dist` is missing. Deployments that need a custom frontend may include built assets in a different directory inside the install root, such as `web/admin/`, and set `admin_frontend_dir` through explicit config.

Run the server from the desired state directory:

```powershell
./.tmp/goginx-server.exe
```

After `goginx-client join <token>`, run the client:

```powershell
./.tmp/goginx-client.exe
```

If the client exits with a missing `data/client-state.json` error, it was started before the join flow wrote managed state, or a custom path was used inconsistently. Run `goginx-client join <new-token>`, confirm `data/client-state.json` exists under the deployment root, then start the client service.

For the supported `systemd` deployment model:

1. Copy the generated bundle to the rendered install root, such as `/opt/go-ginx`.
2. Start the server service and initialize the first administrator.
3. Install `systemd/goginx-server.service` and `systemd/goginx-client.service` into `/etc/systemd/system/`.
4. Run `systemctl daemon-reload`.
5. Start the units with `systemctl enable --now goginx-server goginx-client`.
6. Restart after config or binary updates with `systemctl restart goginx-server goginx-client`.

The server starts SQLite, the QUIC control listener, the optional TCP+TLS fallback listener, the HTTP entry listener, the optional HTTPS entry listener, TCP entry listeners, and UDP entry listeners for enabled proxies found in SQLite. The HTTPS entry uses SNI to choose the proxy. If that proxy has `cert_file` and `key_file`, the server terminates TLS and forwards the decrypted HTTP request to the configured local HTTP target; otherwise it checks for an active managed certificate for that HTTPS host and uses it if available. If neither static nor managed certificate material is active, it preserves passthrough behavior and forwards encrypted bytes to the client target. The client authenticates, receives the proxy snapshot, sends heartbeats, serves proxy streams to configured local targets over QUIC or framed TCP+TLS fallback, and retries transient control-plane failures using the configured reconnect backoff.

When configless server startup is used, the server starts an administrator-only management listener on `admin_listen` and authenticates SQLite administrator users initialized by `init-admin`. `admin_credentials_file` remains a compatibility override. The `/api/admin/*` namespace remains reserved for management API behavior and exposes:

1. `POST /api/admin/login` for administrator session creation.
2. `GET /api/admin/session` for browser session bootstrap.
3. `POST /api/admin/logout` for session invalidation.
4. `POST /api/admin/graphql` for administrator GraphQL queries and mutations.

The same listener serves the dedicated admin frontend on non-API `GET` and `HEAD` routes. Browser routes such as `/`, `/login`, `/users`, `/clients`, `/proxies`, `/certificates`, and `/audit` resolve through the frontend shell, deep links serve `index.html`, and real asset files are served directly from the selected frontend directory. Missing asset-like paths such as `/assets/missing.js` still return `404 Not Found`. `admin_frontend_dir` is an override for development or custom deployments rather than a baseline requirement.

The first session-authenticated management slice is intentionally narrow: it excludes ordinary-user self-service, quota editing, log search, domain lifecycle management, advanced alerts, and realtime subscriptions. The legacy server-rendered admin UI and browser-facing `/graphql` route are not served in this slice.

When `acme_enabled` is true, the server reads the Cloudflare token from `acme_cloudflare_token_env`, stores managed certificates under `certificate_dir/managed/<host>/`, renews expiring managed certificates inside `acme_renewal_window`, hot reloads future TLS handshakes, and keeps the previous active certificate pair for rollback. SQLite stores only lifecycle metadata and file paths.

Managed certificate admin commands:

```powershell
$env:CF_DNS_API_TOKEN="<cloudflare-token>"
./.tmp/goginx-admin.exe issue-managed-certificate -db data/go-ginx.db -proxy https-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
./.tmp/goginx-admin.exe renew-managed-certificate -db data/go-ginx.db -proxy https-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
./.tmp/goginx-admin.exe managed-certificate-status -db data/go-ginx.db -proxy https-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
```

## Troubleshooting

1. Unknown config fields: Config loading rejects unknown JSON fields. Remove fields that are not defined in `internal/config/config.go`.
2. Missing TLS files: `control_tls_cert_file`, `control_tls_key_file`, and `server_ca_file` must point to readable files from the process working directory, unless absolute paths are used.
3. CA or SNI mismatch: The client verifies the server certificate with `server_ca_file` and matches it against `server_name`. Use a CA file that signed the control certificate and a `server_name` present in the certificate SANs.
4. Auth rejected: Confirm `client_id` exists in SQLite and `credential` matches the value seeded by `goginx-admin create-client`.
5. No TCP or UDP listener: The server only starts TCP and UDP listeners for enabled proxies stored in SQLite. Check the proxy `client_id`, entry port, status, and the server `tcp_entry_host`.
6. HTTP Host mismatch: HTTP routing uses the request `Host`. Send requests with the host seeded for the HTTP proxy, such as `app.example.com`.
7. Target unreachable: The client connects to each proxy target from the client machine. Confirm the target host and port are reachable there, not only from the server.
8. UDP response missing: UDP routing keeps one QUIC stream per external source address until idle cleanup. Make sure replies return to the same source address and that the local UDP target responds before the session idles out.
9. HTTPS SNI mismatch: HTTPS routing uses the TLS ClientHello SNI value. Connect with a server name matching the seeded HTTPS proxy host, such as `secure.example.com`.
10. HTTPS termination certificate errors: `cert_file` and `key_file` must point to a certificate/key pair valid for the HTTPS proxy host. If either field is omitted, the proxy uses an active managed certificate when present; otherwise it runs in passthrough mode.
11. Stats flush and shutdown: TCP, UDP, and HTTP cumulative totals are flushed to SQLite during daemon shutdown. Stop the server cleanly with `SIGINT` or `SIGTERM` when you need the latest totals persisted. Active connection counts are runtime state and reset after restart.
12. Managed certificate issuance fails immediately if the Cloudflare token environment variable is missing, the proxy host is not delegated to Cloudflare, or the process cannot reach the ACME directory or Cloudflare API.
13. Client reconnect loops: transient dial or runtime failures now retry using the configured reconnect backoff, including control-listener outages or daemon restarts. Authentication rejection still exits immediately, so re-check the stored credential instead of waiting for automatic recovery.
14. `systemd` install paths: the generated service files assume the rendered `install_root` passed to `build-deploy-bundle`. Rebuild the bundle or edit the unit files if you deploy somewhere other than that path.
15. Upgrade and rollback: replace the bundle contents in the install root, including `admin-ui/`, then restart the units. Roll back by restoring the previous bundle directory contents and restarting the same units.
16. Administrator management credentials: `admin_credentials_file` must point to a readable JSON file containing administrator usernames and bcrypt password hashes. The file is separate from SQLite users and should be readable only by the service account.
17. Management transport protection: the admin listener uses session-authenticated same-origin API routes and is expected to run behind TLS. Local loopback access is accepted for development and automated tests; loopback testing may issue non-`Secure` cookies because browsers and HTTP clients do not send `Secure` cookies over plain `http://127.0.0.1` development traffic.
18. Configless port conflicts: set `GOGINX_ADMIN_LISTEN`, `GOGINX_CONTROL_QUIC_LISTEN`, `GOGINX_CONTROL_TLS_LISTEN`, or `GOGINX_HTTP_ENTRY_LISTEN` to free addresses, or switch to explicit JSON config.
19. Managed control TLS state: if `data/certs/control-ca.crt`, `control.crt`, or `control.key` is missing or corrupted, stop the server and restore the set from backup; deleting the set forces regeneration and breaks existing joined clients until they join again.
20. Missing administrator bootstrap: configless management login has no default password. Run `goginx-admin init-admin` before logging in. By default the CLI writes to `data/go-ginx.db` under the deployment root derived from the `goginx-admin` binary location; use `-db` when targeting a custom server SQLite path.
21. Join token failures: expired, already-used, tampered, or revoked join tokens are rejected by `/api/client/enroll`; generate a new token with `goginx-admin create-client-join`.
22. Client managed state damage: if `data/client-state.json` or `data/certs/server-ca.crt` is missing on the client host, run `goginx-client join <new-token>` again.

## Not Implemented

1. Forward proxying.
2. Quotas and rate limits.
3. Wildcard/platform-domain ownership verification.
4. Native installers and non-`systemd` service managers.
5. Ordinary-user self-service, quota/settings UI, log search, advanced alerts, backup/restore, and broader production hardening.
