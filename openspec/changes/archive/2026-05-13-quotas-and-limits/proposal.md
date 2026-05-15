## Why

The roadmap identifies `quotas-and-limits` as a required baseline because product and design documents require user/proxy limits, traffic quotas, bandwidth controls, and observable denial reasons, while current `go-ginx-2` evidence explicitly marks quotas and rate limits as not implemented. This change records the required contract as a gap baseline so future implementation can be measured without overstating current runtime behavior.

## What Changes

- Add a `quotas-and-limits` OpenSpec capability for user-level and proxy-level resource limits.
- Specify required behavior for proxy-count limits, concurrent-connection limits, port-range limits, traffic quotas, bandwidth limits, monthly/yearly quota periods, enforcement points, and observable denial reasons.
- Mark all quota, limit, and rate-limit enforcement as required/design behavior that remains unimplemented in the current baseline.
- Keep this change documentation/spec-only; it does not implement runtime enforcement, admin editing, API/UI behavior, data model changes, dependencies, or deployment changes.

## Capabilities

### New Capabilities
- `quotas-and-limits`: Defines quota, limit, and rate-limit requirements plus the explicit current implementation gap for enforcement and observability.

### Modified Capabilities

- None.

## Impact

- Affected documentation systems: OpenSpec change artifacts and future baseline specs.
- Source documents: `docs/requirements.md`, `docs/design.md`, `openspec/changes/archive/2026-05-13-align-docs-roadmap-specs/roadmap-gap-matrix.md`, and current `go-ginx-2` progress/runtime documentation.
- No application code, runtime behavior, public API, dependency, database, or deployment impact.
