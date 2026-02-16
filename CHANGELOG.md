# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

<!-- next version -->

## 1.2.0


### New Components


- `github-actions`: `send-log-event` GitHub Action to send log events to Dash0 directly from your GitHub workflows. (#40)
  The action is standalone and installs the Dash0 CLI automatically.
  If the `setup` action has already run, the existing installation is reused.
  


### Enhancements


- `assets`: Add deeplink URLs to `get` and `list -o wide` output for all asset types (#36)
  The `get` command now shows a `URL` field linking directly to the asset in the Dash0 web UI.
  The `list -o wide` command includes a `URL` column with deeplink URLs for each asset.
  

## 1.1.0


### Breaking Changes


- `logging`: The `zerolog` logging library has been removed in favor of the standard `log` package. (#30)
  This affects the logging output format and may require updates to any log parsing tools or
  scripts that were previously used with `zerolog`'s output.
  (But let's be real here: there are no known users of the CLI's logging output, so this is
  effectively a non-breaking change.)
  


### New Components


- `github-actions`: Add `setup` GitHub Action to install the Dash0 CLI in CI/CD workflows (#37)
  The `dash0hq/dash0-cli/.github/actions/setup` action installs and caches the Dash0 CLI binary.
  It supports optional profile configuration via inputs (`api-url`, `otlp-url`, `auth-token`, `dataset`)
  and runs on Linux x64 and arm64 runners.
  


### Enhancements


- `apply`: `dash0 apply -f` now accepts directories, recursively discovering and applying all `.yaml` and `.yml` files. (#28)
  Hidden files and directories (starting with `.`) are automatically skipped.
  All documents across all files are validated before any are applied; if any document fails validation, no changes are made.
  The apply output now includes asset IDs alongside names, and when applying from a directory, each line is prefixed with the relative file path.
  Dry-run output groups documents by file and shows file and document counts.
  

- `check-rules`: `dash0 check-rules create` now accepts PrometheusRule CRD files in addition to plain CheckRule definitions. (#29)
  When a PrometheusRule CRD file is provided, each alerting rule in the CRD is created as a separate check rule.
  Recording rules are skipped.
  

- `config`: Allow creating profiles without requiring all fields upfront (#37)
  `dash0 config profiles create` no longer requires `--auth-token` and at least one of `--api-url` or `--otlp-url`.
  All profile fields are now optional at creation time; missing values can be supplied later via `config profiles update`
  or overridden at runtime with environment variables or CLI flags.
  

- `config`: Add dataset to configuration profiles, with `DASH0_DATASET` environment variable support (#22)
  Profiles now support a `--dataset` flag in `config profiles create` and `config profiles update`.
  The dataset is shown in `config show` and `config profiles list`.
  When no dataset is configured, `default` is displayed.
  The `DASH0_DATASET` environment variable can override the profile's dataset.
  

- `logs`: Added `dash0 logs create` command to create log records from the CLI, with support for setting all the attributes and fields of log records. (#3)
  This is the first step in a larger effort to add support for logs in the `dash0` CLI.
  The `dash0 logs create` command allows users to create log records from the CLI, which can be useful for integration in CI/CD and other automation workflows.
  The `scope` name and version are hard-coded to `dash0-cli` and the version of the `dash0` binary.
  


### Bug Fixes


- `check-rules`: Fix check rule re-import failing with 400 Bad Request (#34)
  Exported check rule YAML (from `check-rules get -o yaml`) could not be re-imported
  via `check-rules create` or `apply` because the server-managed `dataset` field was not
  stripped before sending the request to the API.
  

- `dashboards`: Fix dashboard re-import failing with 400 Bad Request (#33)
  Exported dashboard YAML (from `dashboards get -o yaml`) could not be re-imported
  via `dashboards create` or `apply` because the server-managed `metadata.dash0Extensions.dataset`
  field was not stripped before sending the request to the API.
  

## 1.0.1


### Bug Fixes


- `config`: Fix a bug that would prevent the creation of new profiles in the config file when no profiles existed yet. (#19)
  - The bug was caused by a check that would prevent any command with "config" as an ancestor from running, which would include "config profiles create" when no profiles existed yet.

## 1.0.0


### New Components


- `cli`: Expanding the scope of the Dash0 CLI to managing assets (#2)
  Provides commands for managing Dash0 assets including dashboards, check rules,
  synthetic checks, and views. Supports multiple configuration profiles and various
  output formats.
  


### Enhancements


- `config`: Improved error messages with colored output (#)
  Error messages now display "Error:" in red and "Hint:" in cyan for better visibility.
  The error message for invalid profile JSON now includes the actual file path instead
  of a hardcoded value, making it easier to identify and fix configuration issues.
