## Why

The roadmap identifies `reverse-proxy-runtime` as the next baseline spec after `control-channel` because current `go-ginx-2` evidence supports TCP, UDP, and HTTP reverse-proxy MVP behavior over QUIC. HTTPS proxying, access passwords, share links, quotas, richer error handling, and full production controls remain gaps and need to be recorded without overstating implementation status.

## What Changes

- Add a `reverse-proxy-runtime` OpenSpec capability that captures the evidence-backed TCP, UDP, and HTTP reverse-proxy MVP baseline.
- Specify baseline behavior for TCP stream forwarding, UDP per-source session forwarding, HTTP Host routing, client-side target forwarding, and basic traffic statistics.
- Record HTTPS proxying, forward proxying, access passwords, share links, quotas/rate limits, richer error handling, and production-grade observability as required/design gaps rather than implemented behavior.
- Keep this change documentation/spec-only; it does not implement runtime behavior, code changes, API changes, dependencies, database changes, or deployment changes.

## Capabilities

### New Capabilities
- `reverse-proxy-runtime`: Defines the reverse-proxy runtime contract for TCP, UDP, and HTTP MVP behavior, plus explicit tracking for HTTPS and control-plane policy gaps.

### Modified Capabilities

- None.

## Impact

- Affected documentation systems: OpenSpec change artifacts and future baseline specs.
- Source documents: `docs/requirements.md`, `docs/design.md`, `openspec/specs/control-channel/spec.md`, `openspec/changes/archive/2026-05-13-align-docs-roadmap-specs/roadmap-gap-matrix.md`, and current `go-ginx-2` progress documentation.
- No application code, runtime behavior, public API, dependency, database, or deployment impact.
