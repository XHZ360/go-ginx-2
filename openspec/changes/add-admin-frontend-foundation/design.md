## Context

The admin backend already exposes a same-origin browser contract with `POST /api/admin/login`, `GET /api/admin/session`, `POST /api/admin/logout`, and `POST /api/admin/graphql`. The admin frontend shell change has also defined the confirmed route set, the protected shell boundary, shared page-state semantics, and page-scoped polling expectations for login, dashboard, users, clients, proxies, certificates, and audit.

What remains before implementation is the foundation architecture that later frontend code will follow. The team needs one approved application shape for route entry, session bootstrap, GraphQL transport, CSRF-aware mutations, page-owned refresh behavior, and same-origin delivery integration so implementation does not rediscover those decisions while building page code.

The most important constraint is that this admin UI is a server-state-heavy management console rather than an offline-first local application. That means the architecture should treat the admin API as the source of truth, keep shell concerns separate from resource-page concerns, and avoid over-centralizing resource state in one global browser store.

## Goals / Non-Goals

**Goals:**

- Define one implementation-ready frontend architecture for the dedicated admin UI under the same admin origin.
- Define the shell, route-entry, and session-bootstrap boundaries so protected routes behave consistently on hard load, deep link, and post-login navigation.
- Define how page containers consume `/api/admin/graphql` while login, logout, and session bootstrap remain on the dedicated admin HTTP endpoints.
- Define the state model split between shell session state, page-owned server state, and local UI state.
- Define page-scoped polling and targeted post-mutation refresh behavior that preserves the canonical admin page model.
- Define the local development and production delivery shape so frontend assets and `/api/admin/*` can coexist under one external origin.

**Non-Goals:**

- Do not implement the frontend framework, routes, or components in this change.
- Do not redesign the backend session model, GraphQL schema, or admin read and command service boundaries.
- Do not introduce shell-global polling, realtime subscriptions, offline sync, or an application-wide business-data store.
- Do not widen scope into quotas, settings, alerts, broader observability, RBAC redesign, or ordinary-user self-service.
- Do not make final visual design language the focus of this change.

## Decisions

1. Use one protected shell that owns session state, navigation chrome, and route-level auth handling, but not resource-page business data.
   - Decision: the authenticated shell owns current-administrator context, logout access, route bootstrap, and auth-expiry handling, while page containers own their own resource queries, mutations, and view refresh behavior.
   - Rationale: shell concerns are cross-cutting and stable across pages, but dashboard, users, clients, proxies, certificates, and audit have different query lifecycles and should not be coupled through one global resource store.
   - Alternatives considered:
   - Let the shell fetch and cache most page data globally. Rejected because it would blur page ownership and make refresh behavior harder to reason about.
   - Treat every page as a stand-alone entry with duplicated session handling. Rejected because it would duplicate auth and navigation logic.

2. Treat session state as the only required global application state.
   - Decision: the global frontend state model is limited to session bootstrap status, authenticated administrator identity, CSRF token, and minimal route-restoration context. Business data remains page-owned server state.
   - Rationale: the admin console is driven by backend truth and does not need a large client-owned domain store for users, clients, proxies, certificates, or audit events.
   - Alternatives considered:
   - Introduce one large global store for all resources. Rejected because it adds coordination cost without solving a real cross-page data-sharing problem in the first batch.

3. Use page containers as the primary data-consumption boundary.
   - Decision: each confirmed route maps to a page container that owns its canonical GraphQL reads, mutations, empty or error states, polling cadence, and post-mutation refresh logic.
   - Rationale: this keeps resource concerns aligned with the page model already defined in the shell change and prevents shared layout code from absorbing resource-specific behavior.
   - Alternatives considered:
   - Fetch GraphQL data directly in presentational components. Rejected because it scatters contract ownership and complicates reuse of page-state rules.

4. Treat server state and local UI state as separate concerns.
   - Decision: GraphQL responses and session bootstrap results are treated as server state, while filters, sort direction, pagination controls, dialog visibility, and in-progress form input remain local page state.
   - Rationale: this prevents UI concerns from leaking into transport caching and makes page refresh behavior predictable.
   - Alternatives considered:
   - Collapse filters and fetched resource pages into one reducer or store. Rejected because it couples transient UI input to refreshable backend data.

