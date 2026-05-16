## Context

The current admin direction in `go-ginx-2` is already clear at the architecture level:

1. The browser-facing admin surface is now intended to be API-only and same-origin.
2. Browser session behavior belongs to dedicated HTTP endpoints under `/api/admin/*`.
3. Business data for management pages belongs to GraphQL.
4. Backend composition already has the right split: `internal/adminquery` for read models and `internal/admin` for command use cases.

What is still missing is a concrete contract blueprint for the frontend-facing GraphQL layer. The separated admin frontend needs stable page-oriented list/detail shapes, mutation inputs and payloads, error semantics, and resource-specific behaviors that do not require the frontend to reverse-engineer backend storage or runtime details. The backend also needs a clear boundary for what is a read projection, what is a command mutation, and what failure semantics must be preserved across resources.

This change captures that blueprint without implementing schema, resolvers, or frontend pages.

## Goals / Non-Goals

**Goals:**
- Define the target same-origin admin contract as session endpoints plus a GraphQL business entrypoint.
- Preserve the current backend query/command split instead of collapsing page behavior into transport handlers.
- Define frontend-consumable list, detail, and mutation contract patterns for dashboard, users, clients, proxies, certificates, and audit.
- Capture resource-specific lifecycle and validation semantics that the frontend and backend must share.
- Define listener-admission and conflict semantics for TCP/UDP proxy entry sockets.
- Record polling expectations and current known gaps in listener-conflict enforcement.

**Non-Goals:**
- Do not implement GraphQL schema, resolvers, DTOs, storage changes, or frontend components.
- Do not change current application behavior or claim the full blueprint is already implemented.
- Do not introduce subscriptions, websocket push, or multi-origin admin deployment.
- Do not widen the scope into quotas, settings, domain lifecycle management, alert workflows, or general observability search.

## Decisions

1. Keep the admin backend API-only and same-origin.
   - Decision: the browser-facing admin backend remains same-origin and API-only, with `POST /api/admin/login`, `GET /api/admin/session`, `POST /api/admin/logout`, and `POST /api/admin/graphql` as the relevant browser contract surface.
   - Rationale: this preserves the current session-authenticated admin direction and avoids splitting browser auth concerns across multiple origins or page-rendering paths.

2. Keep GraphQL as the business contract and keep HTTP endpoints narrow.
   - Decision: resource reads and writes for dashboard, users, clients, proxies, certificates, and audit belong to GraphQL, while login/session/logout remain plain HTTP endpoints.
   - Rationale: GraphQL already fits the management domain well, while browser session lifecycle is simpler and clearer as narrow HTTP behavior.

3. Preserve the read/write split between `internal/adminquery` and `internal/admin`.
   - Decision: GraphQL queries should compose page-ready read models from `internal/adminquery`, while GraphQL mutations should delegate command semantics to `internal/admin`.
   - Rationale: the codebase already distinguishes read projections from command behavior. The contract blueprint should reinforce that split rather than blur it.

4. Design the GraphQL contract around frontend pages, not storage tables.
   - Decision: list queries return page-oriented shapes with explicit pagination, filter, and sort inputs, while detail queries return resource-focused read models that can include runtime overlays where appropriate.
   - Rationale: the frontend should not reconstruct list state, totals, filters, or runtime composition from low-level fields.

5. Standardize mutations as input/payload operations.
   - Decision: write operations use explicit mutation names with one input object and one payload object, and payloads should carry the mutated resource or resulting status context needed for UI refresh.
   - Rationale: this keeps form handling, validation mapping, and mutation evolution predictable for the frontend.

6. Scope the first GraphQL contract around six resource areas.
   - Decision: the blueprint covers dashboard, users, clients, proxies, certificates, and audit only.
   - Rationale: those are the confirmed admin-console domains already established by the current management direction.

7. Treat clients as pre-registered managed nodes rather than self-service browser identities.
   - Decision: client management semantics assume an administrator pre-registers a managed node, issues a write-only credential, and later views persisted identity plus runtime overlay information.
   - Rationale: that matches the existing control-plane direction and avoids implying interactive client self-service behavior.

