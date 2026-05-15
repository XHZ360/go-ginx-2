## 1. Evidence Review

- [x] 1.1 Verify reverse-proxy source references in `docs/requirements.md`, `docs/design.md`, and the archived roadmap/gap matrix.
- [x] 1.2 Verify current TCP, UDP, and HTTP implementation evidence in `go-ginx-2/docs/continuation.md`, `go-ginx-2/docs/milestone-one-e2e.md`, `go-ginx-2/docs/daemon-runtime.md`, and `go-ginx-2/README.md`.
- [x] 1.3 Confirm HTTPS, forward proxying, access passwords, share links, quotas/rate limits, richer errors, and production observability remain documented as gaps.

## 2. Baseline Spec Review

- [x] 2.1 Review TCP scenarios for traceability to current MVP behavior and traffic statistics evidence.
- [x] 2.2 Review UDP scenarios for traceability to per-source session forwarding and traffic statistics evidence.
- [x] 2.3 Review HTTP scenarios for traceability to Host routing, response forwarding, and traffic statistics evidence.
- [x] 2.4 Review daemon startup scenarios for traceability to server/client runtime documentation.
- [x] 2.5 Review HTTPS, policy, and forward-proxy gap scenarios for accurate implemented/gap boundaries.

## 3. Validation

- [x] 3.1 Run OpenSpec status for `reverse-proxy-runtime` and confirm all artifacts are complete.
- [x] 3.2 Run OpenSpec validation for `reverse-proxy-runtime`.
- [x] 3.3 Before archive, sync `reverse-proxy-runtime` into `openspec/specs/reverse-proxy-runtime/spec.md` only after the baseline spec is accepted.
