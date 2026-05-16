## Purpose

Define the admin resource-management contract for milestone-one non-interactive CLI seeding of users, client credentials, and TCP/UDP/HTTP proxy records; administrator-only management authentication; a same-origin administrator session and API surface with an API-only admin listener for the current browser-facing slice; the API-first backend contracts that a dedicated administrator frontend will consume for dashboard, users, clients, proxies, managed certificates, and recent audit visibility; plus explicit tracking of remaining GraphQL/API/UI, quota, settings, observability, and broader management gaps.

## Requirements

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

### Requirement: Administrator authentication baseline
The system SHALL protect the separated management frontend and API with administrator-only authentication that is separate from client credentials.

#### Scenario: Administrator credentials loaded from protected configuration
- **WHEN** the management plane starts for the V1 admin-only batch
- **THEN** administrator usernames and password verifiers are loaded from protected server-side configuration instead of the existing SQLite `users` table

#### Scenario: Administrator credentials remain separate from product users and client credentials
- **WHEN** the management plane authenticates an administrator for the separated console
- **THEN** administrator identity remains separate from client credentials and does not require coupling browser login semantics to runtime machine identities

#### Scenario: Browser login establishes a session from file-backed administrator credentials
- **WHEN** a valid administrator signs in to the separated management console
- **THEN** the management plane verifies that administrator against the protected administrator credential source and establishes a server-managed browser session over TLS rather than relying on repeated HTTP Basic Auth prompts

#### Scenario: Legacy browser admin flow is not retained as a fallback
- **WHEN** the administrator session model is introduced for the browser-facing management surface
- **THEN** the system does not retain the server-rendered administrator UI or repeated browser-facing Basic Auth as a fallback path

#### Scenario: Unauthenticated frontend or API access is rejected
- **WHEN** a caller accesses protected separated-console routes or API operations without a valid administrator session
- **THEN** the system rejects the request without exposing administrator-managed resources

#### Scenario: Management access requires TLS transport
- **WHEN** V1 administrator credentials are used to access the management plane
- **THEN** the management endpoint is expected to run behind TLS so credentials are not sent over plaintext transport

### Requirement: Separated administrator frontend baseline
The system SHALL treat the administrator management console as a dedicated browser frontend application target rather than as page-specific server-rendered HTML embedded in the management backend.

#### Scenario: Frontend owns browser routing and presentation
- **WHEN** an authenticated administrator uses the post-migration separated management console
- **THEN** browser route rendering, page composition, loading states, empty states, and form interaction behavior are handled by the frontend application rather than backend HTML templates

#### Scenario: Backend page rendering becomes transitional only
- **WHEN** the session-authenticated administrator API surface is introduced ahead of the dedicated frontend
- **THEN** the backend does not retain server-rendered administrator pages as the target management-console architecture

### Requirement: API-first management backend baseline
The system SHALL expose the administrator management surface through API contracts consumable by the separated frontend.

#### Scenario: GraphQL remains the primary resource contract
- **WHEN** the separated frontend reads or mutates administrator-managed resources
- **THEN** dashboard, user, client, proxy, certificate, and audit resource operations are exposed through the session-authenticated management GraphQL surface backed by the admin query and mutation services

#### Scenario: Auxiliary HTTP endpoints remain narrow
- **WHEN** the separated frontend or bootstrap flow needs login, logout, session bootstrap, or similar browser-session behavior
- **THEN** the backend exposes minimal non-GraphQL HTTP endpoints for those concerns without duplicating the core resource-management contract

### Requirement: Same-origin management delivery baseline
The system SHALL present the separated administrator frontend and the administrator API to the browser as one origin.

#### Scenario: Same-origin frontend and API delivery
- **WHEN** an operator deploys the separated administrator console
- **THEN** the frontend application and the administrator API are composed under one external origin even if internal serving or proxying responsibilities differ

#### Scenario: API paths remain distinct from frontend routes
- **WHEN** the separated administrator console handles browser routes and API requests
- **THEN** administrator API paths remain explicitly namespaced so frontend route handling and API routing are not ambiguous

### Requirement: Administrator session endpoint baseline
The system SHALL expose dedicated same-origin administrator session endpoints for the separated admin console.

