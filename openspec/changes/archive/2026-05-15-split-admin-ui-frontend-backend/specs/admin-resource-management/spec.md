## ADDED Requirements

### Requirement: Separated administrator frontend baseline
The system SHALL provide the administrator management console as a dedicated browser frontend application rather than as page-specific server-rendered HTML embedded in the management backend.

#### Scenario: Frontend owns browser routing and presentation
- **WHEN** an authenticated administrator uses the separated management console
- **THEN** browser route rendering, page composition, loading states, empty states, and form interaction behavior are handled by the frontend application rather than backend HTML templates

#### Scenario: Backend page rendering becomes transitional only
- **WHEN** the separated frontend is introduced
- **THEN** any remaining server-rendered administrator pages are treated as a temporary migration path rather than the target management-console architecture

### Requirement: API-first management backend baseline
The system SHALL expose the administrator management surface through API contracts consumable by the separated frontend.

#### Scenario: GraphQL remains the primary resource contract
- **WHEN** the separated frontend reads or mutates administrator-managed resources
- **THEN** dashboard, user, client, proxy, certificate, and audit resource operations are exposed through the management GraphQL surface backed by the admin query and mutation services

#### Scenario: Auxiliary HTTP endpoints remain narrow
- **WHEN** the separated frontend needs login, logout, session bootstrap, or similar browser-session behavior
- **THEN** the backend may expose minimal non-GraphQL HTTP endpoints for those concerns without duplicating the core resource-management contract

### Requirement: Same-origin management delivery baseline
The system SHALL present the separated administrator frontend and the administrator API to the browser as one origin.

#### Scenario: Same-origin frontend and API delivery
- **WHEN** an operator deploys the separated administrator console
- **THEN** the frontend application and the administrator API are composed under one external origin even if internal serving or proxying responsibilities differ

#### Scenario: API paths remain distinct from frontend routes
- **WHEN** the separated administrator console handles browser routes and API requests
- **THEN** administrator API paths remain explicitly namespaced so frontend route handling and API routing are not ambiguous

### Requirement: Session bootstrap baseline
The system SHALL provide a browser-session bootstrap contract for the separated administrator console.

#### Scenario: Administrator login establishes a browser session
- **WHEN** a valid administrator signs in to the separated management console
- **THEN** the management plane creates a server-managed browser session over TLS and returns the session state needed for the frontend to enter protected routes

#### Scenario: Frontend bootstrap discovers current session state
- **WHEN** the separated frontend loads or reloads a protected route
- **THEN** the frontend can query a dedicated session/bootstrap endpoint to determine whether the current browser already has a valid administrator session

### Requirement: Frontend-grade list interaction baseline
The system SHALL support frontend-grade interaction for administrator list views in the separated console.

#### Scenario: Primary list views support pagination and filtering
- **WHEN** an authenticated administrator uses `users`, `clients`, `proxies`, `certificates`, or `audit` list views in the separated console
- **THEN** the management API supports pagination and view-appropriate filtering so the frontend does not rely on whole-table dumps

#### Scenario: Primary list views support explicit sorting semantics
- **WHEN** an authenticated administrator changes the ordering of a supported list view in the separated console
- **THEN** the management API applies explicit sort behavior instead of relying on implicit storage order

### Requirement: Frontend-consumable error semantics baseline
The system SHALL expose frontend-consumable authorization, validation, and failure semantics for the separated administrator console.

#### Scenario: Validation errors are structured for form UX
- **WHEN** a separated-console create or update operation fails validation
- **THEN** the response includes machine-readable validation semantics and field-level error details where applicable so the frontend can present actionable form feedback

#### Scenario: Authentication failures are distinguishable from generic server errors
- **WHEN** a separated-console request fails because the administrator session is missing, expired, or invalid
- **THEN** the frontend can distinguish that authentication failure from authorization denial, validation failure, and unexpected backend errors

## MODIFIED Requirements

### Requirement: Administrator authentication baseline
The system SHALL protect the separated management frontend and API with administrator-only authentication that is separate from client credentials.

#### Scenario: Administrator credentials remain separate from product users and client credentials
- **WHEN** the management plane authenticates an administrator for the separated console
- **THEN** administrator identity remains separate from client credentials and does not require coupling browser login semantics to runtime machine identities

#### Scenario: Browser login establishes a server-managed session
- **WHEN** a valid administrator signs in to the separated management console
- **THEN** the management plane establishes a server-managed browser session over TLS rather than relying on repeated HTTP Basic Auth prompts

#### Scenario: Unauthenticated frontend or API access is rejected
- **WHEN** a caller accesses protected separated-console routes or API operations without a valid administrator session
- **THEN** the system rejects the request without exposing administrator-managed resources

### Requirement: Polling refresh baseline
The system SHALL use fixed client-side polling instead of realtime subscriptions for the first separated admin-console batch.

#### Scenario: Five-second view-scoped polling refresh
- **WHEN** an administrator views runtime-oriented separated-console pages such as dashboard, clients, proxies, or certificates
- **THEN** the frontend refreshes the relevant API queries using a 5-second polling interval instead of whole-page refreshes or realtime push behavior

### Requirement: GraphQL admin API gap tracking
The admin-resource-management spec SHALL treat the administrator GraphQL management API as the primary business-data contract for the separated console while continuing to track broader management capabilities as future work.

#### Scenario: Separated console uses GraphQL for scoped management resources
- **WHEN** an authenticated administrator uses the separated management console
- **THEN** the console reads and mutates the confirmed scoped management resources through the administrator GraphQL surface

#### Scenario: Broader management domains remain a gap
- **WHEN** ordinary-user self-service, subscriptions, advanced log/audit analysis, settings control, quota editing, domain lifecycle management, or alert-center workflow behavior is referenced from product or design documents
- **THEN** that behavior MUST remain future work until explicitly scoped and implemented

### Requirement: Admin web UI gap tracking
The admin-resource-management spec SHALL treat the dedicated administrator frontend application as the target management UI shape while continuing to track broader web-management behavior as future work.

#### Scenario: Dedicated frontend baseline exists
- **WHEN** an authenticated administrator uses the post-migration management console
- **THEN** the system provides the confirmed administrator views through a dedicated frontend application for dashboard, user management, client visibility, proxy management, managed certificate actions, and recent audit views

#### Scenario: Broader frontend capability remains a gap
- **WHEN** advanced dashboards, quota/settings UI, domain workflows, alert-center workflows, or broader observability pages are referenced from product or design documents
- **THEN** that behavior MUST remain future work until explicitly scoped and implemented
