## Context

The current management listener in `go-ginx-2/internal/adminapi/server.go` loads administrator credentials from a protected JSON file and applies one Basic Auth middleware to every route. That means:

1. The legacy server-rendered pages and GraphQL endpoint are both protected by repeated `Authorization: Basic ...` requests.
2. There is no dedicated login or logout route.
3. There is no browser-session bootstrap endpoint for route guards or shell initialization.
4. There is no CSRF protection model because the current browser experience is a narrow Basic Auth + server-rendered flow.

The separated admin-console design now exists in spec, but the code does not yet have the auth foundation required to support it and still carries an inline HTML admin surface that the user no longer wants to keep. This change is the first implementation-oriented slice of that migration and removes the server-rendered admin surface instead of treating it as a fallback.

## Goals / Non-Goals

**Goals:**
- Establish a server-managed administrator session model for the future separated frontend.
- Keep administrator credentials separate from product users and client credentials.
- Define the first same-origin admin API route layout for login, logout, session bootstrap, and session-authenticated GraphQL.
- Remove the server-rendered admin UI and browser-facing form workflow from the management backend.
- Add CSRF rules appropriate for cookie-backed browser mutations.

**Non-Goals:**
- Do not implement the frontend shell or browser routes in this change.
- Do not add pagination, filtering, or sorting contracts for resource lists in this change.
- Do not add distributed session storage, SSO, MFA, or ordinary-user authentication.
- Do not implement the replacement frontend pages in this change.

## Decisions

1. Keep administrator credential verification file-backed in the first session slice.
   - Decision: the new session login flow still verifies administrator usernames and password hashes from `admin_credentials_file` rather than introducing a new administrator database.
   - Rationale: the previous design deliberately kept administrator identity distinct from product users. This change should unlock frontend auth without widening the identity model.

2. Use a server-managed in-memory session store for the first slice.
   - Decision: administrator sessions are represented by random session identifiers stored server-side in process memory with idle and absolute expiry tracking.
   - Rationale: `go-ginx-2` is still a single-node runtime with a narrow admin surface. In-memory sessions keep the first slice small and avoid prematurely designing distributed auth concerns.
   - Trade-off: administrator sessions are lost on process restart. That is acceptable for the first slice and should be documented.

3. Introduce a dedicated same-origin admin API prefix for the surviving browser-facing admin surface.
   - Decision: the new session-oriented admin API lives under `/api/admin/*`.
   - Route set:
     - `POST /api/admin/login`
     - `POST /api/admin/logout`
     - `GET /api/admin/session`
     - `POST /api/admin/graphql`
   - Rationale: a stable prefix keeps frontend route handling unambiguous and makes it possible to delete the old HTML routes cleanly.

4. Remove browser-facing Basic Auth, the server-rendered admin surface, and the legacy GraphQL browser entrypoint in this slice.
   - Decision: once this change lands, browser-facing administrator access no longer relies on repeated Basic Auth prompts, the inline server-rendered admin pages plus their form handlers are removed rather than carried forward, and the legacy `POST /graphql` route is retired in favor of `POST /api/admin/graphql`.
   - Rationale: the user explicitly does not want to preserve the old auth model or the old server-rendered admin experience, so carrying either forward would add migration complexity without enough value.
   - Constraint: the management backend becomes API-only for browser-facing administration until the dedicated frontend shell arrives.

5. Make removed browser-facing admin routes fail explicitly instead of silently redirecting.
   - Decision: after this slice, the admin listener serves only the session-oriented API routes under `/api/admin/*`. Removed browser-facing routes such as `/`, `/users`, `/clients`, `/proxies`, `/certificates`, `/audit`, and the legacy `POST /graphql` path return `404 Not Found` rather than redirecting.
   - Rationale: redirecting would imply a replacement browser UI that does not exist yet. Explicit `404` behavior keeps the listener honest and simplifies operator expectations until the frontend shell lands.

6. Use cookie-backed sessions plus explicit CSRF tokens for browser mutations.
   - Decision: successful login returns a session cookie plus a CSRF token in the bootstrap payload. Session-authenticated mutation requests, including GraphQL mutations and logout, must send the CSRF token through a dedicated request header.
   - Rationale: cookie-backed browser auth is the best fit for a same-origin admin console, but it requires CSRF protection for state-changing requests.
   - Scope note: login itself does not require a preexisting CSRF token because it is the entry to session creation.

7. Make session bootstrap a minimal auth-context endpoint, not a business-data endpoint.
   - Decision: `GET /api/admin/session` only returns the minimal information required to bootstrap the frontend shell: whether a valid session exists, administrator identity metadata, CSRF token, and small configuration hints such as polling defaults if needed.
   - Rationale: bootstrap is for auth context recovery and route guards, not for initial dashboard hydration.

8. Share one administrator actor context across all surviving session-authenticated admin entrypoints.
   - Decision: all session-authenticated admin routes populate the same administrator actor context consumed by `internal/admin/service.go`, `internal/adminquery/service.go`, and GraphQL resolvers.
   - Rationale: backend command/query behavior should stay independent from whether the caller reached it through session bootstrap or session-authenticated GraphQL.

