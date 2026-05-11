# go-ginx-2

This is the new implementation target for the Simp-Frp/go-ginx design in `../docs`.

The repository currently contains a milestone-one MVP foundation. It is not a complete production daemon yet, but the core control plane and TCP/HTTP proxy paths are implemented and covered by local end-to-end tests.

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

## Commands

Run the full validation suite:

```powershell
$env:CGO_ENABLED="0"
go test ./...
go build ./cmd/goginx-server ./cmd/goginx-client ./cmd/goginx-admin
```

Run focused E2E tests:

```powershell
$env:CGO_ENABLED="0"
go test ./internal/control
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

## Current Limitations

- `goginx-server` and `goginx-client` currently load and validate config only; they do not yet wire the long-running control/proxy runtime.
- TCP and HTTP proxy behavior is runnable through package APIs and verified by tests, not yet exposed as complete daemon commands.
- UDP, HTTPS, TCP+TLS fallback, forward proxy, quotas, rate limiting, GraphQL, admin UI, ACME/Cloudflare DNS, persistent stats, alerts, and production deployment docs are not implemented yet.

## Next Steps

1. Wire `goginx-server` and `goginx-client` into runnable daemon modes.
2. Add basic runtime configuration for control listener, TCP entries, and HTTP entry.
3. Persist or periodically flush stats if milestone-one needs restart survival.
4. Add deployment and troubleshooting docs after daemon wiring exists.
