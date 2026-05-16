## 1. Schema And Transport Alignment

- [x] 1.1 Update the admin GraphQL schema and resolver wiring in `internal/adminapi/server.go` so dashboard, users, clients, proxies, certificates, and audit expose canonical page-oriented list/detail operations through `/api/admin/graphql`.
- [x] 1.2 Standardize admin mutations on explicit input and payload objects while preserving `internal/adminquery` for reads and `internal/admin` for command execution.
- [x] 1.3 Add shared pagination, filter, and sort input types or envelopes for the primary admin list views where the canonical contract expects consistent frontend behavior.

## 2. Read Model And DTO Alignment

- [x] 2.1 Reshape the admin read models behind `internal/adminquery/service.go` and supporting DTO files so user, client, proxy, certificate, and audit lists return page-oriented results with totals and paging context.
- [x] 2.2 Update resource detail read models so client and proxy detail compose the canonical runtime overlays and page-specific fields without exposing secret material.
- [x] 2.3 Update audit read models to expose actor identity using `actorType` plus `actorId` and keep the audit surface as a lightweight recent-event timeline.

## 3. Mutation And Lifecycle Alignment

- [x] 3.1 Update user, client, proxy, and certificate mutation shapes so payloads return the identity or status context needed for targeted list/detail refresh after success.
- [x] 3.2 Implement the one-time client credential behavior so create and rotate mutations may return the credential exactly once and subsequent list/detail queries never expose it.
- [x] 3.3 Ensure proxy lifecycle mutations surface canonical delete-after-disable, type-immutability, and `ENTRY_CONFLICT` behavior through the GraphQL contract.

## 4. Error And Validation Semantics

- [x] 4.1 Centralize GraphQL error translation so admin auth, authorization, validation, not-found, conflict, unsupported, listener-admission, and unexpected failures map to `UNAUTHENTICATED`, `FORBIDDEN`, `VALIDATION_FAILED`, `NOT_FOUND`, `CONFLICT`, `UNSUPPORTED`, `ENTRY_CONFLICT`, and `INTERNAL`.
- [x] 4.2 Include frontend-consumable field-level validation details for `VALIDATION_FAILED` responses where admin mutations can map failures to specific inputs.
- [x] 4.3 Verify the admin transport does not leak internal-only errors or secret fields in GraphQL responses while aligning the canonical error contract.

## 5. Contract Verification

- [x] 5.1 Add or update GraphQL contract tests for canonical page-oriented list/detail queries across dashboard, users, clients, proxies, certificates, and audit.
- [x] 5.2 Add or update mutation tests covering shared input/payload shapes, one-time client credential return behavior, and structured validation/error semantics.
- [x] 5.3 Add or update tests for audit actor identity, proxy `ENTRY_CONFLICT` behavior, and certificate responses that remain secret-safe.
