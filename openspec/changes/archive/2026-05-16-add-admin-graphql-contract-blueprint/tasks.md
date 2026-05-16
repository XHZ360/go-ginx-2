## 1. Contract Baseline

- [x] 1.1 Record the same-origin admin route topology with HTTP session endpoints and the GraphQL business entrypoint at `/api/admin/graphql`.
- [x] 1.2 Record that `internal/adminquery` remains the read-model boundary and `internal/admin` remains the command-use-case boundary.
- [x] 1.3 Limit the blueprint scope to dashboard, users, clients, proxies, certificates, and audit.

## 2. Frontend-Consumable GraphQL Shape

- [x] 2.1 Define page-oriented list contracts with shared pagination, filter, and sort expectations.
- [x] 2.2 Define detail-query expectations for runtime overlays and resource-focused page models.
- [x] 2.3 Define mutation input/payload conventions for frontend forms and post-mutation refresh behavior.

## 3. Resource Semantics

- [x] 3.1 Define user list/detail/create/disable/password mutation semantics.
- [x] 3.2 Define client list/detail/create/enable/disable/rotate-credential semantics, including write-only credential handling and `managedProxies` in client detail.
- [x] 3.3 Define proxy list/detail/lifecycle semantics, type-specific config blocks, proxy type immutability, and delete-after-disable behavior.
- [x] 3.4 Define certificate lifecycle/status list semantics and issue/renew actions without private-key exposure.
- [x] 3.5 Define audit timeline semantics with actor identity direction that is broader than a user-only model.

## 4. Listener Admission And Error Semantics

- [x] 4.1 Define the ListenerClaim model across static listeners and enabled TCP/UDP proxies.
- [x] 4.2 Define the conservative V1 conflict rule of same network plus same port and exclude disabled proxies from the active claim set.
- [x] 4.3 Define `ENTRY_CONFLICT` behavior for socket-listener admission failures and document the current gap between database uniqueness and whole-runtime listener admission.
- [x] 4.4 Define structured GraphQL/frontend-consumable error semantics for authentication, authorization, validation, not-found, conflict, unsupported, entry-conflict, and internal failures.

## 5. Refresh And Validation Expectations

- [x] 5.1 Define the polling model for dashboard, clients, proxies, certificates, users, and audit.
- [x] 5.2 Define validation expectations for field-level error details where applicable.
- [x] 5.3 Define future acceptance-test targets for list contracts, write-only credentials, listener admission, certificate actions, and audit actor semantics.
