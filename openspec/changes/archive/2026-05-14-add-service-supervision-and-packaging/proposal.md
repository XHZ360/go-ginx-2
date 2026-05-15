## Why

`go-ginx-2` now has a usable milestone-one runtime, including reconnect, restart recovery, and persistent stats, but it still lacks the packaging and supervision layer needed to operate it as a deployable service instead of a manually launched pair of binaries. This is the highest remaining gap on the path from local runtime proof to an operable single-node deployment.

## What Changes

- Add a first production-operations batch for reproducible server, client, and admin packaging outputs aimed at single-node deployment.
- Add service supervision support for `goginx-server` and `goginx-client`, including explicit install/run/start/stop/restart behavior and clean shutdown expectations.
- Add deployment validation that proves packaged artifacts start correctly, survive supervised restarts, and preserve the runtime guarantees already implemented in the daemon layer.
- Extend operator documentation with packaging layout, service lifecycle procedures, upgrade/restart flow, and failure-recovery guidance.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `deployment-operations`: change packaging and service supervision from tracked gaps into implemented baseline behavior for the first supported deployment model.

## Impact

- Affected code: daemon command entrypoints, packaging/build scripts, service templates or wrappers, deployment-oriented validation, and runtime/operator documentation.
- Affected systems: local deployment workflow, release artifact layout, service lifecycle handling, and restart validation.
- No product-level database schema or control protocol changes are required for this batch.
