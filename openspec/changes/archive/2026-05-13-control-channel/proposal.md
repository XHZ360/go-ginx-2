## Why

The roadmap identifies `control-channel` as the first baseline spec because it is the foundation for client authentication, configuration delivery, session liveness, and proxy stream routing. Current `go-ginx-2` evidence supports a QUIC MVP, but TCP+TLS fallback and deeper reconnect/recovery semantics are still gaps and need to be recorded without overstating implementation status.

## What Changes

- Add a `control-channel` OpenSpec capability that captures the current evidence-backed QUIC control-channel baseline.
- Specify required control-channel behaviors for secure transport, client authentication, server certificate verification, proxy snapshot delivery, heartbeat tracking, and latest-session routing.
- Record TCP+TLS fallback and deeper reconnect/recovery behavior as required/designed gaps rather than implemented behavior.
- Keep this change documentation/spec-only; it does not implement runtime fallback, recovery, code changes, API changes, dependencies, or schema changes.

## Capabilities

### New Capabilities
- `control-channel`: Defines the client/server control channel contract, including secure transport, authentication, certificate verification, proxy snapshot sync, heartbeat/session tracking, latest-session routing, and explicit fallback/recovery gaps.

### Modified Capabilities

- None.

## Impact

- Affected documentation systems: OpenSpec change artifacts and future baseline specs.
- Source documents: `docs/requirements.md`, `docs/design.md`, `openspec/specs/documentation-alignment/spec.md`, `openspec/changes/archive/2026-05-13-align-docs-roadmap-specs/roadmap-gap-matrix.md`, and current `go-ginx-2` progress documentation.
- No application code, runtime behavior, public API, dependency, database, or deployment impact.
