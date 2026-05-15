## ADDED Requirements

### Requirement: Documentation source hierarchy
The documentation alignment process SHALL use a documented source hierarchy that distinguishes product intent, design approach, implementation evidence, and OpenSpec requirements.

#### Scenario: Classifying a documentation claim
- **WHEN** a roadmap or spec claim describes product scope, technical design, or implementation status
- **THEN** the claim MUST identify whether it is derived from product requirements, design documentation, implementation evidence, or an OpenSpec requirement

#### Scenario: Resolving implementation status
- **WHEN** product or design documentation describes a capability but current progress documentation does not show implementation evidence
- **THEN** the capability MUST NOT be marked as implemented

### Requirement: Roadmap gap matrix
The documentation alignment process SHALL maintain a roadmap/gap matrix for major capabilities that records requirement coverage, design coverage, implementation evidence, current status, and next action.

#### Scenario: Recording a major capability
- **WHEN** a major capability from the requirements or design documents is added to the roadmap/gap matrix
- **THEN** the matrix MUST include source references, status, evidence when implemented, and a next action when the capability is not complete

#### Scenario: Marking implemented capability
- **WHEN** a capability is marked implemented in the roadmap/gap matrix
- **THEN** the matrix MUST cite current implementation evidence such as progress notes, validation documentation, tests, build output, or active implementation references

### Requirement: Controlled status vocabulary
The documentation alignment process SHALL classify roadmap entries using only `required`, `designed`, `implemented`, `gap`, or `out-of-scope` status values.

#### Scenario: Reviewing roadmap status
- **WHEN** a roadmap entry is reviewed
- **THEN** its status MUST use one of the controlled values and MUST be consistent with the cited source and evidence

#### Scenario: Identifying missing implementation
- **WHEN** a capability is required and designed but lacks implementation evidence
- **THEN** its status MUST be `gap` unless it is explicitly excluded from the current product scope

### Requirement: No unsupported feature claims
The documentation alignment process SHALL prevent documentation from claiming unsupported production features as current implementation capabilities.

#### Scenario: Comparing design scope with current progress
- **WHEN** design documentation includes capabilities that current progress documents list as missing
- **THEN** alignment documentation MUST preserve those capabilities as gaps rather than current capabilities

#### Scenario: Updating implementation progress
- **WHEN** future work completes a previously missing capability
- **THEN** alignment documentation MUST update the status only after citing new implementation evidence
