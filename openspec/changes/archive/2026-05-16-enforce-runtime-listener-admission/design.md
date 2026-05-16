## Context

The current admin proxy write paths can accept TCP or UDP states that look valid at persistence time but later fail when the runtime assembles or binds listeners. Existing database constraints only protect part of the listener space and do not account for the full set of active runtime listeners, including fixed listeners from server configuration and enabled proxies across both TCP and UDP. This change is correctness-focused: it defines the admission model needed to reject invalid create, update, and enable operations before invalid state is stored where possible.

The likely implementation path crosses multiple areas. `internal/admin/service.go` is the current write-path entry for proxy lifecycle mutations, admin API layers translate backend failures into frontend-consumable error semantics, and runtime/config assembly owns the listener topology that must inform admission. The design therefore needs one shared ListenerClaim view of the active listener space that can be evaluated consistently before persistence.

## Goals / Non-Goals

**Goals:**
- Define a ListenerClaim model that normalizes static listeners and enabled TCP/UDP proxies into one active listener set.
- Require pre-admission checks for TCP/UDP proxy create, update, and enable flows using the V1 conflict rule of `same network + same port`.
- Make disabled proxies non-participants in active listener admission.
- Reserve explicit `ENTRY_CONFLICT` semantics for rejected operations so admin APIs can report deterministic, user-actionable conflicts.
- Identify the code paths and topology inputs the later implementation must update.

**Non-Goals:**
- Redesign the full GraphQL schema or broader admin product UX.
- Introduce quota, RBAC, observability overhaul, or unrelated admin productization behavior.
- Expand the admission model to HTTP/HTTPS host routing or forward-proxy semantics.
- Solve every possible future listener-policy concern beyond the V1 `same network + same port` rule.

## Decisions

### Decision: Model admission around a shared ListenerClaim set
The design uses a single ListenerClaim abstraction for active runtime listeners instead of keeping separate ad hoc checks for configured listeners, enabled TCP proxies, and enabled UDP proxies.

Rationale: the current problem exists because conflict knowledge is fragmented. A shared claim model allows the write path to ask one question: whether a proposed active TCP/UDP listener would collide with any existing active claim.

Alternatives considered:
- Extend only database uniqueness checks. Rejected because static config listeners and some cross-source conflicts are outside the database view.
- Detect conflicts only during runtime startup or enable. Rejected because that still accepts invalid persisted state and preserves the correctness gap.

### Decision: Admission uses the active runtime listener space, not all persisted proxies
The active claim set includes static listeners from configuration (`control_quic_listen`, `control_tls_listen`, `admin_listen`, `http_entry_listen`, `https_entry_listen` where configured), enabled TCP proxies, and enabled UDP proxies. Disabled proxies are excluded.

Rationale: the runtime only needs to reject collisions against listeners that would actually bind or already occupy a bind slot. Excluding disabled proxies preserves lifecycle flexibility and matches current operator expectations for disable-before-reconfigure flows.

Alternatives considered:
- Include disabled proxies. Rejected because it would block harmless staging edits and make disable ineffective as a conflict-resolution step.
- Check only same-type proxies. Rejected because the stated runtime listener space is shared and the admission problem exists precisely because conflicts are broader than a single persistence table rule.

### Decision: Perform admission before persistence where possible
Proxy create, update, and enable flows should assemble the effective post-operation claim set and reject conflicts before committing the new state. Update flows should ignore the proxy's current claim when evaluating its replacement claim so self-updates do not self-conflict.

Rationale: pre-admission keeps the database and runtime topology aligned and avoids accepting states that the daemon cannot honor.

Alternatives considered:
- Persist first and rely on reconciliation errors. Rejected because it preserves invalid state and complicates rollback semantics.
- Limit checks to enable only. Rejected because updates can also create invalid enabled state, and create may create an already-enabled proxy.

### Decision: Surface conflicts as explicit ENTRY_CONFLICT semantics
Admission failures should map to a distinct domain error that the admin API can surface as `ENTRY_CONFLICT` rather than collapsing into generic validation or persistence failures.

Rationale: operators need a stable, machine-readable conflict result that corresponds to listener admission and can be presented consistently across API surfaces.

Alternatives considered:
- Reuse generic `CONFLICT`. Rejected because the requested scope calls for explicit `ENTRY_CONFLICT` semantics and the listener problem is more specific than a generic collision bucket.

## Risks / Trade-offs

- [Runtime topology drift between admission and actual bind behavior] -> Mitigation: build admission from the same listener topology/config interpretation used by runtime startup rather than duplicating independent rules.
- [Cross-protocol semantics may be under-specified for some listener types] -> Mitigation: keep the V1 rule explicit as `same network + same port` and document the exact static listener sources included in the claim set.
- [Pre-admission adds more logic to admin write paths] -> Mitigation: centralize claim assembly and conflict detection in shared helpers used by create, update, and enable flows.
- [Future listener types may need richer conflict dimensions] -> Mitigation: keep ListenerClaim extensible, but scope current decisions to TCP/UDP and static bind listeners only.

## Migration Plan

No data migration is required for this change proposal. Implementation should land as a behavioral tightening in admin write paths, with rollback consisting of reverting the admission checks and error mapping if necessary. Existing invalid persisted states, if any, remain an implementation concern to handle deliberately during rollout rather than broadening this proposal.

## Open Questions

- Whether current runtime/config helpers already expose enough normalized listener topology to reuse directly, or whether implementation will need a small shared topology builder for admission.
- Whether enabling a proxy should report the first discovered conflict only or return structured details for multiple conflicting claims; this change requires explicit `ENTRY_CONFLICT` semantics but not a richer conflict payload.
- Whether CLI seeding flows that create enabled proxies should use the same admission path as the admin API write service or remain explicitly out of scope until implementation confirms shared plumbing.
