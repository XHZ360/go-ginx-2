# go-ginx-2

This is the new implementation target for the Simp-Frp/go-ginx design in `../docs`.

The repository currently contains a milestone-one MVP foundation. It is not a complete production daemon yet, but the core control plane, TCP/HTTP proxy paths, and daemon startup helpers are implemented and covered by local tests.

## Current Capabilities

- Go module pinned to `github.com/quic-go/quic-go v0.59.1`.
- cgo-free SQLite persistence through repository interfaces.
- Strict server/client config loading and validation.
- Core domain models for users, clients, proxies, credentials, and audit events.
- QUIC control handshake with client authentication and certificate verification.
- Proxy snapshot sync after successful authentication.
- Heartbeat and session tracking with latest-session replacement.
- TCP proxy MVP over QUIC streams.
- HTTP proxy MVP over QUIC streams, routed by `Host`.
- Non-interactive admin setup CLI for milestone-one resource seeding.
- In-memory proxy stats for TCP and HTTP traffic.
- `goginx-server` starts SQLite, QUIC control, TCP entries, and HTTP entry from config.
- `goginx-client` authenticates, reads proxy snapshots, sends heartbeats, and serves proxy streams.

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
go test ./internal/proxy/http
```

Seed a local SQLite database with the admin CLI:

```powershell
$env:CGO_ENABLED="0"
go run ./cmd/goginx-admin create-user -db ./.tmp/go-ginx.db -id user-1 -username alice
go run ./cmd/goginx-admin create-client -db ./.tmp/go-ginx.db -id client-1 -user user-1 -name home -credential secret
go run ./cmd/goginx-admin create-tcp-proxy -db ./.tmp/go-ginx.db -id tcp-1 -user user-1 -client client-1 -name ssh -port 10022 -target-host 127.0.0.1 -target-port 22
go run ./cmd/goginx-admin create-http-proxy -db ./.tmp/go-ginx.db -id web-1 -user user-1 -client client-1 -name web -host app.example.com -target-host 127.0.0.1 -target-port 8080
```

More detailed flows are documented in `docs/milestone-one-e2e.md` and `docs/examples/admin-seed-sqlite.md`.

## Runtime Config Fields

Server config now requires runtime TLS and proxy listener fields in addition to the existing SQLite path:

```json
{
  "control_quic_listen": "127.0.0.1:8443",
  "control_tls_cert_file": "data/certs/control.crt",
  "control_tls_key_file": "data/certs/control.key",
  "tcp_entry_host": "0.0.0.0",
  "http_entry_listen": "0.0.0.0:8081",
  "sqlite_path": "data/go-ginx.db"
}
```

Client config requires the server CA used to verify the QUIC control certificate:

```json
{
  "server_address": "127.0.0.1:8443",
  "server_name": "localhost",
  "server_ca_file": "data/certs/ca.crt",
  "client_id": "client-1",
  "credential": "secret"
}
```

## Current Limitations

- Daemon startup is wired, but production packaging, service supervision, and deployment docs are not implemented yet.
- TCP and HTTP proxy behavior is exposed through daemon commands and package E2E tests; full external process E2E is still pending.
- UDP, HTTPS, TCP+TLS fallback, forward proxy, quotas, rate limiting, GraphQL, admin UI, ACME/Cloudflare DNS, persistent stats, alerts, and production deployment docs are not implemented yet.

## Next Steps

1. Add external process smoke tests for `goginx-server` and `goginx-client`.
2. Persist or periodically flush stats if milestone-one needs restart survival.
3. Add deployment and troubleshooting docs after daemon wiring exists.