8. Treat proxies as control-plane resources with admission rules beyond simple DB uniqueness.
   - Decision: TCP and UDP proxy enablement/create/update flows must respect a shared ListenerClaim admission model, where active listener conflicts are checked against the whole runtime listener space rather than only proxy-table uniqueness constraints.
   - Rationale: the runtime conflict domain is broader than the current database uniqueness boundary.

9. Keep ListenerClaim V1 conservative.
   - Decision: V1 listener admission uses `same network + same port => conflict`, disabled proxies do not participate in the active claim set, and HTTP/HTTPS host-routing uniqueness remains outside ListenerClaim.
   - Rationale: a conservative rule is easier to reason about and safer than a partially permissive listener model during the first frontend contract batch.

10. Keep certificate management status-oriented and secret-safe.
    - Decision: certificate queries expose lifecycle and status information for managed HTTPS proxies, while mutations cover issue/renew actions and responses must never expose private-key material.
    - Rationale: the admin page needs operational visibility and actions, not secret exfiltration paths.

11. Keep audit identity semantics broader than a user-only model.
    - Decision: audit actor identity should be expressed in a direction such as actor type plus actor ID, rather than implying every action actor is a product user row.
    - Rationale: administrator actions, system actors, and future control-plane actors should not be misrepresented as only `userId` semantics.

12. Standardize frontend-consumable error semantics across GraphQL operations.
    - Decision: GraphQL errors must expose machine-readable codes including `UNAUTHENTICATED`, `FORBIDDEN`, `VALIDATION_FAILED`, `NOT_FOUND`, `CONFLICT`, `UNSUPPORTED`, `ENTRY_CONFLICT`, and `INTERNAL`, with field-level validation details where applicable.
    - Rationale: page flows need deterministic failure handling for auth redirects, form validation, conflict messaging, and unsupported operations.

13. Keep polling view-scoped and selective.
    - Decision: dashboard, clients, and proxies use 5-second polling; certificates may use lower-frequency polling though 5 seconds is acceptable; users and audit stay manual or low-frequency.
    - Rationale: runtime-oriented pages need freshness, while identity and timeline pages do not justify constant refresh.

14. Explicitly track the current uniqueness gap.
    - Decision: the blueprint records that current database uniqueness only covers proxy namespaces and does not yet cover whole-runtime listener admission.
    - Rationale: frontend and backend work should not mistake current storage guarantees for complete socket conflict safety.

## Contract Topology

The browser-facing admin contract stays intentionally narrow:

- `POST /api/admin/login`
  - establishes the administrator browser session
- `GET /api/admin/session`
  - returns minimal authenticated session/bootstrap context
- `POST /api/admin/logout`
  - invalidates the session
- `POST /api/admin/graphql`
  - serves dashboard, users, clients, proxies, certificates, and audit business operations

The GraphQL entrypoint is same-origin with the frontend and sits behind the same administrator session model as the session endpoints.

## GraphQL Shape Principles

### Page-Oriented Read Models

The contract should optimize for frontend page rendering rather than for direct table exposure.

- List queries return page-shaped results such as:
  - `items`
  - `totalCount`
  - `page`
  - `pageSize`
  - applied `filter` and `sort` echoes when helpful
- Detail queries return a stable detail view model for one page, including runtime overlays where the page needs them.
- Dashboard queries return widget-oriented read models rather than generic metric bags when the backend can produce meaningful operational slices.

### Shared List Inputs

Primary admin list queries should follow one predictable input pattern:

- pagination input:
  - page
  - pageSize
- filter input:
  - resource-specific structured fields rather than ad hoc search strings only
- sort input:
  - explicit sort key
  - explicit sort direction

The baseline pagination model is offset/page-oriented rather than cursor-oriented because the current control-plane views are operational tables with bounded sizes and SQLite-backed reads.

### Shared Mutation Pattern

Mutations should follow one contract pattern:

