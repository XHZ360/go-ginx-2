## MODIFIED Requirements

### Requirement: HTTPS TLS termination baseline
The system SHALL support HTTPS TLS termination for enabled HTTPS proxies with file-backed certificate and private-key paths or managed reloadable certificates. The server SHALL select the proxy and certificate by SNI, complete the public TLS handshake, and forward the decrypted HTTP request to the client-side local HTTP target.

#### Scenario: HTTPS termination reaches local HTTP target
- **WHEN** an enabled HTTPS proxy exists for the TLS ClientHello SNI host, has an active static or managed certificate and key, and its client is online
- **THEN** the runtime terminates public TLS and forwards the decrypted HTTP request to the configured local HTTP target through the client

#### Scenario: HTTPS certificate selected by SNI
- **WHEN** HTTPS termination traffic arrives for a configured HTTPS proxy host
- **THEN** the server uses the active static or managed certificate and private key configured for that proxy host

#### Scenario: Managed certificate hot reload applies to new handshakes
- **WHEN** a managed certificate replacement is activated for an HTTPS proxy host
- **THEN** new TLS handshakes for that SNI host use the replacement certificate without restarting the HTTPS listener

#### Scenario: HTTPS passthrough remains available
- **WHEN** an enabled HTTPS proxy has no active static or managed certificate and key configured
- **THEN** the runtime preserves SNI passthrough behavior and forwards encrypted TLS bytes without requiring proxy private-key access

### Requirement: HTTPS certificate lifecycle gap tracking
The reverse-proxy runtime spec SHALL track richer HTTPS certificate lifecycle and policy behavior that is not implemented in the current baseline.

#### Scenario: Remaining advanced HTTPS behavior planned but not implemented
- **WHEN** access-password page, temporary share flow, HTTP status inspection, rich HTTPS error response behavior, external alerts, or admin UI lifecycle behavior is referenced from product or design documents
- **THEN** the spec MUST identify that behavior as a future gap until evidence-backed implementation exists
