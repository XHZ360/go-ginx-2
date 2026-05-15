# go-ginx-2

This is the new implementation target for the Simp-Frp/go-ginx design in `../docs`.

The repository currently contains a milestone-one runtime plus a first deployment baseline. It is not yet a complete production platform, but the core control plane, TCP/UDP/HTTP/HTTPS proxy paths, daemon startup helpers, reconnect/restart recovery, and a reproducible bundle plus `systemd` service template workflow are implemented and covered by local tests.

## Current Capabilities

- Go module pinned to `github.com/quic-go/quic-go v0.59.1`.
- cgo-free SQLite persistence through repository interfaces.
- Strict server/client config loading and validation.
- Core domain models for users, clients, proxies, credentials, and audit events.
- QUIC and TCP+TLS control handshakes with client authentication and certificate verification.
- Proxy snapshot sync after successful authentication.
- Heartbeat and session tracking with latest-session replacement.
- TCP proxy MVP over QUIC streams or framed TCP+TLS substreams.
- UDP proxy MVP over QUIC streams or framed TCP+TLS substreams with per-source sessions.
- HTTP proxy MVP over QUIC streams or framed TCP+TLS substreams, routed by `Host`.
- HTTPS proxy MVP using SNI passthrough, file-backed TLS termination, or managed ACME DNS-01 TLS termination with SNI certificate selection over QUIC streams or framed TCP+TLS substreams.
- Non-interactive admin setup CLI for milestone-one resource seeding.
- Admin CLI commands for issuing, renewing, and inspecting managed HTTPS certificates.
- Restart-surviving proxy stats for TCP, UDP, and HTTP traffic, backed by SQLite flushes.
- `goginx-server` starts SQLite, QUIC control, optional TCP+TLS fallback, TCP entries, HTTP entry, and optional HTTPS entry for SNI passthrough or file-backed TLS termination from config.
- `goginx-client` authenticates, reads proxy snapshots, sends heartbeats, serves proxy streams, and retries transient control-plane failures with reconnect backoff.
- `goginx-admin build-deploy-bundle` creates a reproducible deployment bundle with sample config, environment examples, and `systemd` service templates.
- An administrator-only management listener is available through session-based same-origin admin API endpoints for login, logout, session bootstrap, and GraphQL management operations.

## Commands

Run the full validation suite:

```powershell
$env:CGO_ENABLED="0"
go test ./...
go build ./cmd/goginx-server ./cmd/goginx-client ./cmd/goginx-admin
```

Run focused runtime and E2E tests:

```powershell
$env:CGO_ENABLED="0"
go test ./internal/control
go test ./internal/daemon
go test ./internal/proxy/tcp
go test ./internal/proxy/udp
go test ./internal/proxy/http
```

Seed a local SQLite database with the admin CLI:

```powershell
$env:CGO_ENABLED="0"
go run ./cmd/goginx-admin create-user -db ./.tmp/go-ginx.db -id user-1 -username alice
go run ./cmd/goginx-admin create-client -db ./.tmp/go-ginx.db -id client-1 -user user-1 -name home -credential secret
go run ./cmd/goginx-admin create-tcp-proxy -db ./.tmp/go-ginx.db -id tcp-1 -user user-1 -client client-1 -name ssh -port 10022 -target-host 127.0.0.1 -target-port 22
go run ./cmd/goginx-admin create-udp-proxy -db ./.tmp/go-ginx.db -id udp-1 -user user-1 -client client-1 -name dns -port 10053 -target-host 127.0.0.1 -target-port 53
go run ./cmd/goginx-admin create-http-proxy -db ./.tmp/go-ginx.db -id web-1 -user user-1 -client client-1 -name web -host app.example.com -target-host 127.0.0.1 -target-port 8080
go run ./cmd/goginx-admin create-https-proxy -db ./.tmp/go-ginx.db -id secure-1 -user user-1 -client client-1 -name secure -host secure.example.com -target-host 127.0.0.1 -target-port 8443
go run ./cmd/goginx-admin create-https-proxy -db ./.tmp/go-ginx.db -id secure-term-1 -user user-1 -client client-1 -name secure-term -host term.example.com -target-host 127.0.0.1 -target-port 8080 -cert-file data/certs/term.crt -key-file data/certs/term.key
```

Build the first supported deployment bundle:

```powershell
$env:CGO_ENABLED="0"
go run ./cmd/goginx-admin build-deploy-bundle -output ./.tmp/linux-systemd-bundle -goos linux -goarch amd64 -install-root /opt/go-ginx
```

More detailed flows are documented in `docs/milestone-one-e2e.md`, `docs/daemon-runtime.md`, and `docs/examples/admin-seed-sqlite.md`.

## Runtime Config Fields

Server config now requires runtime TLS and proxy listener fields in addition to the existing SQLite path:

```json
{
  "control_quic_listen": "127.0.0.1:8443",
  "control_tls_listen": "127.0.0.1:9443",
  "control_tls_cert_file": "data/certs/control.crt",
  "control_tls_key_file": "data/certs/control.key",
  "tcp_entry_host": "0.0.0.0",
  "http_entry_listen": "0.0.0.0:8081",
  "https_entry_listen": "0.0.0.0:8444",
  "sqlite_path": "data/go-ginx.db"
}
```

Managed certificate automation is optional and requires additional server config:

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

When `acme_enabled` is true, the server loads the Cloudflare API token from the configured environment variable, stores managed certificate files under `certificate_dir/managed/<host>/`, renews certificates inside the renewal window, hot reloads new certificates for future TLS handshakes, and retains the previous certificate pair for rollback. SQLite stores lifecycle metadata and file paths only.

Managed certificate CLI examples:

```powershell
$env:CF_DNS_API_TOKEN="<cloudflare-token>"
go run ./cmd/goginx-admin issue-managed-certificate -db ./.tmp/go-ginx.db -proxy secure-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
go run ./cmd/goginx-admin renew-managed-certificate -db ./.tmp/go-ginx.db -proxy secure-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
go run ./cmd/goginx-admin managed-certificate-status -db ./.tmp/go-ginx.db -proxy secure-1 -certificate-dir data/certs -acme-account-email ops@example.com -acme-terms-accepted
```

Client config requires the server CA used to verify the control certificate. `server_tls_address` is optional; when set and both protocols are allowed, the client falls back from QUIC to TCP+TLS for the control channel and framed proxy substreams. TCP+TLS fallback uses one TCP connection, so multiplexed streams can experience normal TCP head-of-line effects. `reconnect` controls client retry backoff after transient dial or runtime failures.

```json
{
  "server_address": "127.0.0.1:8443",
  "server_tls_address": "127.0.0.1:9443",
  "server_name": "localhost",
  "server_ca_file": "data/certs/ca.crt",
  "client_id": "client-1",
  "credential": "secret",
  "allowed_protocols": ["quic", "tcp_tls"],
  "reconnect": {
    "initial_delay": 1000000000,
    "max_delay": 30000000000
  }
}
```

Administrator management access is optional and requires an additional server setting pointing at a protected credential file:

```json
{
  "admin_listen": "127.0.0.1:8080",
  "admin_credentials_file": "config/admin-creds.json"
}
```

The credentials file stores administrator usernames and bcrypt password hashes:

```json
{
  "administrators": [
    {
      "username": "admin",
      "password_hash": "$2a$10$replace.with.bcrypt.hash"
    }
  ]
}
```

When enabled, the admin listener is API-only and is expected to run behind TLS. Browser-facing administrator access uses login-created server-managed sessions backed by `admin_credentials_file`, plus a session bootstrap endpoint and CSRF-protected mutation flow. The currently exposed management GraphQL scope remains administrator-only and includes a cumulative dashboard summary, user management, client list/detail, full reverse-proxy CRUD plus lifecycle actions, managed-certificate status/issue/renew, and a minimal recent audit list. Legacy server-rendered admin pages and the browser-facing legacy `/graphql` route are not served in this slice.

## Current Limitations

- The first supported deployment model is a reproducible bundle plus `systemd` service templates; native installers and non-`systemd` supervisors are not implemented yet.
- TCP, UDP, HTTP, and HTTPS proxy behavior is covered through package tests and external process smoke tests.
- Forward proxy, quotas, rate limiting, ordinary-user self-service, alerts, backup/restore tooling, capacity validation, wildcard/platform-domain ownership verification, and broader production operations docs are not implemented yet.

## Next Steps

1. Continue closing product gaps: limits, admin API/UI, backup/restore, capacity validation, and broader production operations.
