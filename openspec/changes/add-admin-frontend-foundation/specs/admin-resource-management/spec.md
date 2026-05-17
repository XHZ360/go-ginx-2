## ADDED Requirements

### Requirement: Admin frontend route-entry and guard semantics
The system SHALL preserve canonical route-entry and session-guard behavior for the dedicated administrator frontend delivered on the admin origin.

#### Scenario: Root route redirects by authentication state
- **WHEN** a browser requests the dedicated administrator frontend root route `/`
- **THEN** the frontend resolves administrator session state through the dedicated session bootstrap endpoint and redirects authenticated administrators to `/dashboard` and unauthenticated visitors to `/login`

#### Scenario: Login is the only public frontend route
- **WHEN** a browser requests a dedicated administrator frontend route without a valid administrator session
- **THEN** `/login` is the only public frontend page and other administrator frontend routes do not render protected page content before session validation completes

#### Scenario: Authenticated visit to login redirects to dashboard
- **WHEN** an authenticated administrator requests `/login`
- **THEN** the frontend redirects that administrator to `/dashboard` instead of rendering the login form again

#### Scenario: Protected routes bootstrap before rendering content
- **WHEN** a browser loads, reloads, or directly opens a protected administrator frontend route such as `/dashboard`, `/users`, `/clients/:id`, or `/proxies/:id`
- **THEN** the frontend validates the current administrator session through the dedicated session bootstrap endpoint before rendering protected administrator-managed resource content

#### Scenario: Protected deep link restores after successful login
- **WHEN** an unauthenticated visitor is redirected to `/login` from a valid protected administrator frontend route
- **THEN** the frontend restores that intended protected destination after successful administrator authentication

#### Scenario: Invalid intended destination falls back safely
- **WHEN** a post-login intended destination is missing, unsafe, unsupported, or not a valid protected administrator frontend route
- **THEN** the frontend redirects the administrator to `/dashboard` instead of following the invalid destination

### Requirement: Admin frontend page-state semantics
The system SHALL preserve shared page-state semantics across the dedicated administrator frontend so protected-route behavior, list views, detail views, and refresh behavior remain consistent.

#### Scenario: Authentication expiry remains distinct from generic backend failure
- **WHEN** a protected administrator frontend query or mutation fails because the administrator session is missing, expired, or invalid
- **THEN** the frontend treats that result as authentication-expiry behavior and returns the browser to the login flow instead of presenting only a generic backend failure state

#### Scenario: List pages distinguish baseline empty state from filtered empty state
- **WHEN** an authenticated administrator uses a list page such as `users`, `clients`, `proxies`, `certificates`, or `audit`
- **THEN** the frontend distinguishes between a baseline empty state for an unpopulated resource set and a filtered empty state caused by the current filter or search scope

#### Scenario: Detail pages distinguish missing resource from generic backend failure
- **WHEN** an authenticated administrator opens a valid detail route such as `users/:id`, `clients/:id`, or `proxies/:id`
- **THEN** the frontend distinguishes a missing managed resource from a generic backend failure instead of collapsing both into one generic error state

#### Scenario: Runtime summary pages do not use empty-state semantics for zero-value summaries
- **WHEN** an authenticated administrator views the dashboard summary and current trustworthy aggregates are zero-valued
- **THEN** the frontend renders the dashboard summary as content with zero-value fields rather than replacing the page with a generic empty-state view

#### Scenario: Validation failure remains scoped to the active form or action surface
- **WHEN** a separated-console create, update, or lifecycle action fails with structured validation semantics
- **THEN** the frontend presents that validation failure within the active form or action surface instead of collapsing the entire page into a generic error state

#### Scenario: Polling remains scoped to the active page
- **WHEN** the administrator frontend refreshes runtime-oriented views through polling
- **THEN** polling remains scoped to the active page context rather than running as one shell-global refresh loop that reloads unrelated page data

## MODIFIED Requirements

### Requirement: API-only administrator browser surface baseline
The system SHALL stop serving the legacy server-rendered administrator UI once the session-authenticated admin surface is introduced and SHALL allow the dedicated administrator frontend to own browser-facing administrator routes when that frontend is introduced.

#### Scenario: Server-rendered administrator routes are removed
- **WHEN** the session-authenticated administrator API surface is introduced
- **THEN** the legacy server-rendered administrator pages and browser-facing form handlers are no longer served

#### Scenario: Dedicated frontend routes replace transitional browser not-found behavior
- **WHEN** the dedicated administrator frontend is introduced on the admin origin and a browser requests frontend paths such as `/`, `/login`, `/dashboard`, `/users`, `/clients`, `/proxies`, `/certificates`, or `/audit`
- **THEN** those browser-facing paths are handled by the dedicated administrator frontend route model instead of returning the transitional `404 Not Found` behavior used before the frontend existed, while administrator API behavior remains under explicitly namespaced paths such as `/api/admin/*`

#### Scenario: Separated-console API routes use administrator session auth
- **WHEN** the separated admin frontend calls the new same-origin admin API routes
- **THEN** those routes authenticate administrators through the server-managed session model rather than repeated Basic Auth prompts

#### Scenario: Legacy browser-facing GraphQL route is removed
- **WHEN** the session-authenticated administrator API surface is introduced
- **THEN** the legacy browser-facing `POST /graphql` route is no longer served for administrator browser access and browser clients use the session-authenticated GraphQL entrypoint instead
