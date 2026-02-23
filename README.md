# Dash0 CLI

A command-line interface designed for humans, agentic AIs and CI/CD to interact with the [Dash0](https://www.dash0.com) observability platform.

## An ergonomic CLI for agentic AI

The `dash0` CLI's capabilities are discoverable via `--help`, alongside a comprehensive [command reference](docs/commands.md) with detailed flags, expected outputs, and ready-to-use workflow examples.
Authentication and connection settings can be configured entirely through profiles and environment variables, no need to juggle (and risk leaking) secrets in agentic context.
Commands use consistent naming conventions and flags.
Structured and parseable output formats (`--output json`, `--output yaml`, `--output csv`).
Interactive prompts can be skipped with `--force`.

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

For the full command reference with detailed flags, output examples, and AI-agent workflows, see [docs/commands.md](docs/commands.md).

### Configuration

Configure API access using profiles.
All profile fields are optional at creation time.
Missing values can be supplied later via `config profiles update` or overridden at runtime with [environment variables or CLI flags](#common-settings).

```bash
dash0 config profiles create dev \
    --api-url https://api.us-west-2.aws.dash0.com \
    --otlp-url https://ingress.us-west-2.aws.dash0.com \
    --auth-token auth_xxx

dash0 config profiles list
dash0 config profiles select prod
dash0 config show
```

You can find the API endpoint for your organization on the [Endpoints](https://app.dash0.com/settings/endpoints) page, under the `API` entry, and the OTLP HTTP endpoint under the `OTLP via HTTP` entry.
Currently only HTTP OTLP endpoints are supported.

### Applying assets

Apply asset definitions from a file, directory, or stdin.
The input may contain multiple YAML documents separated by `---`.
Supported asset types: `Dashboard`, `CheckRule`, `SyntheticCheck`, and `View`.

```bash
dash0 apply -f assets.yaml
dash0 apply -f dashboards/
cat assets.yaml | dash0 apply -f -
dash0 apply -f assets.yaml --dry-run
```

**Note:** In Dash0, dashboards, views, synthetic checks and check rules are called "assets", rather than the more common "resources".
The reason for this is that the word "resource" is overloaded in OpenTelemetry, where it describes "where telemetry comes from".

### Dashboards

```bash
dash0 dashboards list
dash0 dashboards get <id>
dash0 dashboards get <id> -o yaml
dash0 dashboards create -f dashboard.yaml
dash0 dashboards update <id> -f dashboard.yaml
dash0 dashboards delete <id> [--force]
```

### Check rules

```bash
dash0 check-rules list
dash0 check-rules get <id>
dash0 check-rules create -f rule.yaml
dash0 check-rules create -f prometheus-rules.yaml
dash0 check-rules update <id> -f rule.yaml
dash0 check-rules delete <id> [--force]
```

Both `apply` and `check-rules create` also accept PrometheusRule CRD files.

### Synthetic checks

```bash
dash0 synthetic-checks list
dash0 synthetic-checks get <id>
dash0 synthetic-checks create -f check.yaml
dash0 synthetic-checks update <id> -f check.yaml
dash0 synthetic-checks delete <id> [--force]
```

### Views

```bash
dash0 views list
dash0 views get <id>
dash0 views create -f view.yaml
dash0 views update <id> -f view.yaml
dash0 views delete <id> [--force]
```

### Logging

#### Sending logs to Dash0

> [!NOTE]
> The `dash0 logs send` command requires an OTLP URL configured in the active profile, or via the `--otlp-url` flag or the `DASH0_OTLP_URL` environment variable.

```bash
dash0 logs send "Application started" \
    --resource-attribute service.name=my-service \
    --log-attribute user.id=12345 \
    --severity-text INFO --severity-number 9
```

#### Querying logs from Dash0

> [!WARNING]
> This command is **experimental** and requires the `--experimental` (or `-X`) flag.
> The command syntax — especially the `--filter` format — may change in future releases.

> [!NOTE]
> The `dash0 logs query` command requires an API URL and auth token configured in the active profile, or via flags or environment variables.

```bash
dash0 -X logs query
dash0 -X logs query --from now-1h --to now --limit 100
dash0 -X logs query --filter "service.name is my-service"
dash0 -X logs query --filter "otel.log.severity.range is_one_of ERROR WARN"
dash0 -X logs query -o csv
dash0 -X logs query --column time --column service.name --column body
```

See the [filter syntax reference](docs/commands.md#filter-syntax) for the full list of operators.

### Tracing

#### Sending spans to Dash0

> [!WARNING]
> This command is **experimental** and requires the `--experimental` (or `-X`) flag.

> [!NOTE]
> The `dash0 spans send` command requires an OTLP URL configured in the active profile, or via the `--otlp-url` flag or the `DASH0_OTLP_URL` environment variable.

```bash
dash0 -X spans send --name "GET /api/users" \
    --kind SERVER --status-code OK --duration 100ms \
    --resource-attribute service.name=my-service
```

#### Querying spans from Dash0

> [!WARNING]
> This command is **experimental** and requires the `--experimental` (or `-X`) flag.

> [!NOTE]
> The `dash0 spans query` command requires an API URL and auth token configured in the active profile, or via flags or environment variables.

```bash
dash0 -X spans query
dash0 -X spans query --from now-1h --to now --limit 100
dash0 -X spans query --filter "service.name is my-service"
dash0 -X spans query --filter "otel.span.status.code is ERROR"
dash0 -X spans query -o csv
dash0 -X spans query --column otel.span.start_time --column otel.span.duration --column "span name" --column http.request.method
```

See the [filter syntax reference](docs/commands.md#filter-syntax) for the full list of operators.

#### Getting a trace from Dash0

> [!WARNING]
> This command is **experimental** and requires the `--experimental` (or `-X`) flag.

> [!NOTE]
> The `dash0 traces get` command requires an API URL and auth token configured in the active profile, or via flags or environment variables.

```bash
dash0 -X traces get <trace-id>
dash0 -X traces get <trace-id> --from now-2h
dash0 -X traces get <trace-id> --follow-span-links
dash0 -X traces get <trace-id> -o json
dash0 -X traces get <trace-id> --column otel.span.start_time --column otel.span.duration --column "span name" --column otel.span.status.code
```

### Metrics

```bash
dash0 metrics instant --query 'sum(rate(http_requests_total[5m]))'
```

### Common settings

| Flag | Short | Env Variable | Description |
|------|-------|--------------|-------------|
| `--api-url` | | `DASH0_API_URL` | Override API URL from profile. Find yours [here](https://app.dash0.com/goto/settings/endpoints?endpoint_type=api_http). |
| `--otlp-url` | | `DASH0_OTLP_URL` | Override OTLP URL from profile. Find yours [here](https://app.dash0.com/goto/settings/endpoints?endpoint_type=otlp_http). |
| `--auth-token` | | `DASH0_AUTH_TOKEN` | Override auth token from profile. Find yours [here](https://app.dash0.com/goto/settings/auth-tokens). |
| `--color` | | `DASH0_COLOR` | Color mode for output: `semantic` (default) or `none`. Ignored when piping output. |
| `--dataset` | | `DASH0_DATASET` | Override dataset from profile. Use the `identifier`, not `Name`. |
| `--experimental` | `-X` | | Enable experimental features (required for commands marked `[experimental]`) |
| `--file` | `-f` | | Input file path (use `-` for stdin) |
| `--output` | `-o` | | Output format: `table`, `wide`, `json`, `yaml`, `csv` |
| | | `DASH0_CONFIG_DIR` | Override the configuration directory (default: `~/.dash0`) |

### Output formats

The `list` and `get` commands for assets support multiple output formats via `-o`:

- **`table`** (default): Compact view with essential columns (name and ID)
- **`wide`**: Similar to `table`, with additional columns (dataset, origin, and URL)
- **`json`**: Full asset data in JSON format
- **`yaml`**: Full asset data in YAML format
- **`csv`**: Comma-separated values with the same columns as `wide`, suitable for piping and automation

The `logs query`, `spans query`, and `traces get` commands support a different set of formats via `-o`:

- **`table`** (default): Columnar output (`logs query` shows timestamp, severity, and body; `spans query` shows timestamp, duration, name, status, service, and trace ID; `traces get` shows a hierarchical span tree)
- **`json`**: Full OTLP/JSON payload
- **`csv`**: Comma-separated values

### Shell completions

Enable tab completion for your shell:

**Bash** (requires `bash-completion`):
```bash
source <(dash0 completion bash)
# Permanent (macOS): dash0 completion bash > $(brew --prefix)/etc/bash_completion.d/dash0
```

**Zsh**:
```bash
source <(dash0 completion zsh)
# Permanent (macOS): dash0 completion zsh > $(brew --prefix)/share/zsh/site-functions/_dash0
```

**Fish**:
```bash
dash0 completion fish | source
# Permanent: dash0 completion fish > ~/.config/fish/completions/dash0.fish
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development instructions.
