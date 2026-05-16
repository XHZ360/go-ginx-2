## Context

`go-ginx-2` already has the canonical admin-resource-management requirements for the admin listener, session-authenticated GraphQL entrypoint, page-oriented admin reads, mutation semantics, listener-admission behavior, and frontend-consumable error handling. The remaining gap is implementation alignment: the existing admin GraphQL/API code still needs to converge on those canonical contracts without reopening the broader product scope.

This change follows the archived admin GraphQL blueprint and runtime listener-admission work. It is the next implementation-focused step and should keep the established backend split intact: `internal/adminquery` remains the read-model boundary for dashboard and resource page queries, while `internal/admin` remains the command boundary for admin mutations and lifecycle actions.

The hotspots are already clear:

- `internal/adminapi/server.go` owns the admin GraphQL transport and error translation boundary.
- `internal/adminquery/service.go` owns the page-oriented read composition and runtime overlays.
- `internal/admin/service.go` owns command semantics such as lifecycle changes, credential rotation, and listener-admission-aware mutations.
- Supporting DTO and read-model files likely need reshaping so list/detail responses match the canonical frontend contract rather than transitional internal shapes.

## Goals / Non-Goals

**Goals:**

- Align the existing admin GraphQL schema and transport with the canonical dashboard, user, client, proxy, certificate, and audit contracts.
- Preserve the `/api/admin/graphql` business entrypoint and the backend read/command split instead of collapsing behavior into transport handlers.
- Standardize page-oriented list/detail contracts, shared page input shapes, and one-input/one-payload mutation patterns where the canonical spec requires them.
- Ensure client credentials remain write-only outside create and rotate mutation payloads.
- Align GraphQL error semantics and field-level validation details with the frontend-consumable codes already defined in the canonical spec.
- Move audit read models toward `actorType` plus `actorId` semantics without widening the audit surface into observability search.

**Non-Goals:**

- Redesign quotas, rate limiting, observability, admin session persistence, RBAC, or forward proxy support.
- Introduce unrelated deployment, backup/restore, or broader management policy work.
- Re-scope the admin surface beyond dashboard, users, clients, proxies, certificates, and audit.
- Replace GraphQL with alternate transports or collapse login/session/logout concerns into the GraphQL contract.

## Decisions

1. Keep transport alignment scoped to the existing admin GraphQL entrypoint.
   - Decision: implementation work stays centered on `/api/admin/graphql` and the current admin API wiring instead of adding parallel endpoints for resource management.
   - Rationale: the canonical spec already establishes GraphQL as the resource contract and keeping one business entrypoint avoids duplicating frontend integration work.
   - Alternative considered: expose additional REST-style resource endpoints for alignment work. Rejected because it would split the contract and expand scope beyond the canonical admin direction.

2. Preserve the `internal/adminquery` read boundary and `internal/admin` command boundary.
   - Decision: queries continue to compose page-ready read models from `internal/adminquery`, while mutations continue to delegate command behavior to `internal/admin`.
   - Rationale: this keeps read DTO reshaping separate from mutation-side validation, listener admission, and persistence behavior.
   - Alternative considered: let GraphQL resolvers compose business logic directly. Rejected because it would make contract alignment harder to test and easier to drift across resources.

3. Standardize list contracts around shared page inputs plus resource-specific filters and sorts.
   - Decision: list queries should converge on shared pagination/page inputs and consistent filter/sort envelopes, with resource-specific fields layered on top as needed.
   - Rationale: the admin frontend needs predictable list interaction semantics across users, clients, proxies, certificates, and audit.
   - Alternative considered: keep each list query shape independently tailored. Rejected because it preserves transitional inconsistency and raises frontend integration cost.

4. Standardize mutations around one input object and one payload object.
   - Decision: admin mutations should expose explicit input/payload patterns and return enough identity or status context for list/detail refresh after success.
   - Rationale: this matches the canonical contract and creates one predictable pattern for form submission, post-success refresh, and error handling.
   - Alternative considered: reuse mixed scalar argument signatures or ad hoc payloads. Rejected because they make validation mapping and client integration inconsistent.

5. Treat client credentials as one-time mutation outputs only.
   - Decision: client create and rotate mutations may return the credential once in the payload, and all list/detail/read DTOs must remain secret-safe afterward.
   - Rationale: the canonical contract intentionally keeps client credentials write-only outside the mutation moment.
   - Alternative considered: expose masked or retrievable credentials in detail views. Rejected because it weakens secret-handling semantics and is not required by the spec.

6. Keep error semantics centralized at the GraphQL transport boundary.
   - Decision: `internal/adminapi/server.go` or closely-related transport code should translate auth, validation, not-found, conflict, unsupported, listener-admission, and unexpected failures into stable GraphQL error extensions.
   - Rationale: the frontend needs one machine-readable error contract regardless of which read or command path produced the failure.
   - Alternative considered: let each resolver or service invent its own GraphQL error shape. Rejected because it would create inconsistent frontend behavior.

7. Reshape audit reads toward actor identity semantics without expanding the feature set.
   - Decision: audit list/detail DTOs should expose `actorType` plus `actorId` semantics and keep the view as a lightweight recent-event timeline.
   - Rationale: this fixes the actor-identity direction needed by the canonical contract without pulling in advanced observability or audit-search scope.
   - Alternative considered: keep a user-only actor model until a later redesign. Rejected because it would continue misrepresenting administrator and system actors.

## Risks / Trade-offs

- [Risk] Shared page input shapes can force awkward fit for one or two resources. -> Mitigation: keep pagination and sort envelopes shared, but allow resource-specific filter fields instead of forcing a single generic filter blob.
- [Risk] DTO reshaping for page-oriented list/detail contracts can leak secrets or stale internal fields. -> Mitigation: review all admin DTO and read-model files touched by clients, users, proxies, certificates, and audit, with explicit checks for secret-safe responses.
- [Risk] Error-code alignment can regress existing frontend or tests if translation stays fragmented. -> Mitigation: centralize GraphQL error mapping and add contract tests that assert codes and validation detail structure.
- [Risk] Client credential one-time return can be implemented inconsistently across create and rotate flows. -> Mitigation: treat both mutations as one credential-handling path and add tests that prove later reads never expose the secret.
- [Risk] Audit actor reshaping can break existing consumers if the old field shape is assumed. -> Mitigation: update the canonical contract and implementation in one slice and cover the new actor fields in GraphQL contract tests.

## Migration Plan

1. Update schema and resolver transport to the canonical query and mutation envelopes.
2. Reshape read models and DTOs behind `internal/adminquery` to match the page-oriented list/detail contracts.
3. Reshape command-side mutation inputs and payloads behind `internal/admin` while preserving existing lifecycle and listener-admission semantics.
4. Centralize GraphQL error translation and validation detail mapping.
5. Update or add GraphQL contract tests for list/detail queries, mutation payloads, one-time credentials, error codes, audit actor identity, and secret-safe responses.

Rollback remains low-risk because this change only aligns existing admin GraphQL behavior and does not require deployment-time data migration.

## Open Questions

- No blocking product question remains for this change. Implementation can choose exact type and field names as long as the canonical page, mutation, credential, error, and actor-identity semantics are preserved.
