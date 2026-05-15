# Roadmap Gap Matrix

This matrix aligns the product requirements, design document, and current `go-ginx-2` implementation evidence. It uses the controlled status vocabulary from `specs/documentation-alignment/spec.md`: `required`, `designed`, `implemented`, `gap`, and `out-of-scope`.

## Source Hierarchy

1. Product intent: `docs/requirements.md`
2. Technical approach and acceptance mapping: `docs/design.md`
3. Current implementation evidence: `go-ginx-2/docs/continuation.md`, `go-ginx-2/docs/milestone-one-e2e.md`, `go-ginx-2/docs/daemon-runtime.md`, and `go-ginx-2/README.md`
4. OpenSpec requirements and deltas: `openspec/changes/**/specs/**/*.md`

Implementation status is determined only from current implementation evidence. Product or design scope without implementation evidence is classified as `gap`, not `implemented`.

## Matrix

| Capability | Requirement source | Design source | Current status | Implementation evidence | Gap / next action |
|---|---|---|---|---|---|
| Control channel, auth, heartbeat, and session tracking | `docs/requirements.md:13`, `docs/requirements.md:19`, `docs/requirements.md:203` | `docs/design.md:17`, `docs/design.md:32`, `docs/design.md:58`, `docs/design.md:92`, `docs/design.md:137` | `implemented` for QUIC MVP; `gap` for TCP+TLS fallback and full recovery semantics | `go-ginx-2/docs/continuation.md:16`, `go-ginx-2/docs/continuation.md:67`, `go-ginx-2/docs/milestone-one-e2e.md:22`, `go-ginx-2/docs/daemon-runtime.md:8` | Add TCP+TLS fallback runtime, explicit protocol negotiation tests, and deeper reconnect/recovery coverage. |
| TCP reverse proxy | `docs/requirements.md:14`, `docs/requirements.md:47`, `docs/requirements.md:116` | `docs/design.md:36`, `docs/design.md:169`, `docs/design.md:486` | `implemented` for MVP | `go-ginx-2/docs/continuation.md:27`, `go-ginx-2/docs/continuation.md:70`, `go-ginx-2/docs/milestone-one-e2e.md:65`, `go-ginx-2/docs/daemon-runtime.md:11` | Promote TCP behavior into a future baseline proxy spec and continue hardening limits, access controls, and error handling. |
| UDP reverse proxy | `docs/requirements.md:14`, `docs/requirements.md:47`, `docs/requirements.md:119` | `docs/design.md:36`, `docs/design.md:182`, `docs/design.md:490` | `implemented` for MVP | `go-ginx-2/docs/continuation.md:70`, `go-ginx-2/docs/milestone-one-e2e.md:99`, `go-ginx-2/docs/daemon-runtime.md:12` | Promote UDP behavior into a future baseline proxy spec and verify cleanup, limits, and error classification as the runtime matures. |
| HTTP reverse proxy | `docs/requirements.md:14`, `docs/requirements.md:47`, `docs/requirements.md:126` | `docs/design.md:36`, `docs/design.md:198`, `docs/design.md:486` | `implemented` for MVP | `go-ginx-2/docs/continuation.md:33`, `go-ginx-2/docs/continuation.md:70`, `go-ginx-2/docs/milestone-one-e2e.md:82`, `go-ginx-2/docs/daemon-runtime.md:13` | Add access password, share-link, and richer error/observability flows before claiming full HTTP product coverage. |
| HTTPS reverse proxy | `docs/requirements.md:14`, `docs/requirements.md:133`, `docs/requirements.md:305` | `docs/design.md:214`, `docs/design.md:486`, `docs/design.md:490` | `gap` | `go-ginx-2/docs/continuation.md:78`, `go-ginx-2/docs/milestone-one-e2e.md:132`, `go-ginx-2/docs/daemon-runtime.md:111` | Next implementation batch: decide TLS termination vs SNI passthrough order, add certificate selection/storage boundaries, wire HTTPS entry startup, and add certificate validation tests. |
| Forward proxy | `docs/requirements.md:15`, `docs/requirements.md:105`, `docs/requirements.md:321` | `docs/design.md:224`, `docs/design.md:490` | `gap` | `go-ginx-2/docs/continuation.md:78`, `go-ginx-2/docs/milestone-one-e2e.md:134`, `go-ginx-2/docs/daemon-runtime.md:113` | Add a controlled forward-proxy spec before implementation, including target allow rules, audit, limits, and abuse prevention. |
| Admin bootstrap and resource seeding | `docs/requirements.md:17`, `docs/requirements.md:180`, `docs/requirements.md:300` | `docs/design.md:28`, `docs/design.md:318`, `docs/design.md:486` | `implemented` for non-interactive milestone-one seeding; `gap` for full admin API/UI | `go-ginx-2/docs/continuation.md:39`, `go-ginx-2/docs/milestone-one-e2e.md:116`, `go-ginx-2/docs/examples/admin-seed-sqlite.md:3`, `go-ginx-2/README.md:45` | Use current CLI as setup evidence, then create baseline specs for GraphQL/admin UI, dashboards, filtering, and user-facing management. |
| GraphQL admin API and web UI | `docs/requirements.md:17`, `docs/requirements.md:180`, `docs/requirements.md:182` | `docs/design.md:318`, `docs/design.md:354`, `docs/design.md:486` | `gap` | `go-ginx-2/docs/continuation.md:78`, `go-ginx-2/docs/milestone-one-e2e.md:134`, `go-ginx-2/docs/daemon-runtime.md:115` | Define admin API/UI baseline specs after proxy runtime gaps are prioritized, then implement resource queries, mutations, permissions, and dashboard views. |
| SQLite persistence and repository boundaries | `docs/requirements.md:59`, `docs/requirements.md:272`, `docs/requirements.md:295` | `docs/design.md:16`, `docs/design.md:40`, `docs/design.md:294` | `implemented` for current repositories and MVP data | `go-ginx-2/docs/continuation.md:17`, `go-ginx-2/docs/continuation.md:67`, `go-ginx-2/README.md:10`, `go-ginx-2/docs/daemon-runtime.md:7` | Add future specs for backup/restore, schema evolution, and production operational boundaries when those areas are scheduled. |
| Traffic stats and observability | `docs/requirements.md:18`, `docs/requirements.md:157`, `docs/requirements.md:247`, `docs/requirements.md:308` | `docs/design.md:62`, `docs/design.md:312`, `docs/design.md:411`, `docs/design.md:524` | `implemented` for basic cumulative TCP/UDP/HTTP stats; `gap` for full observability | `go-ginx-2/docs/continuation.md:44`, `go-ginx-2/docs/continuation.md:72`, `go-ginx-2/docs/milestone-one-e2e.md:44`, `go-ginx-2/docs/milestone-one-e2e.md:80`, `go-ginx-2/docs/milestone-one-e2e.md:97`, `go-ginx-2/docs/milestone-one-e2e.md:114`, `go-ginx-2/docs/daemon-runtime.md:17` | Add durable metrics, log query, alert state, export, and retention specs; keep active connection reset behavior documented until changed. |
| Quotas, limits, and rate limiting | `docs/requirements.md:85`, `docs/requirements.md:155`, `docs/requirements.md:310`, `docs/requirements.md:335` | `docs/design.md:61`, `docs/design.md:165`, `docs/design.md:395`, `docs/design.md:480` | `gap` | `go-ginx-2/docs/continuation.md:78`, `go-ginx-2/docs/milestone-one-e2e.md:134`, `go-ginx-2/docs/daemon-runtime.md:114` | Create quota/rate-limit baseline specs before implementation, covering user and proxy limits, periodic quotas, enforcement points, and observable denial reasons. |
| Certificate management and ACME automation | `docs/requirements.md:50`, `docs/requirements.md:149`, `docs/requirements.md:151`, `docs/requirements.md:334` | `docs/design.md:18`, `docs/design.md:42`, `docs/design.md:245`, `docs/design.md:494` | `gap` except current control TLS verification | `go-ginx-2/docs/milestone-one-e2e.md:24`, `go-ginx-2/docs/continuation.md:78`, `go-ginx-2/docs/daemon-runtime.md:48`, `go-ginx-2/docs/daemon-runtime.md:116` | Implement certificate lifecycle after HTTPS boundaries are defined: storage, selection, ACME DNS-01, renewal, hot reload, rollback, and private-key handling. |
| Security model and sensitive data handling | `docs/requirements.md:20`, `docs/requirements.md:225`, `docs/requirements.md:239`, `docs/requirements.md:245` | `docs/design.md:379`, `docs/design.md:391`, `docs/design.md:401`, `docs/design.md:407` | `designed`; partially `implemented` for credential auth and TLS verification; broader controls remain `gap` | `go-ginx-2/docs/milestone-one-e2e.md:24`, `go-ginx-2/docs/milestone-one-e2e.md:29`, `go-ginx-2/docs/continuation.md:20`, `go-ginx-2/docs/continuation.md:42` | Add specs for permissions, resource isolation, secret handling, audit coverage, access passwords, share tokens, and disabled-resource enforcement. |
| Deployment and operations documentation | `docs/requirements.md:22`, `docs/requirements.md:292`, `docs/requirements.md:343` | `docs/design.md:437`, `docs/design.md:484`, `docs/design.md:502` | `implemented` for local milestone-one daemon guidance; `gap` for production operations | `go-ginx-2/docs/daemon-runtime.md:1`, `go-ginx-2/docs/daemon-runtime.md:3`, `go-ginx-2/docs/daemon-runtime.md:71`, `go-ginx-2/README.md:56`, `go-ginx-2/README.md:87` | Expand production packaging, service supervision, deployment automation, backup/restore, and operations guides after core product gaps close. |
| Excluded product areas | `docs/requirements.md:61`, `docs/requirements.md:392` | `docs/design.md:15` | `out-of-scope` | Product and design documents explicitly exclude these areas from the first product scope. | Keep billing, SaaS marketplace, organization/team spaces, SSO, client GUI, global node scheduling, and organization-level audit export out of baseline specs unless a later proposal changes scope. |

