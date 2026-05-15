## ADDED Requirements

### Requirement: Admin CLI user seeding baseline
The system SHALL support non-interactive creation of milestone-one user records through the admin CLI.

#### Scenario: Create user record
- **WHEN** an operator runs the admin CLI create-user flow with a database path, user ID, and username
- **THEN** the CLI persists the user resource into SQLite

### Requirement: Admin CLI client credential seeding baseline
The system SHALL support non-interactive creation of milestone-one client credentials through the admin CLI.

#### Scenario: Create client credential record
- **WHEN** an operator runs the admin CLI create-client flow with a user ID, client ID, display name, and credential
- **THEN** the CLI persists a client credential resource into SQLite for that user

### Requirement: Admin CLI proxy seeding baseline
The system SHALL support non-interactive creation of milestone-one TCP, UDP, and HTTP proxy records through the admin CLI.

#### Scenario: Create TCP proxy record
- **WHEN** an operator runs the admin CLI create-tcp-proxy flow with user, client, entry port, and local target settings
- **THEN** the CLI persists a TCP proxy resource into SQLite

#### Scenario: Create UDP proxy record
- **WHEN** an operator runs the admin CLI create-udp-proxy flow with user, client, entry port, and local target settings
- **THEN** the CLI persists a UDP proxy resource into SQLite

#### Scenario: Create HTTP proxy record
- **WHEN** an operator runs the admin CLI create-http-proxy flow with user, client, Host, and local target settings
- **THEN** the CLI persists an HTTP proxy resource into SQLite

### Requirement: Admin CLI audit baseline
The system SHALL record successful milestone-one admin create operations as audit events when current implementation evidence supports that behavior.

#### Scenario: Create operation audit event
- **WHEN** an admin CLI create operation succeeds and the implementation records an audit event for that operation
- **THEN** the audit event is persisted with the created resource context

### Requirement: GraphQL admin API gap tracking
The admin-resource-management spec SHALL track GraphQL management API behavior as required/design behavior that is not implemented in the current baseline.

#### Scenario: GraphQL API planned but not implemented
- **WHEN** GraphQL queries, mutations, subscriptions, permissions, pagination, filtering, or resource detail behavior is referenced from product or design documents
- **THEN** the spec MUST identify that behavior as a future gap until evidence-backed implementation exists

#### Scenario: Future GraphQL API implementation
- **WHEN** future work implements GraphQL management API behavior
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented

### Requirement: Admin web UI gap tracking
The admin-resource-management spec SHALL track web dashboard and management UI behavior as required/design behavior that is not implemented in the current baseline.

#### Scenario: Admin UI planned but not implemented
- **WHEN** dashboard, user list, client list, proxy list, detail page, domain/certificate management, log query, audit query, alert, or settings UI behavior is referenced from product or design documents
- **THEN** the spec MUST identify that behavior as a future gap until evidence-backed implementation exists

#### Scenario: Future admin UI implementation
- **WHEN** future work implements an admin UI behavior
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented

### Requirement: Full management policy gap tracking
The admin-resource-management baseline SHALL NOT claim quota editing, permission enforcement, resource filtering, settings changes, log/audit querying, or certificate operations as implemented unless current evidence supports those behaviors.

#### Scenario: Policy behavior remains a gap
- **WHEN** policy or management behavior is required by product/design docs but lacks current implementation evidence
- **THEN** the behavior MUST remain a gap in this spec
