## MODIFIED Requirements

### Requirement: Administrator proxy-management baseline
The system SHALL provide full administrator CRUD and lifecycle control for the currently supported reverse-proxy resource types in the first API/UI batch, and SHALL reject TCP or UDP lifecycle changes that would violate active runtime listener admission before invalid state is accepted where possible.

#### Scenario: Manage supported reverse-proxy types
- **WHEN** an authenticated administrator creates or updates a proxy in V1
- **THEN** the management plane supports the currently implemented reverse-proxy types `TCP`, `UDP`, `HTTP`, and `HTTPS`, and does not claim forward-proxy creation in this batch

#### Scenario: View proxy list and detail
- **WHEN** an authenticated administrator queries proxies in V1
- **THEN** the system returns proxy list and detail views that combine persisted configuration with available runtime and aggregate status information

#### Scenario: Create proxy
- **WHEN** an authenticated administrator creates a valid proxy in V1
- **THEN** the system persists the proxy resource and records the control-plane action

#### Scenario: Create TCP or UDP proxy rejected by listener admission
- **WHEN** an authenticated administrator creates a TCP or UDP proxy whose requested active listener conflicts with the active runtime listener set under the V1 `same network + same port` rule
- **THEN** the system rejects the operation with `ENTRY_CONFLICT` semantics before accepting invalid persisted state where possible

#### Scenario: Update proxy without type mutation
- **WHEN** an authenticated administrator updates an existing proxy in V1
- **THEN** the system allows updates to supported proxy fields but rejects in-place mutation of the proxy type

#### Scenario: Update TCP or UDP proxy rejected by listener admission
- **WHEN** an authenticated administrator updates a TCP or UDP proxy so its resulting active listener would conflict with the active runtime listener set under the V1 `same network + same port` rule
- **THEN** the system rejects the operation with `ENTRY_CONFLICT` semantics before accepting invalid persisted state where possible

#### Scenario: Enable or disable proxy
- **WHEN** an authenticated administrator enables or disables a proxy in V1
- **THEN** the system treats that action as an explicit lifecycle operation rather than an incidental status field edit

#### Scenario: Enable TCP or UDP proxy rejected by listener admission
- **WHEN** an authenticated administrator enables a TCP or UDP proxy whose active listener would conflict with the active runtime listener set under the V1 `same network + same port` rule
- **THEN** the system rejects the enable operation with `ENTRY_CONFLICT` semantics before accepting invalid active state where possible

#### Scenario: Listener admission evaluates full active runtime listener space
- **WHEN** the management plane evaluates listener admission for a TCP or UDP create, update, or enable operation
- **THEN** it checks configured static listeners from `control_quic_listen`, `control_tls_listen`, `admin_listen`, `http_entry_listen`, and `https_entry_listen` where applicable, plus enabled TCP proxies and enabled UDP proxies

#### Scenario: Disabled proxies are excluded from active listener admission
- **WHEN** the management plane evaluates listener admission for a TCP or UDP create, update, or enable operation
- **THEN** disabled proxies are excluded from the active claim set used for conflict detection

#### Scenario: Delete requires disabled proxy
- **WHEN** an authenticated administrator requests proxy deletion in V1
- **THEN** the system only allows delete after the proxy has first been disabled

### Requirement: Proxy listener-admission semantics
The system SHALL evaluate TCP and UDP proxy socket admission through a shared ListenerClaim model over the active runtime listener space and surface active listener conflicts as explicit contract behavior.

#### Scenario: ListenerClaim conflict rejects create, update, or enable operations
- **WHEN** an authenticated administrator creates, updates, or enables a TCP or UDP proxy whose requested active listener conflicts with an existing active claim under the V1 `same network + same port` rule
- **THEN** the operation is rejected with `ENTRY_CONFLICT` semantics rather than a generic persistence failure

#### Scenario: Active ListenerClaim set includes configured static listeners
- **WHEN** listener admission is evaluated for TCP or UDP proxy activity
- **THEN** the active ListenerClaim set includes configured listeners derived from `control_quic_listen`, `control_tls_listen`, `admin_listen`, `http_entry_listen`, and `https_entry_listen` where those listeners are configured and participate in runtime binding

#### Scenario: Active ListenerClaim set includes enabled TCP and UDP proxies
- **WHEN** listener admission is evaluated for TCP or UDP proxy activity
- **THEN** the active ListenerClaim set includes enabled TCP proxies and enabled UDP proxies that would occupy active runtime listeners

#### Scenario: Disabled proxies do not participate in active ListenerClaim admission
- **WHEN** listener admission is evaluated for TCP or UDP proxy activity
- **THEN** disabled proxies do not participate in the active claim set used for conflict detection
