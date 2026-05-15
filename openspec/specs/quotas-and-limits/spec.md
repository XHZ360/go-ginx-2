## Purpose

Define the quota, limit, and rate-limit contract for user/proxy resource controls, quota periods, enforcement points, and observable denial reasons, while explicitly tracking all current enforcement behavior as a gap because the existing implementation only has basic traffic counters.

## Requirements

### Requirement: User resource limit contract
The system SHALL support user-level resource limits for proxy count, concurrent connections, allowed port ranges, total traffic quota, and bandwidth ceiling. Current implementation evidence SHALL treat this behavior as a gap until enforcement exists.

#### Scenario: User resource limit remains a gap
- **WHEN** user-level proxy count, concurrent connection, port range, traffic quota, or bandwidth behavior is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future user limit enforcement
- **WHEN** future work implements user-level resource limit enforcement
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented

### Requirement: Proxy resource limit contract
The system SHALL support proxy-level traffic quotas, bandwidth ceilings, concurrent connection limits, and proxy-specific denial or pause behavior. Current implementation evidence SHALL treat this behavior as a gap until enforcement exists.

#### Scenario: Proxy resource limit remains a gap
- **WHEN** proxy-level quota, bandwidth, concurrency, denial, or pause behavior is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future proxy limit enforcement
- **WHEN** future work implements proxy-level resource limit enforcement
- **THEN** this spec MUST be updated with evidence-backed scenarios before the behavior is claimed as implemented

### Requirement: Quota period contract
The system SHALL support traffic quota periods at least for monthly and yearly quota windows. Current implementation evidence SHALL treat periodic quota behavior as a gap until storage, rollover, and enforcement exist.

#### Scenario: Periodic quota remains a gap
- **WHEN** monthly or yearly traffic quota behavior is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future quota period implementation
- **WHEN** future work implements monthly or yearly quota windows
- **THEN** this spec MUST be updated with evidence-backed scenarios covering quota period calculation and rollover before the behavior is claimed as implemented

### Requirement: Proxy enablement enforcement contract
The system SHALL validate applicable user, client, proxy, port, domain, quota, and limit constraints before enabling or accepting a proxy configuration. Current implementation evidence SHALL treat quota and limit checks as gaps until enforcement exists.

#### Scenario: Enablement quota check remains a gap
- **WHEN** a proxy is created, enabled, or updated and quota or limit validation is required by product or design documents
- **THEN** the quota or limit validation behavior MUST remain a gap until evidence-backed implementation exists

#### Scenario: Future enablement rejection
- **WHEN** future work rejects a proxy create, enable, or update operation because a limit is exceeded
- **THEN** this spec MUST be updated with evidence-backed scenarios for the rejected operation and denial reason

### Requirement: Runtime enforcement contract
The system SHALL enforce applicable quotas and limits during runtime traffic handling for TCP connections, UDP packets/sessions, HTTP requests, HTTPS proxying, and forward proxying where those protocols are implemented. Current implementation evidence SHALL treat runtime quota and rate-limit behavior as a gap until enforcement exists.

#### Scenario: Runtime enforcement remains a gap
- **WHEN** runtime quota, concurrency, bandwidth, or rate-limit enforcement is referenced from product or design documents
- **THEN** the behavior MUST be tracked as a future gap until evidence-backed implementation exists

#### Scenario: Future runtime denial or throttling
- **WHEN** future work denies, pauses, terminates, or throttles traffic because a runtime quota or limit is exceeded
- **THEN** this spec MUST be updated with evidence-backed scenarios for the affected protocol and observable result

### Requirement: Observable denial reason contract
The system SHALL expose quota, limit, and rate-limit denials using observable error classifications that can distinguish limit exhaustion from unrelated failures. Current implementation evidence SHALL treat quota denial observability as a gap until classification exists.

#### Scenario: Denial classification remains a gap
- **WHEN** quota refusal, bandwidth throttling, permission denial, or limit exhaustion behavior is referenced from product or design documents
- **THEN** the behavior MUST remain a gap until evidence-backed classification and reporting exist

#### Scenario: Future denial classification
- **WHEN** future work records or returns a quota, limit, or rate-limit denial
- **THEN** this spec MUST be updated with evidence-backed scenarios for the classification and resource context

### Requirement: Basic statistics exclusion
The quotas-and-limits baseline SHALL NOT claim quota enforcement from basic traffic counters alone.

#### Scenario: Counters do not imply enforcement
- **WHEN** current implementation evidence shows TCP, UDP, or HTTP traffic counters
- **THEN** those counters MUST NOT be treated as quota, bandwidth, concurrency, or rate-limit enforcement
