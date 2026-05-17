## ADDED Requirements

### Requirement: Admin frontend route shell baseline
The system SHALL define the dedicated administrator frontend as one same-origin route shell that consumes `/api/admin/session` for guard decisions, `/api/admin/login` and `/api/admin/logout` for session lifecycle, and `/api/admin/graphql` for business data.

#### Scenario: Shell separates public and protected route groups
- **WHEN** the dedicated administrator frontend route model is defined for the confirmed first batch
- **THEN** it contains one unauthenticated route group for `login` and one authenticated application-shell route group for `dashboard`, `users`, `clients`, `proxies`, `certificates`, and `audit`

#### Scenario: Shell keeps backend contract boundaries intact
- **WHEN** the frontend route shell maps browser interactions to backend calls
- **THEN** login, session bootstrap, and logout use the same-origin admin HTTP endpoints and page business data continues to use the canonical GraphQL contract at `/api/admin/graphql`

### Requirement: Guarded-route bootstrap semantics
The system SHALL define protected administrator routes to resolve session state through the same-origin session endpoint before protected page content is considered active.

#### Scenario: Initial protected navigation with valid session
- **WHEN** an administrator loads or deep-links to a protected route with a valid existing session
- **THEN** the frontend bootstrap flow checks `/api/admin/session`, initializes the authenticated shell, and renders the requested protected page without routing through `login`

#### Scenario: Initial protected navigation without valid session
- **WHEN** an administrator loads or deep-links to a protected route without a valid session
- **THEN** the frontend does not render protected page content and instead routes the browser to the `login` page

#### Scenario: Session expires while using a protected page
- **WHEN** a protected page request or poll cycle receives an authentication-expired result from the same-origin admin API
- **THEN** the frontend treats that condition as session expiry, clears authenticated shell state, and returns the browser to the `login` flow instead of presenting a generic page error

### Requirement: Confirmed admin page hierarchy baseline
The system SHALL define the confirmed first-batch admin frontend page hierarchy around the already-aligned backend contracts and SHALL not require unsupported page detail flows.

#### Scenario: Confirmed top-level pages are present
- **WHEN** the dedicated admin frontend information architecture is defined
- **THEN** the confirmed top-level pages are `login`, `dashboard`, `users`, `clients`, `proxies`, `certificates`, and `audit`

#### Scenario: List and detail hierarchy applies only to supported resource pages
- **WHEN** the first-batch page hierarchy is defined
- **THEN** `users`, `clients`, and `proxies` include list and detail page definitions, while `dashboard`, `certificates`, and `audit` remain top-level pages unless a future spec explicitly adds separate detail behavior

### Requirement: Admin shell navigation baseline
The system SHALL define a flat first-batch administrator navigation model that exposes only the confirmed operational areas.

#### Scenario: Navigation exposes confirmed pages only
- **WHEN** an authenticated administrator uses the dedicated frontend shell
- **THEN** primary navigation exposes `Dashboard`, `Users`, `Clients`, `Proxies`, `Certificates`, and `Audit` and does not expose quotas, settings, alerts, broader observability, domain workflows, RBAC redesign, or ordinary-user self-service entries

#### Scenario: Navigation is shell-owned rather than page-specific
- **WHEN** the dedicated admin frontend renders a protected page
- **THEN** the authenticated shell owns shared navigation chrome, current-administrator context display, and logout access rather than each page redefining those controls independently

### Requirement: Shared page-state semantics baseline
The system SHALL define consistent page-level loading, empty, error, and not-found behavior for the dedicated admin frontend.

#### Scenario: Protected shell preserves navigation during page loading
- **WHEN** an authenticated administrator navigates to a protected page that is still resolving its primary data
- **THEN** the frontend keeps the authenticated shell visible and presents route-level or page-level loading state instead of blanking the full application frame

