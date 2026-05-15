## Purpose

Define the reverse-proxy runtime contract for TCP, UDP, HTTP, HTTPS SNI-passthrough, and HTTPS static or managed TLS termination forwarding over the authenticated control-channel path, plus explicit tracking of forward-proxy, access-control, quota, richer error, and production-observability gaps.

## Requirements

### Requirement: TCP reverse proxy baseline
The system SHALL support TCP reverse-proxy forwarding from a public TCP entry to a client-side local target over the authenticated control-channel stream path.

#### Scenario: TCP traffic reaches local target
- **WHEN** an enabled TCP proxy is configured for an online authenticated client
- **THEN** external TCP traffic to the proxy entry is forwarded to the configured local target through the client

#### Scenario: TCP traffic statistics are recorded
- **WHEN** TCP traffic flows through the proxy runtime
- **THEN** the runtime records basic TCP connection and byte counters for the proxy

### Requirement: UDP reverse proxy baseline
The system SHALL support UDP reverse-proxy forwarding from a public UDP entry to a client-side local UDP target over the authenticated control-channel stream path.

#### Scenario: UDP packet reaches local target
- **WHEN** an enabled UDP proxy is configured for an online authenticated client
- **THEN** external UDP packets to the proxy entry are forwarded to the configured local UDP target through the client

#### Scenario: UDP per-source session is maintained
- **WHEN** UDP traffic arrives from an external source address
- **THEN** the runtime maintains a per-source session for forwarding responses back to the original source until idle cleanup

#### Scenario: UDP traffic statistics are recorded
- **WHEN** UDP traffic flows through the proxy runtime
- **THEN** the runtime records basic UDP packet and byte counters for the proxy

### Requirement: HTTP reverse proxy baseline
The system SHALL support HTTP reverse-proxy forwarding by matching the request Host to an enabled HTTP proxy and forwarding the request to a client-side local HTTP target.

#### Scenario: HTTP request reaches local target
- **WHEN** an enabled HTTP proxy exists for the request Host and its client is online
- **THEN** the runtime forwards the HTTP request to the configured local HTTP target through the client

#### Scenario: HTTP response returns to caller
- **WHEN** the local HTTP target returns a response
- **THEN** the runtime returns the response status, headers, and body to the external caller

#### Scenario: HTTP traffic statistics are recorded
- **WHEN** HTTP traffic flows through the proxy runtime
- **THEN** the runtime records basic HTTP request, status-code, byte, and error counters for the proxy

### Requirement: Daemon runtime proxy startup
The server daemon SHALL start reverse-proxy entries from enabled proxy records, and the client daemon SHALL authenticate, receive proxy configuration, and serve reverse-proxy streams to local targets.

#### Scenario: Server starts configured entries
- **WHEN** the server daemon starts with enabled TCP, UDP, or HTTP proxy records in SQLite
- **THEN** it starts the corresponding reverse-proxy entry listeners

#### Scenario: Client serves proxy streams
- **WHEN** the client daemon authenticates and receives proxy configuration
- **THEN** it serves TCP, UDP, and HTTP proxy streams to configured local targets

### Requirement: HTTPS reverse proxy passthrough baseline
The system SHALL support HTTPS reverse-proxy SNI passthrough by routing the TLS ClientHello SNI host to an enabled HTTPS proxy and forwarding the encrypted TCP stream to the client-side local HTTPS target without terminating TLS on the public server.

#### Scenario: HTTPS passthrough reaches local target
- **WHEN** an enabled HTTPS proxy exists for the TLS ClientHello SNI host and its client is online
- **THEN** the runtime forwards the encrypted TLS stream to the configured local HTTPS target through the client

#### Scenario: Public server does not terminate passthrough TLS
- **WHEN** HTTPS passthrough traffic flows through the proxy runtime
- **THEN** the public server uses SNI only for routing and MUST NOT require proxy certificate selection or private-key access for that passthrough connection

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

#### Scenario: Advanced HTTPS behavior planned but not implemented
- **WHEN** access-password page, temporary share flow, HTTP status inspection, rich HTTPS error response behavior, external alerts, or admin UI lifecycle behavior is referenced from product or design documents
- **THEN** the spec MUST identify that behavior as a future gap until evidence-backed implementation exists

### Requirement: Runtime policy gap tracking
The reverse-proxy runtime spec SHALL track access passwords, temporary share links, quotas, rate limits, richer error responses, and production-grade observability as required/design behavior that is not implemented in the current baseline.

#### Scenario: Policy behavior planned but not implemented
- **WHEN** access control, quota, rate-limit, share-link, rich error response, or production observability behavior is referenced from product or design documents
- **THEN** the spec MUST identify that behavior as a future gap until evidence-backed implementation exists

#### Scenario: Future policy implementation
- **WHEN** future work implements one of the policy or observability behaviors
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented

### Requirement: Forward proxy exclusion
The reverse-proxy runtime baseline SHALL NOT claim forward-proxy behavior as implemented.

#### Scenario: Forward proxy remains separate
- **WHEN** forward proxy behavior is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future or separate capability and MUST NOT be included in the reverse-proxy MVP baseline
