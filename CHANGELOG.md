# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

<!-- next version -->

## 1.5.0


### New Components


- `teams`: Add experimental team and member management commands (#47)
  New `teams` commands: list, get, create, update, delete, add-members, remove-members.
  New `members` commands: list, invite, remove.
  All commands require the `--experimental` (`-X`) flag.
  


### Enhancements


- `assets`: Add CSV output format to asset list commands (#62)
  The `dashboards list`, `check-rules list`, `views list`, and `synthetic-checks list` commands now accept `-o csv`.
  CSV output includes all columns from the `wide` format (name, id, dataset, origin, url).
  

- `query`: `--column` flag for `logs query`, `spans query`, and `traces get` to customize displayed columns (#56)
  Users can now select which columns appear in table and CSV output using repeatable `--column` flags.
  Predefined columns have short aliases (e.g., `time`, `severity`, `body` for logs).
  Any OTLP attribute key can be used as a column, enabling ad-hoc display of arbitrary attributes.
  

## 1.4.0


### New Components


- `spans`: Add `spans query` command to query spans from Dash0 (#51)
  Supports table, CSV, and OTLP JSON output formats with filtering, time range selection, and pagination.
  

- `spans`: Add `spans send` command to send spans to Dash0 via OTLP (#51)
  Supports span kind, status, duration, trace/span ID (auto-generated or explicit), parent span, span links, resource/span/scope attributes, and custom instrumentation scope.
  

- `traces`: Add `traces get` command to retrieve all spans in a trace from Dash0 (#51)
  Displays spans in table, JSON (OTLP/JSON), or CSV format.
  Table output shows timestamp, duration, trace ID, span ID, parent ID, span name, status, service name, and span links.
  Supports `--follow-span-links` to recursively fetch traces linked through span links.
  


### Enhancements


- `logs`: Color-code severity levels in `logs query` table output (#46)
  Severity levels (FATAL, ERROR, WARN, DEBUG, TRACE) are now color-coded when output is a terminal.
  A new global `--color` flag (env: `DASH0_COLOR`) controls color output: `semantic` (default) or `none`.
  

- `output`: Add `--skip-header` flag to suppress the header row in tabular output formats (#50)
  Available on all asset `list` commands (table and wide formats), `config profiles list` and `logs query` (table and CSV formats).
  

## 1.3.0


### Enhancements


- `logs`: Add `dash0 logs query` command to query log records from Dash0 (#41)
  The command syntax — especially the `--filter` format — is experimental and may change in future releases.

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