## Implemented Evidence Snapshot

- QUIC control authentication, certificate verification, proxy snapshot sync, heartbeat, and latest-session tracking are implemented and test-backed.
- TCP, UDP, and HTTP reverse proxy MVP paths are implemented over QUIC and covered by package and external-process smoke tests.
- SQLite persistence, repository boundaries, admin seeding, daemon startup, and basic cumulative TCP/UDP/HTTP stats are implemented for the current milestone-one runtime.
- Local daemon deployment and troubleshooting are documented for milestone-one use.

## Explicit Gaps

- HTTPS proxying.
- TCP+TLS fallback runtime.
- Forward proxying.
- Quotas, rate limits, and bandwidth enforcement.
- GraphQL admin API and web UI.
- ACME/Cloudflare DNS automation and full certificate lifecycle.
- Alerting, log query, export, and full observability surface.
- Production packaging, service supervision, deployment automation, backup/restore, and full operations documentation.

## Future Baseline Spec Candidates

| Priority | Capability spec candidate | Reason |
|---|---|---|
| 1 | `control-channel` | Captures implemented QUIC behavior and missing TCP+TLS fallback/recovery semantics. |
| 2 | `reverse-proxy-runtime` | Captures TCP/UDP/HTTP implemented behavior and the next HTTPS expansion. |
| 3 | `admin-resource-management` | Distinguishes current CLI seeding from the required GraphQL/admin UI surface. |
| 4 | `quotas-and-limits` | Needed before claiming user/proxy quota, rate-limit, and bandwidth behavior. |
| 5 | `certificate-management` | Required for HTTPS, custom domains, ACME DNS-01, renewal, hot reload, and rollback. |
| 6 | `observability-and-audit` | Covers metrics, logs, alerts, audit records, retention, and query/export behavior. |
| 7 | `deployment-operations` | Covers packaging, service supervision, backup/restore, production docs, and troubleshooting. |

Future specs should cite these source documents and summarize the requirement contract instead of copying the full PRD or design prose.

## Validation Checklist

- Every `implemented` row cites current implementation evidence.
- Every `gap` row includes a next action.
- Missing production features remain gaps and are not described as current capabilities.
- Out-of-scope areas remain excluded unless a later proposal explicitly changes scope.
- OpenSpec validation should be run for `align-docs-roadmap-specs` after task checkboxes are updated.
