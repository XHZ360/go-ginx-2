## ADDED Requirements

### Requirement: Administrator authentication baseline
The system SHALL protect the V1 management API and web UI with administrator-only authentication that is separate from client credentials.

#### Scenario: Administrator credentials loaded from protected configuration
- **WHEN** the management plane starts for the V1 admin-only batch
- **THEN** administrator usernames and password verifiers are loaded from protected server-side configuration instead of the existing SQLite `users` table

#### Scenario: HTTP Basic Auth guards admin management access
- **WHEN** a caller accesses the V1 management API or web UI without valid administrator credentials
- **THEN** the system rejects the request using HTTP Basic Auth semantics and does not expose management resources

#### Scenario: Management access requires TLS transport
- **WHEN** V1 administrator credentials are used to access the management plane
- **THEN** the management endpoint is expected to run behind TLS so credentials are not sent over plaintext transport

### Requirement: Administrator dashboard baseline
The system SHALL provide a minimal administrator dashboard summary aligned with currently trustworthy runtime aggregates.

#### Scenario: Dashboard summary fields
- **WHEN** an administrator loads the V1 dashboard summary
- **THEN** the response includes `onlineClientCount`, `enabledProxyCount`, `activeTCPConnectionCount`, `cumulativeUploadBytes`, `cumulativeDownloadBytes`, `cumulativeTCPErrorCount`, `cumulativeUDPErrorCount`, and `cumulativeHTTPErrorCount`

#### Scenario: Dashboard excludes unfinished observability projections
- **WHEN** an administrator views the V1 dashboard
- **THEN** the dashboard does not claim alert-state rollups, time-window traffic summaries, or richer observability projections that lack implemented evidence

### Requirement: Administrator user-management baseline
The system SHALL provide administrator-only user management for the first API/UI batch.

#### Scenario: List and view users
- **WHEN** an authenticated administrator queries the V1 user-management surface
- **THEN** the system returns user list and detail views for managed users

#### Scenario: Create user
- **WHEN** an authenticated administrator creates a user in the V1 management plane
- **THEN** the system persists the user resource without requiring initial quota or limit assignment fields

#### Scenario: Disable user
- **WHEN** an authenticated administrator disables a user in the V1 management plane
- **THEN** the system updates the user status so future runtime admission checks can treat that user as disabled

#### Scenario: Modify user password
- **WHEN** an authenticated administrator updates a managed user password in the V1 management plane
- **THEN** the system stores the updated password verifier without exposing plaintext password material in management responses

### Requirement: Administrator client-visibility baseline
The system SHALL provide administrator-only client list and detail views in the first API/UI batch.

#### Scenario: List clients
- **WHEN** an authenticated administrator queries clients in the V1 management plane
- **THEN** the system returns client list data suitable for an administrator control console

#### Scenario: View client detail with runtime state
- **WHEN** an authenticated administrator views client detail in the V1 management plane
- **THEN** the system composes persisted client data with currently available runtime/session state for that client

### Requirement: Administrator proxy-management baseline
The system SHALL provide full administrator CRUD and lifecycle control for the currently supported reverse-proxy resource types in the first API/UI batch.

#### Scenario: Manage supported reverse-proxy types
- **WHEN** an authenticated administrator creates or updates a proxy in V1
- **THEN** the management plane supports the currently implemented reverse-proxy types `TCP`, `UDP`, `HTTP`, and `HTTPS`, and does not claim forward-proxy creation in this batch

#### Scenario: View proxy list and detail
- **WHEN** an authenticated administrator queries proxies in V1
- **THEN** the system returns proxy list and detail views that combine persisted configuration with available runtime and aggregate status information

#### Scenario: Create proxy
- **WHEN** an authenticated administrator creates a valid proxy in V1
- **THEN** the system persists the proxy resource and records the control-plane action

#### Scenario: Update proxy without type mutation
- **WHEN** an authenticated administrator updates an existing proxy in V1
- **THEN** the system allows updates to supported proxy fields but rejects in-place mutation of the proxy type

#### Scenario: Enable or disable proxy
- **WHEN** an authenticated administrator enables or disables a proxy in V1
- **THEN** the system treats that action as an explicit lifecycle operation rather than an incidental status field edit

