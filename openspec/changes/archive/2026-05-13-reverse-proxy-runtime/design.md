## Context

`docs/requirements.md` requires TCP, UDP, HTTP, and HTTPS reverse-proxy behavior, plus proxy lifecycle validation, statistics, access controls, and user-facing errors. `docs/design.md` expands this into protocol-specific flows for TCP, UDP, HTTP, HTTPS, forward proxying, temporary sharing, access passwords, quotas, and limits.

Current `go-ginx-2` evidence supports a milestone-one reverse-proxy MVP for TCP, UDP, and HTTP over the QUIC control channel. The runtime docs and validation docs explicitly show TCP stream forwarding, UDP per-source session forwarding, HTTP Host routing, client-side target forwarding, daemon startup, and basic cumulative traffic statistics. They also explicitly mark HTTPS, forward proxying, quotas/rate limits, GraphQL/admin UI, ACME, and production deployment as not implemented.

## Goals / Non-Goals

**Goals:**

- Establish a baseline `reverse-proxy-runtime` capability spec from existing product, design, and current implementation evidence.
- Separate implemented TCP/UDP/HTTP MVP behavior from required-but-unimplemented HTTPS, forward proxy, access-control, quota, and production behaviors.
- Make each scenario usable as a future acceptance-test seed.
- Preserve traceability to product/design/current-progress docs without copying all source prose.

**Non-Goals:**

- Do not implement HTTPS proxying, forward proxying, access passwords, share links, quotas, limits, or richer error handling.
- Do not change control-channel behavior, protocol frames, config fields, repositories, daemon startup, tests, or code.
- Do not claim the full product proxy design is implemented.
- Do not define certificate lifecycle, GraphQL/admin UI, or observability beyond basic reverse-proxy runtime evidence.

## Decisions

1. Model this as a new baseline capability.
   - Rationale: no `reverse-proxy-runtime` spec exists, and the archived roadmap identifies it as the next baseline after `control-channel`.
   - Alternative considered: split TCP, UDP, HTTP, and HTTPS into separate capabilities immediately. That would be premature while the current evidence-backed milestone groups TCP/UDP/HTTP together.

2. Treat TCP, UDP, and HTTP as MVP baselines.
   - Rationale: current progress and validation docs show these paths are implemented and test-backed through package and external-process smoke tests.
   - Alternative considered: mark them only as designed. That would understate current evidence.

3. Treat HTTPS and forward proxying as gaps.
   - Rationale: current docs explicitly list HTTPS and forward proxying as not implemented. Their design remains useful for future specs, but they are not current capabilities.

4. Keep policy and product controls as gap-tracked concerns.
   - Rationale: access passwords, temporary share links, quotas, bandwidth limits, and richer error responses are product/design requirements but not evidenced in the current MVP.

## Risks / Trade-offs

- [Risk] Future readers may infer full production proxy support from TCP/UDP/HTTP MVP evidence. -> Mitigation: the spec labels unsupported controls and HTTPS behavior as gaps.
- [Risk] Proxy runtime scope can overlap with certificate management, quotas, observability, and admin UI. -> Mitigation: this spec tracks only reverse-proxy runtime behavior and references future capability specs for adjacent domains.
- [Risk] Basic statistics may be confused with full observability. -> Mitigation: scenarios distinguish basic traffic counters from alerting, log query, export, and production observability gaps.
- [Risk] HTTP MVP may be mistaken for full HTTP product coverage. -> Mitigation: access-password, share-link, quota, and rich error handling are explicitly gap-tracked.
