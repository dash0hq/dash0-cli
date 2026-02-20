# Command reference

This document is a comprehensive reference for the `dash0` CLI, aimed at enabling AI agents and automation workflows to use it effectively.

For every command, this reference lists the exact syntax, all flags, expected outputs, and concrete examples.

## Prerequisites

Every command that talks to the Dash0 API or OTLP endpoint needs credentials.
The CLI resolves configuration in this order (first match wins):

1. Environment variables (`DASH0_API_URL`, `DASH0_OTLP_URL`, `DASH0_AUTH_TOKEN`, `DASH0_DATASET`)
2. CLI flags (`--api-url`, `--otlp-url`, `--auth-token`, `--dataset`)
3. The active profile (stored in `~/.dash0/`)

Commands that read from the API (asset CRUD, `logs query`, `metrics instant`) require `api-url` and `auth-token`.
Commands that write via OTLP (`logs send`) require `otlp-url` and `auth-token`.

## Global flags

These flags are available on every command:

| Flag | Short | Env variable | Description |
|------|-------|--------------|-------------|
| `--api-url` | | `DASH0_API_URL` | API endpoint URL |
| `--otlp-url` | | `DASH0_OTLP_URL` | OTLP HTTP endpoint URL |
| `--auth-token` | | `DASH0_AUTH_TOKEN` | Authentication token |
| `--dataset` | | `DASH0_DATASET` | Dataset identifier (not display name) |
| `--color` | | `DASH0_COLOR` | `semantic` (default) or `none` |
| `--experimental` | `-X` | | Enable experimental commands |
| | | `DASH0_CONFIG_DIR` | Override config directory (default: `~/.dash0`) |

## Configuration

### `config profiles create`

Create a new named profile.
All fields are optional at creation time.
The first profile created becomes the active profile automatically.

```bash
dash0 config profiles create <name> \
    [--api-url <url>] \
    [--otlp-url <url>] \
    [--auth-token <token>] \
    [--dataset <dataset>]
```

Example:

```bash
$ dash0 config profiles create dev \
    --api-url https://api.us-west-2.aws.dash0.com \
    --otlp-url https://ingress.us-west-2.aws.dash0.com \
    --auth-token auth_xxx
Profile "dev" added and set as active
```

Aliases: `add`

### `config profiles update`

Update an existing profile.
Only the specified flags are changed; unspecified flags are left as-is.
Pass an empty string to remove a field.

```bash
dash0 config profiles update <name> \
    [--api-url <url>] \
    [--otlp-url <url>] \
    [--auth-token <token>] \
    [--dataset <dataset>]
```

Example:

```bash
$ dash0 config profiles update prod --api-url https://api.us-east-1.aws.dash0.com
Profile 'prod' updated successfully
```

### `config profiles list`

List all profiles.
The active profile is marked with `*`.

```bash
dash0 config profiles list [--skip-header]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--skip-header` | `false` | Omit the header row from output |

Example output:

```
  NAME  API URL                              OTLP URL                                    DATASET  AUTH TOKEN
* dev   https://api.us-west-2.aws.dash0.com  https://ingress.us-west-2.aws.dash0.com     default  ...ULSzVkM
  prod  https://api.eu-west-1.aws.dash0.com  https://ingress.eu-west-1.aws.dash0.com     default  ...uth_yyy
```

Use `--skip-header` to omit the header row:

```bash
$ dash0 config profiles list --skip-header
```

Aliases: `ls`

### `config profiles select`

Set the active profile.

```bash
dash0 config profiles select <name>
```

Example:

```bash
$ dash0 config profiles select prod
Profile 'prod' is now active
```

### `config profiles delete`

Delete a profile.

```bash
dash0 config profiles delete <name>
```

Aliases: `remove`

### `config show`

Display the resolved configuration (active profile + environment variable overrides).

```bash
dash0 config show
```

Example output:

```
Profile:    prod
API URL:    https://api.eu-west-1.aws.dash0.com
OTLP URL:   https://ingress.eu-west-1.aws.dash0.com
Dataset:    default
Auth Token: ...uth_yyy
```

