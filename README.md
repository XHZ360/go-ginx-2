# go-ginx-2

This is the new implementation target for the Simp-Frp/go-ginx design in `../docs`.

The current baseline implements the milestone-one foundation:

- Go module with `github.com/quic-go/quic-go v0.59.1` pinned.
- cgo-free SQLite persistence through repository interfaces.
- Server and client config loading with strict validation.
- Core domain models for users, clients, proxies, credentials, and audit events.
- Initial tests for configuration, domain validation, and SQLite repository constraints.

Run verification with:

```powershell
$env:CGO_ENABLED="0"
go test ./...
go build ./cmd/goginx-server ./cmd/goginx-client
```

The next milestone is the QUIC control channel: authentication, heartbeat, session registry, and proxy configuration sync.
