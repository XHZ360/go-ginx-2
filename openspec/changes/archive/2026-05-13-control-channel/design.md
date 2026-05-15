## Context

`docs/requirements.md` requires a secure client/server control channel that can negotiate QUIC or TCP+TLS, authenticate clients, report state, receive proxy configuration, send heartbeats, and support reconnect/recovery behavior. `docs/design.md` expands this into protocol negotiation, authentication, heartbeat/status reporting, configuration sync, transfer subchannel management, reconnect/recovery, and duplicate-session handling.

Current `go-ginx-2` documentation shows an evidence-backed QUIC MVP: client authentication, server certificate verification, proxy snapshot sync, heartbeat updates, session tracking, latest-session replacement, and daemon startup are covered by progress notes and validation docs. It also explicitly records TCP+TLS fallback runtime and deeper recovery semantics as gaps.

## Goals / Non-Goals

**Goals:**

- Establish a baseline `control-channel` capability spec from existing product, design, and implementation evidence.
- Separate implemented QUIC MVP behavior from required-but-unimplemented TCP+TLS fallback and deeper reconnect/recovery behavior.
- Make each requirement scenario usable as a future acceptance-test seed.
- Preserve traceability to product/design/current-progress docs without copying all source prose.

**Non-Goals:**

- Do not implement TCP+TLS fallback runtime.
- Do not implement new reconnect, event replay, or duplicate-session grace behavior.
- Do not change protocol frames, config fields, storage, daemon startup, tests, or code.
- Do not claim the full product control-channel design is implemented.

## Decisions

1. Model this as a new baseline capability.
   - Rationale: no existing `control-channel` spec exists, and the roadmap identifies it as the first future baseline spec.
   - Alternative considered: fold it into `documentation-alignment`. That would mix runtime behavior with documentation governance and make future implementation tracking harder.

2. Use explicit implemented/gap language in requirements.
   - Rationale: the current implementation has strong QUIC evidence, while TCP+TLS fallback and recovery semantics remain missing. The spec must preserve that boundary.
   - Alternative considered: write only future-state requirements. That would obscure what is already test-backed and what still needs implementation.

3. Keep transport security requirements independent of certificate automation.
   - Rationale: control-channel TLS identity verification is already part of the MVP, but ACME/Cloudflare DNS certificate lifecycle belongs to a future `certificate-management` capability.
   - Alternative considered: include certificate issuance requirements here. That would duplicate certificate-management scope.

4. Treat latest-session routing as the current baseline, not full duplicate-session grace handling.
   - Rationale: current evidence supports latest-session replacement; the design describes richer duplicate-session grace behavior that should remain a gap until implemented.

## Risks / Trade-offs

- [Risk] Future readers may treat required TCP+TLS fallback as implemented. -> Mitigation: scenarios label fallback as a gap and tasks require evidence before status changes.
- [Risk] The spec may drift from runtime behavior after later control-channel work. -> Mitigation: require future changes to update the spec with new evidence-backed scenarios.
- [Risk] Authentication, certificate verification, and credential lifecycle could be split across security specs later. -> Mitigation: keep this spec focused on control-channel connection admission and reference future security/certificate specs for broader lifecycle details.
- [Risk] Reconnect/recovery requirements can become too broad. -> Mitigation: define current gap boundaries and defer detailed event replay/session-generation behavior to implementation proposals.
