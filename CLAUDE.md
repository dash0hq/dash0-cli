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
| `update`   | -        | Update an existing profile |
| `delete`   | `remove` | Delete a profile |
| `select`   | -        | Set the active profile |

### Aliases
- `add` → `create`
- `remove` → `delete`
- `ls` → `list`

### Asset Kind Display Names
In user-facing output (success messages, dry-run listings, error messages), use human-readable names for asset kinds — **not** PascalCase identifiers:

| Kind identifier   | Display name       |
|-------------------|--------------------|
| `Dashboard`       | Dashboard          |
| `CheckRule`       | Check rule         |
| `SyntheticCheck`  | Synthetic check    |
| `View`            | View               |
| `PrometheusRule`  | PrometheusRule     |

For example: `Check rule "High Error Rate" created successfully`, not `CheckRule "High Error Rate" created successfully`.

### Naming Rules
1. **Prefer verbs** for actions: `create`, `delete`, `list`, `get`, `update`, `select`
2. **Use plural** for asset type commands: `dashboards` not `dashboard`
3. **Use kebab-case** for multi-word commands: `check-rules` not `checkRules`
4. **Provide aliases** when renaming commands for backwards compatibility
5. **Be consistent** across all asset types - if `dashboards` has `create`, all assets should have `create`

### Parity Between `apply` and CRUD Commands
The `apply` command and the individual CRUD subcommands (e.g., `check-rules create`, `dashboards create`) must have the same expressiveness. Any asset format accepted by `apply` must also be accepted by the corresponding `create` command, and vice versa. For example, `dash0 apply -f prometheusrule.yaml` and `dash0 check-rules create -f prometheusrule.yaml` both accept PrometheusRule CRD files. Shared parsing and import logic lives in `internal/asset/` so that both code paths stay in sync.

