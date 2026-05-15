## Context

The current administrator surface in `go-ginx-2` is intentionally narrow and implementation-driven: one inline HTML template, one inline stylesheet, page selection inside a Go HTTP handler, form posts for mutations, and 5-second full-page refreshes for runtime views. That design proved the backend read/write split and validated the first admin workflows, but it does not scale into a richer control console.

The architectural tension is now clear:

1. The backend already has the right domain direction for management behavior: `internal/admin/service.go` handles writes and `internal/adminquery/service.go` handles read models.
2. The browser-facing layer still behaves like a transport convenience rather than a product-grade console.
3. A richer admin UI needs independent routing, page state, client-side interaction, structured loading/error/empty states, and better auth semantics than HTTP Basic Auth.

This change captures the design target without implementing it.

## Goals / Non-Goals

**Goals:**
- Define the management console as a dedicated frontend application that consumes a backend API.
- Preserve the existing query/mutation backend split rather than moving business composition into frontend-specific handlers.
- Replace browser-facing Basic Auth interaction with an administrator login/session model suitable for a decoupled frontend.
- Establish deployment and migration constraints that keep the separated UI operationally simple.
- Scope the first separated frontend around the already-existing admin domains: dashboard, users, clients, proxies, certificates, and audit.

**Non-Goals:**
- Do not choose a specific frontend framework in this design capture.
- Do not implement the frontend workspace, build pipeline, API handlers, or session store here.
- Do not expand the first scoped pages into quotas, settings, domains, alerts, log search, or ordinary-user self-service.
- Do not introduce realtime subscriptions in this change.

## Decisions

1. The admin console becomes a dedicated frontend application.
   - Decision: the long-term admin UI is no longer rendered as page-specific HTML templates inside the backend. The browser loads a dedicated frontend application responsible for route rendering, page composition, client state, and interaction flows.
   - Rationale: this is the cleanest way to support richer list interactions, status semantics, confirmations, drafts, filtering, and progressive UI improvements without pushing presentation concerns back into Go handlers.
   - Alternatives considered:
     - Keep enhancing the server-rendered template: smallest short-term change, but keeps presentation and interaction tightly coupled to backend code.
     - Rebuild the UI as a set of progressively enhanced server templates: better than the current state, but still leaves the backend owning browser rendering concerns.

2. The management backend becomes API-first.
   - Decision: after migration, the backend management surface serves API contracts and authentication/session endpoints rather than page-specific HTML.
   - Rationale: a separated frontend needs a stable, reusable backend contract. The existing `adminquery` and `admin` service split already points to an API-first management plane.
   - Alternatives considered:
     - Keep a mixed backend that permanently owns both HTML rendering and frontend APIs: viable for compatibility, but adds duplicate surface area and slows product evolution.

3. GraphQL remains the primary business-data contract, with minimal auxiliary HTTP endpoints.
   - Decision: resource queries and mutations for dashboard, users, clients, proxies, certificates, and audit continue to use GraphQL, while auxiliary HTTP endpoints are reserved for login, logout, session/bootstrap, and similar non-resource interactions.
   - Rationale: the project already has a GraphQL management surface and a query/mutation backend split that maps cleanly to it. Reusing that contract avoids inventing parallel resource APIs while still allowing browser session flows to use simpler HTTP endpoints where appropriate.
   - Alternatives considered:
     - Replace GraphQL with REST for all admin resources: would discard the current admin API direction and duplicate read/write contract design.
     - Put authentication into GraphQL mutations only: possible, but less natural for browser session bootstrap and middleware-driven auth behavior.

4. Browser-facing administrator auth moves from Basic Auth to session-based login.
   - Decision: the separated admin console uses an administrator login flow that establishes a server-managed session over TLS, with secure cookie transport and CSRF protection for browser mutation semantics.
   - Rationale: Basic Auth is acceptable for a narrow V1 surface, but it creates a poor browser UX and does not fit a dedicated frontend application. A session model better supports route guards, logout, bootstrap, and future role-aware navigation.
   - Alternatives considered:
     - Keep Basic Auth for the frontend: simple, but awkward for browser UX and weak as a foundation for a richer console.
     - Introduce token-only browser auth: workable, but usually adds more client storage and rotation concerns than a same-origin session model.