#### Scenario: Login creates an administrator browser session
- **WHEN** a valid administrator submits credentials to the separated admin-console login endpoint
- **THEN** the system verifies those credentials against the protected administrator credential source, creates a server-managed administrator session, sets a browser session cookie, and returns the minimal bootstrap information needed for the frontend shell

#### Scenario: Session bootstrap returns current auth context
- **WHEN** the separated frontend calls the administrator session bootstrap endpoint with a valid browser session
- **THEN** the system returns the minimal authenticated administrator context needed for route guards, shell initialization, and CSRF-aware follow-up requests

#### Scenario: Logout invalidates the administrator browser session
- **WHEN** the separated frontend calls the administrator logout endpoint for a current browser session
- **THEN** the system invalidates the corresponding server-managed session and clears the browser session cookie

### Requirement: Session bootstrap baseline
The system SHALL provide a browser-session bootstrap contract for the separated administrator console.

#### Scenario: Bootstrap returns minimal administrator session context
- **WHEN** the separated frontend loads or reloads a protected route with a valid administrator session
- **THEN** the frontend can query the dedicated session/bootstrap endpoint to recover the minimal current-session context, including the information needed for route guards and follow-up authenticated browser requests

### Requirement: Administrator session lifecycle baseline
The system SHALL enforce lifecycle rules for separated-console administrator sessions.

#### Scenario: Session expiry rejects further separated-console access
- **WHEN** an administrator browser session is missing, expired, or invalid
- **THEN** the separated-console session bootstrap endpoint and session-authenticated API operations reject access without exposing protected administrator resources

#### Scenario: Process restart invalidates in-memory administrator sessions
- **WHEN** the server process restarts while the first separated-console session implementation uses in-memory session storage
- **THEN** previously issued administrator sessions are no longer valid and administrators must authenticate again

### Requirement: Browser mutation CSRF baseline
The system SHALL protect session-authenticated separated-console mutations against CSRF.

#### Scenario: Session-authenticated mutation requires a valid CSRF token
- **WHEN** a separated-console browser mutation uses a valid administrator session
- **THEN** the system requires a valid CSRF token in addition to the session cookie before allowing the mutation to proceed

#### Scenario: Session-authenticated query access does not require CSRF
- **WHEN** a separated-console browser request performs a session-authenticated read-only operation
- **THEN** the system may allow that request without a CSRF token as long as the administrator session is valid

### Requirement: API-only administrator browser surface baseline
The system SHALL stop serving the legacy server-rendered administrator UI once the session-authenticated admin surface is introduced.

#### Scenario: Server-rendered administrator routes are removed
- **WHEN** the session-authenticated administrator API surface is introduced
- **THEN** the legacy server-rendered administrator pages and browser-facing form handlers are no longer served

#### Scenario: Removed administrator page paths return not found
- **WHEN** a browser requests removed administrator page paths such as `/`, `/users`, `/clients`, `/proxies`, `/certificates`, or `/audit` on the admin listener after this slice lands
- **THEN** the admin listener returns `404 Not Found` instead of serving HTML or redirecting to a replacement UI that does not yet exist

#### Scenario: Separated-console API routes use administrator session auth
- **WHEN** the separated admin frontend calls the new same-origin admin API routes
- **THEN** those routes authenticate administrators through the server-managed session model rather than repeated Basic Auth prompts

#### Scenario: Legacy browser-facing GraphQL route is removed
- **WHEN** the session-authenticated administrator API surface is introduced
- **THEN** the legacy browser-facing `POST /graphql` route is no longer served for administrator browser access and browser clients use the session-authenticated GraphQL entrypoint instead

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
The system SHALL provide full administrator CRUD and lifecycle control for the currently supported reverse-proxy resource types in the first API/UI batch, and SHALL reject TCP or UDP lifecycle changes that would violate active runtime listener admission before invalid state is accepted where possible.

#### Scenario: Manage supported reverse-proxy types
- **WHEN** an authenticated administrator creates or updates a proxy in V1
- **THEN** the management plane supports the currently implemented reverse-proxy types `TCP`, `UDP`, `HTTP`, and `HTTPS`, and does not claim forward-proxy creation in this batch

