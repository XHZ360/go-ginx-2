## Context

The current codebase already has an embryonic write-side admin layer in `internal/admin/service.go`, plus SQLite repositories, runtime session state, certificate lifecycle operations, and cumulative stats. What it does not have is a management-plane read model suitable for a real API/UI. Product and design documents expect a much richer admin surface with GraphQL, dashboards, list/detail pages, permissions, audit/log views, and settings, but the present backend shape only supports point writes and a few lookup patterns.

The core architectural risk is starting GraphQL or the web UI by binding resolvers directly to the existing repositories and in-memory runtime primitives. That would push query aggregation, pagination, permission filtering, and runtime state composition into the transport layer and make the eventual management plane harder to evolve.

## Goals / Non-Goals

**Goals:**
- Capture the architectural conclusion that the management plane should split write-oriented admin use cases from read-oriented admin query models.
- Establish that GraphQL is a transport layer on top of backend query/mutation services, not the place where storage and runtime state are stitched together.
- Force scoping decisions for the first API/UI batch: roles served, resources covered, and runtime truth sources.
- Keep the first management-plane implementation from accidentally absorbing quotas, logs, alerts, settings, and other adjacent domains all at once.

**Non-Goals:**
- Do not implement GraphQL, web UI pages, human-authentication flows, or repository changes in this capture.
- Do not finalize the exact first-batch screen list or schema fields.
- Do not solve quota enforcement, settings management, or full observability query surfaces here.

## Decisions

1. Separate mutation and query concerns in the admin backend.
   - Decision: keep `internal/admin/service.go` as the write-side mutation/use-case layer and introduce a distinct read/query layer for future API/UI work.
   - Rationale: current admin service methods are command-shaped: validate input, write a resource, and record audit. Admin API/UI reads need different behavior: list/detail DTOs, pagination, filtering, aggregation, viewer-aware access control, and runtime state composition.
   - Alternatives considered:
     - Extend current repositories and let GraphQL call them directly: fast initially, but would mix business queries, runtime joins, and transport concerns in resolvers.
     - Force everything through domain aggregates: too awkward for management pages whose data shape is intentionally denormalized and presentation-oriented.

2. Treat GraphQL as an exposure layer, not the primary architecture boundary.
   - Decision: future GraphQL queries should call query services, and mutations should call admin use-case services.
   - Rationale: the management plane will likely need the same backend semantics from multiple entry points later, such as UI, admin CLI expansions, export jobs, or internal tooling. Keeping GraphQL thin preserves that flexibility.
   - Alternatives considered:
     - Let resolver code perform direct store/session/stats joins: easier short-term, but creates resolver-centric backend logic and permission sprawl.

3. Build read models as API/UI-facing DTOs instead of reusing domain structs directly.
   - Decision: list/detail/dashboard queries should return purpose-built shapes such as `UserListItem`, `ClientDetail`, or `ProxyDetail`, composed from SQLite config state plus runtime/session/stats overlays.
   - Rationale: current domain models represent core persisted concepts, not management views. Admin pages need counts, summaries, status rollups, last-activity fields, and error summaries that do not belong in the core domain structs.
   - Alternatives considered:
     - Inflate domain structs with UI-specific fields: would blur domain boundaries and make runtime/query concerns leak into persistence-facing types.

4. Require explicit role and scope decisions before the first API/UI implementation batch.
   - Decision: the next implementation-oriented change must choose whether V1 serves administrators only or both administrators and ordinary users, and must choose a reduced first-batch resource set.
   - Rationale: role scope multiplies authorization, navigation, and query filtering complexity. Resource scope decides whether the first batch remains a manageable management console or balloons into a full control plane.
   - Alternatives considered:
     - Build for all roles and all resources immediately: too much surface area while the backend query model is still undefined.

5. Treat human authentication as a separate identity system from client credentials.
   - Decision: management-plane login/session design must be handled independently instead of extending `client_id + credential` semantics.
   - Rationale: product requirements call for human login, authorization, and UI access. Client credentials are machine identities for the control plane and should not be repurposed for user-facing sessions.