- one input object per mutation
- one payload object per mutation
- payload contains:
  - the updated or created resource when useful
  - resource identity and status context when full resource hydration is unnecessary
  - structured error semantics through GraphQL error extensions rather than ad hoc payload flags

Successful mutations should return enough identity and status context for the frontend to refresh the affected list or detail view with a targeted re-query instead of forcing a full application reload or broad cache reset.

## Resource Blueprint

### Dashboard

Dashboard remains an operator-oriented status page rather than a generic analytics surface.

- Query shape should support summary cards and issue framing for currently trustworthy runtime aggregates.
- The page is poll-driven at 5-second intervals.
- The frontend should be able to refresh dashboard widgets without reloading unrelated pages.

### Users

User management keeps clear administrator lifecycle semantics.

- List view:
  - paginated
  - filterable
  - sortable
  - suitable for frontend scanning and create entry points
- Detail view:
  - identity
  - status
  - management context appropriate to the admin page
- Mutation set:
  - create user
  - disable user
  - modify user password

User mutation semantics:

- `createUser` creates the managed user identity.
- `disableUser` is an explicit control-plane lifecycle action, not a generic patch of arbitrary status fields.
- password mutation updates the stored verifier without exposing plaintext password material in queries or payloads.

Users are not a high-frequency runtime page, so polling should be manual or low-frequency.

### Clients

Client resources represent pre-registered managed nodes.

- List view:
  - paginated
  - filterable by ownership/state as needed
  - sortable
  - includes runtime-aware online/offline or last-seen context suitable for an operator table
- Detail view:
  - persisted client identity
  - runtime/session overlay when available
  - related managed proxies

The detail page should include `managedProxies` so the frontend can present the managed-node relationship without stitching multiple unrelated calls.

Client mutation set:

- create client
- enable client
- disable client
- rotate client credential

Client credential semantics:

- client credentials are administrator-issued secrets for managed nodes
- the credential is write-only in management flows
- creation or rotation may return the new credential exactly once in the mutation payload
- list and detail queries must not expose the stored credential material afterward

Clients are runtime-oriented and should use 5-second polling for list/detail overlays where the page is visible.

### Proxies

Proxy resources are central control-plane objects and require stronger lifecycle semantics than a generic editable record.

- List view:
  - paginated
  - filterable
  - sortable
  - returns lifecycle and runtime summary fields for frontend table rendering
- Detail view:
  - persisted configuration
  - runtime/status overlay
  - type-specific configuration block

The detail contract should expose type-specific config in a stable nested block so the frontend can render TCP, UDP, HTTP, and HTTPS configuration without flattening unrelated fields.

Proxy mutation set should cover:

- create proxy
- update proxy
- enable proxy
- disable proxy
- delete proxy

Proxy mutation semantics:

- proxy type is immutable after creation
- delete requires the proxy to already be disabled
- enable, disable, and delete remain explicit lifecycle operations

#### ListenerClaim Admission Model

ListenerClaim is the shared admission model for active socket listeners owned by:

- static listeners
- enabled TCP proxies
- enabled UDP proxies

V1 behavior:

- a conflict exists when `network` and `port` are the same
- disabled proxies do not participate in the active claim set
- HTTP and HTTPS host-routing uniqueness stays outside ListenerClaim and remains a separate concern

Operations that create, update, or enable a TCP/UDP proxy must evaluate ListenerClaim admission against the active claim set. When admission fails because another active listener already owns the socket, the contract must surface `ENTRY_CONFLICT` semantics rather than a generic persistence failure.

This design explicitly records the current gap: database uniqueness only covers proxy namespaces and does not yet represent the whole runtime listener space.

Proxies are runtime-oriented and should use 5-second polling.

### Certificates

Certificate management is a lifecycle/status page for managed HTTPS proxies.

- List view:
  - status-oriented
  - filterable
  - sortable
  - focused on host, owning proxy, lifecycle status, and expiry context
- Actions:
  - issue certificate
  - renew certificate

Certificate queries and payloads must not expose private keys or other private-key material. The frontend should receive only the lifecycle/status fields required for operations and status display.