#### Scenario: View proxy list and detail
- **WHEN** an authenticated administrator queries proxies in V1
- **THEN** the system returns proxy list and detail views that combine persisted configuration with available runtime and aggregate status information

#### Scenario: Create proxy
- **WHEN** an authenticated administrator creates a valid proxy in V1
- **THEN** the system persists the proxy resource and records the control-plane action

#### Scenario: Create TCP or UDP proxy rejected by listener admission
- **WHEN** an authenticated administrator creates a TCP or UDP proxy whose requested active listener conflicts with the active runtime listener set under the V1 `same network + same port` rule
- **THEN** the system rejects the operation with `ENTRY_CONFLICT` semantics before accepting invalid persisted state where possible

#### Scenario: Update proxy without type mutation
- **WHEN** an authenticated administrator updates an existing proxy in V1
- **THEN** the system allows updates to supported proxy fields but rejects in-place mutation of the proxy type

#### Scenario: Update TCP or UDP proxy rejected by listener admission
- **WHEN** an authenticated administrator updates a TCP or UDP proxy so its resulting active listener would conflict with the active runtime listener set under the V1 `same network + same port` rule
- **THEN** the system rejects the operation with `ENTRY_CONFLICT` semantics before accepting invalid persisted state where possible

#### Scenario: Enable or disable proxy
- **WHEN** an authenticated administrator enables or disables a proxy in V1
- **THEN** the system treats that action as an explicit lifecycle operation rather than an incidental status field edit

#### Scenario: Enable TCP or UDP proxy rejected by listener admission
- **WHEN** an authenticated administrator enables a TCP or UDP proxy whose active listener would conflict with the active runtime listener set under the V1 `same network + same port` rule
- **THEN** the system rejects the enable operation with `ENTRY_CONFLICT` semantics before accepting invalid active state where possible

#### Scenario: Listener admission evaluates full active runtime listener space
- **WHEN** the management plane evaluates listener admission for a TCP or UDP create, update, or enable operation
- **THEN** it checks configured static listeners from `control_quic_listen`, `control_tls_listen`, `admin_listen`, `http_entry_listen`, and `https_entry_listen` where applicable, plus enabled TCP proxies and enabled UDP proxies

#### Scenario: Disabled proxies are excluded from active listener admission
- **WHEN** the management plane evaluates listener admission for a TCP or UDP create, update, or enable operation
- **THEN** disabled proxies are excluded from the active claim set used for conflict detection

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

### Requirement: Polling refresh baseline
The system SHALL use fixed client-side polling instead of realtime subscriptions for the first separated admin-console batch.

#### Scenario: Five-second view-scoped polling refresh
- **WHEN** an administrator views runtime-oriented separated-console pages such as dashboard, clients, proxies, or certificates
- **THEN** the frontend refreshes the relevant API queries using a 5-second polling interval instead of whole-page refreshes or realtime push behavior

### Requirement: GraphQL admin API gap tracking
The admin-resource-management spec SHALL treat the administrator GraphQL management API as the primary business-data contract for the separated console while continuing to track broader management capabilities as future work.

#### Scenario: Separated console uses GraphQL for scoped management resources
- **WHEN** an authenticated administrator uses the separated management console
- **THEN** the console reads and mutates the confirmed scoped management resources through the session-authenticated administrator GraphQL entrypoint rather than through repeated Basic Auth browser prompts

#### Scenario: Broader management domains remain a gap
- **WHEN** ordinary-user self-service, subscriptions, advanced log/audit analysis, settings control, quota editing, domain lifecycle management, or alert-center workflow behavior is referenced from product or design documents
- **THEN** that behavior MUST remain future work until explicitly scoped and implemented

### Requirement: Admin web UI gap tracking
The admin-resource-management spec SHALL treat the dedicated administrator frontend application as the target management UI shape while continuing to track broader web-management behavior as future work.

#### Scenario: Dedicated frontend target covers confirmed views
- **WHEN** an authenticated administrator uses the post-migration management console
- **THEN** the system provides the confirmed administrator views through a dedicated frontend application for dashboard, user management, client visibility, proxy management, managed certificate actions, and recent audit views

