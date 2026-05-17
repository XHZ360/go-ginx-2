## Context

The admin backend direction is already settled for this slice. The browser consumes a same-origin admin surface with `POST /api/admin/login`, `GET /api/admin/session`, `POST /api/admin/logout`, and `POST /api/admin/graphql`, while the canonical GraphQL contracts for dashboard, users, clients, proxies, certificates, and audit have already been aligned as the backend baseline. What remains undefined is the frontend page model that will sit on top of those contracts.

Without a page-model design, later frontend implementation would have to make implicit decisions about route grouping, shell ownership, navigation depth, page refresh behavior, and failure states. Those choices are cross-cutting because they affect login bootstrap, guarded navigation, list/detail composition, mutation refresh strategy, and how each page consumes the same-origin admin API.

This change captures the target frontend shell and page definitions without selecting a specific framework or implementing any application code.

## Goals / Non-Goals

**Goals:**
- Define one dedicated admin frontend shell for the confirmed administrator views only.
- Define the route hierarchy, shell boundaries, navigation model, and guarded-route behavior for the same-origin admin console.
- Define shared page-state semantics for loading, empty, error, not-found, and session-expiry handling.
- Define how each confirmed page consumes the canonical GraphQL contracts, including where list/detail composition is expected.
- Define page-scoped polling expectations so runtime-oriented pages stay fresh without whole-app refresh behavior.
- Leave later frontend implementation with apply-ready planning tasks rather than framework ambiguity.

**Non-Goals:**
- Do not implement frontend routes, components, or framework setup.
- Do not redesign the admin backend contracts, session model, or GraphQL schema.
- Do not widen scope into quotas/settings pages, alerts center, broader observability pages, domain workflows, RBAC redesign, or ordinary-user self-service.
- Do not assume a specific SPA, SSR, or build tool stack unless a future implementation change chooses one.

## Decisions

1. Use one dedicated route shell with two top-level route groups.
   - Decision: the admin frontend is organized into an unauthenticated `login` route group and an authenticated application-shell group for all confirmed admin pages.
   - Rationale: this keeps session bootstrap and route-guard behavior consistent while avoiding per-page duplication of navigation chrome, auth checks, and global error/session handling.
   - Alternatives considered:
   - Render every page as an independent entry route with duplicated auth bootstrapping. This simplifies single-page ownership but fragments navigation, layout, and session-expiry handling.
   - Use multiple nested shells by resource area. This could help future expansion, but it adds structure before the first batch proves the basic console layout.

2. Keep navigation flat in the first frontend batch.
   - Decision: primary shell navigation includes `Dashboard`, `Users`, `Clients`, `Proxies`, `Certificates`, and `Audit`, with no additional top-level items.
   - Rationale: these are the only confirmed views, and a flat navigation model matches the current admin scope without inventing future areas.
   - Alternatives considered:
   - Add placeholder nav items for settings, quotas, alerts, or observability. This would imply scope the project has explicitly excluded.
   - Group pages into "Operations" and "Security" sections now. This may become useful later, but it is premature for the first shell definition.

3. Treat login as the only unauthenticated page.
   - Decision: `/login` is the sole public route, and all other admin routes require successful bootstrap from `/api/admin/session`.
   - Rationale: the backend contract is administrator-only and same-origin. Defining a single unauthenticated entry keeps route guards simple and avoids implying public informational pages.
   - Alternatives considered:
   - Allow public landing or help pages under the admin frontend. No current requirement or backend need justifies them.

4. Guarded routes are bootstrap-first and redirect-driven.
   - Decision: protected routes resolve session state through `/api/admin/session` before rendering page content; missing or expired sessions redirect to `/login`, and logout clears shell state before returning there.
   - Rationale: this matches the backend session model and provides one consistent path for hard reloads, direct deep links, and mid-session expiry.
   - Alternatives considered:
   - Trust cached client auth state first and lazily recover later. This risks stale rendering and inconsistent redirect behavior after restart or expiry.

5. Page hierarchy is list-first with detail routes only where the backend contract already supports detail views.
   - Decision: `users`, `clients`, and `proxies` include list and detail pages; `dashboard`, `login`, `certificates`, and `audit` are top-level pages without a required separate detail route in this change.
   - Rationale: this matches the already-confirmed backend scope and avoids inventing unsupported detail experiences for certificates or audit.
   - Alternatives considered:
   - Define detail routes for every resource area. This would overfit the shell to future possibilities not yet confirmed.