6. Use configuration-file-backed administrator credentials plus HTTP Basic Auth for the first admin API/UI batch.
   - Decision: V1 admin authentication uses administrator username/password credentials stored in protected server-side configuration rather than in the existing SQLite `users` table, and the management API/UI is protected by HTTP Basic Auth over TLS.
   - Rationale: this keeps the first management-plane authentication model small and operationally controlled, avoids prematurely coupling product-user records to human admin login, and lets the team focus on query/mutation boundaries and runtime-management views before building a richer session system.
   - Alternatives considered:
     - Reuse `client_id + credential`: invalid because client credentials are machine identities, not human management identities.
     - Add full username/password plus cookie-session login immediately: more product-like long term, but too much surface area before the first admin query model exists.
     - Store admin passwords in the current SQLite `users` table: would blur the boundary between product users and control-plane administrators before role and session semantics are nailed down.

7. Keep the first API/UI batch tightly scoped to runtime-oriented administrator resources.
   - Decision: V1 covers `DashboardSummary`, `User` management, `Client` list/detail views, `Proxy` list/detail views, and managed-certificate status/issue/renew flows. Within `User` management, V1 includes administrator-facing list, detail, create, disable, and password-modification flows. Quota/settings editing, domain lifecycle management, log search, audit list views, and alert-center workflows remain out of scope for this first batch.
   - Rationale: adding user management is still coherent with an administrator-only first release because administrators already own user bootstrap and resource control in the product model. Combining `User`, `Client`, `Proxy`, and certificate surfaces gives V1 a usable control-console shape without dragging in every adjacent management domain at once, while still keeping the user surface focused on core operational actions rather than full tenant lifecycle complexity.
   - Alternatives considered:
    - Start with the full admin surface including users, logs, quotas, settings, and alerts: too broad while the backend read model is still being established.
    - Start with user management first: less aligned with the current runtime-operability strengths of the codebase and more likely to entangle product-tenant administration before runtime views are solved.

8. Treat `Proxy` as a first-class managed resource in V1, but keep its lifecycle rules explicit.
   - Decision: V1 proxy management includes list, detail, create, update, enable, disable, and delete flows for the currently supported reverse-proxy types (`TCP`, `UDP`, `HTTP`, `HTTPS`). Proxy type is immutable after creation, and delete is a controlled lifecycle action that requires the proxy to be disabled first.
   - Rationale: proxy management is the core control-plane surface most directly tied to current runtime value. Making proxy type immutable avoids turning update into a hidden migration workflow across entry semantics, certificate semantics, and runtime wiring. Requiring disable-before-delete preserves clearer lifecycle semantics and reduces the chance of destructive operations racing active runtime state.
   - Alternatives considered:
     - Allow proxy type mutation in place: simpler on the surface, but semantically closer to delete-and-recreate and much harder to reason about for runtime updates, audit, and validation.
     - Allow direct delete regardless of current status: operationally faster, but blurs lifecycle boundaries and makes runtime shutdown behavior less predictable.

9. Use 5-second polling instead of realtime subscriptions for the first admin API/UI batch.
   - Decision: V1 administrator pages use polling with a 5-second refresh interval for runtime-oriented views instead of GraphQL subscriptions or other realtime push mechanisms.
   - Rationale: the first management batch already has enough complexity in query shaping, runtime state composition, and control actions. A fixed polling interval gives predictable behavior for dashboard, client, proxy, and certificate status views without introducing subscription lifecycle, connection fan-out, or event-filtering design work too early.
   - Alternatives considered:
     - Add GraphQL subscriptions in V1: closer to the long-term design target, but too much extra transport and authorization complexity for the first slice.
     - Poll more aggressively: gives fresher state, but increases backend read pressure before query paths are optimized.
     - Poll less frequently: simpler operationally, but weakens the usefulness of the runtime control console.

10. Keep `DashboardSummary` minimal and aligned to currently trustworthy runtime aggregates.
   - Decision: V1 `DashboardSummary` uses a runtime-oriented, cumulative summary instead of a product-style overview with time-window traffic or alert state. The mandatory V1 fields are `onlineClientCount`, `enabledProxyCount`, `activeTCPConnectionCount`, `cumulativeUploadBytes`, `cumulativeDownloadBytes`, `cumulativeTCPErrorCount`, `cumulativeUDPErrorCount`, and `cumulativeHTTPErrorCount`.
   - Rationale: this preserves honesty about what the current system can answer reliably without prematurely inventing daily aggregations, alert rollups, or richer observability projections that belong to later observability work.
   - Alternatives considered:
     - Include `today` traffic and alert summaries immediately: more product-shaped, but would force unfinished time-window metrics and alert-state modeling into the first admin batch.

