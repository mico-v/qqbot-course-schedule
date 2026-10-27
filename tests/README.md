# Test layout

- `unit/`: public API and pure domain tests.
- `contract/`: the same port behavior against adapters and fakes.
- `integration/`: real SQLite, migrations, admin, server and webhook boundaries.
- `e2e/`: complete inbound-message or HTTP workflows.
- `testutil/`: reusable fakes, fixtures and request builders.
- `testdata/`: input-only fixtures such as ICS, payloads and backups.

Tests in production source directories are limited to files ending in
`_whitebox_test.go` that genuinely require unexported state.
