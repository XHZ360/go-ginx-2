## Why

`go-ginx-2` now has enough runtime, certificate, stats, and deployment baseline to justify starting the real management plane, but the current admin surface is still only a non-interactive CLI plus write-focused service methods. If GraphQL or a web UI is built directly on top of the current store and runtime primitives, the project will likely entangle transport, query aggregation, permissions, and runtime state composition too early.

## What Changes

- Define the first management API/UI implementation batch for the admin plane rather than leaving GraphQL and UI as abstract gaps.
- Establish an explicit backend split between write-oriented admin use cases and read-oriented admin query models.
- Scope the first API/UI batch around a narrowed set of roles and resources instead of attempting the full dashboard, settings, quota, and log platform at once.
- Capture the need for a separate human-authentication model for the management plane rather than reusing client credentials.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `admin-resource-management`: change GraphQL/API/UI from an undifferentiated future gap into a staged implementation plan with explicit query/mutation boundaries, role scope, and management-surface constraints.

## Impact

- Affected systems: management backend architecture, GraphQL schema shape, future web UI page model, authorization boundaries, and runtime state aggregation.
- Affected code areas: `internal/admin`, store/query abstractions, session/stats/runtime state exposure, and future management transport layers.
- No implementation is included in this capture; it records architectural direction for the next management-plane change.