5. The first separated frontend stays administrator-only.
   - Decision: the frontend-backend split does not widen the role model. The first separated console still serves administrators only.
   - Rationale: the current backend, docs, and V1 UI are all administrator-scoped. Adding ordinary-user self-service at the same time would multiply authorization, information architecture, and API-scope complexity.

6. The browser should experience the admin console as same-origin, even if deployment internals are separate.
   - Decision: deployment should present the admin frontend and admin API under one external origin, whether by backend static serving or reverse proxy composition.
   - Rationale: same-origin delivery simplifies cookie-based auth, CSRF protection, local development expectations, and operator deployment. It avoids pulling CORS and cross-site cookie complexity into the first separated-console milestone.
   - Alternatives considered:
     - Separate frontend and API origins: possible, but adds operational and security complexity too early.

7. Runtime-oriented views keep polling, but polling becomes client-side and view-scoped.
   - Decision: dashboard, clients, proxies, and certificates continue to use a 5-second refresh contract in the first separated console, but the frontend polls only the relevant API queries instead of refreshing whole pages.
   - Rationale: this preserves the current runtime honesty while improving usability and avoiding realtime subscription scope.
   - Alternatives considered:
     - Add subscriptions immediately: more responsive, but not necessary to unlock the frontend split.
     - Keep full-page refresh: undermines the point of a dedicated frontend.

8. Primary management list views must support frontend-grade interaction.
   - Decision: the first separated-console API must support list pagination, filtering, sorting, and explicit loading/error states for `users`, `clients`, `proxies`, `certificates`, and `audit` views.
   - Rationale: once the UI is a dedicated frontend, shipping raw unpaginated table dumps would preserve the current usability ceiling. Frontend-backend separation is only worthwhile if the contract supports real list workflows.

9. Migration happens in phases, not as a flag day rewrite.
   - Decision: the current server-rendered admin UI may coexist temporarily during migration, but it becomes a transitional surface rather than the target architecture.
   - Rationale: this reduces delivery risk and allows page-by-page parity work, starting with the highest-value views.

## Administrator Session Model

The separated admin console keeps administrator identity distinct from product users and client credentials.

- Credential source: the first separated-console milestone continues to validate administrators against protected administrator credentials rather than against runtime client credentials or ordinary product-user records.
- Login flow: the frontend submits administrator credentials to a dedicated login endpoint over TLS. On success, the backend rotates any preexisting session identifier, creates a new administrator session, and returns an authenticated browser session cookie.
- Session transport: the session cookie is `HttpOnly`, `Secure`, and `SameSite=Lax` at minimum. The frontend never stores administrator credentials or long-lived bearer tokens in browser storage.
- Session lifetime: the backend enforces an idle timeout plus an absolute lifetime so abandoned consoles do not remain valid indefinitely. Session expiry is surfaced to the frontend as an authentication failure that redirects the user back through login.
- Session bootstrap: after page load, the frontend calls a bootstrap/session endpoint to determine whether the browser already has a valid administrator session and to retrieve minimal viewer metadata needed for route guards and navigation.
- Mutation protection: browser mutations require CSRF protection in addition to the session cookie. The first separated-console batch uses a same-origin CSRF mechanism suitable for cookie-backed sessions.
- Logout: logout invalidates the server-side session and clears the browser cookie. Frontend logout is never treated as client-only state removal.

This keeps the admin auth model narrow and operationally controlled while replacing the poor browser ergonomics of repeated Basic Auth prompts.

## Same-Origin Deployment Model

The browser must experience the admin frontend and the admin API as one origin.

