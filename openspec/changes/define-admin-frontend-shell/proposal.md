## Why

The admin backend is now same-origin and API-only, with dedicated session endpoints and canonical GraphQL contracts already defined for the confirmed management domains. The remaining gap before frontend implementation is a clear page model and route-shell design that tells later frontend work how the admin console should be structured, guarded, navigated, and refreshed without reinterpreting backend behavior ad hoc.

This change captures that frontend page-definition layer now so implementation can proceed against one approved shell, hierarchy, and page-state model instead of rediscovering it during framework-specific coding.

## What Changes

- Define the dedicated admin frontend information architecture for the confirmed views: login, dashboard, users, clients, proxies, certificates, and audit.
- Define the authenticated route shell, guarded-route behavior, page hierarchy, and navigation model for the same-origin admin console.
- Define page-level loading, empty, error, and session-expiry behavior so later frontend work has consistent UX semantics across pages.
- Define page-specific polling expectations and how each list or detail page consumes the already-aligned canonical GraphQL contracts at `/api/admin/graphql`.
- Explicitly record excluded future areas so this frontend page-structure change does not widen into quotas/settings, alerts, broader observability, domain workflows, RBAC redesign, or ordinary-user self-service.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `admin-resource-management`: define the dedicated admin frontend page model and same-origin route shell that consume the existing session and GraphQL contracts for the confirmed administrator views.

## Impact

- Affected systems: future admin frontend application structure, same-origin route handling, session bootstrap and route guards, and page-level GraphQL query planning.
- Affected code areas: future frontend routes, navigation chrome, page containers, GraphQL query wiring, and admin UX state handling.
- Dependencies and external systems: depends on the already-implemented `/api/admin/login`, `/api/admin/session`, `/api/admin/logout`, and `/api/admin/graphql` backend contract baseline; no application implementation is included in this change.