5. Keep polling page-scoped and activate it only while the corresponding page is active.
   - Decision: dashboard, clients, and proxies poll on the canonical 5-second cadence; certificates use low-frequency or action-driven refresh; users and audit use manual or low-frequency refresh; login never polls.
   - Rationale: the backend contracts and page model already expect different freshness levels, and page-scoped ownership avoids unrelated reloads or shell-wide timers.
   - Alternatives considered:
   - Run one shell-global polling loop. Rejected because it would over-refresh low-churn pages and entangle unrelated resource state.

6. Use targeted post-mutation refresh rather than full-shell or full-route reloads.
   - Decision: successful mutations re-query the affected detail view, the affected list view, or both, based on the current page context; the architecture must not rely on browser reloads to synchronize page state.
   - Rationale: this preserves current navigation context, filter state, and shell continuity while keeping server-state freshness explicit.
   - Alternatives considered:
   - Force full-page reload after mutations. Rejected because it would regress UX and work against the dedicated shell design.

7. Centralize transport concerns in a small request layer that separates session endpoints from GraphQL business traffic.
   - Decision: the frontend foundation should expose one session-oriented client for login, session bootstrap, and logout, plus one GraphQL transport client for resource reads and mutations, including CSRF-aware mutation handling and shared error translation.
   - Rationale: auth lifecycle traffic and resource-management traffic have different concerns and should not be mixed into one ad hoc fetch utility.
   - Alternatives considered:
   - Let each page call `fetch` directly. Rejected because it would duplicate CSRF, auth-expiry, and error-mapping behavior.

8. Normalize frontend error handling around canonical backend semantics rather than framework-default exceptions.
   - Decision: the foundation treats `UNAUTHENTICATED`, `FORBIDDEN`, `VALIDATION_FAILED`, `NOT_FOUND`, `CONFLICT`, `UNSUPPORTED`, `ENTRY_CONFLICT`, and `INTERNAL` as stable contract categories that page containers map into session expiry, form validation, not-found, conflict, or generic failure states.
   - Rationale: the backend already defines frontend-consumable error semantics, and the foundation should preserve those semantics rather than collapse them into generic runtime errors.
   - Alternatives considered:
   - Rely on generic GraphQL or network errors only. Rejected because it would lose the distinction the admin UI needs for forms and lifecycle actions.

9. Keep delivery same-origin in both local development and production.
   - Decision: production serves the dedicated frontend under the admin origin while reserving `/api/admin/*` for backend APIs, and local development must preserve that same-origin contract through dev-server proxying or equivalent composition rather than inventing cross-origin browser behavior.
   - Rationale: the admin backend contract and CSRF model are intentionally same-origin, so development should mirror that contract closely.
   - Alternatives considered:
   - Develop the UI as an unrelated cross-origin frontend and fix same-origin concerns later. Rejected because it would hide integration risks until late.

## Architecture Shape

```text
Browser
  │
  ▼
Admin Frontend Router
  ├─ Public Route Group
  │   └─ /login
  └─ Protected Shell
      ├─ Session Bootstrap
      ├─ Navigation Chrome
      ├─ Current Administrator Context
      ├─ Logout Action
      ├─ Route-Level Auth Expiry Handling
      └─ Page Containers
          ├─ Dashboard
          ├─ Users List / Detail
          ├─ Clients List / Detail
          ├─ Proxies List / Detail
          ├─ Certificates
          └─ Audit
                │
                ▼
        Request Layer
          ├─ Session Client
          ├─ GraphQL Client
          ├─ CSRF Mutation Handling
          └─ Error Mapping
                │
                ▼
        Same-Origin Admin API
          ├─ /api/admin/login
          ├─ /api/admin/session
          ├─ /api/admin/logout
          └─ /api/admin/graphql
```

## State Model

The frontend foundation uses three state categories.