- Production topology: operators expose a dedicated administrator origin such as `https://admin.example.com/` or a dedicated administrator path on a protected origin. The frontend application and the admin API are composed behind that same origin.
- Asset serving options: the backend may serve the built frontend assets directly, or a reverse proxy may serve static assets while proxying admin API traffic to the backend. Either option is acceptable if the browser still sees one origin.
- API namespacing: API traffic is isolated under a stable prefix such as `/api/admin/*` or `/admin/api/*` so SPA route handling and API routing remain unambiguous.
- Local development: a frontend development server may exist internally, but browser traffic should still be mediated through a proxy pattern that preserves same-origin behavior for cookies and CSRF expectations.
- Security posture: the separated console requires TLS in real deployments because session cookies, CSRF exchanges, and administrator actions are security-sensitive.

## API Contract

### Session And Bootstrap Endpoints

The separated console uses a small auxiliary HTTP surface for browser session management.

- `POST <admin-api-prefix>/login`
  - Accepts administrator username and password.
  - Creates a new administrator session on success.
  - Returns a session cookie and a minimal bootstrap payload or a redirect-safe success response.
- `POST <admin-api-prefix>/logout`
  - Invalidates the current administrator session.
  - Clears the session cookie.
- `GET <admin-api-prefix>/session`
  - Returns the current authenticated administrator bootstrap payload when a valid session exists.
  - Returns an unauthenticated response when no valid session exists.

The bootstrap payload should stay intentionally small. It only needs enough information to drive route guards, current-user chrome, and frontend capability gating, such as administrator username, coarse role/capability flags, and polling configuration.

### GraphQL Contract Extensions

GraphQL remains the primary resource contract for dashboard, users, clients, proxies, certificates, and audit.

The first separated-console batch extends the V1 GraphQL contract with frontend-oriented list behavior:

- Explicit pagination inputs for primary list views.
- Explicit filter inputs rather than ad hoc string concatenation in query arguments.
- Explicit sort keys and sort directions rather than implicit storage order.
- Stable view models for list rows and detail pages, so frontend pages do not need to reconstruct backend semantics.
- Query shapes that support view-scoped polling for runtime-oriented pages.

For the first separated-console batch, offset/limit pagination is the preferred baseline over cursor pagination because the current admin surfaces are operational tables backed by SQLite and runtime joins rather than high-volume end-user feeds. Each list query should return:

- `items`
- `totalCount`
- `page`
- `pageSize`
- applied filter/sort echoes where useful for the frontend

Minimum filter/sort expectations by page:

- `users`: search by username or ID, filter by role and status, sort by username or creation order.
- `clients`: search by name or ID, filter by user, runtime online state, and status, sort by name or most recent activity.
- `proxies`: search by name or ID, filter by type, client, configured status, and runtime status, sort by name, type, or recent activity/order.
- `certificates`: filter by host, proxy, status, and expiration window, sort by host or expiry.
- `audit`: filter by actor, resource type, action, result, and time window, with reverse chronological ordering as the default sort.

### Error, Validation, And Authorization Semantics

The frontend needs stable failure semantics rather than transport-specific guesswork.

- Session endpoints return structured JSON error envelopes with a machine-readable error code, human-facing message, and optional field-level details.
- GraphQL operations use structured error extensions so the frontend can distinguish `UNAUTHENTICATED`, `FORBIDDEN`, `VALIDATION_FAILED`, `NOT_FOUND`, `CONFLICT`, and `INTERNAL` cases.
- Validation failures include field-level error details when the failure maps to specific input fields.
- Session expiry and explicit logout are surfaced in a way that lets the frontend redirect to login without treating the response as a generic server failure.
- Authorization failures remain deliberately terse and must not leak protected resource existence beyond what current admin authorization policy allows.

## Frontend Information Architecture

### Route Shell And Guards

The separated console has two top-level route groups:

- Unauthenticated routes:
  - `/login`
- Authenticated application shell:
  - `/`
  - `/dashboard`
  - `/users`
  - `/users/:id`
  - `/clients`
  - `/clients/:id`
  - `/proxies`
  - `/proxies/:id`
  - `/certificates`
  - `/audit`

