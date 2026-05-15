## Why

HTTPS TLS termination currently works only with manually supplied certificate and key file paths. ACME DNS-01 automation is the next product gap because operators need server-managed issuance, renewal, and hot reload without storing private keys in SQLite or weakening TLS verification.

## What Changes

- Add server-side ACME DNS-01 certificate issuance for eligible HTTPS proxy hosts.
- Add Cloudflare DNS provider support through least-privilege API token configuration supplied outside SQLite.
- Persist certificate lifecycle metadata in SQLite while keeping certificate private-key material on disk under `certificate_dir`.
- Add renewal scheduling, validated certificate replacement, hot reload, and old-certificate rollback files.
- Add admin CLI commands to request issuance and inspect/renew managed HTTPS certificates.
- Update HTTPS termination to use a reloadable certificate cache backed by managed certificate files.
- Keep manual file-backed certificate paths and SNI passthrough behavior intact.

## Capabilities

### New Capabilities
- `acme-certificate-automation`: ACME account, DNS-01 provider, issuance, renewal, storage, reload, and rollback behavior for managed HTTPS proxy certificates.

### Modified Capabilities
- `certificate-management`: Replace ACME/renewal/hot reload gap scenarios with evidence-backed requirements for managed ACME certificates while preserving manual file-backed certificate behavior and private-key boundaries.
- `reverse-proxy-runtime`: Update HTTPS runtime behavior so TLS termination can use reloadable managed certificates in addition to static file-backed certificate paths.

## Impact

- Affected code: server config, admin CLI/service, SQLite repositories, domain certificate models, HTTPS proxy entry, daemon startup, docs, and tests.
- New dependency likely required: a Go ACME client and DNS-01 Cloudflare provider library, unless implemented directly against ACME/Cloudflare APIs.
- External resources needed for live issuance: registered domain delegated to Cloudflare, Cloudflare API token with DNS edit permission for the zone, outbound HTTPS access to the ACME directory, and an ACME account email.
- Tests should use fake ACME/DNS providers for deterministic local validation; live ACME/Cloudflare calls remain manual/integration-only.
