## ADDED Requirements

### Requirement: Basic proxy statistics baseline
The system SHALL record basic cumulative TCP, UDP, and HTTP proxy statistics where current implementation evidence supports those counters.

#### Scenario: TCP statistics baseline
- **WHEN** TCP proxy traffic flows through the current runtime
- **THEN** the runtime records basic TCP connection and byte counters for the proxy

#### Scenario: UDP statistics baseline
- **WHEN** UDP proxy traffic flows through the current runtime
- **THEN** the runtime records basic UDP packet and byte counters for the proxy

#### Scenario: HTTP statistics baseline
- **WHEN** HTTP proxy traffic flows through the current runtime
- **THEN** the runtime records basic HTTP request, status-code, byte, and error counters for the proxy

### Requirement: Statistics persistence boundary
The system SHALL distinguish SQLite-backed cumulative statistics from volatile runtime active counts.

#### Scenario: Cumulative stats survive clean shutdown
- **WHEN** the server runtime shuts down cleanly after TCP, UDP, or HTTP traffic
- **THEN** cumulative proxy statistics are flushed to SQLite where current implementation evidence supports that behavior

#### Scenario: Active counts reset after restart
- **WHEN** the runtime restarts
- **THEN** active connection or session counts MUST be treated as runtime state that resets unless future implementation evidence proves durable recovery

### Requirement: Full metrics gap tracking
The observability-and-audit spec SHALL track full metrics aggregation as required/design behavior that is not fully implemented in the current baseline.

#### Scenario: Metrics aggregation remains a gap
- **WHEN** global, user, client, proxy, time-range, response-time, error, quota-denial, target-unreachable, certificate, or GraphQL metrics are referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future metrics implementation
- **WHEN** future work implements full metrics aggregation
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented

### Requirement: Log collection and query gap tracking
The observability-and-audit spec SHALL track service runtime logs, client connection logs, proxy access logs, management operation logs, certificate task logs, retention, query, and export behavior as required/design behavior that is not implemented in the current baseline.

#### Scenario: Log query remains a gap
- **WHEN** log collection, retention, filtering, time-range query, export, or access-log behavior is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future log implementation
- **WHEN** future work implements log collection or query behavior
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented

### Requirement: Audit gap tracking
The observability-and-audit spec SHALL track audit coverage and audit-query behavior as required/design behavior that is not fully implemented in the current baseline.

#### Scenario: Audit query remains a gap
- **WHEN** login, proxy creation, proxy modification, proxy deletion, proxy enablement, proxy disablement, certificate upload, quota modification, system setting, high-risk operation, or audit query behavior is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future audit implementation
- **WHEN** future work implements audit recording or query behavior
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented

### Requirement: Error classification gap tracking
The observability-and-audit spec SHALL track error classification as required/design behavior that is not fully implemented in the current baseline.

#### Scenario: Error taxonomy remains a gap
- **WHEN** authentication failure, permission denial, invalid certificate, unverified domain, entry conflict, client offline, target unreachable, timeout, quota denial, bandwidth throttle, proxy disabled, credential revoked, duplicate session, protocol negotiation failure, DNS validation failure, or certificate renewal failure is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future error taxonomy implementation
- **WHEN** future work implements error classification
- **THEN** this spec MUST be updated with evidence-backed scenarios for the classification and resource context before the behavior is claimed as implemented

### Requirement: Alert gap tracking
The observability-and-audit spec SHALL track in-admin alert behavior as required/design behavior that is not implemented in the current baseline.

#### Scenario: Alert state remains a gap
- **WHEN** client frequent-offline, proxy error-rate, quota nearing, certificate expiry, certificate renewal failure, authentication spike, entry conflict, log backlog, or resource-capacity alert behavior is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future alert implementation
- **WHEN** future work implements alert state or alert display behavior
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented

### Requirement: Sensitive-data redaction gap tracking
The observability-and-audit spec SHALL track sensitive-data redaction in logs and audit records as required/design behavior that is not fully implemented in the current baseline.

#### Scenario: Redaction remains a gap
- **WHEN** Authorization, Cookie, password, token, private-key, share-token, access-password, or other sensitive value logging behavior is referenced from product or design documents
- **THEN** redaction behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future redaction implementation
- **WHEN** future work implements log or audit redaction
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented
