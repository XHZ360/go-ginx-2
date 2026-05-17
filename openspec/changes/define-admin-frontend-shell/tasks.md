## 1. Information Architecture And Route Planning

- [ ] 1.1 Translate the approved page hierarchy into a concrete frontend route map for `login`, `dashboard`, `users`, `users/:id`, `clients`, `clients/:id`, `proxies`, `proxies/:id`, `certificates`, and `audit`
- [ ] 1.2 Define the authenticated shell responsibilities for navigation chrome, current-session display, logout affordance, and route-level error handling
- [ ] 1.3 Decide the root-route behavior for implementation (`/` shell landing versus redirect to `/dashboard`) without changing the approved page hierarchy

## 2. Session Guard And Shell Bootstrap Planning

- [ ] 2.1 Define the frontend bootstrap sequence around `/api/admin/session` for hard loads, deep links, and post-login initialization
- [ ] 2.2 Define guarded-route behavior for missing session, expired session, and explicit logout flows using the same-origin session endpoints
- [ ] 2.3 Define how intended-destination restoration works after a successful login without exposing protected page content before session validation

## 3. Page Container And Data-Consumption Planning

- [ ] 3.1 Map each confirmed page to its canonical backend contract usage, distinguishing session-endpoint interactions from GraphQL list, detail, and mutation flows
- [ ] 3.2 Define the list and detail container boundaries for `users`, `clients`, and `proxies`, including how route params drive canonical detail queries
- [ ] 3.3 Define top-level page container expectations for `dashboard`, `certificates`, and `audit` without assuming unsupported detail routes

## 4. Shared UX State Planning

- [ ] 4.1 Define reusable page-state patterns for loading, empty baseline, empty filtered, not-found, validation failure, backend failure, and session-expiry handling
- [ ] 4.2 Define post-mutation refresh expectations for list and detail pages so implementation can re-query canonical GraphQL data without whole-shell reloads
- [ ] 4.3 Define destructive or lifecycle-action confirmation expectations for proxy and certificate operations in later frontend implementation

## 5. Polling And Validation Planning

- [ ] 5.1 Define page-scoped polling ownership and cadence for dashboard, clients, proxies, certificates, users, and audit according to the approved frontend model
- [ ] 5.2 Define acceptance scenarios for route guarding, shell navigation, deep-link bootstrap, and page-state handling against the same-origin admin API baseline
- [ ] 5.3 Define acceptance scenarios for canonical GraphQL consumption so later frontend implementation can verify list/detail wiring without redefining backend semantics