Certificates may use low-frequency polling, though 5-second polling remains acceptable if implementation simplicity favors one runtime refresh interval.

### Audit

Audit remains a lightweight control-plane timeline rather than a full observability or log-search system.

- List view:
  - reverse chronological by default
  - lightweight filtering
  - page-oriented result shape for frontend tables or timeline views

Audit event identity should not imply every actor is a product user. The contract should move toward actor identity semantics such as:

- `actorType`
- `actorId`
- optional display label when useful

This keeps administrator actions, system actions, and future actor kinds correctly represented.

Audit should be manual refresh or low-frequency polling.

## Error Semantics

The frontend contract depends on stable machine-readable error categories.

GraphQL operations should expose error codes for:

- `UNAUTHENTICATED`
- `FORBIDDEN`
- `VALIDATION_FAILED`
- `NOT_FOUND`
- `CONFLICT`
- `UNSUPPORTED`
- `ENTRY_CONFLICT`
- `INTERNAL`

Error behavior expectations:

- `UNAUTHENTICATED`
  - used when the administrator session is missing, expired, or invalid
  - allows the frontend to redirect to login
- `FORBIDDEN`
  - used when the session is authenticated but not allowed to perform the operation
- `VALIDATION_FAILED`
  - includes field-level details when failures map to specific input fields
- `NOT_FOUND`
  - used when the target resource does not exist or is no longer available within allowed disclosure rules
- `CONFLICT`
  - used for general state conflicts that are not listener-entry collisions
- `UNSUPPORTED`
  - used when a requested operation or combination is intentionally unsupported in the current contract
- `ENTRY_CONFLICT`
  - reserved for ListenerClaim admission failures involving active socket ownership
- `INTERNAL`
  - used for unexpected server-side failures

Field-level validation details should be machine-readable enough for the frontend to map errors back to specific form fields without parsing human text.

## Polling Model

The first contract batch keeps polling instead of subscriptions.

- `dashboard`: 5-second polling
- `clients`: 5-second polling
- `proxies`: 5-second polling
- `certificates`: low-frequency polling or 5-second polling is acceptable
- `users`: manual refresh or low-frequency polling
- `audit`: manual refresh or low-frequency polling

Polling should be page-scoped so one view refresh does not reset unrelated screen state.

## Validation Strategy

Future implementation work should validate:

- page-shaped list contracts for users, clients, proxies, certificates, and audit
- detail contracts that include runtime overlays only where appropriate
- mutation input/payload consistency across resource areas
- write-only credential behavior for client create/rotate flows
- proxy type immutability and delete-after-disable rules
- ListenerClaim admission failures surfacing `ENTRY_CONFLICT`
- certificate issue/renew actions and status visibility without private-key exposure
- audit actor identity shape that does not collapse to user-only semantics
- structured GraphQL errors with field-level validation details where applicable
- polling behavior assumptions for runtime-oriented pages

## Risks / Trade-offs

- [Risk] A frontend-first contract can drift from backend service boundaries. -> Mitigation: keep queries anchored to `internal/adminquery` read models and mutations anchored to `internal/admin` command semantics.
- [Risk] Listener conflict behavior may be oversimplified if treated as only a database uniqueness problem. -> Mitigation: explicitly define ListenerClaim as a runtime admission model and track the current storage/runtime gap.
- [Risk] List/detail contracts can bloat if every page asks for one-off shapes. -> Mitigation: keep shared pagination/filter/sort and input/payload patterns consistent across resources.
- [Risk] Polling intervals can be misapplied to low-churn pages. -> Mitigation: define which pages need 5-second refresh and keep users/audit low-frequency or manual.
- [Risk] Audit actor semantics can remain misleading if tied to only `userId`. -> Mitigation: establish actor type plus actor ID direction in the blueprint before schema implementation.

## Open Questions

- No blocking contract question remains for this blueprint capture. Future implementation work may still choose exact field names and schema nesting, but it should preserve the semantics defined here.
