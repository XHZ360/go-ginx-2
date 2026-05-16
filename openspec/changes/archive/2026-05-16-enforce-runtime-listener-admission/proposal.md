## Why

Proxy create, update, and enable flows can currently persist TCP or UDP states that only fail later when the runtime attempts to bind listeners. Database uniqueness only covers part of the listener space, so the management plane needs an explicit pre-admission model that checks the full active runtime listener set before invalid state is accepted.

## What Changes

- Define a shared ListenerClaim and runtime listener-admission model for TCP and UDP proxy create, update, and enable operations.
- Require admission to evaluate configured static listeners, enabled TCP proxies, enabled UDP proxies, and the V1 conflict rule of `same network + same port` before persistence is accepted where possible.
- Exclude disabled proxies from the active claim set used for listener conflict detection.
- Require explicit `ENTRY_CONFLICT` semantics for listener-admission failures instead of allowing invalid state to persist and fail later at runtime.
- Capture the current write-path and error-mapping areas likely to be updated, including `internal/admin/service.go`, admin API error translation, and runtime/config listener topology assembly.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `admin-resource-management`: strengthen proxy lifecycle requirements so TCP and UDP create, update, and enable operations must pass runtime listener admission and return explicit `ENTRY_CONFLICT` errors when they would conflict with the active listener set.

## Impact

- Affected systems: admin proxy lifecycle validation, runtime listener topology modeling, and admin API error semantics.
- Affected code areas: likely future work in `internal/admin/service.go`, admin API error mapping layers, and listener topology/config assembly used to determine active runtime listeners.
- Scope excludes quotas, observability overhaul, GraphQL redesign, forward proxy behavior, RBAC, and unrelated admin productization work.
