# Integration Tests

Integration tests use a mock HTTP server (`internal/testutil/mockserver.go`) with JSON fixture files to simulate API responses from the Dash0 API.
The OpenAPI specification of the Dash0 API is available at `https://api-docs.dash0.com/reference`.

## Fixture Location
- Fixtures are stored in `internal/testutil/fixtures/`
- Organized by asset type: `dashboards/`, `checkrules/`, `views/`, `syntheticchecks/`
- Common fixture patterns: `list_success.json`, `list_empty.json`, `get_success.json`, `error_not_found.json`, `error_unauthorized.json`

## Generating Fixtures
- Fixture generation scripts are in `internal/testutil/fixtures/scripts/`
- Scripts follow the naming pattern: `<asset>_<functionality>.fixture.sh`
- Run all scripts: `DASH0_API_URL='https://api...' DASH0_AUTH_TOKEN='auth_...' ./generate_all.sh`
- Scripts output to stdout; redirect to create fixture files

## Writing Integration Tests
- Add `//go:build integration` at the top of integration test files (before `package`)
- Name files with `_integration_test.go` suffix for clarity
- Use `testutil.NewMockServer(t, testutil.FixturesDir())` to create a mock server
- Register routes with `server.On()` for exact paths or `server.OnPattern()` for regex patterns
- Always use `testutil.RequireHeaders` as the validator to ensure auth token and user agent validation
- Use constants for API paths and fixture filenames to avoid duplication
- Use `http.MethodGet`, `http.StatusOK`, etc. instead of string/numeric literals

## When to Update Fixtures
- When the Dash0 API response format changes
- When adding tests for new API endpoints
- When existing tests fail due to outdated fixture data
- Run `generate_all.sh` periodically to keep fixtures in sync with the actual API

# Roundtrip Tests

Roundtrip tests live in `test/roundtrip/` and exercise the CLI end-to-end against a real Dash0 environment.
They create assets, read them back, verify the output, and clean up.

## Prerequisites
- Build the CLI first: `make build`
- An active profile with `api-url` and `auth-token` must be configured.

## Running
- Run all: `bash test/roundtrip/run_all.sh`
- Run one: `bash test/roundtrip/test_dashboard_roundtrip.sh`

## Structure
- `run_all.sh` — orchestrator that runs all test scripts and reports pass/fail counts.
- `test_<asset>_roundtrip.sh` — CRUD roundtrip for each asset type (create, get, list, update, delete).
- `test_apply_<asset>_idempotency.sh` — verifies that `apply` is idempotent (apply twice, second reports no changes).
- `test_<asset>_annotations.sh` — verifies that user-settable annotations (`folder-path`, `sharing`) survive a roundtrip.
- `test_prometheus_rule_roundtrip.sh` — roundtrip for PrometheusRule CRD import via `check-rules create`.
- `test_perses_dashboard_roundtrip.sh` — roundtrip for PersesDashboard CRD import via `dashboards create`.
- `test_log_roundtrip.sh`, `test_span_roundtrip.sh` — send and query telemetry signals.
- `test_team_roundtrip.sh` — team CRUD and membership management.
- `fixtures/` — YAML asset definitions used as input by the test scripts.

## When to Add Roundtrip Tests
- When adding a new asset type or command, add a corresponding `test_<asset>_roundtrip.sh`.
- When adding a new `apply` code path (e.g., a new CRD format), add an idempotency test.
- When adding annotation support to an asset type, add an annotations test.
- When adding a new signal command (e.g., `metrics send`), add a send-and-query roundtrip.
- Register every new test script in the `for` loop in `run_all.sh`.
