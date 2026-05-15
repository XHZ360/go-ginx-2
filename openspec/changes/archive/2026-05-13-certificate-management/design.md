## Context

`docs/requirements.md` requires default domains, custom domain binding, ownership verification, certificate upload/replacement/disablement, expiry reminders, Cloudflare DNS-backed public certificates, ACME DNS-01 issuance and renewal, and private-key protection. `docs/design.md` expands this into platform-domain boundaries, independent custom-domain certificates, Cloudflare ACME flow, advanced Origin CA mode, secure private-key storage, metadata-only SQLite records, renewal, hot reload, and rollback.

Current `go-ginx-2` evidence supports control-channel TLS verification and certificate loading for the QUIC control listener. It does not implement proxy HTTPS, proxy certificate lifecycle, ACME automation, admin certificate management, renewal, hot reload, rollback, or certificate observability.

## Goals / Non-Goals

**Goals:**

- Establish a `certificate-management` capability spec for certificate and domain lifecycle requirements.
- Distinguish implemented control-channel TLS verification from unimplemented proxy certificate management.
- Track ACME/Cloudflare, private-key storage, renewal, hot reload, rollback, expiry state, and admin management as explicit gaps.
- Make each future behavior testable through scenario-oriented requirements.

**Non-Goals:**

- Do not implement ACME, Cloudflare integration, HTTPS proxying, certificate repositories, admin APIs/UI, file permissions, renewal jobs, hot reload, rollback, or alerts.
- Do not weaken existing control-channel certificate verification.
- Do not claim proxy certificate lifecycle from the current control TLS config fields.

## Decisions

1. Treat certificate management as a new baseline capability with most lifecycle behavior gap-tracked.
   - Rationale: product/design requirements are detailed, but current evidence supports only control-channel TLS verification.
   - Alternative considered: fold certificate work into `reverse-proxy-runtime`. That would conflate HTTPS traffic handling with certificate lifecycle management.

2. Keep platform and custom-domain certificate rules explicit.
   - Rationale: requirements state platform wildcard certificates must be constrained to a proxy subdomain and custom domains must not reuse platform wildcard certificates.

3. Treat private-key storage as a first-class contract.
   - Rationale: product/design require private keys to avoid SQLite/plain UI/log exposure, so future implementation needs a clear acceptance boundary.

## Risks / Trade-offs

- [Risk] Control-channel TLS may be mistaken for proxy certificate management. -> Mitigation: the spec separates current control TLS verification from proxy/domain certificate lifecycle gaps.
- [Risk] ACME and Cloudflare behavior can introduce external side effects. -> Mitigation: this baseline defines required behavior only; implementation changes must separately address credentials, DNS side effects, and rollback.
- [Risk] Certificate lifecycle overlaps with HTTPS proxying and admin UI. -> Mitigation: this spec defines lifecycle contracts, while HTTPS traffic and UI/API surfaces remain separate implementation concerns.
