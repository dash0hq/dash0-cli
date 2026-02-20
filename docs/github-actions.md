# GitHub Actions

GitHub Actions created around the `dash0` CLI are located under `.github/actions`, each action with a separate folder named after the action itself, e.g., the `Setup` action resides in `.github/actions`.

## Setup Action

The repository ships a composite GitHub Action at `.github/actions/setup/` that installs and configures the Dash0 CLI in CI workflows.
Its documentation lives in `.github/actions/setup/README.md`.

The minimum supported CLI version is 1.1.0.
The action fails if a lower version is requested.

The action:
1. Resolves the CLI version (latest tag or user-specified) and enforces the minimum version.
2. Downloads the Linux binary (`amd64` or `arm64`) from GitHub Releases.
3. Caches the binary with `actions/cache` keyed on OS, arch, and version.
4. Adds `~/.dash0/bin` to `$GITHUB_PATH`.
5. Creates a `default` profile via `dash0 config profiles create` when any config input (`api-url`, `otlp-url`, `auth-token`, `dataset`) is provided.
   All profile inputs are optional.
6. Verifies the installation with `dash0 version` and `dash0 config show`.

## Send Log Event Action

The repository ships a composite GitHub Action at `.github/actions/send-log-event/` that sends a log event to Dash0.
Its documentation lives in `.github/actions/send-log-event/README.md`.

The action is standalone: it installs the Dash0 CLI automatically if not already on PATH, reusing the same install path and cache key as the setup action.
It accepts connection parameters (`otlp-url`, `auth-token`, `dataset`) and all parameters supported by `dash0 logs send`.
Recommended attributes from the [Dash0 deployment event spec](https://github.com/dash0hq/dash0-semantic-conventions) are exposed as first-level inputs (e.g., `service-name`, `deployment-status`).
Additional attributes can be passed via `other-resource-attributes` and `other-log-attributes` as `key=value` pairs, one per line.

## Keeping the actions in sync with CLI changes

When modifying the logic of `dash0 config`, ensure that the [setup](.github/actions/setup/action.yaml) GitHub Action is not affected negatively.
Ensure that the constraints of `dash0 config profiles create` are enforced in the input validation of the setup GitHub action.

When modifying the flags of `dash0 logs send`, ensure that the [send-log-event](.github/actions/send-log-event/action.yaml) GitHub Action inputs stay in sync.

## Testing the action

The workflow `.github/workflows/test-setup-action.yml` runs on **every push** (not just changes to the action) because CLI changes — especially to `dash0 config profiles create` — can break the action's profile-creation step.
The workflow can also be triggered manually via `workflow_dispatch`.

The profile-creation tests mirror the parameter combinations tested in `TestCreateProfileCmdPartialFields` in `internal/config/config_cmd_test.go`.
Each combination is a separate job that asserts the correct fields are set and the omitted fields show `(not set)` (or `default` for dataset).
When adding or removing flags from `dash0 config profiles create`, update both the unit test and the workflow.