#### Scenario: Broader frontend capability remains a gap
- **WHEN** advanced dashboards, quota/settings UI, domain workflows, alert-center workflows, or broader observability pages are referenced from product or design documents
- **THEN** that behavior MUST remain future work until explicitly scoped and implemented

### Requirement: Full management policy gap tracking
The admin-resource-management baseline SHALL treat the confirmed V1 administrator resource-management surface as implemented while continuing to exclude broader policy and management domains from the first batch.

#### Scenario: V1 management surface is limited to confirmed resources
- **WHEN** an operator describes the V1 administrator management plane
- **THEN** it includes dashboard summary, user list/detail/create/disable/password modification, client list/detail, proxy CRUD plus lifecycle control, managed certificate status and issue/renew operations, and a minimal recent audit list

#### Scenario: Adjacent policy behavior remains a gap
- **WHEN** quota editing, limit enforcement UI, advanced resource filtering, system settings changes, log search, audit export, alert-center workflow, domain lifecycle management, or forward-proxy management behavior is referenced from product or design documents
- **THEN** the behavior MUST remain a gap in this spec until evidence-backed implementation exists

### Requirement: Frontend-consumable admin GraphQL list contracts
The system SHALL expose frontend-consumable GraphQL list contracts for the separated administrator console using page-oriented shapes rather than raw whole-table responses.

#### Scenario: Paginated and filterable management lists
- **WHEN** an authenticated administrator queries `users`, `clients`, `proxies`, `certificates`, or `audit` through `/api/admin/graphql`
- **THEN** each list contract returns a page-oriented result with `items`, `totalCount`, paging context, and structured filter input semantics suitable for a frontend list view

#### Scenario: Explicit sort semantics for management lists
- **WHEN** an authenticated administrator changes ordering for a supported management list
- **THEN** the GraphQL contract applies explicit sort key and sort direction semantics instead of relying on implicit storage order

### Requirement: Frontend-consumable admin GraphQL detail and mutation contracts
The system SHALL expose resource-focused admin GraphQL detail and mutation contracts that support frontend page rendering and targeted refresh behavior.

#### Scenario: Detail queries use resource-focused page models with runtime overlays where appropriate
- **WHEN** an authenticated administrator queries detail for a supported admin resource through `/api/admin/graphql`
- **THEN** the detail contract returns a resource-focused page model and may include runtime overlay fields only where the page needs live operational context

#### Scenario: Mutations return input/payload shapes with refresh context
- **WHEN** an authenticated administrator performs a supported admin mutation through `/api/admin/graphql`
- **THEN** the contract uses one input object and one payload object and returns enough resource identity or status context for the frontend to re-query the affected list or detail view after success

### Requirement: User management contract semantics
The system SHALL expose administrator user-management read models and mutations with explicit lifecycle behavior.

#### Scenario: User list and detail support admin lifecycle views
- **WHEN** an authenticated administrator queries user list or user detail through `/api/admin/graphql`
- **THEN** the contract returns a paginated, filterable, sortable list and a user-focused detail model with identity, status, and management context suitable for the admin page

#### Scenario: User lifecycle mutations keep password material secret
- **WHEN** an authenticated administrator creates a user, disables a user, or updates a user password through `/api/admin/graphql`
- **THEN** the contract treats disable as an explicit lifecycle action and does not expose plaintext password material in queries or mutation payloads

### Requirement: Client management contract semantics
The system SHALL expose administrator client-management mutations and read models that treat clients as pre-registered managed nodes with write-only credential handling.

#### Scenario: Client credential is returned only at create or rotate time
- **WHEN** an authenticated administrator creates a client or rotates a client credential through `/api/admin/graphql`
- **THEN** the new client credential may be returned in that mutation payload exactly once and is not exposed by subsequent list or detail queries

#### Scenario: Client detail includes runtime overlay and managed proxies
- **WHEN** an authenticated administrator views client detail through `/api/admin/graphql`
- **THEN** the detail contract combines persisted client identity with available runtime/session overlay information and includes the client's `managedProxies`

### Requirement: Proxy listener-admission semantics
The system SHALL evaluate TCP and UDP proxy socket admission through a shared ListenerClaim model over the active runtime listener space and surface active listener conflicts as explicit contract behavior.

