## 1. Data Model And Configuration

- [x] 1.1 Add domain types for managed certificate status, lifecycle metadata, ACME provider settings, and issuance/renewal results.
- [x] 1.2 Extend store interfaces with a certificate repository for create, lookup by proxy/host, list renewable, update success, and update failure operations.
- [x] 1.3 Add SQLite certificate lifecycle tables and migrations that preserve existing proxy `cert_file`/`key_file` behavior.
- [x] 1.4 Add server config fields for ACME directory URL, account email, terms acceptance, renewal window, and Cloudflare token environment variable name.

## 2. Certificate Storage And Reload

- [x] 2.1 Implement managed certificate file storage under `certificate_dir/managed/<host>/` with atomic writes and previous-file retention.
- [x] 2.2 Implement certificate/key pair validation for host name, expiry, and key match before activation.
- [x] 2.3 Add a reloadable HTTPS certificate resolver/cache that supports static file-backed certificates and managed active certificates.
- [x] 2.4 Wire the HTTPS entry to use the reloadable resolver without breaking passthrough when no active certificate exists.

## 3. ACME And DNS Providers

- [x] 3.1 Define internal ACME issuer and DNS challenge provider interfaces with fake implementations for tests.
- [x] 3.2 Add production ACME DNS-01 implementation using the selected Go ACME/Cloudflare dependency strategy.
- [x] 3.3 Ensure Cloudflare DNS tokens are read from environment configuration only and redacted from returned errors/status.
- [x] 3.4 Ensure DNS challenge cleanup runs after both successful and failed validation attempts.

## 4. Lifecycle Service And Admin Operations

- [x] 4.1 Implement certificate lifecycle service for issue, renew, status, failure recording, activation, and rollback preservation.
- [x] 4.2 Add admin service methods and `goginx-admin` commands for issuing, renewing, and inspecting managed HTTPS certificates.
- [x] 4.3 Start a daemon renewal loop that renews managed certificates inside the configured renewal window.
- [x] 4.4 Keep existing manually configured HTTPS `-cert-file`/`-key-file` proxies working unchanged.

## 5. Tests And Documentation

- [x] 5.1 Add unit tests for SQLite certificate metadata migration and repository behavior.
- [x] 5.2 Add unit tests for atomic file storage, validation, rollback retention, and reloadable resolver behavior.
- [x] 5.3 Add lifecycle tests using fake ACME/DNS providers for successful issuance, failed issuance, renewal, cleanup, and missing credential cases.
- [x] 5.4 Add daemon/admin tests for command wiring, renewal loop behavior, and static/manual certificate compatibility.
- [x] 5.5 Update README and runtime docs with managed certificate configuration, external resource requirements, and local fake-test limitations.
- [x] 5.6 Run `go test ./...`, build server/client/admin binaries, and validate changed OpenSpec specs.
