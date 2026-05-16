## Why

`go-ginx-2` now has the session-authenticated, API-only admin listener shape and the separated frontend direction, but the frontend-facing GraphQL business contract for administrator pages is still only partially implied across the current implementation and earlier design work. The next frontend and backend work needs one blueprint that captures the intended page-oriented query/mutation shapes, resource semantics, polling expectations, validation/error behavior, and listener-admission rules before code starts evolving independently on both sides.

This change captures that contract blueprint without changing application behavior. It keeps the current same-origin session endpoints and API-only admin backend direction, but makes the intended GraphQL resource semantics explicit enough for frontend page design, backend schema work, and acceptance-test planning.

## What Changes

- Define the admin backend contract as same-origin and API-only, with HTTP session endpoints at `/api/admin/login`, `/api/admin/session`, and `/api/admin/logout`, plus the GraphQL business entrypoint at `/api/admin/graphql`.
- Preserve the backend read/write split where `internal/adminquery` owns read models and `internal/admin` owns command use cases.
- Define a frontend-consumable GraphQL contract for dashboard, users, clients, proxies, certificates, and audit with page-oriented list/detail queries, pagination/filter/sort inputs, and input/payload mutation shapes.
- Capture resource-specific semantics for user lifecycle actions, managed client credentials, proxy lifecycle and listener admission, certificate lifecycle status, audit actor identity, and structured error handling.
- Record the V1 listener-admission model, polling expectations, and the current gap between database uniqueness and whole-runtime listener conflict detection.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `admin-resource-management`: define the administrator GraphQL contract blueprint expected by the separated admin frontend and the API-only management backend.

## Impact

- Affected systems: administrator GraphQL schema design, frontend page contracts, admin validation/error handling, listener admission semantics, and certificate/audit management UX contracts.
- Affected code areas: future work in `internal/adminapi`, GraphQL schema/resolvers, `internal/adminquery`, `internal/admin`, and frontend admin page data models.
- No application code or archived OpenSpec changes are modified by this change; it is design/spec capture only.
