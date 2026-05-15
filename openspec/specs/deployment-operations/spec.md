## Purpose

Define the deployment and operations contract for local daemon setup, configuration, troubleshooting, packaged deployment, supervised service lifecycle, deployment validation, backup/restore, capacity validation, low-resource operation, and operations documentation, while distinguishing implemented local and first-supported production guidance from remaining operations gaps.

## Requirements

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

### Requirement: Packaged deployment bundle baseline
The system SHALL produce a reproducible deployment bundle for the first supported single-node deployment model.

#### Scenario: Bundle contains required runtime artifacts
- **WHEN** an operator builds a deployment bundle for the supported production model
- **THEN** the output includes the `goginx-server`, `goginx-client`, and `goginx-admin` binaries, sample or documented config locations, service-unit templates, and the expected runtime directory layout for config, data, certificates, and logs

#### Scenario: Bundle layout is stable across builds
- **WHEN** operators or automation consume the deployment bundle
- **THEN** the artifact paths and directory structure remain stable enough for documented install and upgrade steps to target them without manual discovery

### Requirement: Service lifecycle baseline
The system SHALL support supervised start, stop, and restart behavior for the first supported deployment model by running the existing foreground binaries under an external service manager.

#### Scenario: Supervised server lifecycle
- **WHEN** the operator installs and starts the supported server service unit
- **THEN** the service manager launches `goginx-server` in the foreground with the configured working directory and config path and can stop it through normal service shutdown behavior

#### Scenario: Supervised client lifecycle
- **WHEN** the operator installs and starts the supported client service unit
- **THEN** the service manager launches `goginx-client` in the foreground with the configured working directory and config path and can restart it after transient failures according to the documented policy

#### Scenario: Graceful shutdown preserves runtime guarantees
- **WHEN** the service manager stops a supervised daemon process
- **THEN** the daemon exits through its normal shutdown path so listeners close cleanly and persisted runtime state such as cumulative proxy stats is flushed before exit

### Requirement: Deployment validation baseline
The system SHALL provide evidence-backed validation for the packaged deployment and supervised restart model.

#### Scenario: Packaged runtime starts from bundle layout
- **WHEN** automated validation runs against the deployment bundle
- **THEN** it proves the packaged server and client binaries start successfully using the documented bundle layout and config paths

#### Scenario: Supervised restart recovery is validated
- **WHEN** automated validation simulates daemon restart under the supported supervision model
- **THEN** it proves the runtime can shut down cleanly and recover client connectivity using the documented restart flow

### Requirement: Production packaging gap tracking
The deployment-operations spec SHALL treat a reproducible single-node deployment bundle as implemented baseline behavior while continuing to track richer packaging and installation behavior as future work.

#### Scenario: Supported packaging baseline exists
- **WHEN** an operator follows the documented deployment packaging workflow for the first supported production model
- **THEN** they can produce a reproducible bundle with the required binaries, configuration layout, and service templates for deployment

#### Scenario: Advanced packaging remains a gap
- **WHEN** native installers, package-manager distribution, signed release artifacts, or multi-platform packaging behavior is referenced from product or design documents
- **THEN** that behavior MUST remain a future gap until evidence-backed implementation exists

### Requirement: Service supervision gap tracking
The deployment-operations spec SHALL treat external service-manager supervision for the first supported deployment model as implemented baseline behavior while continuing to track richer lifecycle management as future work.

#### Scenario: Supported supervision baseline exists
- **WHEN** an operator follows the documented service install and lifecycle steps for the first supported deployment model
- **THEN** they can start, stop, and restart the server and client under the supported service manager using the packaged artifacts

#### Scenario: Advanced supervision remains a gap
- **WHEN** readiness signaling, multi-service orchestration, advanced health management, watchdog integration, or non-supported service managers are referenced from product or design documents
- **THEN** that behavior MUST remain a future gap until evidence-backed implementation exists

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
The deployment-operations spec SHALL treat packaged install and supervised lifecycle guidance for the first supported deployment model as implemented documentation baseline behavior while continuing to track broader production operations documentation as future work.

#### Scenario: Supported operations documentation exists
- **WHEN** an operator follows the current deployment operations documentation for the first supported production model
- **THEN** they can build the bundle, install the service units, run start/stop/restart flows, and troubleshoot the documented failure categories

#### Scenario: Broader production operations documentation remains a gap
- **WHEN** backup/restore runbooks, incident response playbooks, security hardening guides, or multi-environment operational procedures are referenced from product or design documents
- **THEN** that behavior MUST remain a future gap until evidence-backed documentation exists
