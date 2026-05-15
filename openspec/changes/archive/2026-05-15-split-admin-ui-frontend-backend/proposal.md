## Why

`go-ginx-2` currently exposes the administrator management surface as a server-rendered HTML template embedded inside `internal/adminapi/server.go`. That V1 shape was sufficient to prove the first dashboard, resource lists, and control actions, but it now blocks the next stage of UI evolution. The current approach couples presentation, page routing, polling, and form behavior to the backend transport layer, makes richer interaction patterns awkward, and keeps browser-facing authentication tied to HTTP Basic Auth semantics that do not fit a modern management console.

The next management-plane step needs a clear architectural shift: a dedicated frontend application for the admin console, and an API-only backend responsible for authentication, query/mutation semantics, validation, and audit.

## What Changes

- Define the admin console target as a frontend-backend separated architecture instead of an inline server-rendered UI.
- Move the backend management surface toward API-only responsibilities and remove page rendering from the long-term design target.
- Replace browser-facing Basic Auth interaction with a session-based administrator login model suitable for a standalone frontend.
- Keep the current admin query and mutation split as the backend foundation, while extending the API contract for frontend list views, detail views, filtering, pagination, and runtime polling.
- Define a phased migration that allows the current V1 server-rendered UI to coexist temporarily until the separated frontend reaches feature parity for the scoped pages.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `admin-resource-management`: evolve the administrator UI contract from a server-rendered V1 surface into a separated frontend application backed by an API-first management plane.

## Impact

- Affected systems: administrator authentication, management API contract shape, browser routing, deployment topology, and frontend delivery.
- Affected code areas: `internal/adminapi`, future frontend workspace/module structure, auth/session middleware, GraphQL schema and transport boundaries, and deployment documentation.
- No product feature implementation is included in this change; it captures the architecture, requirements, and migration direction for the frontend-backend split.
