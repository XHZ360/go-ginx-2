## 1. Alignment Inventory

- [x] 1.1 Create a roadmap/gap matrix that lists the major capabilities from `docs/requirements.md` and `docs/design.md`.
- [x] 1.2 Add source references for each matrix entry, including PRD IDs or design sections where available.
- [x] 1.3 Classify each entry with one of `required`, `designed`, `implemented`, `gap`, or `out-of-scope`.

## 2. Implementation Evidence

- [x] 2.1 Cross-check implemented claims against `go-ginx-2/docs/continuation.md`, `go-ginx-2/docs/milestone-one-e2e.md`, and `go-ginx-2/README.md`.
- [x] 2.2 Record evidence for currently implemented MVP capabilities such as QUIC control, TCP proxy, UDP proxy, HTTP proxy, SQLite persistence, daemon startup, admin seeding, and basic stats.
- [x] 2.3 Record gaps for missing production capabilities such as HTTPS proxying, TCP+TLS fallback, forward proxy, quotas/rate limits, GraphQL/admin UI, ACME/Cloudflare DNS automation, alerts, and production deployment documentation.

## 3. OpenSpec Baseline Preparation

- [x] 3.1 Identify which roadmap entries should become future baseline OpenSpec capability specs.
- [x] 3.2 Prioritize the first follow-up capability specs based on current roadmap gaps and milestone order.
- [x] 3.3 Ensure future specs cite source documents instead of duplicating full PRD prose.

## 4. Validation

- [x] 4.1 Verify every implemented status has evidence and every gap has a next action.
- [x] 4.2 Verify alignment documentation does not claim unsupported features as implemented.
- [x] 4.3 Run OpenSpec status and validation for `align-docs-roadmap-specs`.
