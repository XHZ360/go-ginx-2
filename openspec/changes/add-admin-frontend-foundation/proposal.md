## Why

The admin backend now exposes a same-origin API-only management surface with dedicated session endpoints and a session-authenticated GraphQL contract for the confirmed administrator views. The admin frontend shell and page hierarchy have also been defined separately, including the protected route set, root-route direction, and page-level responsibilities.

The remaining gap before implementation is the frontend foundation itself: one approved application shape, request model, session-bootstrap flow, page-state model, polling ownership model, and delivery integration path that later implementation can follow without re-deciding core browser behavior while writing code.

Without this foundation change, frontend implementation would have to make cross-cutting decisions ad hoc about root-path delivery, route guarding, intended-destination restoration, CSRF-aware GraphQL mutations, shared loading and failure semantics, page refresh ownership, and same-origin build integration. Those choices affect every route and are more expensive to change once page code exists.

## What Changes

- Define the admin frontend foundation for root-path delivery on the admin origin while preserving `/api/admin/*` as the backend API namespace.
- Define auth-aware root-route behavior, protected-route bootstrap, `/login` behavior, and intended-destination restoration after successful login.
- Define the shared frontend request model across `/api/admin/login`, `/api/admin/session`, `/api/admin/logout`, and `/api/admin/graphql`, including CSRF handling for mutations.
- Define the shared page-state model for route bootstrap, summary pages, list pages, detail pages, and form or action flows, including distinct handling for auth expiry, resource not found, validation failure, and backend failure.
- Define page-scoped polling ownership and targeted post-mutation refresh behavior for the confirmed administrator routes.
- Define local development and production delivery expectations for serving the frontend under the same origin as the admin API.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `admin-resource-management`: define the frontend foundation and browser behavior model that the dedicated admin UI will use to consume the existing same-origin session and GraphQL contracts.

## Impact

- Affected systems: future admin frontend application structure, browser session bootstrap and guarded navigation, frontend GraphQL transport behavior, root-path route handling, and same-origin delivery integration.
- Affected code areas: future frontend app workspace, route entry handling, auth and request layers, shared page-state components, frontend build integration, and admin listener static-route coordination.
- Dependencies and external systems: depends on the existing `/api/admin/login`, `/api/admin/session`, `/api/admin/logout`, and `/api/admin/graphql` backend contract baseline plus the already-defined admin frontend shell and page hierarchy; no application implementation is included in this change.

## Explicitly Excluded

- Implementing frontend routes, components, styling, or framework-specific application code.
- Redesigning backend session contracts, GraphQL schema shape, or admin command/query service boundaries.
- Expanding scope into quotas, settings, alerting, broader observability, RBAC redesign, or ordinary-user self-service.
- Introducing realtime subscriptions or shell-global background refresh behavior.
- Treating final visual design language or polished UI presentation as the primary concern of this change.
