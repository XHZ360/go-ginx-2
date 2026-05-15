## ADDED Requirements

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

### Requirement: HTTPS reverse proxy gap tracking
The reverse-proxy runtime spec SHALL track HTTPS termination and HTTPS passthrough behavior as required/design behavior that is not implemented in the current baseline.

#### Scenario: HTTPS behavior planned but not implemented
- **WHEN** HTTPS reverse-proxy behavior is referenced from product or design documents
- **THEN** the spec MUST identify that behavior as a future gap until evidence-backed implementation exists

#### Scenario: Future HTTPS implementation
- **WHEN** future work implements HTTPS termination or HTTPS passthrough behavior
- **THEN** this spec MUST be updated with evidence-backed scenarios before HTTPS is claimed as implemented

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