When an environment variable overrides a profile value, the output indicates the source:

```bash
$ DASH0_API_URL='http://test' dash0 config show
Profile:    dev
API URL:    http://test    (from DASH0_API_URL environment variable)
OTLP URL:   https://ingress.us-west-2.aws.dash0.com
Dataset:    default
Auth Token: ...ULSzVkM
```

## Asset commands

Dash0 calls dashboards, views, synthetic checks, and check rules "assets" (not "resources", which is an overloaded term in OpenTelemetry).

All four asset types (`dashboards`, `check-rules`, `synthetic-checks`, `views`) share the same CRUD subcommands.
The examples below use `dashboards`, but the same patterns apply to every asset type.

### `list`

List all assets in the dataset.

```bash
dash0 dashboards list [--limit <n>] [-o <format>] [--skip-header]
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--limit` | `-l` | 50 | Maximum number of results |
| `--output` | `-o` | `table` | `table`, `wide`, `json`, or `yaml` |
| `--skip-header` | | `false` | Omit the header row from `table` and `wide` output |

Example:

```bash
$ dash0 dashboards list
NAME                                      ID
Production Overview                       a1b2c3d4-5678-90ab-cdef-1234567890ab
Staging Overview                          d4e5f6a7-8901-23de-f012-4567890abcde
...
```

The `wide` format adds `DATASET`, `ORIGIN`, and `URL` columns:

```bash
$ dash0 dashboards list -o wide
NAME                  ID                                    DATASET   ORIGIN       URL
Production Overview   a1b2c3d4-5678-90ab-cdef-1234567890ab  default   gitops/prod  https://app.dash0.com/goto/dashboards?dashboard_id=a1b2c3d4-...
```

Use `-o json` or `-o yaml` to get the full asset payload, suitable for piping or saving to a file.
Aliases: `ls`

### `get`

Retrieve a single asset by ID.

```bash
dash0 dashboards get <id> [-o <format>]
```

Example (default table output):

```bash
$ dash0 dashboards get a1b2c3d4-5678-90ab-cdef-1234567890ab
Kind: Dashboard
Name: Production Overview
Dataset: default
Origin: gitops/prod
URL: https://app.dash0.com/goto/dashboards?dashboard_id=a1b2c3d4-5678-90ab-cdef-1234567890ab
Created: 2026-01-15 10:30:00
Updated: 2026-01-20 14:45:00
```

Use `-o yaml` to get the full definition, suitable for editing and re-applying:

```bash
$ dash0 dashboards get a1b2c3d4-5678-90ab-cdef-1234567890ab -o yaml
kind: Dashboard
metadata:
  name: a1b2c3d4-5678-90ab-cdef-1234567890ab
  ...
spec:
  display:
    name: Production Overview
  ...
```

### `create`

Create a new asset from a YAML or JSON file.

```bash
dash0 dashboards create -f <file> [--dry-run] [-o <format>]
```

Use `-f -` to read from stdin.
The `--dry-run` flag validates the file without creating anything.

Example:

```bash
$ dash0 dashboards create -f dashboard.yaml
Dashboard "My Dashboard" created successfully
```

`check-rules create` also accepts PrometheusRule CRD files.
Each alerting rule in the CRD is created as a separate check rule (recording rules are skipped):

```bash
$ dash0 check-rules create -f prometheus-rules.yaml
Check rule "High Error Rate Alert" created successfully
```

Aliases: `add`

### `update`

Update an existing asset from a YAML or JSON file.

```bash
dash0 dashboards update <id> -f <file> [--dry-run] [-o <format>]
```

Example:

```bash
$ dash0 dashboards update a1b2c3d4-5678-90ab-cdef-1234567890ab -f dashboard.yaml
Dashboard "My Dashboard" updated successfully
```

### `delete`

Delete an asset by ID.
Prompts for confirmation unless `--force` is passed.

