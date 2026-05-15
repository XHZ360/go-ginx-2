## ADDED Requirements

### Requirement: Local daemon deployment baseline
The system SHALL provide local milestone-one daemon build, run, and configuration guidance where current documentation evidence supports that behavior.

#### Scenario: Build local daemon binaries
- **WHEN** an operator follows current local daemon documentation
- **THEN** they can build server, client, and admin command binaries for local milestone-one use

#### Scenario: Configure local server and client
- **WHEN** an operator follows current local daemon documentation
- **THEN** they can create server and client JSON configuration files using documented runtime fields

#### Scenario: Run local daemon pair
- **WHEN** SQLite resources and TLS files are prepared according to current documentation
- **THEN** the operator can run the local server/client daemon pair for supported milestone-one behavior

### Requirement: Local troubleshooting baseline
The system SHALL provide local troubleshooting guidance for current milestone-one daemon setup and proxy operation.

#### Scenario: Troubleshoot local daemon setup
- **WHEN** an operator encounters known local setup issues such as unknown config fields, missing TLS files, CA/SNI mismatch, auth rejection, missing listeners, Host mismatch, target unreachable, UDP response issues, or stats flush timing
- **THEN** current documentation provides troubleshooting guidance for that issue category

### Requirement: Production packaging gap tracking
The deployment-operations spec SHALL track production packaging and installation behavior as required/design behavior that is not implemented in the current baseline.

#### Scenario: Production packaging remains a gap
- **WHEN** production packages, installers, release artifacts, service integration, or production deployment documentation is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future packaging implementation
- **WHEN** future work implements production packaging or installation behavior
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented

### Requirement: Service supervision gap tracking
The deployment-operations spec SHALL track service supervision and lifecycle management as required/design behavior that is not implemented in the current baseline.

#### Scenario: Service supervision remains a gap
- **WHEN** automatic start, restart, stop, health management, system service, process supervision, or production lifecycle behavior is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future supervision implementation
- **WHEN** future work implements service supervision behavior
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented

### Requirement: Backup and restore gap tracking
The deployment-operations spec SHALL track backup and restore behavior as required/design behavior that is not implemented in the current baseline.

#### Scenario: Backup and restore remain gaps
- **WHEN** SQLite backup, config backup, certificate metadata backup, private-key protected backup, restore, or post-restore reload behavior is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future backup or restore implementation
- **WHEN** future work implements backup or restore behavior
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented

### Requirement: Capacity and low-resource operations gap tracking
The deployment-operations spec SHALL track 1C1G and 800+ concurrent connection goals as required/design behavior that is not validated in the current baseline.

#### Scenario: Capacity target remains a gap
- **WHEN** 1C1G operation, low idle overhead, 800+ concurrent connections, file descriptor limits, memory limits, or capacity strategy behavior is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed validation exists

#### Scenario: Future capacity validation
- **WHEN** future work validates capacity or low-resource behavior
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented

### Requirement: Operations documentation gap tracking
The deployment-operations spec SHALL track complete deployment, configuration, usage, troubleshooting, and security guidance as required/design behavior that is only partially implemented in the current baseline.

#### Scenario: Production operations documentation remains a gap
- **WHEN** production deployment, configuration, usage, troubleshooting, security advice, incident response, or operational runbook behavior is referenced from product or design documents
- **THEN** any behavior beyond current local daemon documentation MUST be tracked as a future gap until evidence-backed documentation exists

#### Scenario: Future operations documentation
- **WHEN** future work adds production operations documentation
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented
