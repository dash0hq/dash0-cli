# Dash0 CLI

A command-line interface for interacting with Dash0 services.

## Installation

### Homebrew (macOS and Linux)

```bash
brew tap dash0hq/dash0-cli https://github.com/dash0hq/dash0-cli
brew install dash0hq/dash0-cli/dash0
```

### GitHub Releases

Download pre-built binaries for your platform from the [releases page](https://github.com/dash0hq/dash0-cli/releases).
Archives are available for Linux, macOS and Windows across multiple architectures.

### GitHub Actions

#### Setup Action

Use the `dash0` CLI in your CI/CD workflows with the [setup](.github/actions/setup/action.yaml) action:

```yaml
steps:
  - uses: actions/checkout@v4

  - name: Setup Dash0 CLI
    uses: dash0hq/dash0-cli/.github/actions/setup@main  # You can use any git ref: @main, @v1.1.0, or @commit-sha
    # with:
    #   version: '1.1.0' # 1.1.0 is the earliest supported version

  - name: List dashboards
    env:
      DASH0_API_URL: ... # Find this at https://app.dash0.com/goto/settings/endpoints?endpoint_type=api_http
      DASH0_OTLP_URL: ... # Find this https://app.dash0.com/goto/settings/endpoints?endpoint_type=otlp_http
      DASH0_AUTH_TOKEN: ... # Get one from https://app.dash0.com/goto/settings/auth-tokens?auth_token_id=39d58aa9-b64e-464c-a675-cc3923085d6c ; keep the auth token in a GitHub secret!
      DASH0_DATASET: my-dataset # Leave empty for the `default` dataset
    run: dash0 dashboards list
```

#### Send Log Event Action

The [`send-log-event`](.github/actions/send-log-event/action.yaml) action sends [log events](https://opentelemetry.io/docs/specs/otel/logs/data-model/#events) to Dash0 directly from your workflows.
It is standalone: it installs the Dash0 CLI automatically if it is not already on `PATH`.
If the [`setup`](#setup-action) action has already run in the same job, the existing installation is reused.

When used on its own, pass `otlp-url` and `auth-token` directly:

```yaml
steps:
  - name: Send deployment event
    uses: dash0hq/dash0-cli/.github/actions/send-log-event@main
    with:
      otlp-url: ${{ vars.DASH0_OTLP_URL }}
      auth-token: ${{ secrets.DASH0_AUTH_TOKEN }}
      event-name: dash0.deployment
      body: 'Deployment completed'
      severity-number: '9'
      service-name: my-service
      deployment-environment-name: production
      deployment-status: succeeded
```

When the `setup` action has already created a profile, the connection parameters are inherited and do not need to be repeated:

```yaml
steps:
  - name: Setup Dash0 CLI
    uses: dash0hq/dash0-cli/.github/actions/setup@main
    with:
      otlp-url: ${{ vars.DASH0_OTLP_URL }}
      auth-token: ${{ secrets.DASH0_AUTH_TOKEN }}

  - name: Send deployment event
    uses: dash0hq/dash0-cli/.github/actions/send-log-event@main
    with:
      event-name: dash0.deployment
      body: 'Deployment completed'
      severity-number: '9'
      service-name: my-service
      deployment-environment-name: production
      deployment-status: succeeded
```

### Docker

```bash
docker run ghcr.io/dash0hq/cli:latest [command]
```

Multi-architecture images (`linux/amd64`, `linux/arm64`) are published to GitHub Container Registry.

### From Source

Requires Go 1.22 or higher.

```bash
git clone https://github.com/dash0hq/dash0-cli.git
cd dash0-cli
make install
```

## Usage

### Configuration

Configure API access using profiles.

```bash
$ dash0 config profiles create dev \
    --api-url https://api.us-west-2.aws.dash0.com \
    --otlp-url https://ingress.us-west-2.aws.dash0.com \
    --auth-token auth_xxx
Profile "dev" added and set as active

$ dash0 config profiles create prod \
    --api-url https://api.eu-west-1.aws.dash0.com \
    --otlp-url https://ingress.eu-west-1.aws.dash0.com \
    --auth-token auth_yyy
Profile "prod" added successfully

$ dash0 config profiles list
  NAME  API URL                              OTLP URL                                    DATASET  AUTH TOKEN
* dev   https://api.us-west-2.aws.dash0.com  https://ingress.us-west-2.aws.dash0.com     default  ...ULSzVkM
  prod  https://api.eu-west-1.aws.dash0.com  https://ingress.eu-west-1.aws.dash0.com     default  ...uth_yyy

$ dash0 config profiles update prod --api-url https://api.us-east-1.aws.dash0.com
Profile 'prod' updated successfully

$ dash0 config profiles select prod
Profile 'prod' is now active

$ dash0 config show
Profile:    prod
API URL:    https://api.eu-west-1.aws.dash0.com
OTLP URL:   https://ingress.eu-west-1.aws.dash0.com
Dataset:    default
Auth Token: ...uth_yyy
```

The last seven digits of the auth token are displayed, matching the format shown in Dash0 as the `dash0.auth.token` attribute.

All profile fields are optional at creation time — you can provide any combination of `--api-url`, `--otlp-url`, `--auth-token`, and `--dataset`.
Missing values can be supplied later via `config profiles update` or overridden at runtime with [environment variables or CLI flags](#common-settings).
Currently only HTTP OTLP endpoints are supported.

```bash
$ DASH0_API_URL='http://test' dash0 config show
Profile:    dev
API URL:    http://test    (from DASH0_API_URL environment variable)
OTLP URL:   https://ingress.us-west-2.aws.dash0.com
Dataset:    default
Auth Token: ...ULSzVkM

$ DASH0_DATASET='production' dash0 config show
Profile:    dev
API URL:    https://api.us-west-2.aws.dash0.com
OTLP URL:   https://ingress.us-west-2.aws.dash0.com
Dataset:    production    (from DASH0_DATASET environment variable)
Auth Token: ...ULSzVkM
```

You can find the API endpoint for your organization on the [Endpoints](https://app.dash0.com/settings/endpoints) page, under the `API` entry, and the OTLP HTTP endpoint under the `OTLP via HTTP` entry.
(Only HTTP OTLP endpoints are supported.)

The API URL, OTLP URL, dataset and authentication token can be overridden using environment variables or CLI flags (see [Common Flags](#common-settings)).


### Applying Assets

Apply asset definitions from a file, directory, or stdin.
The input may contain multiple YAML documents separated by `---`:

```bash
$ dash0 apply -f assets.yaml
Dashboard "Production Overview" (a1b2c3d4-5678-90ab-cdef-1234567890ab) created
Check rule "High Error Rate" (b2c3d4e5-6789-01bc-def0-234567890abc) updated
View "Error Logs" (c3d4e5f6-7890-12cd-ef01-34567890abcd) created

$ dash0 apply -f dashboards/
dashboards/prod.yaml: Dashboard "Production Overview" (a1b2c3d4-...) created
dashboards/staging.yaml: Dashboard "Staging Overview" (d4e5f6a7-...) created
...

$ cat assets.yaml | dash0 apply -f -
Dashboard "Production Overview" (a1b2c3d4-...) created
...

$ dash0 apply -f assets.yaml --dry-run
Dry run: 1 document(s) validated successfully
  1. Dashboard "Production Overview" (a1b2c3d4-5678-90ab-cdef-1234567890ab)
```

When a directory is specified, all `.yaml` and `.yml` files are discovered recursively.
Hidden files and directories (starting with `.`) are skipped.
All documents are validated before any are applied.
If any discovered document fails validation, no document will be applied.

Supported asset types: `Dashboard`, `CheckRule` (both the plain Prometheus YAML and the PrometheusRule CRD), `PrometheusRule`, `SyntheticCheck`, `View`

**Note:** In Dash0, dashboards, views, synthetic checks and check rules are called "assets", rather than the more common "resources".
The reason for this is that the word "resource" is overloaded in OpenTelemetry, where it describes "where telemetry comes from".

### Dashboards

```bash
$ dash0 dashboards list
NAME                                      ID
Production Overview                       a1b2c3d4-5678-90ab-cdef-1234567890ab
...

$ dash0 dashboards get a1b2c3d4-5678-90ab-cdef-1234567890ab
Kind: Dashboard
Name: Production Overview
Dataset: default
Origin: gitops/prod
URL: https://app.dash0.com/goto/dashboards?dashboard_id=a1b2c3d4-5678-90ab-cdef-1234567890ab
Created: 2026-01-15 10:30:00
Updated: 2026-01-20 14:45:00

$ dash0 dashboards get a1b2c3d4-5678-90ab-cdef-1234567890ab -o yaml
kind: Dashboard
metadata:
  name: a1b2c3d4-5678-90ab-cdef-1234567890ab
  ...
spec:
  display:
    name: Production Overview
  ...

$ dash0 dashboards create -f dashboard.yaml
Dashboard "My Dashboard" created successfully

$ dash0 dashboards update a1b2c3d4-5678-90ab-cdef-1234567890ab -f dashboard.yaml
Dashboard "My Dashboard" updated successfully

$ dash0 dashboards delete a1b2c3d4-5678-90ab-cdef-1234567890ab
Are you sure you want to delete dashboard "a1b2c3d4-..."? [y/N]: y
Dashboard "a1b2c3d4-..." deleted successfully
```

### Check Rules

```bash
$ dash0 check-rules list
NAME                                      ID
High Error Rate Alert                     a1b2c3d4-5678-90ab-cdef-1234567890ab
...

$ dash0 check-rules get a1b2c3d4-5678-90ab-cdef-1234567890ab
Name: High Error Rate Alert
Dataset: default
Expression: sum(rate(errors[5m])) > 0.1
Enabled: true
Description: Alert when error rate exceeds threshold
URL: https://app.dash0.com/goto/alerting/check-rules?check_rule_id=a1b2c3d4-5678-90ab-cdef-1234567890ab

$ dash0 check-rules create -f rule.yaml
Check rule "High Error Rate Alert" created successfully

$ dash0 check-rules update a1b2c3d4-5678-90ab-cdef-1234567890ab -f rule.yaml
Check rule "High Error Rate Alert" updated successfully

$ dash0 check-rules delete a1b2c3d4-5678-90ab-cdef-1234567890ab --force
Check rule "a1b2c3d4-..." deleted successfully
```

Both `apply` and `check-rules create` accept `PrometheusRule` CRD files:

```bash
$ dash0 check-rules create -f prometheus-rules.yaml
Check rule "High Error Rate Alert" created successfully

$ dash0 apply -f prometheus-rules.yaml
Check rule "High Error Rate Alert" (b2c3d4e5-...) created
```

### Synthetic Checks

```bash
$ dash0 synthetic-checks list
NAME                                      ID
API Health Check                          a1b2c3d4-5678-90ab-cdef-1234567890ab
...

$ dash0 synthetic-checks get a1b2c3d4-5678-90ab-cdef-1234567890ab
Kind: Dash0SyntheticCheck
Name: API Health Check
Dataset: default
Origin:
Description: Checks API endpoint availability
URL: https://app.dash0.com/goto/alerting/synthetics?check_id=a1b2c3d4-5678-90ab-cdef-1234567890ab

$ dash0 synthetic-checks create -f check.yaml
Synthetic check "API Health Check" created successfully

$ dash0 synthetic-checks update a1b2c3d4-5678-90ab-cdef-1234567890ab -f check.yaml
Synthetic check "API Health Check" updated successfully

$ dash0 synthetic-checks delete a1b2c3d4-5678-90ab-cdef-1234567890ab
Are you sure you want to delete synthetic check "a1b2c3d4-..."? [y/N]: y
Synthetic check "a1b2c3d4-..." deleted successfully
```

### Views

```bash
$ dash0 views list
NAME                                      ID
Error Logs View                           a1b2c3d4-5678-90ab-cdef-1234567890ab
...

$ dash0 views get a1b2c3d4-5678-90ab-cdef-1234567890ab
Kind: Dash0View
Name: Error Logs View
Dataset: default
Origin:
URL: https://app.dash0.com/goto/logs?view_id=a1b2c3d4-5678-90ab-cdef-1234567890ab

$ dash0 views create -f view.yaml
View "Error Logs View" created successfully

$ dash0 views update a1b2c3d4-5678-90ab-cdef-1234567890ab -f view.yaml
View "Error Logs View" updated successfully

$ dash0 views delete a1b2c3d4-5678-90ab-cdef-1234567890ab --force
View "a1b2c3d4-..." deleted successfully
```

### Logs

#### Sending logs to Dash0

> [!NOTE]
> The `dash0 logs send` command requires an OTLP URL configured in the active profile, or via the `--otlp-url` flag or the `DASH0_OTLP_URL` environment variable.

Send log records to Dash0 via OTLP:

```bash
$ dash0 logs send "Application started" \
    --resource-attribute service.name=my-service \
    --log-attribute user.id=12345 \
    --severity-text INFO --severity-number 9
Log record sent successfully
```

#### Querying logs from Dash0

> [!WARNING]
> This command is **experimental** and requires the `--experimental` (or `-X`) flag.
> The command syntax — especially the `--filter` format — may change in future releases.

> [!NOTE] The `dash0 logs query` command requires an API URL and auth token configured in the active profile, or via flags or environment variables.

Query log records from Dash0:

```bash
$ dash0 --experimental logs query
TIMESTAMP                     SEVERITY    BODY
2026-02-16T09:12:03.456Z      INFO        Application started successfully
2026-02-16T09:12:04.789Z      ERROR       Connection timeout
2026-02-16T09:12:05.123Z      WARN        Rate limit approaching threshold
...
```

By default, `logs query` returns up to 50 records from the last 15 minutes (`--from now-15m --to now`).
Use `--from` and `--to` to specify a different time range, and `--limit` to change the maximum number of records.
Both `--from` and `--to` accept relative expressions like `now-1h` or absolute ISO 8601 timestamps.
Absolute timestamps are normalized to millisecond precision, so `2024-01-25T10:00:00Z` and `2024-01-25` are both accepted.

Filter results using the `--filter` flag:

```bash
$ dash0 --experimental logs query --filter "service.name is my-service" --filter "otel.log.severity.number gte 13"
TIMESTAMP                     SEVERITY    BODY
2026-02-16T09:12:04.789Z      ERROR       Connection timeout
...
```

The `--filter` flag accepts expressions in the form `key [operator] value`.
When the operator is omitted, `is` (exact match) is assumed.
The supported operators are the following:

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
| `is_one_of` | | Matches any of the given values |
| `is_not_one_of` | | Matches none of the given values |
| `is_any` | | Matches any value |

Keys containing spaces can be single-quoted, e.g., `'my key' is value`.
The `is_one_of` and `is_not_one_of` operators accept space-separated values.
Values containing spaces can be single-quoted:

```bash
$ dash0 --experimental logs query --filter "otel.log.severity.range is_one_of ERROR WARN"
$ dash0 --experimental logs query --filter "service.name is_not_one_of frontend gateway"
$ dash0 --experimental logs query --filter "deployment.environment.name is_one_of 'us east' 'eu west' staging"
```

Alternative output formats are available:

```bash
$ dash0 --experimental logs query -o otlp-json
{"resourceLogs": [...]}

$ dash0 --experimental logs query -o csv
timestamp,severity,body
2026-02-16T09:12:03.456Z,INFO,Application started successfully
...
```

Pipe through `tail -n +2` to skip the CSV header row.

```bash
$ dash0 --experimental logs query -o csv | tail -n +2
2026-02-16T09:12:03.456Z,INFO,Application started successfully
...
```

### Common settings

| Flag | Short | Env Variable | Description |
|------|-------|--------------|-------------|
| `--api-url` | | `DASH0_API_URL` | Override API URL from profile. Find yours [here](https://app.dash0.com/goto/settings/endpoints?endpoint_type=api_http'). |
| `--otlp-url` | | `DASH0_OTLP_URL` | Override OTLP URL from profile Find yours [here](https://app.dash0.com/goto/settings/endpoints?endpoint_type=otlp_http). |
| `--auth-token` | | `DASH0_AUTH_TOKEN` | Override auth token from profile. Find yours [here](https://app.dash0.com/goto/settings/auth-tokens). |
| `--color` | | `DASH0_COLOR` | Color mode for output: `semantic` (default) or `none`. When set to `semantic`, severity levels and error messages are color-coded in terminal output. |
| `--dataset` | | `DASH0_DATASET` | Override dataset from profile. Find the available datasets in your organization [here](https://app.dash0.com/goto/settings/datasets). Use the value of `identifier`, not `Name`. |
| `--experimental` | `-X` | | Enable experimental features (required for commands marked `[experimental]`) |
| `--file` | `-f` | | Input file path (use `-` for stdin) |
| `--output` | `-o` | | Output format: `table`, `wide`, `json`, `yaml` |
| | | `DASH0_CONFIG_DIR` | Override the configuration directory (default: `~/.dash0`) |

### Output Formats

The `list` commands support four output formats:

- **`table`** (default): Compact view with essential columns (name and ID)
- **`wide`**: Similar to `table`, with additional columns (dataset and origin)
- **`json`**: Full asset data in JSON format
- **`yaml`**: Full asset data in YAML format

```bash
$ dash0 dashboards list
NAME                                      ID
Production Overview                       a1b2c3d4-5678-90ab-cdef-1234567890ab

$ dash0 dashboards list -o wide
NAME                                      ID                                    DATASET          ORIGIN                          URL
Production Overview                       a1b2c3d4-5678-90ab-cdef-1234567890ab  default          gitops/prod                     https://app.dash0.com/goto/dashboards?dashboard_id=a1b2c3d4-...
```

The `wide` format includes the `DATASET` column even though `dash0` operates commands on a single dataset at a time.
This makes it easier to merge and compare outputs from commands run against different datasets.

### Shell Completions

Enable tab completion for your shell:

**Bash** (requires `bash-completion`):
```bash
# Current session
source <(dash0 completion bash)

# Permanent (Linux)
dash0 completion bash > /etc/bash_completion.d/dash0

# Permanent (macOS with Homebrew)
dash0 completion bash > $(brew --prefix)/etc/bash_completion.d/dash0
```

**Zsh**:
```bash
# Current session
source <(dash0 completion zsh)

# Permanent (Linux)
dash0 completion zsh > "${fpath[1]}/_dash0"

# Permanent (macOS with Homebrew)
dash0 completion zsh > $(brew --prefix)/share/zsh/site-functions/_dash0
```

**Fish**:
```bash
# Current session
dash0 completion fish | source

# Permanent
dash0 completion fish > ~/.config/fish/completions/dash0.fish
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development instructions.
