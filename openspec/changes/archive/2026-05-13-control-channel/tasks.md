## 1. Evidence Review

- [x] 1.1 Verify control-channel source references in `docs/requirements.md`, `docs/design.md`, and the archived roadmap/gap matrix.
- [x] 1.2 Verify current implementation evidence in `go-ginx-2/docs/continuation.md`, `go-ginx-2/docs/milestone-one-e2e.md`, `go-ginx-2/docs/daemon-runtime.md`, and `go-ginx-2/README.md`.
- [x] 1.3 Confirm TCP+TLS fallback and deeper reconnect/recovery semantics remain documented as gaps, not implemented behavior.

## 2. Baseline Spec Review

- [x] 2.1 Review secure transport and server certificate verification scenarios for traceability to current QUIC evidence.
- [x] 2.2 Review client authentication and proxy snapshot scenarios for traceability to current MVP behavior.
- [x] 2.3 Review heartbeat, latest-session routing, TCP+TLS fallback, duplicate-session grace, and reconnect/recovery scenarios for accurate implemented/gap boundaries.

## 3. Validation

- [x] 3.1 Run OpenSpec status for `control-channel` and confirm all artifacts are complete.
- [x] 3.2 Run OpenSpec validation for `control-channel`.
- [x] 3.3 Before archive, sync `control-channel` into `openspec/specs/control-channel/spec.md` only after the baseline spec is accepted.
