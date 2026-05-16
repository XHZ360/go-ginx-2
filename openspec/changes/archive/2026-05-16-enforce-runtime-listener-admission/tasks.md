## 1. Listener Admission Model

- [x] 1.1 Inventory the current TCP/UDP proxy create, update, and enable write paths in `internal/admin/service.go` and identify where admission can run before persistence.
- [x] 1.2 Identify or add a shared ListenerClaim representation that can describe static configured listeners plus enabled TCP/UDP proxy listeners under the V1 `same network + same port` rule.
- [x] 1.3 Define how the active claim set is assembled from `control_quic_listen`, `control_tls_listen`, `admin_listen`, `http_entry_listen`, `https_entry_listen`, enabled TCP proxies, and enabled UDP proxies while excluding disabled proxies.

## 2. Admin Write-Path Enforcement

- [x] 2.1 Add pre-admission checks to TCP/UDP proxy create flows so conflicting active listeners are rejected before invalid state is accepted where possible.
- [x] 2.2 Add pre-admission checks to TCP/UDP proxy update flows, ensuring the updated proxy can replace its own prior claim without self-conflict.
- [x] 2.3 Add pre-admission checks to TCP/UDP proxy enable flows so enabling a conflicting disabled proxy returns an admission failure.
- [x] 2.4 Ensure non-conflicting disabled-proxy edits remain allowed because disabled proxies are excluded from the active claim set.

## 3. Error Semantics And API Mapping

- [x] 3.1 Introduce or reuse a distinct domain error for listener-admission failures with explicit `ENTRY_CONFLICT` semantics.
- [x] 3.2 Update admin API error mapping so listener-admission failures surface as `ENTRY_CONFLICT` instead of generic validation or persistence errors.
- [x] 3.3 Verify the admin response path preserves actionable conflict behavior for create, update, and enable operations without broad GraphQL redesign.

## 4. Verification

- [x] 4.1 Add focused tests covering conflicts against static configured listeners for TCP and UDP operations.
- [x] 4.2 Add focused tests covering conflicts against enabled TCP proxies, enabled UDP proxies, and exclusion of disabled proxies from the active claim set.
- [x] 4.3 Add focused tests covering create, update, and enable admission failures and success cases under the V1 `same network + same port` rule.
