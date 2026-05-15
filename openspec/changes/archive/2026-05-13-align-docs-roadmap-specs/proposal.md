## Why

The project has complete product and design documents, but `openspec/specs/` is empty and there is no active OpenSpec change that records how current implementation progress maps back to those documents. This change creates a traceable documentation alignment layer so future implementation work can be planned from explicit specs, known gaps, and verified progress instead of scattered notes.

## What Changes

- Add an OpenSpec capability for documentation alignment and roadmap traceability.
- Define a source-of-truth hierarchy across product requirements, design, current implementation progress, and OpenSpec specs.
- Require a roadmap/gap matrix that classifies each major capability as required, designed, implemented with evidence, gap, or out of scope.
- Require future documentation claims to reference evidence from `docs/requirements.md`, `docs/design.md`, and current progress documents under `go-ginx-2/docs/`.
- Do not change application code, runtime behavior, public APIs, dependencies, or database schemas.

## Capabilities

### New Capabilities
- `documentation-alignment`: Defines how specs, design documentation, implementation progress, and roadmap gaps are kept traceable and non-contradictory.

### Modified Capabilities

- None.

## Impact

- Affected documentation systems: OpenSpec change artifacts and future OpenSpec capability specs.
- Source documents used for alignment: `docs/requirements.md`, `docs/design.md`, `go-ginx-2/docs/continuation.md`, `go-ginx-2/docs/milestone-one-e2e.md`, and `go-ginx-2/README.md`.
- No code, API, dependency, runtime, deployment, or persistence impact.