The authenticated shell owns:

- global navigation
- current-administrator identity display
- logout control
- route-level error boundaries
- global session-expiry handling

Guard behavior:

1. On initial load, the frontend calls the session/bootstrap endpoint.
2. If a valid administrator session exists, the shell renders the requested route.
3. If no valid administrator session exists, protected routes redirect to `/login`.
4. If a session expires during use, the frontend preserves the intended destination when reasonable and returns the user to the login flow.

### Navigation Model

The first separated-console navigation remains intentionally flat and operational:

- `Dashboard`
- `Users`
- `Clients`
- `Proxies`
- `Certificates`
- `Audit`

Navigation should prioritize active operational areas instead of mirroring backend package structure. The first batch does not add settings, quota, alerts, or domain-management navigation items because those domains remain out of scope.

### Page-Level UX Requirements

`Dashboard`
- Show summary cards for the confirmed runtime metrics instead of a single dense table row.
- Surface operator-oriented issue framing such as offline clients, unhealthy proxies, or expiring certificates when the backend can provide those signals.
- Poll only the dashboard queries needed for the visible widgets.

`Users`
- Provide a paginated list with search, role/status filters, and explicit create-user entry points.
- Keep create, disable, and password-reset flows separated from passive list scanning.
- Detail views emphasize identity, status, aggregate resource counts, and recent management context rather than raw storage fields.

`Clients`
- Provide a paginated runtime-aware list with clear online/offline state semantics.
- Detail views combine persisted identity with runtime/session overlays and recent connectivity context.
- Runtime refresh should update only client-specific queries and not reset the full page state.

`Proxies`
- Provide a paginated list with filters for type, client, configured status, and runtime status.
- Separate create/edit flows from high-frequency list scanning and status monitoring.
- Make lifecycle actions such as enable, disable, and delete explicit, with stronger confirmation on destructive actions.

`Certificates`
- Present certificate state as an operational status view, not just a configuration dump.
- Emphasize host, owning proxy, lifecycle status, and renewal/expiry context.
- Keep issue and renew actions close to status but clearly separated from passive reading.

`Audit`
- Provide a reverse-chronological recent-event timeline with lightweight filtering.
- Preserve the V1 scope limit: this is a control-plane activity view, not full observability or log search.

### Shared UI States

The separated console must define shared behavior for common UI states across pages:

- Loading: use route-level or panel-level loading states that preserve shell navigation and avoid blank-page flashes.
- Empty: empty states explain whether no data exists or no records match the current filters, and they should point to the next meaningful action when appropriate.
- Error: distinguish between authorization failure, validation failure, runtime backend failure, and resource-not-found cases.
- Confirmation: lifecycle and destructive actions require explicit confirmation with clear resource identity.
- Post-mutation refresh: the frontend should prefer explicit refetch or local state reconciliation after successful mutations rather than optimistic behavior that invents runtime state.

## Migration And Compatibility

### Page Migration Sequence

The migration should proceed in this order:

1. Session/auth foundation and GraphQL contract extensions.
2. Frontend shell and login flow.
3. `Dashboard` because it establishes the runtime polling and shell experience.
4. `Clients` and `Proxies` because they are the highest-frequency operational views.
5. `Users` once shell, forms, and list interactions are stable.
6. `Certificates` and `Audit` after the core operational pages reach parity.
7. Legacy UI retirement and route cleanup.

### Compatibility Expectations During Coexistence

During migration, the system may temporarily expose both the legacy server-rendered admin UI and the new separated frontend, but the coexistence rules should stay strict:

- The separated frontend is the target experience for any page that has reached parity.
- The legacy UI is frozen except for compatibility or security fixes once migration begins.
- Both surfaces must call the same backend query/mutation semantics so business behavior does not diverge.
- Operators must have a documented fallback path if a specific page has not yet migrated or if the new frontend is temporarily unavailable.
- Legacy and new surfaces should not require administrators to understand different resource semantics; only the presentation and auth/session mechanics may differ temporarily.

