## ADDED Requirements

### Requirement: Control-channel TLS boundary
The system SHALL distinguish current control-channel TLS certificate verification from proxy certificate lifecycle management.

#### Scenario: Control TLS is current evidence
- **WHEN** current implementation evidence shows server certificate loading and client verification for the QUIC control channel
- **THEN** that evidence MAY be used for control-channel TLS verification only

#### Scenario: Control TLS does not imply proxy certificate lifecycle
- **WHEN** proxy certificate management, ACME, HTTPS certificate selection, renewal, hot reload, or rollback behavior is referenced from product or design documents
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

### Requirement: Manual certificate lifecycle contract
The system SHALL support certificate upload, validation, replacement, disablement, and status visibility for managed proxy domains. Current implementation evidence SHALL treat this behavior as a gap until implementation exists.

#### Scenario: Manual upload remains a gap
- **WHEN** certificate upload, validation, replacement, disablement, or status visibility is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future manual certificate implementation
- **WHEN** future work implements manual certificate lifecycle behavior
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented

### Requirement: ACME DNS-01 automation contract
The system SHALL support ACME DNS-01 certificate issuance and renewal for eligible proxy domains using least-privilege DNS provider credentials. Current implementation evidence SHALL treat this behavior as a gap until implementation exists.

#### Scenario: ACME automation remains a gap
- **WHEN** ACME DNS-01 issuance, renewal, DNS challenge cleanup, or Cloudflare provider behavior is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future ACME implementation
- **WHEN** future work implements ACME DNS-01 automation
- **THEN** this spec MUST be updated with evidence-backed scenarios covering order creation, challenge write, validation, certificate retrieval, validation, storage, reload, and cleanup before the behavior is claimed as implemented

### Requirement: Private-key protection contract
The system SHALL protect certificate private keys from SQLite storage, admin UI plaintext display, and ordinary logs. Current implementation evidence SHALL treat proxy private-key lifecycle behavior as a gap until implementation exists.

#### Scenario: Private-key handling remains a gap
- **WHEN** private-key storage, metadata persistence, upload handling, log redaction, or UI display behavior is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future private-key implementation
- **WHEN** future work implements proxy private-key storage or upload behavior
- **THEN** this spec MUST be updated with evidence-backed scenarios proving keys are not stored in SQLite, not displayed in plaintext, and not written to ordinary logs

### Requirement: Renewal, hot reload, and rollback contract
The system SHALL support certificate renewal, validated hot reload, old-certificate retention for rollback, and failure handling without weakening certificate validation. Current implementation evidence SHALL treat this behavior as a gap until implementation exists.

#### Scenario: Renewal and hot reload remain gaps
- **WHEN** renewal, hot reload, rollback, expiry reminder, or certificate failure alert behavior is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future renewal implementation
- **WHEN** future work implements certificate renewal, hot reload, or rollback
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented

### Requirement: Origin CA advanced mode contract
The system SHALL treat Cloudflare Origin CA or custom CA trust as an explicit advanced mode that requires configured trust and MUST NOT introduce insecure certificate skipping.

#### Scenario: Origin CA remains a gap
- **WHEN** Origin CA or custom CA trust behavior is referenced from design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: No insecure certificate skip
- **WHEN** future work implements Origin CA or custom CA trust behavior
- **THEN** it MUST preserve certificate verification and MUST NOT rely on skipping certificate verification
