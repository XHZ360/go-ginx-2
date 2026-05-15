## Context

The repository currently has product-level requirements in `docs/requirements.md`, architecture and acceptance mapping in `docs/design.md`, and implementation progress evidence in `go-ginx-2/docs/continuation.md`, `go-ginx-2/docs/milestone-one-e2e.md`, and `go-ginx-2/README.md`. OpenSpec is configured with the `spec-driven` schema, but there are no baseline specs and no active change capturing how documentation should be kept aligned with implementation progress.

This change treats the existing product and design documents as source inputs, not as artifacts to rewrite. OpenSpec becomes the structured traceability layer used to express what is required, what is designed, what is implemented with evidence, and what remains a roadmap gap.

## Goals / Non-Goals

**Goals:**

- Establish a single source-of-truth hierarchy for documentation alignment.
- Define a repeatable roadmap/gap matrix format for tracking major product capabilities against implementation evidence.
- Require OpenSpec claims to be traceable to product requirements, design sections, or current progress documentation.
- Make future implementation planning possible from OpenSpec without duplicating all PRD prose.

**Non-Goals:**

- Do not implement application features.
- Do not archive this change or convert every product requirement into a baseline spec in this change.
- Do not change `docs/requirements.md` or `docs/design.md` unless a later change identifies a concrete contradiction.
- Do not claim unsupported features are implemented.

## Decisions

1. Use `documentation-alignment` as the initial capability.
   - Rationale: the immediate gap is traceability and roadmap alignment, not runtime behavior.
   - Alternative considered: create one spec per product area immediately. That would be larger and risks copying PRD content without first defining alignment rules.

2. Use a source hierarchy instead of equal-weight documents.
   - Hierarchy: product requirements define product intent; design defines technical approach and acceptance mapping; `go-ginx-2` progress docs define current implementation evidence; OpenSpec specs define structured requirements and deltas for future work.
   - Rationale: this avoids treating progress notes as product requirements or treating design intent as implemented status.

3. Track status with a small controlled vocabulary.
   - Status values: `required`, `designed`, `implemented`, `gap`, `out-of-scope`.
   - Rationale: a controlled vocabulary makes roadmap review mechanical and prevents ambiguous labels such as "done-ish" or "partial" without evidence.

4. Require evidence for implementation claims.
   - Evidence can be progress documentation, validation documentation, tests, build commands, or code references from the active implementation target.
   - Rationale: current docs already distinguish implemented MVP behavior from missing production features; the OpenSpec layer must preserve that distinction.

## Risks / Trade-offs

- [Risk] The alignment spec can become a parallel PRD if it copies every requirement verbatim. -> Mitigation: keep this capability focused on traceability rules and roadmap classification.
- [Risk] Implementation status may drift after future work. -> Mitigation: require every roadmap update to cite current evidence and update status labels deliberately.
- [Risk] Empty baseline specs may remain after this change. -> Mitigation: use the resulting roadmap/gap matrix to drive later per-capability spec creation in smaller changes.
- [Risk] Different docs may disagree over current status. -> Mitigation: prefer current `go-ginx-2` evidence for implementation status and record unresolved contradictions as gaps.
