## Why

The roadmap identifies `deployment-operations` as required because current `go-ginx-2` evidence documents local milestone-one daemon setup and troubleshooting, while product/design documents require deployable, operable production guidance including topology, configuration, backup/restore, capacity, supervision, and troubleshooting. This change records the current local deployment baseline and production operations gaps.

## What Changes

- Add a `deployment-operations` OpenSpec capability for deployment topology, configuration, local daemon operation, backup/restore, capacity, resource use, troubleshooting, and production operations.
- Specify the current baseline for local daemon build/run guidance, config files, SQLite seeding, control certificate configuration, and basic troubleshooting.
- Track production packaging, service supervision, deployment automation, backup/restore, production hardening, capacity validation, and full operations documentation as required/design behavior that remains unimplemented.
- Keep this change documentation/spec-only; it does not implement packaging, service units, installers, backup tooling, deployment automation, config migration, or runtime changes.

## Capabilities

### New Capabilities
- `deployment-operations`: Defines local deployment and operations documentation baseline plus explicit production operations gaps for packaging, supervision, backup/restore, capacity, and troubleshooting.

### Modified Capabilities

- None.

## Impact

- Affected documentation systems: OpenSpec change artifacts and future baseline specs.
- Source documents: `docs/requirements.md`, `docs/design.md`, `openspec/changes/archive/2026-05-13-align-docs-roadmap-specs/roadmap-gap-matrix.md`, `go-ginx-2/docs/daemon-runtime.md`, `go-ginx-2/README.md`, and current `go-ginx-2` continuation notes.
- No application code, runtime behavior, public API, dependency, database, packaging, or deployment impact.