#### Scenario: Empty state distinguishes no-data from no-match
- **WHEN** a list-oriented page such as `users`, `clients`, `proxies`, `certificates`, or `audit` has no items to show
- **THEN** the page model distinguishes between a baseline no-data state and a filter-produced no-match state so later frontend implementation can present the correct operator guidance

#### Scenario: Error state distinguishes auth expiry from other failures
- **WHEN** a protected page request fails
- **THEN** the page model distinguishes session expiry, validation or mutation failure, resource-not-found behavior, and unexpected backend failure instead of collapsing them into one generic error state

### Requirement: Admin page consumption model baseline
The system SHALL define how each confirmed page consumes the canonical same-origin admin API contracts.

#### Scenario: Login page uses session endpoints only
- **WHEN** the frontend defines the `login` page behavior
- **THEN** the page consumes `/api/admin/login` for sign-in and `/api/admin/session` for bootstrap or redirect decisions and does not depend on GraphQL for authentication bootstrap

#### Scenario: Dashboard uses summary-oriented GraphQL reads
- **WHEN** the frontend defines the `dashboard` page behavior
- **THEN** the page consumes the canonical dashboard GraphQL summary contract and models the page as a runtime-oriented overview instead of a resource list/detail flow

#### Scenario: Users page uses canonical list and detail contracts
- **WHEN** the frontend defines `users` list and detail pages
- **THEN** the list page consumes the canonical paginated, filterable, sortable users contract and the detail page consumes the canonical user detail contract plus supported lifecycle mutations

#### Scenario: Clients page uses canonical list and detail contracts
- **WHEN** the frontend defines `clients` list and detail pages
- **THEN** the list page consumes the canonical runtime-aware clients list contract and the detail page consumes the canonical client detail contract including managed-proxy context

#### Scenario: Proxies page uses canonical list and detail contracts
- **WHEN** the frontend defines `proxies` list and detail pages
- **THEN** the list page consumes the canonical proxies list contract and the detail page consumes the canonical proxy detail contract plus supported lifecycle and mutation flows without redefining listener-admission semantics in the frontend

#### Scenario: Certificates and audit pages use top-level page contracts
- **WHEN** the frontend defines `certificates` and `audit` pages
- **THEN** each page consumes the canonical GraphQL list or status-oriented contract as a top-level page without requiring unsupported detail-route assumptions

### Requirement: Page-scoped polling baseline for the dedicated frontend
The system SHALL define polling behavior at the page level for the confirmed administrator frontend views instead of through whole-page reloads or shell-global refresh.

#### Scenario: Runtime-oriented pages poll on active view cadence
- **WHEN** an authenticated administrator keeps `dashboard`, `clients`, or `proxies` open in the dedicated frontend
- **THEN** those pages poll their relevant canonical GraphQL queries on the established 5-second cadence while the page remains active

#### Scenario: Low-churn pages avoid aggressive polling
- **WHEN** an authenticated administrator uses `users`, `audit`, or `certificates`
- **THEN** the page model uses manual refresh or low-frequency polling appropriate to that page instead of inheriting the runtime-page 5-second cadence by default

#### Scenario: Polling remains page-scoped
- **WHEN** one protected page performs a polling refresh
- **THEN** that refresh does not reset unrelated page state or require a full-shell reload because polling ownership stays with the active page model

### Requirement: Admin frontend scope exclusions baseline
The system SHALL treat the dedicated admin frontend page-model definition as limited to the confirmed first-batch administrator views and SHALL not imply broader management scope.

#### Scenario: Excluded future areas stay out of current shell definition
- **WHEN** the frontend shell and page definitions are reviewed for the first batch
- **THEN** quotas and settings pages, alerts center behavior, broader observability pages, domain workflows, RBAC redesign, and ordinary-user self-service remain explicitly out of scope until a future change defines them

#### Scenario: Future touchpoints may be noted conceptually without becoming routes
- **WHEN** adjacent future frontend touchpoints are mentioned in the design
- **THEN** they are recorded only as possible future extension areas and do not become active routes, navigation items, or required page contracts in this change
