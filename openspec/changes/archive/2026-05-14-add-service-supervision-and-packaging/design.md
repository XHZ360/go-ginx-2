## Context

`go-ginx-2` already has foreground daemon commands, clean signal handling, reconnect backoff, restart-surviving stats flushes, and control-plane restart recovery. What it still lacks is the operational wrapper that turns those binaries into a repeatable deployment artifact with an explicit service lifecycle. The existing `deployment-operations` spec and runtime docs still mark packaging and service supervision as gaps.

This batch crosses runtime entrypoints, packaging workflow, deployment documentation, and validation. It needs design up front because the implementation must choose one supported deployment model, define artifact layout, and align shutdown/restart behavior with an external supervisor.

## Goals / Non-Goals

**Goals:**
- Define the first supported deployable packaging model for `goginx-server`, `goginx-client`, and `goginx-admin`.
- Define a first supported service supervision model for running `goginx-server` and `goginx-client` under an external service manager.
- Reuse the existing foreground runtime behavior instead of inventing a second daemon mode.
- Add deployment-oriented validation and operator documentation for build, install, start, stop, restart, and rollback of the packaged runtime.

**Non-Goals:**
- Do not implement Debian/RPM/MSI/native installers in this batch.
- Do not add Kubernetes, container orchestration, or multi-node deployment support.
- Do not redesign control protocol, storage, or proxy runtime behavior beyond what is needed for supervised restarts.
- Do not solve backup/restore, capacity benchmarking, or full production hardening in this batch.

## Decisions

1. Support one concrete deployment model first: Linux single-node bundles managed by `systemd`.
   - Rationale: the current runtime already behaves like a well-formed foreground process with signal-aware shutdown, which maps directly onto `systemd` without adding a second embedded supervisor. This gives a practical production baseline with minimal runtime churn.
   - Alternatives considered:
     - Windows Service first: possible, but adds platform-specific implementation and validation complexity before the primary server deployment path is documented.
     - Custom in-process supervisor: duplicates functionality that `systemd` already provides and complicates failure semantics.

2. Package as a reproducible filesystem bundle, not an OS-native installer.
   - Rationale: a bundle is enough to make deployment repeatable while keeping implementation lightweight and cross-build friendly. It can include binaries, sample config, service units, environment files, and directory layout guidance.
   - Alternatives considered:
     - `.deb`/`.rpm`: better operator ergonomics later, but much higher release and test surface for the first batch.
     - Archive only with no layout contract: too weak to qualify as an operations baseline.

3. Keep service lifecycle outside the binaries and rely on `systemd` policies.
   - Rationale: `goginx-server` and `goginx-client` already run in the foreground and honor shutdown signals. `systemd` can provide restart policy, boot-time startup, working directory, dependency ordering, and environment injection with fewer moving parts.
   - Alternatives considered:
     - Add `sd_notify` readiness integration immediately: useful later, but not required for a first `Type=simple` baseline.
     - Add daemonize/background mode: makes logs, shutdown, and testing harder while duplicating supervisor behavior.

4. Validate the deployment model through packaged-artifact startup and restart tests.
   - Rationale: the spec should be backed by evidence that the packaged layout, service-oriented startup assumptions, and restart semantics actually work. Existing package tests already cover runtime semantics; this batch should add deployment-focused proof.
   - Alternatives considered:
     - Documentation-only packaging: insufficient because packaging and supervision are exactly the areas most likely to drift from runtime assumptions.

## Risks / Trade-offs

- [Risk] `systemd`-first support leaves Windows service integration as a gap. -> Mitigation: document Linux `systemd` as the first supported production model and keep other platforms explicitly out of scope.
- [Risk] `Type=simple` does not provide readiness signaling. -> Mitigation: require startup validation checks in docs/tests and keep readiness integration as a later enhancement.
- [Risk] Bundle scripts can drift from actual runtime requirements. -> Mitigation: validate bundle creation and startup in automated tests.
- [Risk] Operators may overread the first bundle as complete production hardening. -> Mitigation: keep backup/restore, capacity validation, alerts, and advanced hardening explicitly listed as remaining gaps.

## Migration Plan

1. Add bundle-generation workflow and checked-in `systemd` templates.
2. Add deployment-focused validation that uses the packaged layout and supervised restart assumptions.
3. Update runtime and operations docs to point operators at the new package layout and service lifecycle procedures.
4. Roll forward by deploying the new bundle and installing/updating `systemd` units.
5. Roll back by restoring the prior bundle directory and restarting the service units against the previous config/data paths.

## Open Questions

- Whether the first bundle should be version-stamped in layout (`dist/<version>/...`) or emitted into a fixed release directory.
- Whether environment overrides should be modeled primarily through `EnvironmentFile=` or by keeping all runtime options in JSON config only.
