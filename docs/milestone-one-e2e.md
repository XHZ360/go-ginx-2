# Milestone One E2E Validation

This document describes the current executable verification paths for the milestone-one MVP. These flows are test-backed and should be treated as the source of truth for the current daemon runtime.

## Full Suite

```powershell
$env:CGO_ENABLED="0"
go test ./...
go build ./cmd/goginx-server ./cmd/goginx-client ./cmd/goginx-admin
```

## Control Plane

The control-plane tests cover real QUIC and TCP+TLS listener/dialer behavior using test certificates:

```powershell
$env:CGO_ENABLED="0"
go test ./internal/control
```

Covered behavior:

- Client authenticates over QUIC or TCP+TLS with a stored credential hash.
- Server verifies TLS without an insecure skip path.
- Server registers the latest client session after successful authentication.
- Server sends a proxy snapshot after authentication.
- Client sends heartbeat updates and the session manager records them.
- TCP+TLS fallback exposes framed proxy substreams after authentication and proxy snapshot sync.
- Wrong credentials are rejected without registering a session.

## Daemon Runtime

The daemon tests verify that command-level startup helpers wire existing package APIs into a runnable runtime:

```powershell
$env:CGO_ENABLED="0"
go test ./internal/daemon
```

Covered behavior:

- Server startup opens SQLite, loads the control TLS certificate, starts QUIC/TCP+TLS control, starts HTTP/HTTPS entries, and starts TCP/UDP entries discovered from enabled proxies.
- Client startup dials the QUIC control listener or falls back to TCP+TLS, authenticates, reads the proxy snapshot, sends heartbeats, begins serving proxy streams, retries transient failures with reconnect backoff, reconnects after control-listener restarts, and stops immediately on authentication rejection.
- Server shutdown flushes cumulative proxy stats to SQLite so TCP/UDP/HTTP totals survive restart; active connection counts reset after restart.

## Deployment Bundle

The deployment bundle tests cover the reproducible packaging workflow and prove the packaged runtime can start and recover after daemon restart from the generated bundle layout:

```powershell
$env:CGO_ENABLED="0"
go test ./cmd/goginx-admin -run TestRunBuildsDeployBundle
go test ./internal/deploy
go test ./e2e -run TestDeployBundleRuntimeRestartRecovery -count=1
```

Covered behavior:

- `goginx-admin build-deploy-bundle` creates a stable bundle with binaries, config, environment examples, data/log directories, and `systemd` unit files.
- The packaged runtime binaries start successfully from the generated bundle layout.
- A packaged client reconnects after the packaged server process restarts.

## Admin API/UI

The admin management tests cover administrator authentication, GraphQL queries and mutations, query-model aggregation, and external process smoke for the V1 management surface:

```powershell
$env:CGO_ENABLED="0"
go test ./internal/admin
go test ./internal/adminquery
go test ./internal/adminapi
go test ./e2e -run TestExternalProcessesAdminAPIUI -count=1
```

Covered behavior:

- Administrator credentials are loaded from protected configuration independent of SQLite product users.
- HTTP Basic Auth rejects unauthenticated or invalid administrator access.
- The V1 GraphQL surface exposes dashboard, user, client, proxy, managed-certificate, and recent-audit operations through thin resolvers.
- Query models combine persisted configuration with runtime session state and cumulative stats.
- The server-rendered management UI and GraphQL endpoint both work through the real `goginx-server` binary.

## External Process Smoke

The external process smoke tests build real `goginx-server` and `goginx-client` binaries, write temporary JSON configs, generate temporary TLS certificates, seed SQLite, start both processes, and verify TCP, UDP, HTTP, and HTTPS passthrough traffic through the daemon path.

```powershell
$env:CGO_ENABLED="0"
go test ./e2e -run "TestExternalProcessesProxy(TCP|UDP|HTTP|HTTPS)$" -count=1
```

Covered behavior:

- Real command binaries start from config files instead of package APIs.
- The server opens SQLite, QUIC control, the HTTP entry, and a TCP entry discovered from SQLite.
- The client authenticates with the server certificate verified by a generated CA file.
- External TCP traffic reaches a local echo origin through server TCP entry -> client stream -> local target.
- External UDP traffic reaches a local echo origin through server UDP entry -> client stream -> local target.
- External HTTP traffic reaches a local HTTP origin through server HTTP entry -> client stream -> local target.
- External HTTPS passthrough traffic reaches a local TLS origin through server HTTPS entry -> client stream -> local target using SNI routing.
- Child server and client processes are terminated by the test cleanup path.

