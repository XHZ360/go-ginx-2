## Context

`docs/requirements.md` requires global and per-resource metrics, time-range queries, traffic exports, logs, audit records, error classifications, alerts, and sensitive-data redaction. `docs/design.md` defines log categories, metric dimensions, error taxonomy, alert state, retention, low-resource constraints, and asynchronous/batched write behavior.

Current `go-ginx-2` evidence supports only basic cumulative TCP/UDP/HTTP proxy statistics, with active connection counts resetting after restart. Full observability, log query, audit query, alerting, export, and retention behavior remain gaps.

## Goals / Non-Goals

**Goals:**

- Establish an `observability-and-audit` capability spec for metrics, logs, audit, alerts, retention, query/export, and error classification requirements.
- Capture current basic cumulative proxy statistics as implemented evidence.
- Explicitly track full observability and audit behavior as gaps until evidence-backed implementation exists.
- Distinguish cumulative persisted counters from volatile active connection/session state.

**Non-Goals:**

- Do not implement metrics aggregation, log storage, audit query, alert state, exports, retention jobs, APIs/UI, or telemetry dependencies.
- Do not claim full observability from basic proxy counters.
- Do not define external notification channels beyond the product's initial in-admin alert requirement.

## Decisions

1. Treat basic TCP/UDP/HTTP counters as the current baseline.
   - Rationale: current validation docs show connection/request/packet, byte, status-code, and error counters, plus SQLite-backed cumulative persistence.
   - Alternative considered: classify all observability as a gap. That would understate current stats evidence.

2. Treat active counts as runtime-only until durable state exists.
   - Rationale: current docs explicitly state active connection counts reset on restart.

3. Track logs, audit, alerts, query/export, retention, and error taxonomy as gaps.
   - Rationale: product/design requirements are clear, but no current implementation evidence proves those surfaces.

## Risks / Trade-offs

- [Risk] Basic counters may be mistaken for full observability. -> Mitigation: the spec explicitly limits implemented evidence to basic cumulative stats.
- [Risk] Audit overlaps with admin-resource-management and security. -> Mitigation: this spec defines audit observability/query expectations, not admin mutations or permissions.
- [Risk] Low-resource deployment can conflict with rich logs and metrics. -> Mitigation: future implementation should preserve asynchronous/batched writes and retention constraints from the design.
