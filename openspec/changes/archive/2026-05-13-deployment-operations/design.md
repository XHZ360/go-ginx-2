## Context

`docs/requirements.md` requires clear first-use setup, deployment/configuration/use/troubleshooting/security documentation, backup and restore, SQLite-first operation, 1C1G minimum target, and 800+ connection capacity goals. `docs/design.md` defines single-server topology, network entry layout, configuration model, backup/restore content, 1C1G resource choices, capacity strategy, milestone sequencing, and test categories.

Current `go-ginx-2` evidence supports local milestone-one daemon documentation: build/run commands, server/client JSON config examples, SQLite seeding, control certificate configuration, current implemented features, and troubleshooting. It explicitly excludes production packaging, service supervision, deployment automation, and production operations documentation.

## Goals / Non-Goals

**Goals:**

- Establish a `deployment-operations` capability spec for local deployment and production operations requirements.
- Capture current local daemon setup and troubleshooting as implemented evidence.
- Track production packaging, service supervision, deployment automation, backup/restore, production hardening, and capacity validation as gaps.
- Keep operational requirements testable and tied to product/design/current-doc evidence.

**Non-Goals:**

- Do not implement packages, installers, systemd/Windows service units, deployment scripts, backup tooling, restore tooling, config migrations, or production hardening.
- Do not claim current local daemon documentation is complete production operations coverage.
- Do not add runtime or application code.

## Decisions

1. Treat local daemon runtime documentation as the current baseline.
   - Rationale: current docs provide concrete build/run/config/troubleshooting guidance for local milestone-one operation.
   - Alternative considered: classify deployment operations entirely as a gap. That would understate the local daemon docs that already exist.

2. Track production operations separately from local operation.
   - Rationale: current docs explicitly say production packaging and service supervision are out of scope.

3. Include 1C1G and capacity goals as design requirements but not implemented guarantees.
   - Rationale: product/design define capacity targets, but current evidence does not provide production capacity validation.

## Risks / Trade-offs

- [Risk] Local setup may be mistaken for production readiness. -> Mitigation: the spec labels production packaging, supervision, backup/restore, and hardening as gaps.
- [Risk] Operations scope can overlap with deployment automation and observability. -> Mitigation: this spec covers operational lifecycle documentation and packaging boundaries, while metrics/logging remain under `observability-and-audit`.
- [Risk] 1C1G goals may be overclaimed. -> Mitigation: the spec treats capacity goals as future validation unless backed by evidence.
