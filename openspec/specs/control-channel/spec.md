## Purpose

Define the client/server control-channel contract for secure transport, client authentication, server certificate verification, proxy snapshot delivery, heartbeat/session liveness, latest-session routing, TCP+TLS fallback proxy substreams, and explicit tracking of reconnect/recovery gaps.

## Requirements

### Requirement: Secure control transport baseline
The system SHALL provide an authenticated, encrypted client/server control channel over QUIC and TCP+TLS. TCP+TLS SHALL support control authentication, proxy snapshot delivery, heartbeats, and framed proxy substreams over the fallback connection.

#### Scenario: QUIC control connection succeeds
- **WHEN** a client connects to the configured QUIC control listener with trusted server TLS identity and valid credentials
- **THEN** the server accepts the control connection and creates an authenticated session

#### Scenario: TCP+TLS control connection succeeds
- **WHEN** a client connects to the configured TCP+TLS control listener with trusted server TLS identity and valid credentials
- **THEN** the server accepts the control connection and creates an authenticated TCP+TLS session

#### Scenario: TCP+TLS proxy stream routing succeeds
- **WHEN** QUIC is unavailable and a client is authenticated over TCP+TLS
- **THEN** the server can open framed proxy substreams over the TCP+TLS connection and route proxy traffic to the client

#### Scenario: TCP+TLS head-of-line limitation documented
- **WHEN** documentation describes TCP+TLS fallback proxy streams
- **THEN** it MUST identify that multiplexed streams share one TCP connection and can experience TCP head-of-line effects

### Requirement: Server certificate verification
The client SHALL verify the server certificate chain and server name for the control channel, and the control-channel baseline MUST NOT rely on insecure certificate skipping.

#### Scenario: Trusted server certificate
- **WHEN** the server presents a certificate trusted by the configured client CA file and matching the configured server name
- **THEN** the client may continue the control-channel handshake

#### Scenario: Untrusted server certificate
- **WHEN** the server certificate is untrusted, expired, mismatched, or otherwise invalid
- **THEN** the client MUST reject the control-channel connection

### Requirement: Client authentication
The server SHALL authenticate client credentials before registering an active control-channel session or serving proxy configuration to the client.

#### Scenario: Valid client credential
- **WHEN** a client presents a known client ID and matching credential during the control-channel handshake
- **THEN** the server registers the authenticated session for that client

#### Scenario: Invalid client credential
- **WHEN** a client presents an unknown client ID or wrong credential
- **THEN** the server MUST reject the connection and MUST NOT register an active session

### Requirement: Proxy snapshot delivery
The server SHALL send the authenticated client its owned proxy snapshot after successful control-channel authentication.

#### Scenario: Snapshot after authentication
- **WHEN** client authentication succeeds
- **THEN** the server sends proxy configuration owned by that client over the control channel

#### Scenario: No snapshot before authentication
- **WHEN** client authentication has not succeeded
- **THEN** the server MUST NOT send client-owned proxy configuration

### Requirement: Heartbeat and session liveness
The client SHALL send heartbeat or status messages over the control channel, and the server SHALL update session liveness from those messages.

#### Scenario: Heartbeat updates liveness
- **WHEN** an authenticated client sends a heartbeat over the control channel
- **THEN** the server updates the session liveness record for that client

#### Scenario: Missing heartbeat recovery remains a gap
- **WHEN** heartbeat timeout, soft-offline, hard-offline, or recovery behavior is documented beyond current MVP evidence
- **THEN** that behavior MUST remain a gap until evidence-backed implementation exists

### Requirement: Latest authenticated session routing
The server SHALL route new proxy subchannels to the latest valid authenticated session for a client.

#### Scenario: Latest session selected
- **WHEN** multiple sessions have existed for the same client
- **THEN** new proxy subchannels are routed to the latest valid authenticated session

#### Scenario: Duplicate-session grace remains a gap
- **WHEN** documentation describes duplicate-session generation numbers, grace periods, or old-session drain behavior
- **THEN** that behavior MUST remain a gap until evidence-backed implementation exists

### Requirement: Reconnect and recovery gap tracking
The control-channel spec SHALL track reconnect, event replay, configuration-version reconciliation, and proxy restoration semantics as required/design behavior that is not fully implemented in the current baseline.

#### Scenario: Recovery behavior planned but not implemented
- **WHEN** reconnect or session recovery behavior is referenced from product or design documents
- **THEN** the spec MUST identify whether the behavior is evidence-backed or still a future gap

#### Scenario: Future recovery implementation
- **WHEN** future work implements reconnect, event replay, configuration-version reconciliation, or proxy restoration semantics
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented
