## Context

The current HTTPS implementation supports SNI passthrough and file-backed TLS termination. Termination selects an HTTPS proxy by SNI, loads the configured certificate and key files, and forwards decrypted HTTP to the client-side target. Certificate metadata currently lives on the proxy record as file paths, and private keys remain outside SQLite.

The product design calls for Cloudflare DNS API plus ACME DNS-01 as the default server certificate strategy. This change turns the existing ACME, renewal, and hot reload gaps into an implemented server-side certificate lifecycle while preserving static file-backed certificates and passthrough mode.

## Goals / Non-Goals

**Goals:**

- Issue managed certificates for HTTPS proxy hosts through ACME DNS-01.
- Support Cloudflare DNS-01 using credentials supplied from server configuration or environment variables, not SQLite.
- Store managed certificate files under `certificate_dir` and store only lifecycle metadata in SQLite.
- Renew certificates before expiry, validate replacements, atomically update active files, retain previous material for rollback, and hot reload HTTPS termination without restarting the daemon.
- Provide admin CLI operations for issuance, renewal, and status inspection.
- Keep local tests deterministic by using fake ACME and DNS provider implementations.

**Non-Goals:**

- Public admin UI, GraphQL management API, alerts, email notifications, or ACME live integration tests.
- Cloudflare Origin CA or custom CA trust behavior.
- Wildcard/platform-domain ownership verification beyond the configured DNS provider credentials.
- Storing private-key bytes in SQLite or printing them in CLI output.

## Decisions

1. **Managed certificates are separate lifecycle records.**
   - Add certificate lifecycle metadata keyed by proxy ID and host instead of overloading only `Proxy.CertFile` and `Proxy.KeyFile`.
   - Rationale: static/manual paths remain compatible, while managed certificates need status, expiry, renewal errors, and rollback metadata.
   - Alternative considered: add many managed fields directly to `proxies`; rejected because certificate lifecycle is a separate state machine.

2. **ACME and DNS providers are interfaces.**
   - Define internal ACME issuer and DNS challenge provider interfaces so tests use fakes and production can use a real library/provider.
   - Rationale: ACME issuance requires external network and credentials, which cannot be mandatory for local tests.
   - Alternative considered: direct calls from daemon code; rejected because it makes tests brittle and couples daemon startup to external services.

3. **Cloudflare credentials are server runtime configuration.**
   - Add configuration for ACME directory URL, account email, accepted terms flag, renewal window, and Cloudflare token environment variable name.
   - Rationale: least-privilege DNS tokens are deployment secrets and must not be written into SQLite or logs.
   - Alternative considered: storing tokens in SQLite; rejected due private-secret exposure and backup risk.

4. **Certificate files are written atomically under `certificate_dir`.**
   - Managed certificates use deterministic per-host paths under `certificate_dir/managed/<host>/` with active and previous file names.
   - Rationale: HTTPS termination already enforces `certificate_dir`; atomic rename avoids partially written active material.
   - Alternative considered: in-memory certificates only; rejected because restart survival and rollback require disk state.

5. **HTTPS termination uses a reloadable certificate resolver.**
   - The HTTPS entry should resolve the current certificate through a cache that can refresh after successful issuance/renewal.
   - Rationale: hot reload should not require listener restart and should not load key files on every connection.
   - Alternative considered: restart HTTPS listener after each renewal; rejected because it can interrupt unrelated hosts.

6. **Production ACME uses `golang.org/x/crypto/acme` plus a Cloudflare REST adapter.**
   - Use the existing Go ACME client for ACME RFC 8555 order/challenge/finalize flow and a small internal Cloudflare DNS API adapter for TXT record create/delete.
   - Rationale: `lego` was investigated but pulls a very large provider dependency graph for this module; the smaller direct adapter keeps the milestone focused on Cloudflare DNS-01 only.
   - Alternative considered: CertMagic; rejected for this milestone because it brings broader HTTPS/storage/runtime behavior than the current issuer/renewal scope needs.
   - Alternative considered: lego; rejected after dependency resolution showed excessive provider dependencies for the current Cloudflare-only requirement.

7. **Renewal is opportunistic and explicit.**
   - Daemon startup should schedule periodic renewal for managed certificates, and the admin CLI should also expose explicit renewal.
   - Rationale: scheduled renewal handles normal operation, while explicit renewal supports tests and recovery.
   - Alternative considered: only renew at request time; rejected because it couples user traffic to slow issuance operations.

## Risks / Trade-offs

- **External DNS/ACME dependency can fail** → keep existing active certificate on failure, record failure metadata, and do not change HTTPS termination state.
- **Clock or expiry parsing bugs could renew too late** → renew based on a configurable window and parse certificate NotAfter from the issued leaf certificate.
- **Credential leakage risk** → read Cloudflare token from environment only, redact provider errors before audit/status output, and never store token values.
- **Partial file writes can break TLS** → write to temporary files, validate certificate/key pair, then atomically rename and reload.
- **Provider library behavior may vary** → keep provider behind interfaces and cover core lifecycle behavior with fake provider tests.

## Migration Plan

1. Add SQLite certificate lifecycle tables with nullable metadata for existing deployments.
2. Keep existing `cert_file`/`key_file` proxy paths working unchanged.
3. Add managed certificate records only when issuance is explicitly requested.
4. On renewal success, update metadata and active files; on failure, keep previous active material.
5. Rollback consists of switching active metadata/files back to retained previous files and refreshing the certificate cache.

## Open Questions

- Should the first implementation support wildcard names, or only exact HTTPS proxy hosts? Exact hosts are sufficient unless a platform wildcard requirement is explicitly prioritized.
- Should account key storage be a managed file under `certificate_dir`, or generated per issuance? Persistent account key storage is preferable for production ACME rate limits.