```bash
dash0 dashboards delete <id> [--force]
```

Examples:

```bash
$ dash0 dashboards delete a1b2c3d4-5678-90ab-cdef-1234567890ab
Are you sure you want to delete dashboard "a1b2c3d4-..."? [y/N]: y
Dashboard "a1b2c3d4-..." deleted successfully

$ dash0 dashboards delete a1b2c3d4-5678-90ab-cdef-1234567890ab --force
Dashboard "a1b2c3d4-..." deleted successfully
```

Aliases: `remove`

### Asset type quick reference

| Asset type | Command | Notes |
|------------|---------|-------|
| Dashboards | `dash0 dashboards <subcommand>` | |
| Check rules | `dash0 check-rules <subcommand>` | `create` also accepts PrometheusRule CRD files |
| Synthetic checks | `dash0 synthetic-checks <subcommand>` | |
| Views | `dash0 views <subcommand>` | |

## `apply`

Apply asset definitions from a file, directory, or stdin.
If an asset already exists (matched by ID), it is updated; otherwise it is created.

```bash
dash0 apply -f <file|directory> [--dry-run]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--file` | `-f` | Path to a YAML/JSON file, a directory, or `-` for stdin |
| `--dry-run` | | Validate without applying |

When a directory is specified, all `.yaml` and `.yml` files are discovered recursively.
Hidden files and directories (starting with `.`) are skipped.
All documents are validated before any are applied.
If any document fails validation, no changes are made.

Supported `kind` values: `Dashboard`, `CheckRule`, `PrometheusRule`, `SyntheticCheck`, `View`.
A single file may contain multiple documents separated by `---`.

Examples:

```bash
# Apply a single file
$ dash0 apply -f dashboard.yaml
Dashboard "Production Overview" (a1b2c3d4-...) created

# Apply a directory recursively
$ dash0 apply -f assets/
assets/dashboard.yaml: Dashboard "Production Overview" (a1b2c3d4-...) created
assets/rule.yaml: Check rule "High Error Rate" (b2c3d4e5-...) updated
...

# Apply from stdin
$ cat assets.yaml | dash0 apply -f -
Dashboard "Production Overview" (a1b2c3d4-...) created
...

# Dry-run validation
$ dash0 apply -f assets.yaml --dry-run
Dry run: 1 document(s) validated successfully
  1. Dashboard "Production Overview" (a1b2c3d4-5678-90ab-cdef-1234567890ab)
```

### Asset YAML formats

Dashboard:

```yaml
kind: Dashboard
metadata:
  name: a1b2c3d4-5678-90ab-cdef-1234567890ab
spec:
  display:
    name: Production Overview
```

Check rule:

```yaml
kind: CheckRule
id: b2c3d4e5-6789-01bc-def0-234567890abc
name: High Error Rate
expression: sum(rate(http_requests_total{status=~"5.."}[5m])) > 0.1
```

View:

```yaml
kind: View
metadata:
  name: Error Logs
  labels:
    dash0.com/id: c3d4e5f6-7890-12cd-ef01-34567890abcd
spec:
  query: "severity >= ERROR"
```

Synthetic check:

```yaml
kind: SyntheticCheck
metadata:
  name: API Health Check
  labels:
    dash0.com/id: d4e5f6a7-8901-23de-f012-4567890abcde
spec:
  url: https://api.example.com/health
  interval: 60s
```

Multi-document file (separated by `---`):

```yaml
kind: Dashboard
metadata:
  name: e1f2a3b4-5678-90e5-6789-abcdef012345
spec:
  display:
    name: First Dashboard
---
kind: CheckRule
id: f2a3b4c5-6789-01f6-7890-bcdef0123456
name: Second Document Rule
expression: up == 0
```

## Logs

### `logs send`

Send a log record to Dash0 via OTLP.
Requires `otlp-url` and `auth-token`.

```bash
dash0 logs send <body> [flags]
```

Key flags:

