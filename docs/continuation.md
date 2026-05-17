# Continuation Notes

This file records the current implementation state so work can resume after a restart.

## Current State

- Repository: `go-ginx-2`
- Branch: `master`
- Latest commit: `c6e5e24 docs: document managed certificate automation`
- Working tree before this note: dirty; uncommitted TCP+TLS mux, UDP runtime, stats persistence, and smoke-test updates are present
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

8. `29b3801 feat: wire daemon startup helpers`
   - Wired server startup to SQLite, QUIC control, TCP entries, and HTTP entry.
   - Wired client startup to authenticate, read proxy snapshots, send heartbeats, and serve proxy streams.
   - Added daemon startup helper tests.

9. `9f82a93 test: add external process TCP smoke`
   - Added real binary smoke coverage for TCP proxy traffic through server and client processes.

10. `47f03ed test: add HTTP external process smoke`
    - Added real binary smoke coverage for HTTP proxy traffic through server and client processes.

11. `0510a7c feat: persist managed certificate metadata`
    - Added managed certificate lifecycle metadata models and repository plumbing.

12. `fff3b0d feat: persist managed certificate records`
    - Added SQLite-backed managed certificate record persistence and lifecycle updates.

13. `0caf2d6 feat: add ACME server config`
    - Added ACME runtime config fields and server-side validation/loading.

14. `d7d62cb feat: support managed HTTPS certificate resolution`
    - Added managed certificate lookup for HTTPS SNI selection without breaking static certificate or passthrough behavior.

15. `7b18ca1 feat: add ACME certificate manager`
    - Added DNS-01 ACME issuance, validation, file activation, and rollback-preservation plumbing.

16. `39d73a0 feat: add managed certificate admin commands`
    - Added admin CLI issue, renew, and status commands for managed HTTPS certificates.

17. `c4ef7e6 feat: renew managed certificates in daemon`
    - Added daemon renewal loop support for managed certificates inside the configured renewal window.

18. `c6e5e24 docs: document managed certificate automation`
    - Updated README, runtime docs, and local flows for managed certificate operations.

19. Current workspace update: add HTTPS SNI passthrough MVP
    - Added HTTPS proxy host lookup, admin CLI seeding, daemon listener wiring, and SNI passthrough entry.
    - Added package and external-process smoke coverage for HTTPS passthrough traffic.

20. Current workspace update: add TCP+TLS control fallback
    - Added TCP+TLS control listener and client fallback for authentication, proxy snapshot delivery, and heartbeats.

21. Current workspace update: add TCP+TLS proxy-stream multiplexing
    - Added framed TCP+TLS logical substreams for proxy traffic over the fallback connection.
    - Added control and daemon coverage for proxy traffic over TCP+TLS fallback.

22. Current workspace update: add HTTPS TLS termination
    - Added file-backed HTTPS proxy certificates selected by SNI.
    - HTTPS proxies with `cert_file`/`key_file` terminate public TLS and forward HTTP to the client target; proxies without certificate files keep passthrough behavior.

23. Current workspace update: add UDP runtime, cumulative stats persistence, and smoke coverage
    - Added UDP entry listeners with per-source sessions and idle cleanup over QUIC or framed TCP+TLS substreams.
    - Added cumulative TCP/UDP/HTTP stats load-and-flush helpers plus HTTPS and UDP external-process smoke coverage.

24. Current workspace update: honor client reconnect backoff
    - Wired `client.reconnect` into `goginx-client` so transient dial or runtime failures retry with bounded backoff.
    - Added daemon tests proving delayed server startup and control-listener restarts recover automatically while authentication rejection still exits immediately.
    - Updated control listener shutdown to close active QUIC and TCP+TLS control connections so clients detect daemon restarts promptly.

25. Current workspace update: add deployment bundle and `systemd` baseline
    - Added `goginx-admin build-deploy-bundle` to generate a reproducible bundle with binaries, sample config, env examples, and rendered `systemd` units.
    - Added deployment bundle tests for layout generation and packaged runtime restart recovery.

