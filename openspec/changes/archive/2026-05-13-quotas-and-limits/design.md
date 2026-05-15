## Context

`docs/requirements.md` requires user-level resource limits, proxy-level traffic quotas, bandwidth limits, monthly/yearly quota periods, denial/pause behavior after quota exhaustion, and visible management/reporting surfaces. `docs/design.md` places enforcement in proxy startup, reverse/forward proxy entry checks, abuse-control policy, metrics, and error classification.

Current `go-ginx-2` evidence supports basic cumulative TCP/UDP/HTTP statistics, but explicitly lists quotas and rate limits as not implemented. This change therefore defines the required contract as a gap baseline rather than an implemented runtime capability.

## Goals / Non-Goals

**Goals:**

- Establish a `quotas-and-limits` capability spec for user/proxy quota and limit requirements.
- Capture enforcement points for proxy creation/enabling, new connections/requests/packets, bandwidth throttling, and quota exhaustion.
- Track observable denial reasons as required behavior that future implementation must prove.
- Keep current implementation status clear: all quota, limit, and rate-limit enforcement remains a gap.

**Non-Goals:**

- Do not implement quota storage, rate-limit algorithms, admin editing, runtime enforcement, metrics export, or UI/API behavior.
- Do not claim current basic traffic statistics satisfy quota enforcement.
- Do not define billing, plans, payments, or commercial packages.

## Decisions

1. Treat quotas and limits as a new baseline capability with all enforcement scenarios gap-tracked.
   - Rationale: product/design requirements are clear, but current implementation evidence says enforcement is absent.
   - Alternative considered: fold quota tracking into `reverse-proxy-runtime`. That would blur runtime forwarding with policy enforcement and make future validation harder.

2. Include both configuration constraints and runtime enforcement points.
   - Rationale: the product requires proxy count, concurrent connection, port range, traffic quota, and bandwidth controls, and design places checks before proxy enablement and during runtime entry handling.

3. Separate traffic statistics from quota enforcement.
   - Rationale: current counters are useful evidence for future quota work, but counters alone do not reject, pause, throttle, or classify quota-related denials.

## Risks / Trade-offs

- [Risk] Readers may confuse basic traffic counters with quota enforcement. -> Mitigation: the spec explicitly states counters do not satisfy quota/limit behavior.
- [Risk] Enforcement can span admin, repositories, proxy runtime, and observability. -> Mitigation: this spec defines the behavioral contract, while future implementation changes can split technical work across capabilities.
- [Risk] Bandwidth limiting semantics can become algorithm-specific too early. -> Mitigation: this baseline specifies observable behavior without choosing token-bucket/leaky-bucket implementation details.