#### Scenario: Delete requires disabled proxy
- **WHEN** an authenticated administrator requests proxy deletion in V1
- **THEN** the system only allows delete after the proxy has first been disabled

### Requirement: Managed certificate admin baseline
The system SHALL provide administrator certificate status and lifecycle actions for managed HTTPS certificates in the first API/UI batch.

#### Scenario: View managed certificate status
- **WHEN** an authenticated administrator views managed certificate state for an HTTPS proxy in V1
- **THEN** the system returns the managed certificate status surface already supported by current certificate management behavior

#### Scenario: Issue or renew managed certificate
- **WHEN** an authenticated administrator triggers managed certificate issue or renewal in V1
- **THEN** the system performs the supported managed certificate lifecycle action and records the control-plane operation

### Requirement: Minimal administrator audit list baseline
The system SHALL expose a minimal recent audit-event list for the first admin API/UI batch.

#### Scenario: Recent audit events list
- **WHEN** an authenticated administrator requests the V1 audit view
- **THEN** the system returns a recent-event list containing actor, resource type, resource ID, action, result, and timestamp fields in reverse chronological order

#### Scenario: Audit view excludes advanced query behavior
- **WHEN** an authenticated administrator uses the V1 audit surface
- **THEN** the system does not claim advanced filtering, export, or log-correlation behavior in that first batch

### Requirement: Polling refresh baseline
The system SHALL use a fixed polling model instead of realtime subscriptions for the first admin API/UI batch.

#### Scenario: Five-second polling refresh
- **WHEN** an administrator views runtime-oriented V1 management pages
- **THEN** the management plane refreshes those views using a 5-second polling interval instead of GraphQL subscriptions or other realtime push behavior

## MODIFIED Requirements

### Requirement: GraphQL admin API gap tracking
The admin-resource-management spec SHALL treat a first administrator-only GraphQL management API batch as implemented baseline behavior while continuing to track broader GraphQL management capabilities as future work.

#### Scenario: V1 GraphQL admin API baseline exists
- **WHEN** an authenticated administrator uses the first management API/UI batch
- **THEN** the system exposes an administrator-only GraphQL surface for the confirmed V1 management resources and actions

#### Scenario: Advanced GraphQL behavior remains a gap
- **WHEN** ordinary-user GraphQL access, subscriptions, rich filtering, complex pagination, advanced log/audit queries, settings control, quota editing, or broader management-surface behavior is referenced from product or design documents
- **THEN** that behavior MUST remain a future gap until evidence-backed implementation exists

### Requirement: Admin web UI gap tracking
The admin-resource-management spec SHALL treat a first administrator-only management UI batch as implemented baseline behavior while continuing to track broader web management UI behavior as future work.

#### Scenario: V1 admin UI baseline exists
- **WHEN** an authenticated administrator uses the first management UI batch
- **THEN** the system provides the confirmed V1 administrator views for dashboard summary, user management, client visibility, proxy management, managed certificate actions, and recent audit events

#### Scenario: Broader admin UI behavior remains a gap
- **WHEN** ordinary-user self-service pages, advanced dashboards, alert-center workflows, settings UI, quota UI, domain-lifecycle UI, or richer observability pages are referenced from product or design documents
- **THEN** that behavior MUST remain a future gap until evidence-backed implementation exists

### Requirement: Full management policy gap tracking
The admin-resource-management baseline SHALL treat the confirmed V1 administrator resource-management surface as implemented while continuing to exclude broader policy and management domains from the first batch.

#### Scenario: V1 management surface is limited to confirmed resources
- **WHEN** an operator describes the V1 administrator management plane
- **THEN** it includes dashboard summary, user list/detail/create/disable/password modification, client list/detail, proxy CRUD plus lifecycle control, managed certificate status and issue/renew operations, and a minimal recent audit list

#### Scenario: Adjacent policy behavior remains a gap
- **WHEN** quota editing, limit enforcement UI, advanced resource filtering, system settings changes, log search, audit export, alert-center workflow, domain lifecycle management, or forward-proxy management behavior is referenced from product or design documents
- **THEN** the behavior MUST remain a gap in this spec until evidence-backed implementation exists