| Flag | Description |
|------|-------------|
| `--severity-number <1-24>` | OpenTelemetry severity number; determines the severity range in Dash0 |
| `--severity-text <text>` | Severity text (e.g., `INFO`, `WARN`, `ERROR`); separate from severity-number |
| `--event-name <name>` | Event name (e.g., `dash0.deployment`) |
| `--resource-attribute <key=value>` | Resource attribute (repeatable) |
| `--log-attribute <key=value>` | Log record attribute (repeatable) |
| `--scope-attribute <key=value>` | Instrumentation scope attribute (repeatable) |
| `--scope-name <name>` | Instrumentation scope name (default: `dash0-cli`) |
| `--scope-version <version>` | Instrumentation scope version (default: CLI version) |
| `--time <RFC3339>` | Log record timestamp (default: now) |
| `--observed-time <RFC3339>` | Observed timestamp (default: now) |
| `--trace-id <32 hex chars>` | Trace ID to correlate with |
| `--span-id <16 hex chars>` | Span ID to correlate with |
| `--flags <uint32>` | Log record flags |
| `--resource-dropped-attributes-count <n>` | Number of dropped resource attributes |
| `--log-dropped-attributes-count <n>` | Number of dropped log record attributes |
| `--scope-dropped-attributes-count <n>` | Number of dropped scope attributes |

Examples:

```bash
# Simple log message
$ dash0 logs send "Application started"
Log record sent successfully

# Log with severity and attributes
$ dash0 logs send "Application started" \
    --resource-attribute service.name=my-service \
    --log-attribute user.id=12345 \
    --severity-text INFO --severity-number 9
Log record sent successfully

# Deployment event with event name
$ dash0 logs send "Deployment completed" \
    --event-name dash0.deployment \
    --severity-number 9 \
    --resource-attribute service.name=my-service \
    --resource-attribute deployment.environment.name=production \
    --log-attribute deployment.status=succeeded
Log record sent successfully

# Using environment variables for connection
$ DASH0_OTLP_URL=https://ingress.us-west-2.aws.dash0.com \
  DASH0_AUTH_TOKEN=auth_xxx \
  dash0 logs send "Health check passed" \
    --severity-number 9 --severity-text INFO
Log record sent successfully
```

### `logs query` (experimental)

Query log records from Dash0.
Requires the `-X` (or `--experimental`) flag, plus `api-url` and `auth-token`.

```bash
dash0 -X logs query [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--from` | `now-15m` | Start of time range |
| `--to` | `now` | End of time range |
| `--limit` | 50 | Maximum number of records |
| `--filter` | | Filter expression (repeatable) |
| `-o` | `table` | Output format: `table`, `otlp-json`, or `csv` |
| `--skip-header` | `false` | Omit the header row from `table` and `csv` output |

Both `--from` and `--to` accept relative expressions like `now-1h` or absolute ISO 8601 timestamps.
Absolute timestamps are normalized to millisecond precision, so `2024-01-25T10:00:00Z` and `2024-01-25` are both accepted.

Examples:

```bash
# Query recent logs (last 15 minutes, up to 50 records)
$ dash0 -X logs query
TIMESTAMP                     SEVERITY    BODY
2026-02-16T09:12:03.456Z      INFO        Application started successfully
2026-02-16T09:12:04.789Z      ERROR       Connection timeout
...

# Query with time range
$ dash0 -X logs query --from now-1h --to now --limit 100

# Filter by service
$ dash0 -X logs query --filter "service.name is my-service"

# Filter by severity (errors and above)
$ dash0 -X logs query --filter "otel.log.severity.number gte 17"

# Multiple filters (AND logic)
$ dash0 -X logs query \
    --filter "service.name is my-service" \
    --filter "otel.log.severity.range is_one_of ERROR WARN"

# Output as JSON (full OTLP payload)
$ dash0 -X logs query -o otlp-json

# Output as CSV (pipe-friendly)
$ dash0 -X logs query -o csv
timestamp,severity,body
2026-02-16T09:12:03.456Z,INFO,Application started successfully
...

# CSV without header
$ dash0 -X logs query -o csv --skip-header
```

