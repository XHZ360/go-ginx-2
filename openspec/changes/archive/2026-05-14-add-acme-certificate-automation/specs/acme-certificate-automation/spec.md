## ADDED Requirements

### Requirement: ACME provider configuration
The system SHALL configure ACME DNS-01 automation from server runtime configuration and environment-provided DNS credentials without storing provider secrets in SQLite.

#### Scenario: Cloudflare token loaded from environment
- **WHEN** ACME Cloudflare DNS-01 automation is enabled and the configured token environment variable exists
- **THEN** the server uses that token for DNS challenge operations and MUST NOT persist the token value to SQLite

#### Scenario: Missing provider credential blocks issuance
- **WHEN** ACME issuance is requested but the configured Cloudflare token environment variable is missing or empty
- **THEN** issuance fails with a credential configuration error and existing HTTPS certificate files remain unchanged

### Requirement: Managed certificate issuance
The system SHALL issue a managed HTTPS proxy certificate through ACME DNS-01 for an enabled HTTPS proxy host when explicitly requested.

#### Scenario: Successful issuance writes managed files
- **WHEN** an enabled HTTPS proxy requests managed certificate issuance and DNS-01 validation succeeds
- **THEN** the server writes the certificate and private key under `certificate_dir`, validates the pair, and records certificate metadata for the proxy host

#### Scenario: Issuance failure preserves existing state
- **WHEN** ACME order creation, DNS challenge validation, certificate retrieval, certificate validation, or file persistence fails
- **THEN** the server records the failure metadata and MUST NOT replace the currently active certificate files

### Requirement: DNS challenge cleanup
The system SHALL clean up DNS-01 challenge records after ACME validation attempts.

#### Scenario: Cleanup after validation success
- **WHEN** DNS-01 validation succeeds for a managed certificate request
- **THEN** the server removes the temporary DNS challenge record before marking issuance successful

#### Scenario: Cleanup after validation failure
- **WHEN** DNS-01 validation fails after the challenge record was created
- **THEN** the server attempts to remove the temporary DNS challenge record and records any cleanup failure separately from the issuance failure

### Requirement: Renewal scheduling
The system SHALL renew managed HTTPS certificates before expiry using a configured renewal window.

#### Scenario: Certificate enters renewal window
- **WHEN** a managed certificate expires within the configured renewal window
- **THEN** the daemon attempts renewal without requiring `goginx-server` restart

#### Scenario: Certificate outside renewal window
- **WHEN** a managed certificate expires after the configured renewal window
- **THEN** the daemon leaves the active certificate unchanged and does not attempt renewal for that cycle

### Requirement: Hot reload and rollback
The system SHALL hot reload validated managed certificate replacements and retain the previous valid certificate material for rollback.

#### Scenario: Successful renewal hot reloads certificate
- **WHEN** a managed certificate renewal succeeds and the replacement certificate/key pair validates for the proxy host
- **THEN** the HTTPS entry serves the replacement certificate for new TLS handshakes without restarting the listener

#### Scenario: Invalid replacement rolls back
- **WHEN** a replacement certificate/key pair fails validation after issuance or file write
- **THEN** the server keeps serving the previous valid certificate and records renewal failure metadata

### Requirement: Managed certificate status
The system SHALL expose managed certificate status through admin operations without exposing private-key material or provider secrets.

#### Scenario: Status inspection
- **WHEN** an admin inspects managed certificate status for a proxy host
- **THEN** the output includes host, status, expiry, active certificate path, last issuance or renewal result, and MUST NOT include private-key bytes or DNS provider token values