## TCP Proxy

The TCP proxy E2E test starts a local echo server, authenticates a client, starts a TCP entry, and verifies external TCP traffic reaches the echo target through the client stream.

```powershell
$env:CGO_ENABLED="0"
go test ./internal/proxy/tcp
```

Covered behavior:

- TCP entry maps the listening port to a TCP proxy configuration.
- Server opens a client stream to the latest online client session.
- Client connects to the configured local TCP target.
- Bytes are copied in both directions.
- Stats record connection count, active connection count, upload bytes, and download bytes; cumulative totals are flushed to SQLite by the daemon runtime.

## HTTP Proxy

The HTTP proxy E2E test starts an `httptest` origin, authenticates a client, starts an HTTP entry, and verifies an external HTTP request reaches the origin through the client stream.

```powershell
$env:CGO_ENABLED="0"
go test ./internal/proxy/http
```

Covered behavior:

- HTTP entry routes by `Host` through the HTTP proxy repository lookup.
- Server opens a client stream to the latest online client session.
- Client forwards the HTTP request to the configured local HTTP target.
- Response status, headers, and body return to the external caller.
- Stats record request count, status-code distribution, upload bytes, download bytes, and error count; cumulative totals are flushed to SQLite by the daemon runtime.

## UDP Proxy

The UDP proxy E2E test starts a local UDP echo server, authenticates a client, starts a UDP entry, and verifies external UDP packets reach the echo target through the client stream.

```powershell
$env:CGO_ENABLED="0"
go test ./internal/proxy/udp
```

Covered behavior:

- UDP entry maps the listening port to a UDP proxy configuration.
- Server keeps a stream-backed session per external source address until idle cleanup.
- Client forwards datagram frames to the configured local UDP target.
- Responses return to the original external source address.
- Stats record packet count, upload bytes, download bytes, and error count; cumulative totals are flushed to SQLite by the daemon runtime.

## HTTPS Proxy

The HTTPS proxy E2E test covers both passthrough and termination. Passthrough starts a local TLS echo origin and verifies encrypted traffic reaches the origin through the client stream while the server routes by SNI. Termination uses a file-backed certificate/key pair selected by SNI, terminates public TLS on the server, and forwards the decrypted HTTP request to the configured local HTTP target.

```powershell
$env:CGO_ENABLED="0"
go test ./internal/proxy/https
```

Covered behavior:

- HTTPS entry reads the TLS ClientHello SNI for proxy and certificate selection.
- HTTPS entry routes by SNI through the HTTPS proxy repository lookup.
- Passthrough proxies without `cert_file`/`key_file` preserve encrypted bytes to the configured local TLS target.
- Termination proxies with `cert_file`/`key_file` complete the public TLS handshake and forward HTTP to the configured local HTTP target.

## Managed Certificates

The managed certificate tests cover ACME DNS-01 helper behavior, certificate lifecycle service behavior, admin CLI certificate commands, and daemon renewal wiring:

```powershell
$env:CGO_ENABLED="0"
go test ./internal/certmanager
go test ./cmd/goginx-admin -run TestRunManagesCertificates
go test ./internal/daemon -run TestStartServerRenewsManagedCertificates
```

Covered behavior:

- Cloudflare DNS provider loading rejects missing tokens and redacts token values from returned errors.
- Certificate lifecycle service issues managed certificates, records failures, renews expiring certificates, and preserves previous files for rollback.
- Admin CLI issue, renew, and status commands operate on managed HTTPS certificate records.
- Daemon startup wires the renewal loop so certificates inside the configured renewal window are renewed without restart.

## Admin CLI

The admin CLI is tested as a non-interactive SQLite seeding tool:

```powershell
$env:CGO_ENABLED="0"
go test ./cmd/goginx-admin -run TestRunCreatesResources
```

Covered behavior:

- Create a user.
- Create a client credential with a hashed credential.
- Create TCP and UDP proxy configurations.
- Persist resources into SQLite.

## What Is Not Covered Yet

- Forward proxy, quotas, rate limits, ordinary-user self-service, advanced observability search/alerts, backup/restore, capacity validation, and broader deployment automation.