9. Treat session failure semantics as part of the API contract.
   - Decision: login failures, unauthenticated session bootstrap, invalid CSRF, expired sessions, and forbidden access must all have distinct, machine-readable API semantics.
   - Rationale: the future frontend shell needs deterministic auth behavior and cannot rely on generic transport errors.

## Target Flow

```text
Browser / future frontend shell
  -> POST /api/admin/login
       -> verify admin_credentials_file
       -> create in-memory session
       -> set HttpOnly secure cookie
       -> return bootstrap payload + CSRF token

Browser reload / route entry
  -> GET /api/admin/session
       -> validate session cookie
       -> return minimal admin context + CSRF token

Frontend GraphQL query/mutation
  -> POST /api/admin/graphql
       -> validate session cookie
       -> validate CSRF for mutations
       -> inject admin actor into context
       -> execute GraphQL

Logout
  -> POST /api/admin/logout
       -> validate session + CSRF
       -> invalidate session
       -> clear cookie

Removed
  -> server-rendered admin pages
  -> inline admin template rendering path
  -> browser-facing form-post admin workflow
  -> legacy POST /graphql browser entrypoint
  -> legacy admin page paths such as /, /users, /clients, /proxies, /certificates, /audit
```

## Route Contract

### `POST /api/admin/login`

- Input: administrator username and password.
- Success behavior:
  - verify credentials against the protected administrator credential set
  - rotate any existing session identifier
  - create a new administrator session
  - set an `HttpOnly`, `Secure`, `SameSite` session cookie
  - return a small bootstrap payload including administrator identity metadata and a CSRF token
- Failure behavior:
  - return a structured authentication failure without exposing which part of the credentials was incorrect

### `GET /api/admin/session`

- Input: existing session cookie
- Success behavior:
  - validate session existence and expiry
  - return `authenticated: true` plus minimal admin bootstrap data and a CSRF token
- Unauthenticated behavior:
  - return an unauthenticated response shape suitable for route guarding

### `POST /api/admin/logout`

- Input: existing session cookie plus CSRF header
- Behavior:
  - invalidate the session if present
  - clear the browser cookie
  - return a success response even if the browser session has already fallen away, so frontend logout remains idempotent from a UX perspective

### `POST /api/admin/graphql`

- Input: session cookie plus GraphQL request body
- Behavior:
  - queries require a valid session
  - mutations require both a valid session and a valid CSRF header
  - unauthenticated and forbidden failures remain machine-readable

### Removed browser-facing admin routes

- The inline HTML admin pages and their form-post handlers are removed in this slice instead of being moved onto session auth.
- The legacy browser-facing `POST /graphql` route is removed in favor of `POST /api/admin/graphql`.
- Legacy admin page paths such as `/`, `/users`, `/clients`, `/proxies`, `/certificates`, and `/audit` return `404 Not Found` until the dedicated frontend shell exists.
- Browser-facing management behavior remains available only through the session-oriented admin API surface until the dedicated frontend shell is implemented.

## Session Model

- Session identifier: cryptographically random opaque token.
- Storage: in-memory map or manager owned by the admin API runtime.
- Expiry behavior:
  - idle timeout refreshes on valid activity
  - absolute lifetime caps total session age
- Restart behavior:
  - process restart invalidates all administrator sessions
- Cookie behavior:
  - cookie is not readable by frontend JavaScript
  - frontend only handles returned bootstrap payload and CSRF token

## Validation Strategy

Implementation validation for this change should cover:

- successful login and bootstrap
- invalid login rejection
- bootstrap rejection when the session cookie is missing or expired
- logout invalidation and idempotent logout behavior
- session-authenticated GraphQL query access
- CSRF rejection for mutation requests lacking or mismatching the token
- route-removal tests proving the old server-rendered admin paths and legacy `POST /graphql` route are no longer served
- actor-context propagation consistency across all session-authenticated admin entrypoints

## Risks / Trade-offs

- [Risk] In-memory sessions disappear on restart. -> Mitigation: document that first-slice behavior clearly and keep re-login expectations explicit.
- [Risk] Removing the server-rendered admin surface before the dedicated frontend exists creates a temporary UX gap. -> Mitigation: accept this explicitly as a product decision for this slice and keep the surviving admin API documented for the next frontend change.
- [Risk] Operators may interpret a missing UI as an outage rather than an intentional cutover step. -> Mitigation: document that the admin listener is API-only in this slice and that removed browser paths return `404` by design.
- [Risk] CSRF design can sprawl if mixed with general API redesign. -> Mitigation: scope CSRF only to the cookie-backed admin session routes and GraphQL mutations in this change.
- [Risk] Frontend teams may try to use bootstrap as a general data preload endpoint. -> Mitigation: keep the bootstrap payload intentionally minimal in both spec and implementation.

## Open Questions

- No additional schema-level open questions remain for this slice. The main remaining work is implementation detail, route wiring, and clearly documenting that browser-facing administration is API-only until the frontend shell is added.