6. Shared page states are part of the route model, not an implementation afterthought.
   - Decision: the shell and page definitions explicitly require loading, empty, error, and not-found handling at route or page level, with session-expiry treated separately from generic backend failure.
   - Rationale: page-state behavior is one of the main reasons to formalize the frontend model before implementation. It also prevents each page from inventing different semantics for the same backend conditions.
   - Alternatives considered:
   - Leave state handling to whichever framework is chosen later. That would cause contract drift across pages and weaken acceptance planning.

7. Polling remains page-scoped and resource-aware.
   - Decision: dashboard, clients, and proxies use 5-second polling; certificates use low-frequency or manual refresh depending on lifecycle activity; users and audit use manual or low-frequency refresh; login never polls.
   - Rationale: this preserves the canonical backend expectations while making page refresh ownership explicit at the frontend model layer.
   - Alternatives considered:
   - Use one global polling loop for the entire shell. That would over-refresh low-churn pages and create unnecessary cross-page coupling.
   - Introduce subscriptions. This is out of scope for the confirmed first batch.

8. Canonical GraphQL contracts are consumed through page-oriented containers.
   - Decision: each route definition maps to a page container that owns the relevant list, detail, or mutation flows against `/api/admin/graphql`, while login/session/logout remain the only non-GraphQL page interactions.
   - Rationale: this keeps route ownership aligned with the backend contract boundary and avoids mixing resource mutations into shell or navigation concerns.
   - Alternatives considered:
   - Let the shell own broad data fetching for multiple pages. This would complicate refresh scope and blur page boundaries.

9. Future frontend touchpoints are recorded conceptually but not promoted into current IA.
   - Decision: the design acknowledges likely future touchpoints such as quotas, settings, alerting, broader observability, and domain workflows as adjacent areas that may later extend shell navigation, but the current page model does not reserve active routes or UI chrome for them.
   - Rationale: this keeps the current shell honest while leaving room for future extension.
   - Alternatives considered:
   - Hard-code extension slots into the route map now. That creates speculative structure without confirmed requirements.

## Risks / Trade-offs

- [Risk] A framework-agnostic design may still leave some implementation decisions open. -> Mitigation: define route ownership, page hierarchy, states, and polling behavior in enough detail that framework choice does not change the user-facing model.
- [Risk] Keeping navigation flat may need revision if future admin areas expand significantly. -> Mitigation: record future touchpoints conceptually so later changes can regroup navigation without redefining the confirmed first batch.
- [Risk] Detail-route scope could drift during implementation if teams infer unsupported views. -> Mitigation: state explicitly which pages have list/detail hierarchy in this change and keep other areas top-level only.
- [Risk] Polling expectations could become inconsistent across pages. -> Mitigation: attach refresh cadence to each page definition and keep polling page-scoped rather than shell-global.
- [Risk] Session-expiry behavior can be mistaken for a generic page error. -> Mitigation: require auth-expiry handling to remain distinct from validation, not-found, and internal-failure states.

## Migration Plan

1. Use this change to finalize the frontend shell, route map, page hierarchy, and state model against the current same-origin admin API baseline.
2. In a later implementation change, select the frontend framework and map the defined routes and page containers into real application code.
3. Implement login bootstrap and guarded-shell behavior against `/api/admin/login`, `/api/admin/session`, and `/api/admin/logout` before building protected pages.
4. Implement page containers in the confirmed order that best validates the shell, likely starting with dashboard plus one list/detail pair, while preserving the canonical GraphQL contracts at `/api/admin/graphql`.
5. Add route-level and page-level acceptance coverage for navigation, deep-link guarding, empty/error states, and page-scoped polling as part of frontend implementation.

Rollback is documentation-only for this change because no runtime behavior is introduced.

## Open Questions

- The exact URL shape for the shell root can stay `/` plus named child routes or redirect `/` to `/dashboard`; implementation should preserve the same page hierarchy either way.
- Certificates may later gain a dedicated detail route if the backend contract adds a distinct certificate detail view, but this change does not require that route.
- Audit may later evolve into broader observability navigation, but the first shell keeps it as a standalone recent-activity page.
