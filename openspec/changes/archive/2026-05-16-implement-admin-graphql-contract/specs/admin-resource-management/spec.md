## ADDED Requirements

### Requirement: Admin GraphQL implementation alignment baseline
The system SHALL align the existing administrator GraphQL/API implementation with the canonical admin-resource-management contract while preserving the current admin read and command service boundaries.

#### Scenario: Admin GraphQL reads and commands preserve canonical boundaries
- **WHEN** the implementation aligns dashboard, user, client, proxy, certificate, or audit behavior at `/api/admin/graphql`
- **THEN** read operations continue to use `internal/adminquery` for page-oriented list and detail models, and command operations continue to use `internal/admin` for lifecycle and mutation behavior

#### Scenario: Admin GraphQL contracts align with canonical frontend semantics
- **WHEN** the implementation updates existing admin GraphQL list, detail, or mutation operations
- **THEN** the resulting contract preserves canonical page-oriented list/detail behavior, shared pagination/filter/sort inputs where applicable, one-input/one-payload mutation semantics, one-time client credential return behavior, structured GraphQL error codes with validation details, and audit actor identity direction toward `actorType` plus `actorId`

#### Scenario: Alignment excludes unrelated admin redesign work
- **WHEN** the implementation change is scoped for admin GraphQL contract alignment
- **THEN** it does not widen the slice into quotas or rate limiting, observability overhaul, admin session persistence redesign, RBAC redesign, forward proxy support, or unrelated deployment or backup/restore work
