## 1. Session Foundation

- [x] 1.1 Add an administrator session manager with random session IDs, idle expiry, absolute expiry, and explicit invalidation.
- [x] 1.2 Keep administrator credential verification sourced from `admin_credentials_file` for session login.
- [x] 1.3 Add shared administrator actor-context injection for all session-authenticated admin requests.

## 2. Frontend-Facing Admin API Routes

- [x] 2.1 Add `POST /api/admin/login` for administrator session creation.
- [x] 2.2 Add `GET /api/admin/session` for browser session bootstrap.
- [x] 2.3 Add `POST /api/admin/logout` for session invalidation and cookie clearing.
- [x] 2.4 Add `POST /api/admin/graphql` as the session-authenticated GraphQL entrypoint for the separated frontend.

## 3. Cookie And CSRF Protection

- [x] 3.1 Add secure session-cookie issuance and clearing behavior for administrator sessions.
- [x] 3.2 Add CSRF token issuance in the bootstrap/login response path.
- [x] 3.3 Enforce CSRF validation for session-authenticated admin mutations while allowing query operations without CSRF.

## 4. Server-Rendered Admin Removal

- [x] 4.1 Remove the inline server-rendered admin template and browser-facing form-post management workflow.
- [x] 4.2 Remove the legacy browser-facing `/graphql` route and consolidate browser clients onto `POST /api/admin/graphql`.
- [x] 4.3 Make removed browser-facing admin page paths return explicit `404 Not Found` responses.
- [x] 4.4 Document the API-only admin route topology that remains after server-rendered admin removal.

## 5. Validation And Documentation

- [x] 5.1 Add tests for successful login, invalid login, session bootstrap, session expiry, and logout invalidation.
- [x] 5.2 Add tests for session-authenticated GraphQL access and CSRF rejection on mutation requests.
- [x] 5.3 Add route-removal tests proving server-rendered admin paths return `404`, the legacy `/graphql` route is no longer served, and browser-facing management uses the session-oriented admin API only.
- [x] 5.4 Update admin runtime documentation for session login, bootstrap, logout, cookie/CSRF behavior, restart invalidation expectations, and API-only admin listener behavior.
