## 1. Frontend Workspace And Delivery Foundation

- [ ] 1.1 Choose and scaffold the admin frontend workspace and framework in a way that can serve browser routes on the admin origin without conflicting with `/api/admin/*`.
- [ ] 1.2 Define and implement local development same-origin delivery wiring so frontend browser traffic can reach the admin API without introducing cross-origin behavior not present in production.
- [ ] 1.3 Define and implement production-facing frontend asset delivery and route fallback behavior for `/`, `/login`, `/dashboard`, `/users`, `/clients`, `/proxies`, `/certificates`, and `/audit` while preserving `/api/admin/*` as API namespace.

## 2. Shell And Session Bootstrap

- [ ] 2.1 Implement the public and protected route groups with `/login` as the only public route and `/` redirect behavior based on `GET /api/admin/session`.
- [ ] 2.2 Implement protected-shell bootstrap so hard loads, reloads, and deep links validate the current administrator session before protected content renders.
- [ ] 2.3 Implement intended-destination restoration after successful login with safe fallback to `/dashboard` for missing or invalid destinations.
- [ ] 2.4 Implement explicit logout behavior that clears shell session state and returns the browser to `/login`.

## 3. Request Layer And Error Semantics

- [ ] 3.1 Implement a session-oriented request client for `/api/admin/login`, `/api/admin/session`, and `/api/admin/logout`.
- [ ] 3.2 Implement a GraphQL request client for `/api/admin/graphql` with canonical response parsing and shared transport behavior.
- [ ] 3.3 Implement CSRF-aware mutation handling so browser write operations include the session bootstrap token where required.
- [ ] 3.4 Implement centralized frontend error mapping for `UNAUTHENTICATED`, `FORBIDDEN`, `VALIDATION_FAILED`, `NOT_FOUND`, `CONFLICT`, `UNSUPPORTED`, `ENTRY_CONFLICT`, and `INTERNAL`.

## 4. Shared Shell And Page-State Primitives

- [ ] 4.1 Implement authenticated shell primitives for navigation chrome, current-administrator display, and route-level loading or auth-expiry handling.
- [ ] 4.2 Implement shared page-state primitives for loading, baseline empty, filtered empty, not found, validation failure, and backend failure states.
- [ ] 4.3 Implement shared confirmation patterns for destructive or lifecycle actions such as proxy disable or delete and certificate issue or renew.

## 5. Page Container Implementation

- [ ] 5.1 Implement the dashboard page container with summary reads and page-scoped 5-second polling.
- [ ] 5.2 Implement users list and detail page containers with create, disable, and password-change flows plus targeted refresh after mutations.
- [ ] 5.3 Implement clients list and detail page containers with runtime-aware reads and page-scoped polling.
- [ ] 5.4 Implement proxies list and detail page containers with create, update, enable, disable, delete-after-disable, and explicit `ENTRY_CONFLICT` handling plus page-scoped polling.
- [ ] 5.5 Implement the certificates page container with lifecycle-oriented status reads, issue and renew actions, and low-frequency or action-driven refresh.
- [ ] 5.6 Implement the audit page container with recent-event reads, lightweight filtering, and manual or low-frequency refresh.

## 6. Verification

- [ ] 6.1 Add frontend acceptance coverage for root-route redirect behavior, login redirect behavior, protected-route bootstrap, and intended-destination restoration.
- [ ] 6.2 Add frontend acceptance coverage for shared page states, including auth expiry, not found, validation failure, baseline empty, and filtered empty behavior.
- [ ] 6.3 Add frontend acceptance coverage for page-scoped polling ownership and targeted post-mutation refresh behavior.
- [ ] 6.4 Add integration coverage for same-origin frontend plus `/api/admin/*` delivery in local development and production-like serving paths.