### Deployment And Operator Documentation Scope

Operator-facing documentation for the separated console must cover:

- recommended admin origin and TLS expectations
- same-origin composition patterns
- static-asset serving or reverse-proxy composition options
- session-cookie and CSRF assumptions
- migration/coexistence behavior during rollout
- troubleshooting for login/session failures and API reachability
- cutover steps for retiring the legacy UI

## Validation Strategy

### Backend Validation

Backend validation for the separated-console milestone should include:

- successful and failed administrator login
- session bootstrap for authenticated and unauthenticated callers
- logout invalidation behavior
- session-expiry rejection behavior
- CSRF enforcement on browser mutation endpoints when applicable
- GraphQL authorization checks for protected resource queries and mutations
- list pagination, filtering, and sorting contract tests for the primary views
- validation error-shape tests for create/update flows

### End-To-End Validation

End-to-end validation should exercise the real admin listener and the separated frontend together:

- administrator login and logout
- protected-route guarding
- dashboard load and polling refresh
- representative list, detail, and mutation flows for users, clients, proxies, certificates, and audit
- session expiry or invalid-session redirect behavior
- compatibility behavior while legacy and new surfaces coexist

### Cutover Validation

Legacy UI retirement requires explicit cutover validation:

- every scoped page in the separated frontend reaches documented parity
- operators have updated deployment and troubleshooting guidance
- legacy routes are removed or redirected according to the rollout plan
- smoke validation proves administrators can complete the core management workflows without the server-rendered UI

## Target Shape

```text
Browser
  -> Admin Frontend App
      -> login/session bootstrap
      -> route shell
      -> dashboard / users / clients / proxies / certificates / audit
  -> Admin API
      -> session/auth endpoints
      -> GraphQL queries/mutations
  -> adminquery / admin service
  -> SQLite + runtime/session/stats state
```

## Page Model

The first separated frontend should preserve the existing admin domains while changing how they behave:

- `Dashboard`: top-level summary plus operator-oriented issue framing instead of a single metrics row.
- `Users`: paginated list, search/filter, detail, create, disable, and password-reset flows.
- `Clients`: paginated runtime-aware list plus detail views.
- `Proxies`: paginated list, filtering by type/status/client, detail, create, update, enable, disable, and delete flows with explicit confirmations.
- `Certificates`: status-oriented list and certificate lifecycle actions.
- `Audit`: paginated recent-event timeline with lightweight filtering appropriate to the first batch.

## Risks / Trade-offs

- [Risk] The change may look larger than a UI refactor because auth and deployment are affected. -> Mitigation: keep role scope fixed and reuse current backend query/mutation services.
- [Risk] Session auth adds backend responsibilities beyond the current Basic Auth middleware. -> Mitigation: keep the first session model intentionally narrow and administrator-only.
- [Risk] Keeping GraphQL while adding auxiliary HTTP endpoints can blur contract boundaries. -> Mitigation: reserve GraphQL for resource operations and keep HTTP endpoints limited to session/bootstrap concerns.
- [Risk] A transitional period with both server-rendered and separated UIs can create maintenance drag. -> Mitigation: define the legacy UI as temporary and remove it after scoped parity.
- [Risk] Pagination and filtering requirements may expose gaps in current query DTOs. -> Mitigation: treat those gaps as explicit backend work, not as frontend-only concerns.

## Migration Plan

1. Define the new frontend-backend requirements and auth model in spec.
2. Add backend support for administrator sessions, frontend bootstrap, and list/query contract extensions.
3. Introduce the dedicated frontend shell and migrate scoped pages in order: dashboard, clients/proxies, users, certificates, audit.
4. Remove the legacy page-rendering path after parity and documentation updates.

## Open Questions

- Frontend framework selection remains intentionally open.
- Whether the backend serves the frontend build artifacts directly or a reverse proxy composes them remains a deployment implementation choice, as long as the browser experiences one origin.
