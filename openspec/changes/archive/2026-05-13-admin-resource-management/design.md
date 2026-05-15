## Context

`docs/requirements.md` requires a management backend for dashboards, users, clients, proxies, domains, certificates, statistics, logs, audit, settings, and permissions. `docs/design.md` defines this as a GraphQL management API plus web administration modules.

Current `go-ginx-2` evidence supports only a milestone-one non-interactive admin CLI for seeding SQLite resources: users, client credentials, TCP proxy records, UDP proxy records, and HTTP proxy records. Current docs explicitly list GraphQL/admin UI, quotas, alerts, and production deployment as not implemented.

## Goals / Non-Goals

**Goals:**

- Establish a baseline `admin-resource-management` capability spec from current CLI seeding evidence.
- Separate implemented non-interactive SQLite seeding from required-but-unimplemented GraphQL API and web UI management.
- Make each scenario usable as a future acceptance-test seed.
- Preserve traceability to product/design/current-progress docs without copying all source prose.

**Non-Goals:**

- Do not implement GraphQL APIs, web UI, dashboards, filtering, detail pages, permissions, quotas, logs, audit query, or settings.
- Do not change CLI commands, repositories, database schemas, validation rules, tests, or code.
- Do not claim current CLI seeding is a full management backend.

## Decisions

1. Treat admin CLI seeding as the current baseline.
   - Rationale: current evidence shows `goginx-admin` can create user, client credential, TCP proxy, UDP proxy, and HTTP proxy records in SQLite.
   - Alternative considered: mark admin management entirely as a gap. That would understate the implemented milestone-one bootstrap surface.

2. Track GraphQL/admin UI as gaps.
   - Rationale: product/design require a broader management plane, but current progress documents explicitly list GraphQL/admin UI as not implemented.

3. Keep quota, logs, audit query, certificates, and settings in adjacent future capabilities unless the admin surface needs to expose them.
   - Rationale: those domains have product requirements but should not be implied by the current CLI seeding baseline.

## Risks / Trade-offs

- [Risk] CLI seeding may be confused with a production admin API. -> Mitigation: the spec names it as non-interactive milestone-one bootstrap only.
- [Risk] Admin scope can absorb every product capability. -> Mitigation: this spec focuses on resource-management surfaces and tracks adjacent domains as gaps.
- [Risk] Future API/UI work may drift from current CLI semantics. -> Mitigation: future changes should update the spec with evidence-backed scenarios before claiming support.