## Dependencies
- Never add dependencies with licenses incompatible with Apache 2.0 (the project's license). Acceptable licenses: Apache 2.0, MIT, BSD-2-Clause, BSD-3-Clause, ISC, MPL-2.0. Reject GPL, LGPL, AGPL, SSPL, and other copyleft licenses. Always check the license before adding a dependency.

## Code Style
- Use Go 1.24+ features
- Format with `gofmt`
- Add unit tests for all new functionality
- Use zerolog for structured logging
- Error handling: wrap errors with descriptive messages using `fmt.Errorf("... %w", err)`. Never use lazy pluralization like `error(s)` or `file(s)` — use proper singular/plural forms based on the actual count (e.g., "1 error", "3 errors"). Invest the extra line of code to give users clear, natural-sounding messages. When an error wraps a nested cause, put the cause on a new line indented by 2 spaces relative to its parent, so the hierarchy is visually clear:
  ```
  Error: validation failed with 1 error:
    malformed.yaml: failed to parse YAML:
      yaml: line 6: could not find expected ':'
  ```
- Naming: use camelCase for variable names and PascalCase for exported functions/types
- Never introduce test-specific behavior (env var checks, test flags, etc.) in production code. Tests must exercise the real code paths. Use proper configuration (profiles via `DASH0_CONFIG_DIR`, environment variables, or CLI flags) to set up the state tests need.

## Project Structure
- `/cmd/dash0`: Main entrypoint
- `/internal/asset`: Shared asset logic (types, import functions, display helpers) used by both `apply` and the per-asset CRUD commands
- `/internal/apply`: The `apply` command — orchestration only, delegates asset-specific logic to `internal/asset`
- `/internal/dashboards`, `/internal/checkrules`, `/internal/views`, `/internal/syntheticchecks`: Per-asset CRUD commands — delegate asset-specific logic to `internal/asset`
- `/internal/config`: Configuration management
- `/internal/client`: API client factory and error handling
- `/internal/metrics`: Commands and utilities to retrieve metrics from Dash0
- `/internal/log`: Shared logging utilities

Logic that is shared between `apply` and CRUD commands (import with existence check, PrometheusRule conversion, kind display names, file I/O) must live in `internal/asset/`, not be duplicated across packages. The per-asset packages and `apply` import from `internal/asset`, never from each other.

Organize code by domain, make interfaces for testability, and follow standard Go package layout.

## Documentation

Available commands are explained in @README.md . The description of what commands do is kept short and to the point. Providing a sample invocation as shell snippet, and when the output is longer than 4 lines, truncate it meaningfully to 4 lines or less. When modifying `dash0` in ways that affect the outpout displayed to users, always validate that the documentation about the commands is correct.

### Environment Variables Reference Table
The README contains an "Environment Variables" table listing all supported env vars. When adding a new environment variable (e.g., for a new config field), add a row to this table. Keep the table sorted alphabetically by variable name.

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

## GitHub Actions

GitHub Actions created around the `dash0` CLI are located under `.github/actions`, each action with a separate folder named after the action itself, e.g., the `Setup` action resides in `.github/actions`.

### Setup Action

The repository ships a composite GitHub Action at `.github/actions/setup/` that installs and configures the Dash0 CLI in CI workflows.
Its documentation lives in `.github/actions/setup/README.md`.

The action:
1. Resolves the CLI version (latest tag or user-specified).
2. Downloads the Linux binary (`amd64` or `arm64`) from GitHub Releases.
3. Caches the binary with `actions/cache` keyed on OS, arch, and version.
4. Adds `~/.dash0/bin` to `$GITHUB_PATH`.
5. Creates a `default` profile via `dash0 config profiles create` when any config input (`api-url`, `otlp-url`, `auth-token`, `dataset`) is provided.
6. Verifies the installation with `dash0 version` and `dash0 config show`.

### Keeping the action in sync with CLI changes

When modifying the logic of `dash0 config`, ensure that the [setup](.github/actions/setup/action.yaml) GitHub Action is not affected negatively.
Ensure that the constraints of `dash0 config profiles create` are enforced in the input validation of the setup GitHub action.

### Testing the action

The workflow `.github/workflows/test-setup-action.yml` runs on **every push** (not just changes to the action) because CLI changes — especially to `dash0 config profiles create` — can break the action's profile-creation step.
The workflow can also be triggered manually via `workflow_dispatch`.

The profile-creation tests mirror the parameter combinations tested in `TestCreateProfileCmdPartialFields` in `internal/config/config_cmd_test.go`.
Each combination is a separate job that asserts the correct fields are set and the omitted fields show `(not set)` (or `default` for dataset).
When adding or removing flags from `dash0 config profiles create`, update both the unit test and the workflow.

## README.md

Follow these rules when writing or editing prose in this project.

### Line and Paragraph Structure
- **One sentence per line** (semantic line breaks). Each sentence starts on its own line; do not wrap mid-sentence.
- Separate paragraphs with a single blank line.
- Keep paragraphs between 2 and 5 lines (sentences).

### Section headers
Seaction headers should be written in sentence case, e.g., "This is an example".

### Links
- Use inline Markdown links: `[visible text](url)`.
- Link the most specific relevant term, not generic phrases like "click here" or "this page."

### Code Blocks
- Fence with triple backticks and a language identifier (e.g., ` ```yaml `).
- Use code blocks to provide illustrative examples.

### Punctuation and Typography
- End sentences with full stops.
- Use the **Oxford comma** (e.g., "error status, latency thresholds, rate limits, and so on").
- Use curly/typographic quotes in prose (`"..."`, `'...'`); straight quotes are fine inside code blocks.
- Write numbers as digits and spell out "percent" (e.g., "10 percent", not "10%" or "ten percent").

## Changelog

This project uses [chloggen](https://github.com/open-telemetry/opentelemetry-go-build-tools/tree/main/chloggen) to manage changelog entries. Each user-facing change gets its own YAML file in `.chloggen/`.

### Creating a Changelog Entry
1. Run `make chlog-new` — this creates `.chloggen/<branch-name>.yaml` from the template
2. Fill in the fields:
   - `change_type`: one of `breaking`, `deprecation`, `new_component`, `enhancement`, `bug_fix`
   - `component`: area of concern (e.g., `dashboards`, `config`, `apply`, `logs`)
   - `note`: brief description of the change; wrap in quotes if it starts with a backtick
   - `issues`: list of related issue or PR numbers, e.g., `[28]`
   - `subtext`: (optional) additional detail, use `|` for multiline
3. Run `make chlog-validate` to verify the entry is well-formed
4. Run `make chlog-preview` to see how it will render in `CHANGELOG.md`
5. Commit the `.chloggen/<branch-name>.yaml` file with the rest of the changes

### When to Skip
If a change doesn't affect end users (refactoring, CI changes, etc.), prefix the PR title with `chore` or add the "Skip Changelog" label instead of creating an entry.

### Reference
- Template: `.chloggen/TEMPLATE.yaml`
- Config: `.chloggen/config.yaml`
- Render template: `.chloggen/summary.tmpl`
- See existing entries (e.g., `.chloggen/logs_create.yaml`) for examples
