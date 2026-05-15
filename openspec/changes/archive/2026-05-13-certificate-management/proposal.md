## Why

The roadmap identifies `certificate-management` as required for HTTPS proxying, custom domains, ACME DNS-01 automation, renewal, hot reload, rollback, and private-key handling. Current `go-ginx-2` evidence supports control-channel TLS verification but explicitly leaves proxy certificate lifecycle and ACME automation as gaps.

## What Changes

- Add a `certificate-management` OpenSpec capability for domain/certificate lifecycle requirements.
- Specify required behavior for default/platform domains, custom domain ownership verification, certificate upload/replacement/disablement, ACME DNS-01 issuance and renewal, Cloudflare token handling, secure private-key storage, hot reload, rollback, and expiry/alert tracking.
- Mark proxy certificate lifecycle, ACME automation, HTTPS certificate selection, and admin certificate management as required/design behavior that remains unimplemented in the current baseline.
- Keep current control-channel certificate verification distinct from proxy certificate management.
- Keep this change documentation/spec-only; it does not implement certificate storage, ACME integration, admin APIs/UI, runtime HTTPS, dependencies, database changes, or deployment changes.

## Capabilities

### New Capabilities
- `certificate-management`: Defines certificate and domain lifecycle requirements plus explicit current gaps for ACME, proxy certificates, private-key storage, renewal, hot reload, rollback, and admin management.

### Modified Capabilities

- None.

## Impact

- Affected documentation systems: OpenSpec change artifacts and future baseline specs.
- Source documents: `docs/requirements.md`, `docs/design.md`, `openspec/changes/archive/2026-05-13-align-docs-roadmap-specs/roadmap-gap-matrix.md`, and current `go-ginx-2` control/runtime documentation.
- No application code, runtime behavior, public API, dependency, database, or deployment impact.