#### Scenario: ListenerClaim conflict rejects create, update, or enable operations
- **WHEN** an authenticated administrator creates, updates, or enables a TCP or UDP proxy whose requested active listener conflicts with an existing active claim under the V1 `same network + same port` rule
- **THEN** the operation is rejected with `ENTRY_CONFLICT` semantics rather than a generic persistence failure

#### Scenario: Active ListenerClaim set includes configured static listeners
- **WHEN** listener admission is evaluated for TCP or UDP proxy activity
- **THEN** the active ListenerClaim set includes configured listeners derived from `control_quic_listen`, `control_tls_listen`, `admin_listen`, `http_entry_listen`, and `https_entry_listen` where those listeners are configured and participate in runtime binding

#### Scenario: Active ListenerClaim set includes enabled TCP and UDP proxies
- **WHEN** listener admission is evaluated for TCP or UDP proxy activity
- **THEN** the active ListenerClaim set includes enabled TCP proxies and enabled UDP proxies that would occupy active runtime listeners

#### Scenario: Disabled proxies do not participate in active ListenerClaim admission
- **WHEN** listener admission is evaluated for TCP or UDP proxy activity
- **THEN** disabled proxies do not participate in the active claim set used for conflict detection

### Requirement: Structured admin GraphQL error semantics
The system SHALL expose structured GraphQL error semantics that the separated administrator frontend can consume directly.

#### Scenario: GraphQL errors expose machine-readable contract codes
- **WHEN** an admin GraphQL operation fails through `/api/admin/graphql`
- **THEN** the response exposes machine-readable error semantics for `UNAUTHENTICATED`, `FORBIDDEN`, `VALIDATION_FAILED`, `NOT_FOUND`, `CONFLICT`, `UNSUPPORTED`, `ENTRY_CONFLICT`, and `INTERNAL`

#### Scenario: Validation failures include field-level details
- **WHEN** an admin GraphQL operation fails input validation for one or more fields
- **THEN** the response includes field-level validation details where applicable so the frontend can map failures to form fields without parsing only human prose

### Requirement: Certificate lifecycle status contract
The system SHALL expose a status-oriented certificate management contract for managed HTTPS proxies without exposing private-key material.

#### Scenario: Certificate list is lifecycle-oriented
- **WHEN** an authenticated administrator queries certificates through `/api/admin/graphql`
- **THEN** the contract returns a status-oriented list suitable for frontend display of host, owning proxy, lifecycle status, and expiry context

#### Scenario: Certificate lifecycle actions do not expose secret material
- **WHEN** an authenticated administrator triggers certificate issue or renew actions through `/api/admin/graphql`
- **THEN** the mutation contract returns operational lifecycle results without exposing private keys or other private-key material

### Requirement: Audit actor identity semantics
The system SHALL expose audit timeline semantics that do not misleadingly model every actor as only a user ID.

#### Scenario: Audit timeline uses actor identity direction
- **WHEN** an authenticated administrator queries the audit timeline through `/api/admin/graphql`
- **THEN** each audit event exposes actor identity semantics using a direction such as actor type plus actor ID, along with action, resource, result, and timestamp context suitable for a control-plane timeline

#### Scenario: Audit remains a lightweight control-plane timeline
- **WHEN** an authenticated administrator uses the initial audit timeline contract
- **THEN** the contract remains a lightweight recent-event control-plane view rather than claiming full observability search or log-correlation behavior

### Requirement: Admin polling model
The system SHALL define view-scoped polling expectations for the initial administrator GraphQL contract instead of relying on subscriptions.

#### Scenario: Runtime-oriented pages poll frequently
- **WHEN** an authenticated administrator uses dashboard, clients, proxies, or certificates views through `/api/admin/graphql`
- **THEN** dashboard, clients, and proxies use 5-second polling, and certificates use either low-frequency polling or the same 5-second interval when one runtime refresh cadence is preferred

#### Scenario: Low-churn pages avoid aggressive refresh
- **WHEN** an authenticated administrator uses users or audit views through `/api/admin/graphql`
- **THEN** those views use manual refresh or low-frequency polling and polling remains scoped to the active page rather than resetting unrelated screen state

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
