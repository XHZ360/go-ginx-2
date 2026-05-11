# Milestone One E2E Validation

This document describes the current executable verification paths for the milestone-one MVP. These flows are test-backed and should be treated as the source of truth for the current daemon runtime.

## Full Suite

```powershell
$env:CGO_ENABLED="0"
go test ./...
go build ./cmd/goginx-server ./cmd/goginx-client ./cmd/goginx-admin
```

## Control Plane

The control-plane tests cover real QUIC listener/dialer behavior using test certificates:

```powershell
$env:CGO_ENABLED="0"
go test ./internal/control
```

Covered behavior:

- Client authenticates over QUIC with a stored credential hash.
- Server verifies TLS without an insecure skip path.
- Server registers the latest client session after successful authentication.
- Server sends a proxy snapshot after authentication.
- Client sends heartbeat updates and the session manager records them.
- Wrong credentials are rejected without registering a session.

## Daemon Runtime

The daemon tests verify that command-level startup helpers wire existing package APIs into a runnable runtime:

```powershell
$env:CGO_ENABLED="0"
go test ./internal/daemon
```

Covered behavior:

- Server startup opens SQLite, loads the control TLS certificate, starts QUIC control, starts one HTTP entry, and starts TCP entries discovered from enabled TCP proxies.
- Client startup dials the QUIC control listener, authenticates, reads the proxy snapshot, sends heartbeats, and begins serving proxy streams.

## External Process Smoke

The external process smoke test builds real `goginx-server` and `goginx-client` binaries, writes temporary JSON configs, generates temporary TLS certificates, seeds SQLite, starts both processes, and verifies TCP echo traffic through the daemon path.

```powershell
$env:CGO_ENABLED="0"
go test ./e2e -run TestExternalProcessesProxyTCP -count=1
```

Covered behavior:

- Real command binaries start from config files instead of package APIs.
- The server opens SQLite, QUIC control, the HTTP entry, and a TCP entry discovered from SQLite.
- The client authenticates with the server certificate verified by a generated CA file.
- External TCP traffic reaches a local echo origin through server TCP entry -> QUIC client stream -> local target.
- Child server and client processes are terminated by the test cleanup path.

HTTP external process smoke is intentionally deferred; HTTP Host routing and response forwarding are already covered by package E2E tests.

## TCP Proxy

The TCP proxy E2E test starts a local echo server, authenticates a QUIC client, starts a TCP entry, and verifies external TCP traffic reaches the echo target through the QUIC client stream.

```powershell
$env:CGO_ENABLED="0"
go test ./internal/proxy/tcp
```

Covered behavior:

- TCP entry maps the listening port to a TCP proxy configuration.
- Server opens a QUIC stream to the latest online client session.
- Client connects to the configured local TCP target.
- Bytes are copied in both directions.
- In-memory stats record connection count, active connection count, upload bytes, and download bytes.

## HTTP Proxy

The HTTP proxy E2E test starts an `httptest` origin, authenticates a QUIC client, starts an HTTP entry, and verifies an external HTTP request reaches the origin through the QUIC client stream.

```powershell
$env:CGO_ENABLED="0"
go test ./internal/proxy/http
```

Covered behavior:

- HTTP entry routes by `Host` through the HTTP proxy repository lookup.
- Server opens a QUIC stream to the latest online client session.
- Client forwards the HTTP request to the configured local HTTP target.
- Response status, headers, and body return to the external caller.
- In-memory stats record request count, status-code distribution, upload bytes, download bytes, and error count.

## Admin CLI

The admin CLI is tested as a non-interactive SQLite seeding tool:

```powershell
$env:CGO_ENABLED="0"
go test ./cmd/goginx-admin -run TestRunCreatesResources
```

Covered behavior:

- Create a user.
- Create a client credential with a hashed credential.
- Create a TCP proxy configuration.
- Persist resources into SQLite.

## What Is Not Covered Yet

- HTTP external OS process smoke for `goginx-server` and `goginx-client`.
- UDP, HTTPS, TCP+TLS fallback, forward proxy, quotas, rate limits, persistent stats, GraphQL, admin UI, ACME, and deployment automation.
