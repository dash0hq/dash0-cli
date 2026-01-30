# Dash0 CLI Development Guide

This repository provides a CLI utility to perform various tasks related with Dash0, enabling users via terminal,
AI agents and CI/CD workflows to perform tasks like creating, updating and deleting a number of assets in Dash0 like dashboards, alerting rules, views and more.

## Commands
- Build: `make build`
- Test all: `make test`
- Test unit only: `make test-unit`
- Test integration only: `make test-integration`
- Test specific: `go test -v ./internal/config -run TestServiceAddContext`
- Run locally: `./dash0 [command]`

## CLI Naming Conventions

In Dash0, dashboards, views, synthetic checks and check rules are called "assets", rather than the more common "resources". The reason for this is that the word "resource" is overloaded in OpenTelemetry, where it describes "where telemetry comes from". Use the word "asset" consistently where appropriate.

### Top-level Asset Commands
- Use **plural form**: `dashboards`, `views`, `check-rules`, `synthetic-checks`
- Use **kebab-case** for multi-word names: `check-rules`, `synthetic-checks`
- Group related functionality: `config profiles` for profile management

### Standard CRUD Subcommands for Assets
All asset commands (`dashboards`, `check-rules`, `views`, `synthetic-checks`) use these subcommands:

| Subcommand | Alias    | Description |
|------------|----------|-------------|
| `list`     | `ls`     | List all assets |
| `get`      | -        | Get a single asset by ID |
| `create`   | `add`    | Create a new asset from a file |
| `update`   | -        | Update an existing asset from a file |
| `delete`   | `remove` | Delete an asset by ID |

### Config Profiles Subcommands
The `config profiles` command uses:

| Subcommand | Alias    | Description |
|------------|----------|-------------|
| `list`     | `ls`     | List all profiles |
| `create`   | `add`    | Create a new profile |
| `delete`   | `remove` | Delete a profile |
| `select`   | -        | Set the active profile |

### Aliases
- `add` → `create`
- `remove` → `delete`
- `ls` → `list`

### Naming Rules
1. **Prefer verbs** for actions: `create`, `delete`, `list`, `get`, `update`, `select`
2. **Use plural** for asset type commands: `dashboards` not `dashboard`
3. **Use kebab-case** for multi-word commands: `check-rules` not `checkRules`
4. **Provide aliases** when renaming commands for backwards compatibility
5. **Be consistent** across all asset types - if `dashboards` has `create`, all assets should have `create`

## Code Style
- Use Go 1.24+ features
- Format with `gofmt`
- Add unit tests for all new functionality
- Use zerolog for structured logging
- Error handling: wrap errors with descriptive messages using `fmt.Errorf("... %w", err)`
- Naming: use camelCase for variable names and PascalCase for exported functions/types
- Set `DASH0_TEST_MODE=1` when writing tests that need to bypass validation

## Project Structure
- `/cmd/dash0`: Main entrypoint
- `/internal/config`: Configuration management
- `/internal/metrics`: Commands and utilities to retrieve metrics from Dash0
- `/internal/log`: Shared logging utilities

Organize code by domain, make interfaces for testability, and follow standard Go package layout.

## Documentation

Available commands are explained in @README.md . The description of what commands do is kept short and to the point. Providing a sample invocation as shell snippet, and when the output is longer than 4 lines, truncate it meaningfully to 4 lines or less. When modifying `dash0` in ways that affect the outpout displayed to users, always validate that the documentation about the commands is correct.

## Validation of changes

When modifying `dash0` in ways that affect the outpout displayed to users, always built the tool anew and validate the output.

## Integration Tests

Integration tests use a mock HTTP server (`internal/testutil/mockserver.go`) with JSON fixture files to simulate API responses from the Dash0 API.
The OpenAPI specification of the Dash0 API is available at `https://api-docs.dash0.com/reference`.

### Fixture Location
- Fixtures are stored in `internal/testutil/fixtures/`
- Organized by asset type: `dashboards/`, `checkrules/`, `views/`, `syntheticchecks/`
- Common fixture patterns: `list_success.json`, `list_empty.json`, `get_success.json`, `error_not_found.json`, `error_unauthorized.json`

### Generating Fixtures
- Fixture generation scripts are in `internal/testutil/fixtures/scripts/`
- Scripts follow the naming pattern: `<asset>_<functionality>.fixture.sh`
- Run all scripts: `DASH0_API_URL='https://api...' DASH0_AUTH_TOKEN='auth_...' ./generate_all.sh`
- Scripts output to stdout; redirect to create fixture files

### Writing Integration Tests
- Add `//go:build integration` at the top of integration test files (before `package`)
- Name files with `_integration_test.go` suffix for clarity
- Use `testutil.NewMockServer(t, testutil.FixturesDir())` to create a mock server
- Register routes with `server.On()` for exact paths or `server.OnPattern()` for regex patterns
- Always use `testutil.RequireAuthHeader` as the validator to ensure auth token validation
- Use constants for API paths and fixture filenames to avoid duplication
- Use `http.MethodGet`, `http.StatusOK`, etc. instead of string/numeric literals

### When to Update Fixtures
- When the Dash0 API response format changes
- When adding tests for new API endpoints
- When existing tests fail due to outdated fixture data
- Run `generate_all.sh` periodically to keep fixtures in sync with the actual API
