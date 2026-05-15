## 1. Evidence Review

- [x] 1.1 Verify certificate and domain source requirements in `docs/requirements.md`.
- [x] 1.2 Verify certificate lifecycle, ACME, private-key, renewal, hot reload, and rollback design references in `docs/design.md`.
- [x] 1.3 Verify the archived roadmap classifies certificate management and ACME as gaps except current control TLS verification.
- [x] 1.4 Verify current `go-ginx-2` docs support control-channel TLS verification but not proxy certificate lifecycle.

## 2. Baseline Spec Review

- [x] 2.1 Review control TLS boundary scenarios to avoid claiming proxy certificate lifecycle from control-channel evidence.
- [x] 2.2 Review domain ownership and certificate binding scenarios for platform/custom domain boundaries.
- [x] 2.3 Review manual certificate and ACME automation scenarios for accurate gap tracking.
- [x] 2.4 Review private-key protection scenarios for SQLite, UI, and log exposure boundaries.
- [x] 2.5 Review renewal, hot reload, rollback, and Origin CA scenarios for future acceptance coverage.

## 3. Validation

- [x] 3.1 Run OpenSpec status for `certificate-management` and confirm all artifacts are complete.
- [x] 3.2 Run OpenSpec validation for `certificate-management`.
- [x] 3.3 Before archive, sync `certificate-management` into `openspec/specs/certificate-management/spec.md` only after the baseline spec is accepted.
