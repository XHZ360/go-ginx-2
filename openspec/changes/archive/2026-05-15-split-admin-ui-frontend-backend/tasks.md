## 1. Architecture And Auth Design

- [x] 1.1 Define the administrator session model that replaces browser-facing Basic Auth while preserving separate admin credentials.
- [x] 1.2 Define same-origin deployment expectations for the frontend app and admin API.
- [x] 1.3 Define the legacy server-rendered UI deprecation and migration plan.

## 2. Backend API Contract

- [x] 2.1 Define session/bootstrap HTTP endpoints for login, logout, and frontend auth bootstrap.
- [x] 2.2 Extend GraphQL list and detail contracts for pagination, filtering, sorting, and runtime polling needs.
- [x] 2.3 Define API error, validation, and authorization response semantics for the frontend.

## 3. Frontend Information Architecture

- [x] 3.1 Define the admin frontend route shell, guarded routes, and navigation model.
- [x] 3.2 Define page-level UX requirements for dashboard, users, clients, proxies, certificates, and audit.
- [x] 3.3 Define shared UI states for loading, empty, error, confirmation, and destructive actions.

## 4. Migration Delivery Plan

- [x] 4.1 Sequence the page migration from the V1 server-rendered UI to the separated frontend.
- [x] 4.2 Define compatibility expectations while legacy and new surfaces coexist.
- [x] 4.3 Update deployment and operator documentation for the new management-console topology.

## 5. Validation Strategy

- [x] 5.1 Define backend tests for session auth, API contracts, and authorization boundaries.
- [x] 5.2 Define end-to-end validation for the separated admin frontend against the real management API.
- [x] 5.3 Define cutover validation proving the legacy server-rendered UI can be retired.
