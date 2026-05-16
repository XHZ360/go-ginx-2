## Why

The canonical `admin-resource-management` spec now includes the archived admin GraphQL blueprint and runtime listener-admission requirements, but the existing admin GraphQL/API implementation still needs an implementation-focused change that brings its transport, DTO, and mutation behavior into conformance. This change creates the apply-ready plan for that alignment so the admin frontend can build against stable page-oriented contracts instead of transitional shapes.

## What Changes

- Align the existing admin GraphQL list and detail contracts for dashboard, users, clients, proxies, certificates, and audit with the canonical page-oriented admin spec.
- Standardize shared pagination, filter, and sort input shapes for admin list views where the canonical contract expects consistent frontend behavior.
- Standardize admin mutations around explicit input and payload objects while preserving the read boundary in `internal/adminquery` and the command boundary in `internal/admin`.
- Enforce the canonical client credential behavior so create and rotate flows may return a credential exactly once in the mutation payload and never in subsequent list or detail queries.
- Align GraphQL error mapping and validation details with the canonical frontend-consumable semantics, including `UNAUTHENTICATED`, `FORBIDDEN`, `VALIDATION_FAILED`, `NOT_FOUND`, `CONFLICT`, `UNSUPPORTED`, `ENTRY_CONFLICT`, and `INTERNAL`.
- Align audit read models with the actor identity direction toward `actorType` plus `actorId`.
- Keep this implementation change explicitly out of quotas, rate limiting, observability overhaul, admin session persistence redesign, RBAC redesign, forward proxy support, and unrelated deployment or backup/restore work.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `admin-resource-management`: add an implementation-alignment requirement for the existing admin GraphQL/API surface so the canonical page contracts, one-time credential handling, error semantics, and read/command boundaries are preserved during implementation.

## Impact

- Affected systems: admin GraphQL schema and resolver transport, admin read-model DTOs, admin command mutation payloads, validation/error mapping, and admin contract tests.
- Affected code areas: likely work in `internal/adminapi/server.go`, `internal/adminquery/service.go`, `internal/admin/service.go`, and DTO or read-model files that support admin list and detail shapes.
- Dependencies and external systems: no new runtime dependency is introduced; this change aligns the existing admin API implementation with the already-archived canonical OpenSpec requirements.
