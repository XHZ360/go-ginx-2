## Purpose

Define the certificate and domain lifecycle contract for platform/custom domains, file-backed HTTPS proxy certificates, ACME DNS-01 automation, private-key protection, renewal, hot reload, rollback, and Origin CA/custom CA boundaries, while distinguishing current control-channel TLS verification from proxy certificate lifecycle gaps.

## Requirements

### Requirement: Control-channel TLS boundary
The system SHALL distinguish current control-channel TLS certificate verification from proxy certificate lifecycle management.

#### Scenario: Control TLS is current evidence
- **WHEN** current implementation evidence shows server certificate loading and client verification for the QUIC control channel
- **THEN** that evidence MAY be used for control-channel TLS verification only

#### Scenario: Control TLS does not imply unrelated proxy certificate lifecycle
- **WHEN** domain ownership, manual certificate lifecycle, or Origin CA/custom CA behavior is referenced from product or design documents
- **THEN** the behavior MUST remain a gap until evidence-backed implementation exists

### Requirement: Domain ownership and certificate binding contract
The system SHALL support platform-domain boundaries, user custom-domain ownership verification, and certificate binding rules for proxy domains. Current implementation evidence SHALL treat this behavior as a gap until implementation exists.

#### Scenario: Platform certificate scope remains a gap
- **WHEN** platform proxy-domain certificate scope is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Custom domain ownership remains a gap
- **WHEN** custom domain ownership verification or binding behavior is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Custom domain certificate isolation remains a gap
- **WHEN** product requirements state custom domains must not reuse the platform wildcard certificate
- **THEN** that behavior MUST remain a gap until evidence-backed implementation exists

### Requirement: File-backed HTTPS proxy certificate selection
The system SHALL support file-backed certificate and private-key paths for HTTPS proxy TLS termination, selected by the proxy host SNI. Private keys SHALL remain outside SQLite.

#### Scenario: HTTPS proxy certificate selected by host
- **WHEN** an HTTPS proxy has configured certificate and key files and public TLS traffic arrives with matching SNI
- **THEN** the server uses that certificate and key to terminate TLS for the proxy

#### Scenario: Private key path only
- **WHEN** HTTPS proxy certificate metadata is persisted
- **THEN** SQLite stores file paths only and MUST NOT store private-key material

### Requirement: Manual certificate lifecycle contract
The system SHALL support certificate upload, validation, replacement, disablement, and status visibility for managed proxy domains. Current implementation evidence SHALL treat upload/UI lifecycle behavior as a gap until implementation exists.

#### Scenario: Manual upload remains a gap
- **WHEN** certificate upload, replacement, disablement, or status visibility is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future manual certificate implementation
- **WHEN** future work implements manual certificate lifecycle behavior
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented

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

### Requirement: Private-key protection contract
The system SHALL protect certificate private keys from SQLite storage, admin UI plaintext display, and ordinary logs. Current implementation evidence SHALL treat private-key file paths as implemented and private-key upload/display lifecycle behavior as a gap until implementation exists.

#### Scenario: Private-key handling remains a gap
- **WHEN** private-key material storage, upload handling, log redaction, or UI display behavior is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future private-key implementation
- **WHEN** future work implements proxy private-key storage or upload behavior
- **THEN** this spec MUST be updated with evidence-backed scenarios proving keys are not stored in SQLite, not displayed in plaintext, and not written to ordinary logs

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

### Requirement: Origin CA advanced mode contract
The system SHALL treat Cloudflare Origin CA or custom CA trust as an explicit advanced mode that requires configured trust and MUST NOT introduce insecure certificate skipping.

#### Scenario: Origin CA remains a gap
- **WHEN** Origin CA or custom CA trust behavior is referenced from design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: No insecure certificate skip
- **WHEN** future work implements Origin CA or custom CA trust behavior
- **THEN** it MUST preserve certificate verification and MUST NOT rely on skipping certificate verification
