## MODIFIED Requirements

### Requirement: ACME DNS-01 automation contract
The system SHALL support ACME DNS-01 certificate issuance and renewal for eligible proxy domains using least-privilege DNS provider credentials supplied outside SQLite.

#### Scenario: ACME automation issues managed certificate
- **WHEN** ACME DNS-01 issuance is requested for an eligible HTTPS proxy host and provider validation succeeds
- **THEN** the system retrieves, validates, stores, and activates a managed certificate for that host

#### Scenario: ACME automation preserves private-key boundary
- **WHEN** ACME DNS-01 issuance or renewal stores certificate metadata
- **THEN** SQLite stores lifecycle metadata and file paths only and MUST NOT store private-key material or DNS provider token values

#### Scenario: ACME challenge cleanup is required
- **WHEN** ACME DNS-01 validation completes or fails after creating a DNS challenge record
- **THEN** the system attempts challenge cleanup and records cleanup failures without exposing provider credentials

### Requirement: Renewal, hot reload, and rollback contract
The system SHALL support certificate renewal, validated hot reload, old-certificate retention for rollback, and failure handling without weakening certificate validation.

#### Scenario: Renewal hot reloads valid replacement
- **WHEN** a managed certificate is renewed successfully and the replacement certificate/key pair validates for the configured proxy host
- **THEN** new HTTPS termination handshakes use the replacement certificate without restarting the HTTPS listener

#### Scenario: Renewal failure preserves active certificate
- **WHEN** renewal, validation, file write, or reload fails
- **THEN** the system keeps serving the previously active valid certificate and records failure status for inspection

#### Scenario: Rollback material is retained
- **WHEN** a managed certificate replacement becomes active
- **THEN** the previous valid certificate and key are retained for rollback until replaced by a later successful lifecycle operation
