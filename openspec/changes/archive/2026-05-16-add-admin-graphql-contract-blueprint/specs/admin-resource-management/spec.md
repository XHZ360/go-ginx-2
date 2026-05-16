## ADDED Requirements

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
The system SHALL evaluate TCP and UDP proxy socket admission through a shared ListenerClaim model and surface active listener conflicts as explicit contract behavior.

#### Scenario: ListenerClaim conflict rejects create, update, or enable operations
- **WHEN** an authenticated administrator creates, updates, or enables a TCP or UDP proxy whose requested active listener conflicts with an existing active claim under the V1 `same network + same port` rule
- **THEN** the operation is rejected with `ENTRY_CONFLICT` semantics rather than a generic persistence failure

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
