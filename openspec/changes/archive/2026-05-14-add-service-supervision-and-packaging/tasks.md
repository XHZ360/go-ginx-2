## 1. Packaging Workflow

- [x] 1.1 Add a reproducible bundle-generation workflow for `goginx-server`, `goginx-client`, and `goginx-admin`.
- [x] 1.2 Define and generate the bundle directory layout for binaries, config, data, certificates, and logs.
- [x] 1.3 Include sample config and environment/template files required by the supported deployment model.

## 2. Service Supervision Baseline

- [x] 2.1 Add `systemd` unit templates for supervised `goginx-server` and `goginx-client` execution.
- [x] 2.2 Ensure the packaged runtime and current shutdown behavior satisfy supervised stop/restart expectations.
- [x] 2.3 Document restart policy, working directory, config path, and dependency assumptions for the supported service units.

## 3. Validation And Documentation

- [x] 3.1 Add deployment-focused validation that starts the packaged runtime from the bundle layout.
- [x] 3.2 Add deployment-focused validation that proves client recovery after supervised daemon restart.
- [x] 3.3 Update runtime and deployment docs with bundle creation, install, start/stop/restart, upgrade, rollback, and troubleshooting guidance.
- [x] 3.4 Run `go test ./...`, build the packaged artifacts, and verify the new deployment workflow end to end.
