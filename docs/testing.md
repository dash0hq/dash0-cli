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
- Always use `testutil.RequireAuthHeader` as the validator to ensure auth token validation
- Use constants for API paths and fixture filenames to avoid duplication
- Use `http.MethodGet`, `http.StatusOK`, etc. instead of string/numeric literals

## When to Update Fixtures
- When the Dash0 API response format changes
- When adding tests for new API endpoints
- When existing tests fail due to outdated fixture data
- Run `generate_all.sh` periodically to keep fixtures in sync with the actual API
