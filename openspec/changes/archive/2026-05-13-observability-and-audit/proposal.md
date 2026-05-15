## Why

The roadmap identifies `observability-and-audit` as required because current `go-ginx-2` evidence supports only basic cumulative TCP/UDP/HTTP stats, while product and design documents require metrics, logs, error classifications, alerts, audit records, query/export behavior, retention, and user-facing diagnostics. This change records the implemented statistics baseline and full observability/audit gaps.

## What Changes

- Add an `observability-and-audit` OpenSpec capability for metrics, logs, audit, alert, retention, query, export, and error-classification requirements.
- Specify the current baseline for basic cumulative TCP/UDP/HTTP proxy counters and active-count reset behavior.
- Track full metrics aggregation, log collection/query, audit query, alert state, export, retention, sensitive-data redaction, and error classification as required/design behavior that remains incomplete or unimplemented.
- Keep this change documentation/spec-only; it does not implement logging, metrics pipelines, alerting, APIs/UI, data model changes, dependencies, or deployment changes.

## Capabilities

### New Capabilities
- `observability-and-audit`: Defines the observability and audit contract, including current basic proxy statistics and explicit gaps for full logs, metrics, alerts, audit, query/export, retention, and error classification.

### Modified Capabilities

- None.

## Impact

- Affected documentation systems: OpenSpec change artifacts and future baseline specs.
- Source documents: `docs/requirements.md`, `docs/design.md`, `openspec/changes/archive/2026-05-13-align-docs-roadmap-specs/roadmap-gap-matrix.md`, and current `go-ginx-2` progress/runtime validation documentation.
- No application code, runtime behavior, public API, dependency, database, or deployment impact.
