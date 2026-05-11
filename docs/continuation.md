# Continuation Notes

This file records the current implementation state so work can resume after a restart.

## Current State

- Repository: `go-ginx-2`
- Branch: `master`
- Latest commit: `eb99805 docs: refresh milestone one MVP guide`
- Working tree before this note: clean
- Source of truth: `../docs/requirements.md` and `../docs/design.md`
- Reference only: `../go-ginx-1`

## Completed Batches

1. `3e3ef73 feat: initialize QUIC control foundation`
   - Initialized the Go module.
   - Added config/domain/store foundations.
   - Added cgo-free SQLite repositories.
   - Added QUIC control protocol, authentication, sessions, heartbeat, and real QUIC handshake tests.

2. `f8b36e9 feat: sync proxy snapshot after auth`
   - Added client-owned proxy lookup.
   - Sent `ProxySnapshot` after successful auth.
   - Added client snapshot read path and tests.

3. `ed8a37a feat: proxy TCP connections over QUIC`
   - Added `open_stream` frame.
   - Stored QUIC stream opener on active sessions.
   - Implemented TCP entry and client-side target stream handling.
   - Added TCP echo E2E test.

4. `6881689 feat: proxy HTTP requests over QUIC`
   - Added HTTP stream kind handling.
   - Implemented HTTP entry routed by `Host`.
   - Implemented client-side HTTP target forwarding.
   - Added HTTP E2E test with `httptest` origin.

5. `26c7b8a feat: add admin setup CLI`
   - Added `internal/admin` use cases.
   - Added `cmd/goginx-admin` commands for user/client/TCP proxy/HTTP proxy seeding.
   - Added audit events for successful create operations.

6. `d5a2dc6 feat: record basic proxy stats`
   - Added in-memory stats by proxy ID.
   - Wired TCP and HTTP entries to record basic traffic counters.
   - Added stats assertions to E2E tests.

7. `eb99805 docs: refresh milestone one MVP guide`
   - Updated README.
   - Added milestone-one E2E documentation.
   - Added admin CLI SQLite seed example.

## Current Capabilities

- `CGO_ENABLED=0 go test ./...` passes.
- `CGO_ENABLED=0 go build ./cmd/goginx-server ./cmd/goginx-client ./cmd/goginx-admin` passes.
- QUIC control handshake, auth, heartbeat, proxy snapshot sync are implemented and tested.
- TCP and HTTP proxy paths work through package APIs and E2E tests.
- `goginx-admin` can seed SQLite resources.
- Basic in-memory TCP/HTTP stats are implemented.

## Important Limitations

- `goginx-server` and `goginx-client` still only load config; they do not start daemon runtime yet.
- TCP/HTTP proxy runtime is test-backed through package APIs, not yet wired into command-line daemon modes.
- No UDP, HTTPS, TCP+TLS fallback, forward proxy, quotas/rate limits, GraphQL/admin UI, ACME, persistent stats, alerts, deployment automation, or production docs yet.

## Recommended Next Batch

Implement daemon wiring for milestone-one runtime:

1. Extend config structs with fields needed for runtime startup:
   - server SQLite path, control QUIC listen address, TLS cert/key paths or test/dev certificate mode, TCP entry addresses/ports, HTTP entry address.
   - client server address, SNI, credential, allowed protocols.
2. Wire `cmd/goginx-server` to:
   - open SQLite store,
   - create session manager and stats recorder,
   - start QUIC control listener,
   - start TCP entries for enabled TCP proxies,
   - start one HTTP entry for enabled HTTP proxies.
3. Wire `cmd/goginx-client` to:
   - dial and authenticate over QUIC,
   - read proxy snapshot,
   - send periodic heartbeat,
   - serve proxy streams.
4. Add integration tests around startup helpers rather than full OS process tests first.
5. Validate with:
   - `CGO_ENABLED=0 go test ./...`
   - `CGO_ENABLED=0 go build ./cmd/goginx-server ./cmd/goginx-client ./cmd/goginx-admin`

## Working Rules To Continue

- Keep `go-ginx-1` as reference only.
- Continue making one focused commit per completed batch.
- Run LSP diagnostics, tests, and builds before each commit.
- Do not claim unsupported production features.
