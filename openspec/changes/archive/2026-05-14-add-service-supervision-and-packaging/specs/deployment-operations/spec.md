## ADDED Requirements

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

## MODIFIED Requirements

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

### Requirement: Operations documentation gap tracking
The deployment-operations spec SHALL treat packaged install and supervised lifecycle guidance for the first supported deployment model as implemented documentation baseline behavior while continuing to track broader production operations documentation as future work.

#### Scenario: Supported operations documentation exists
- **WHEN** an operator follows the current deployment operations documentation for the first supported production model
- **THEN** they can build the bundle, install the service units, run start/stop/restart flows, and troubleshoot the documented failure categories

#### Scenario: Broader production operations documentation remains a gap
- **WHEN** backup/restore runbooks, incident response playbooks, security hardening guides, or multi-environment operational procedures are referenced from product or design documents
- **THEN** that behavior MUST remain a future gap until evidence-backed documentation exists