11. Exclude initial resource-limit assignment from V1 user creation.
   - Decision: V1 administrator user-creation flow does not assign initial proxy, port, traffic, or bandwidth limits. Resource-limit assignment stays out of scope until the quota/limit model is implemented as a real backend capability.
   - Rationale: the codebase does not yet have persisted quota models or enforcement behavior. Exposing editable limit fields before those semantics exist would create configuration that looks authoritative but is not actually enforced.
   - Alternatives considered:
     - Add limit fields as non-enforced metadata placeholders: rejected because it would mislead operators about actual runtime behavior.

12. Include a minimal audit list in V1, but keep it intentionally shallow.
   - Decision: V1 includes a recent audit-event list for administrators rather than leaving audit visibility entirely to later work. The initial list stays minimal: reverse chronological events with actor, resource type, resource ID, action, result, and timestamp, without advanced filtering, export, or full observability workflow.
   - Rationale: once V1 includes user management, proxy lifecycle control, and certificate operations, administrators need a direct control-plane feedback loop for what was changed. A lightweight audit list provides that without dragging full log-query scope into the first batch.
   - Alternatives considered:
     - Leave audit out of V1 completely: simpler, but weakens operational confidence in control actions.
     - Add full audit filtering/export/query surface immediately: too large for the first management-plane slice.

## Risks / Trade-offs

- [Risk] Adding a dedicated query layer may feel like extra structure before GraphQL/UI code exists. -> Mitigation: keep the split lightweight at first, with a small `adminquery` service surface instead of a large framework.
- [Risk] V1 scope remains ambiguous even with the architectural decision captured. -> Mitigation: make role scope and first-batch resource scope explicit prerequisites for the next implementation change.
- [Risk] Runtime state composition becomes a hidden complexity sink. -> Mitigation: define per-view truth sources up front, such as SQLite config state plus session/stats overlays, before schema design starts.
- [Risk] Product documents may keep pushing adjacent concerns into the first admin batch. -> Mitigation: treat quotas, settings, full log search, and advanced alerts as separate follow-on domains unless explicitly pulled into scope.
- [Risk] Adding user management to V1 can expand the backend faster than the initial query layer can comfortably support. -> Mitigation: keep user management focused on administrator list/detail/create/disable/password-modification flows and avoid pulling quota editing, self-service password recovery, or full tenant self-service into the same batch.
- [Risk] Full proxy CRUD in V1 can leak advanced proxy-policy concerns into the first batch. -> Mitigation: keep proxy CRUD limited to core reverse-proxy resource fields and lifecycle actions, while leaving access-passwords, share links, quota policy, forward-proxy policy, and richer domain workflows for later changes.
- [Risk] Fixed 5-second polling can feel less responsive during fast-changing incidents. -> Mitigation: accept the latency trade-off for V1 and treat subscriptions or adaptive refresh as a later enhancement once query surfaces are stable.
- [Risk] A minimal dashboard may feel underpowered compared with the PRD wording. -> Mitigation: explicitly frame V1 dashboard data as trusted runtime summary and leave time-window metrics and alerts to later observability work.
- [Risk] Excluding user-limit assignment from create-user may frustrate administrators expecting one-step provisioning. -> Mitigation: document that quotas and limits remain a later capability instead of exposing non-enforced placeholders.
- [Risk] Even a minimal audit list can tempt the scope toward full observability query features. -> Mitigation: keep V1 audit list intentionally shallow and defer filtering/export/log correlation to later changes.
- [Risk] Configuration-file-backed admin credentials can become awkward once multiple administrators or richer session flows are needed. -> Mitigation: treat Basic Auth plus config-file credentials as a V1 baseline only, and leave migration to a fuller human-auth system as explicit future work.

## Migration Plan

1. Use this design as the architectural baseline for the next admin API/UI proposal.
2. In the next change, keep the now-confirmed role scope, first-batch resources, and config-file-backed admin authentication approach fixed unless a stronger product reason appears.
3. Define the first query DTOs and GraphQL schema slices against the now-confirmed V1 scope.

## Open Questions

- No additional open questions remain for the V1 scope-definition pass. Future changes may refine implementation details, but the current V1 management scope is now fixed.
