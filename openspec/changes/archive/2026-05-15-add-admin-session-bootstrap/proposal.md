## Why

`go-ginx-2` already has the architectural direction for a separated administrator frontend, but the live management backend still authenticates every browser request with HTTP Basic Auth at the transport edge. The current implementation in `internal/adminapi/server.go` protects both `/` and `/graphql` with one Basic Auth middleware and has no session lifecycle, no logout semantics, no bootstrap endpoint, and no CSRF model for browser-driven mutations.

That is sufficient for the narrow V1 server-rendered admin surface, but it now conflicts with the target architecture. A dedicated admin frontend cannot rely on repeated Basic Auth prompts or on storing administrator credentials in browser-accessible state, and the inline server-rendered management UI is no longer a desired long-term fallback. The next implementation step must establish a server-managed administrator session model, a same-origin bootstrap contract, and remove the server-rendered admin surface so the management plane becomes API-first in practice.

## What Changes

- Introduce the first implementation slice for administrator session authentication and session bootstrap.
- Keep administrator credentials file-backed, but convert browser-facing administrator access from repeated Basic Auth to a server-managed session cookie.
- Add dedicated login, logout, session bootstrap, and session-authenticated GraphQL entrypoints for the separated frontend.
- Add CSRF protection for cookie-backed browser mutations.
- Remove browser-facing Basic Auth from the management surface in this slice instead of carrying a dual-auth migration path.
- Remove the server-rendered admin pages, inline template rendering path, and browser-facing form workflow from the management backend.
- Make the admin listener API-only in this slice so legacy browser-facing admin routes are no longer served.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `admin-resource-management`: turn the session/bootstrap portion of the separated admin-console design into a concrete implementation slice with explicit route topology, session lifecycle behavior, and removal of the server-rendered admin surface.

## Impact

- Affected systems: administrator authentication, admin HTTP routing, GraphQL transport entrypoints, browser-session lifecycle, CSRF handling, and legacy admin-surface removal.
- Affected code areas: `internal/adminapi`, future admin-session helper package or module, auth context propagation, tests, and operator documentation.
- No frontend page implementation is included in this change; it establishes the auth/bootstrap foundation and removes the old server-rendered admin surface so future frontend work builds on an API-only backend with explicit removed-route behavior.