26. Current workspace update: add administrator-only management surface
    - Added config-file-backed administrator credentials with HTTP Basic Auth for a first admin-only GraphQL and server-rendered UI surface.
    - Added dedicated admin query models, user-management password support, reverse-proxy lifecycle mutations, and minimal recent-audit visibility.
    - Added package, daemon, and external-process tests covering authenticated management access and bundled runtime behavior.

27. Current workspace update: replace server-rendered admin with session-authenticated admin API
    - Replaced browser-facing administrator Basic Auth with session login, session bootstrap, logout, and session-authenticated GraphQL endpoints under `/api/admin/*`.
    - Removed the server-rendered administrator pages, browser-facing form-post workflow, and legacy `/graphql` route in favor of the dedicated frontend plus `/api/admin/*` split.
    - Added CSRF protection for session-authenticated admin mutations plus tests for login, bootstrap, logout, GraphQL access, and the no-frontend `404` fallback behavior.

28. Current workspace update: serve dedicated admin frontend from configured runtime assets
    - Added `admin_frontend_dir` server config support so the admin listener can load a built frontend directory with `index.html`.
    - Same-origin browser routes now serve the dedicated admin frontend when configured, while `/api/admin/*` remains reserved for login, session, logout, and GraphQL management APIs.
    - Missing asset-like paths still return `404`, and non-API browser routes still return `404` when no frontend directory is configured.

## Current Capabilities

- `CGO_ENABLED=0 go test ./...` passes.
- `CGO_ENABLED=0 go build ./cmd/goginx-server ./cmd/goginx-client ./cmd/goginx-admin` passes.
- QUIC and TCP+TLS control handshake, auth, heartbeat, proxy snapshot sync are implemented and tested.
- `goginx-client` retries transient control-plane failures with configured reconnect backoff and does not retry permanent authentication rejection.
- Server shutdown closes active control connections so clients can reconnect after daemon restarts.
- `goginx-admin` can generate a reproducible deployment bundle for the first supported `systemd`-based deployment model.
- Deployments can carry dedicated admin frontend assets inside the install root and point `admin_frontend_dir` at that directory without changing the `/api/admin/*` API contract.
- `goginx-server` can optionally expose an administrator-only management listener backed by configuration-file credentials, browser sessions, session bootstrap, logout, CSRF-protected mutations, a session-authenticated GraphQL endpoint, and same-origin dedicated frontend delivery when `admin_frontend_dir` is configured.
- TCP, UDP, HTTP, HTTPS SNI passthrough, and HTTPS TLS termination proxy paths work through daemon commands, package E2E tests, and external process smoke tests. TCP fallback proxy traffic is covered through daemon tests.
- `goginx-admin` can seed SQLite resources.
- Basic TCP/UDP/HTTP stats are implemented with SQLite-backed restart survival for cumulative totals.
- Daemon runtime deployment and troubleshooting guidance is documented in `docs/daemon-runtime.md`.

## Important Limitations

- Restart-surviving stats are intentionally cumulative only; active connection counts reset on process restart.
- No forward proxy, quotas/rate limits, ordinary-user self-service, advanced alerts/log search, backup/restore tooling, capacity validation, or broader deployment automation yet.

## Recommended Next Batch

Close the current runtime batch first, then move to production operations:

1. Commit the in-flight `TCP+TLS` mux, UDP runtime, persistent stats, and smoke-test batch after keeping validation green.
2. Continue implementing `add-service-supervision-and-packaging` until all bundle, `systemd`, and deployment-documentation tasks are committed.
3. Add follow-on production-operations work for backup/restore and capacity validation.
4. Validate with:
    - `CGO_ENABLED=0 go test ./...`
    - `CGO_ENABLED=0 go build ./cmd/goginx-server ./cmd/goginx-client ./cmd/goginx-admin`

## Working Rules To Continue

- Keep `go-ginx-1` as reference only.
- Continue making one focused commit per completed batch.
- Run LSP diagnostics, tests, and builds before each commit.
- Do not claim unsupported production features.