### Filter syntax

The `--filter` flag accepts expressions in the form `key [operator] value`.
When the operator is omitted, `is` (exact match) is assumed.

| Operator | Alias | Description |
|----------|-------|-------------|
| `is` | `=` | Exact match (default when operator is omitted) |
| `is_not` | `!=` | Not equal |
| `contains` | | Value contains substring |
| `does_not_contain` | | Value does not contain substring |
| `starts_with` | | Value starts with prefix |
| `does_not_start_with` | | Value does not start with prefix |
| `ends_with` | | Value ends with suffix |
| `does_not_end_with` | | Value does not end with suffix |
| `matches` | `~` | Regular expression match |
| `does_not_match` | `!~` | Negated regular expression match |
| `gt` | `>` | Greater than |
| `gte` | `>=` | Greater than or equal |
| `lt` | `<` | Less than |
| `lte` | `<=` | Less than or equal |
| `is_set` | | Attribute is present |
| `is_not_set` | | Attribute is absent |
| `is_one_of` | | Matches any of the given values (space-separated) |
| `is_not_one_of` | | Matches none of the given values (space-separated) |
| `is_any` | | Matches any value |

Keys containing spaces can be single-quoted: `'my key' is value`.
Values containing spaces can be single-quoted: `deployment.environment.name is_one_of 'us east' 'eu west' staging`.

Common log attribute keys: `service.name`, `otel.log.severity.number`, `otel.log.severity.range`, `otel.log.severity.text`, `otel.log.body`.
Valid values for `otel.log.severity.range`: `TRACE`, `DEBUG`, `INFO`, `WARN`, `ERROR`, `FATAL`, `UNKNOWN`.

## Metrics

### `metrics instant`

Run an instant PromQL query against the Dash0 API.

```bash
dash0 metrics instant --query <promql> [--time <timestamp>] [--dataset <dataset>]
```

| Flag | Description |
|------|-------------|
| `--query` | PromQL query expression (required) |
| `--time` | Evaluation timestamp (default: now); supports relative expressions |
| `--dataset` | Dataset to query |

Example:

```bash
$ dash0 metrics instant --query 'sum(rate(http_requests_total[5m]))'
```

## Common workflows for AI agents

### Set up credentials from environment variables

When environment variables are already set, no profile is needed:

```bash
export DASH0_API_URL=https://api.us-west-2.aws.dash0.com
export DASH0_OTLP_URL=https://ingress.us-west-2.aws.dash0.com
export DASH0_AUTH_TOKEN=auth_xxx
export DASH0_DATASET=default

# All commands now work without --api-url/--auth-token flags
dash0 dashboards list
dash0 -X logs query --from now-1h
```

### Export an asset, modify it, and re-apply

```bash
# Export to YAML
dash0 dashboards get a1b2c3d4-... -o yaml > dashboard.yaml

# Edit the file, then update
dash0 dashboards update a1b2c3d4-... -f dashboard.yaml

# Or use apply (auto-detects create vs update)
dash0 apply -f dashboard.yaml
```

### Bulk export all assets of one type

```bash
dash0 dashboards list -o yaml > all-dashboards.yaml
```

### Send a deployment event

```bash
dash0 logs send "Deployment v2.3.0 completed" \
    --event-name dash0.deployment \
    --severity-number 9 \
    --resource-attribute service.name=my-service \
    --resource-attribute deployment.environment.name=production \
    --log-attribute deployment.status=succeeded
```

### Query errors in the last hour

```bash
dash0 -X logs query \
    --from now-1h \
    --filter "otel.log.severity.range is_one_of ERROR WARN" \
    --limit 200
```

### Non-interactive deletion (for automation)

Always pass `--force` to skip the confirmation prompt:

```bash
dash0 dashboards delete a1b2c3d4-... --force
```

### Validate assets before applying

Use `--dry-run` to check for errors without making changes:

```bash
dash0 apply -f assets/ --dry-run
```
