## 1. Management Authentication Baseline

- [x] 1.1 Add protected configuration for administrator usernames and password verifiers independent of SQLite product users.
- [x] 1.2 Add TLS-aware HTTP Basic Auth protection for the V1 admin API/UI entrypoints.
- [x] 1.3 Add tests covering valid admin access, invalid credentials, and unauthenticated rejection for management endpoints.

## 2. Admin Query Layer

- [x] 2.1 Introduce a dedicated admin query service for dashboard, user, client, proxy, certificate, and audit read models.
- [x] 2.2 Add V1 dashboard queries for the confirmed runtime-oriented cumulative summary fields.
- [x] 2.3 Add V1 user list/detail queries and matching DTOs.
- [x] 2.4 Add V1 client list/detail queries that combine persisted data with runtime session state.
- [x] 2.5 Add V1 proxy list/detail queries that combine persisted data with runtime and aggregate status information.
- [x] 2.6 Add V1 managed-certificate status queries and minimal recent-audit list queries.

## 3. Admin Mutation Layer

- [x] 3.1 Extend the admin write-side service for V1 user create, disable, and password-modification flows.
- [x] 3.2 Extend the admin write-side service for V1 proxy create, update, enable, disable, and delete flows.
- [x] 3.3 Enforce V1 proxy lifecycle rules: supported reverse-proxy types only, immutable proxy type, and disable-before-delete.
- [x] 3.4 Reuse existing managed-certificate issue and renew operations through the V1 admin mutation surface.

## 4. GraphQL Management Surface

- [x] 4.1 Add the administrator-only GraphQL schema for the confirmed V1 queries and mutations.
- [x] 4.2 Keep GraphQL resolvers thin by routing reads through the admin query service and writes through the admin service.
- [x] 4.3 Disable realtime subscriptions for V1 and document the 5-second polling contract at the API boundary.

## 5. V1 Admin UI

- [x] 5.1 Add the V1 administrator dashboard view for the confirmed summary fields.
- [x] 5.2 Add V1 user management pages for list, detail, create, disable, and password modification.
- [x] 5.3 Add V1 client pages for list and detail.
- [x] 5.4 Add V1 proxy pages for list, detail, create, update, enable, disable, and delete.
- [x] 5.5 Add V1 managed-certificate status and issue/renew views plus the minimal recent-audit list view.
- [x] 5.6 Implement 5-second polling for runtime-oriented UI views instead of realtime push.

## 6. Validation And Documentation

- [x] 6.1 Add tests for V1 dashboard queries, user-management mutations, proxy lifecycle mutations, and recent-audit queries.
- [x] 6.2 Add tests proving proxy type immutability and disable-before-delete behavior.
- [x] 6.3 Update runtime and admin documentation to describe the V1 admin-only scope, admin credential configuration, polling model, and remaining gaps.
- [x] 6.4 Run the relevant test suites and validate the V1 admin API/UI end to end.
