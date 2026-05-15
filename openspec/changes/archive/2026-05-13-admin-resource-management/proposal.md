## Why

The roadmap identifies `admin-resource-management` as the next baseline because current `go-ginx-2` evidence supports non-interactive admin CLI seeding for milestone-one resources, while the product/design documents require a broader GraphQL admin API and web management surface. This change records the implemented CLI baseline and keeps dashboards, filtering, detail views, permissions, and full UI/API management as gaps.

## What Changes

- Add an `admin-resource-management` OpenSpec capability for current admin resource bootstrap and future admin management boundaries.
- Specify baseline behavior for creating users, client credentials, TCP proxy records, UDP proxy records, HTTP proxy records, and persisting them into SQLite through the admin CLI.
- Record GraphQL admin API, web UI, dashboards, resource list/detail views, filtering, user-facing permissions, quota editing, logs/audit querying, and system settings as required/design gaps rather than implemented behavior.
- Keep this change documentation/spec-only; it does not implement APIs, UI, code changes, dependencies, database changes, or deployment changes.

## Capabilities

### New Capabilities
- `admin-resource-management`: Defines the admin resource-management contract, including current CLI seeding behavior and explicit GraphQL/API/UI/dashboard/resource-management gaps.

### Modified Capabilities

- None.

## Impact

- Affected documentation systems: OpenSpec change artifacts and future baseline specs.
- Source documents: `docs/requirements.md`, `docs/design.md`, `openspec/changes/archive/2026-05-13-align-docs-roadmap-specs/roadmap-gap-matrix.md`, and current `go-ginx-2` progress/admin documentation.
- No application code, runtime behavior, public API, dependency, database, or deployment impact.