1. **Shell session state**
   - Owns `unknown`, `checking`, `authenticated`, and `unauthenticated` route-entry status.
   - Stores only current administrator identity, CSRF token, bootstrap status, and intended destination.
   - Lives at the protected-shell boundary.

2. **Page-owned server state**
   - Owns dashboard summary, paginated list views, detail views, and mutation refresh results.
   - Stays aligned with canonical GraphQL contracts at `/api/admin/graphql`.
   - Refreshes by page activity rather than by shell-global orchestration.

3. **Local UI state**
   - Owns filter inputs, sort selection, pagination controls, dialog state, and in-progress forms.
   - Is disposable and page-scoped.

## Page Ownership Model

- `login`
  - Uses only `/api/admin/login` and `/api/admin/session`.
  - Owns intended-destination restoration after successful authentication.
- `dashboard`
  - Owns dashboard summary query and 5-second polling.
- `users` and `users/:id`
  - Own list and detail queries plus create, disable, and password-change actions.
- `clients` and `clients/:id`
  - Own runtime-aware list and detail queries plus page-scoped polling.
- `proxies` and `proxies/:id`
  - Own list and detail queries plus create, update, lifecycle, and delete-after-disable actions with explicit `ENTRY_CONFLICT` handling.
- `certificates`
  - Own lifecycle-oriented list/status reads plus issue and renew actions.
- `audit`
  - Own recent-event list reads with lightweight filtering and low-frequency or manual refresh.

## Delivery Model

1. Production delivery
   - The admin origin serves browser routes such as `/`, `/login`, `/dashboard`, `/users`, `/clients`, `/proxies`, `/certificates`, and `/audit` from the dedicated frontend.
   - `/api/admin/*` remains reserved for backend API behavior.
   - Unknown browser-facing routes inside the frontend scope are handled by the frontend route model, while unknown API paths remain API not-found behavior.

2. Local development delivery
   - The frontend may run through a development server, but browser requests must still appear same-origin relative to `/api/admin/*` through proxying or equivalent composition.
   - Development must not depend on a different cross-origin auth or CSRF behavior than production.

## Risks / Trade-offs

- [Risk] Leaving framework selection open can still allow drift during implementation. -> Mitigation: keep the architecture decisions focused on route, transport, state, polling, and delivery boundaries that survive framework choice.
- [Risk] Page-owned server state may duplicate a small amount of query wiring across pages. -> Mitigation: share only transport, error, and table primitives; keep resource query ownership with the page containers.
- [Risk] Targeted refresh after mutations can become inconsistent across resources. -> Mitigation: define refresh expectations per page and verify them during implementation acceptance.
- [Risk] Same-origin local development can be deferred and cause integration surprises later. -> Mitigation: include development delivery wiring in the foundation implementation slice rather than treating it as an afterthought.
- [Risk] Session expiry could still leak into generic page errors if the request layer is fragmented. -> Mitigation: centralize error mapping in the request layer and route auth-expiry outcomes back to the shell.

## Migration Plan

1. Choose the frontend workspace and framework within the boundaries defined here, without changing the approved route or contract model.
2. Implement the route shell, route-entry handling, and session-bootstrap flow first.
3. Implement the request layer for session endpoints, GraphQL traffic, CSRF-aware mutations, and canonical error mapping.
4. Implement shared shell and page-state primitives, then wire dashboard plus one list/detail resource pair to validate the model.
5. Implement the remaining resource pages using page-owned queries, mutations, and polling.
6. Integrate same-origin development and production delivery so frontend routes and `/api/admin/*` can coexist under one origin.
7. Add acceptance coverage for bootstrap, login redirect behavior, auth expiry, page-state handling, polling ownership, and targeted refresh after mutations.

Rollback remains low-risk because this change defines implementation architecture and introduces no data migration requirements by itself.

## Open Questions

- The framework and build tool can still be selected later as long as they support the shell, route, polling, and same-origin delivery constraints defined here.
- The exact query-cache library can remain implementation choice as long as it supports page-owned server state, targeted invalidation, and page-scoped polling without forcing a global business-data store.
