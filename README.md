# Dash0 CLI

A command-line interface designed for humans, agentic AIs and CI/CD to interact with the [Dash0](https://www.dash0.com) observability platform.
Humans authenticate interactively via OAuth 2.0 with `dash0 login`; CI/CD and agent workflows use static auth tokens — see the [quick start](#quick-start).

## An ergonomic CLI for agentic AI

The `dash0` CLI is designed to be driven by AI coding agents as naturally as by humans.
Its capabilities are discoverable via `--help`, alongside a comprehensive [command reference](docs/commands.md) with detailed flags, expected outputs, and ready-to-use workflow examples.
Authentication and connection settings can be configured entirely through profiles and environment variables, avoiding the need to pass secrets as command-line arguments.
Commands use consistent naming conventions and flags.
Structured and parseable output formats (`--output json`, `--output yaml`, `--output csv`).
[Agent mode](#agent-mode) makes all of this automatic: JSON output, structured help, JSON errors, no prompts, and no colors — with zero configuration.
Run `dash0 skill install` in a project to add a local [Agent Skill](docs/commands.md#agent-tooling-commands) — a `SKILL.md` plus reference docs — so any Claude Code, Cursor, Codex, or GitHub Copilot session there discovers this command surface without spending turns on `--help` exploration.

## Quick start

Install the CLI — Homebrew shown here, see [Installation](#installation) for all channels:

```bash
brew install --cask dash0hq/dash0-cli/dash0
```

Log in — no auth token needed:

```bash
# Find your api_url here: https://app.dash0.com/goto/settings/endpoints?endpoint_type=api_http
dash0 login --profile default --api-url <api_url>
```

`dash0 login` creates the profile on first use, opens your browser to complete OAuth 2.0 with PKCE, and refreshes tokens automatically from then on.

Run your first command:

```bash
dash0 dashboards list
```

In environments without a browser — CI runners, AI agent sessions — set `DASH0_AUTH_TOKEN` to a static [auth token](https://app.dash0.com/goto/settings/auth-tokens) instead of logging in.
See the [quickstart walkthrough](docs/quickstart.md) for a five-minute tour from login to sending your first deployment event, and [Profiles](#profiles) for managing multiple environments.

## Installation

### Homebrew (macOS and Linux)

```bash
brew install --cask dash0hq/dash0-cli/dash0
```

No `brew tap`, no `brew trust`, no extra ceremony.
The qualified install path taps the [`dash0hq/homebrew-dash0-cli`](https://github.com/dash0hq/homebrew-dash0-cli) repository on first use.

> [!NOTE]
> If you previously installed `dash0` from the legacy formula (`brew install dash0` after `brew tap dash0hq/dash0-cli <URL>`), see the [Homebrew tap migration notes](docs/brew-tap-migration-2026-06.md) for the one-time switch to the new cask.

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

Static auth tokens are the right fit for non-interactive environments like CI; on a workstation, prefer the browser-based OAuth flow via [`dash0 login`](#quick-start).

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

### Nix / NixOS

The repository is a Nix flake that builds the CLI with `buildGoModule` and installs shell completions for Bash, Zsh, and Fish.

Run the CLI without installing it:

```bash
nix run github:dash0hq/dash0-cli -- dashboards list
```

Install it into your profile:

```bash
nix profile install github:dash0hq/dash0-cli
```

Add it to a NixOS or Home Manager configuration by consuming the flake's `overlays.default`, which exposes the package as `pkgs.dash0`:

```nix
{
  inputs.dash0-cli.url = "github:dash0hq/dash0-cli";
  # nixpkgs.overlays = [ dash0-cli.overlays.default ];
  # environment.systemPackages = [ pkgs.dash0 ];
}
```

This flake's `dash0` package builds from source.
A pre-built binary is published separately to the Dash0 Nix User Repository (NUR) at [`dash0hq/nur`](https://github.com/dash0hq/nur) — the Nix counterpart to the Homebrew cask — which skips compilation and is useful on small or non-`x86_64` machines:

```bash
nix profile install github:dash0hq/nur#dash0
```

Build from a local checkout — `nix build` for flake users, or `nix-build` on systems without flakes enabled:

```bash
nix build .#dash0
```

A development shell with the Go toolchain and the project's lint and changelog tooling is available via `nix develop` (or `nix-shell`):

```bash
nix develop
```

#### Declarative profiles with Home Manager

The flake ships a Home Manager module at `homeManagerModules.default` that declares Dash0 profiles under `~/.dash0` so they live alongside the rest of your dotfiles.

```nix
{
  inputs.dash0-cli.url = "github:dash0hq/dash0-cli";

  # In your Home Manager configuration:
  imports = [ dash0-cli.homeManagerModules.default ];

  programs.dash0 = {
    enable = true;
    activeProfile = "prod";
    profiles.prod = {
      apiUrl  = "https://api.eu-west-1.aws.dash0.com";
      otlpUrl = "https://ingress.eu-west-1.aws.dash0.com";
      dataset = "default";
      auth    = "oauth";   # run `dash0 login --profile prod` once
    };
    profiles.ci = {
      apiUrl        = "https://api.us-west-2.aws.dash0.com";
      auth          = "static";
      authTokenFile = "/run/secrets/dash0-ci-token";   # e.g. an agenix/sops-nix secret
    };
  };
}
```

The module never writes auth tokens into the world-readable Nix store: static tokens are read from `authTokenFile` at activation time, and OAuth tokens are obtained by `dash0 login` at runtime.
Because the CLI rewrites `profiles.json` on OAuth refresh and login, the module merges declared profiles into the live file rather than overwriting it, so logging in once survives every subsequent `home-manager switch`.
Set `programs.dash0.pruneUndeclared = true` to make the module the sole authority over `profiles.json` and remove profiles that are not declared.
When pruning is enabled, `activeProfile` is checked at evaluation time and must name a declared profile, so a typo fails the build instead of leaving the CLI pointed at a profile that was pruned away.

The module installs the source-built `dash0` by default.
To avoid compiling — for example on a small VM — point it at the pre-built binary from the [`dash0hq/nur`](https://github.com/dash0hq/nur) flake (add it as an input):

```nix
programs.dash0.package = inputs.dash0-nur.packages.${pkgs.stdenv.hostPlatform.system}.dash0;
```

To try the module — either by activating it for your user inside an existing NixOS machine or VM, or on a fresh throwaway NixOS guest — see the [`nix/examples/home-manager-vm`](nix/examples/home-manager-vm) example.

Maintainers: see [CONTRIBUTING.md](CONTRIBUTING.md#nix-packaging) for how the Nix packaging is structured and how to refresh the `vendorHash`.

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

The CLI resolves connection settings from profiles stored on disk, [environment variables](#common-settings), and CLI flags, in that order.
Profiles are the recommended way to manage credentials locally; environment variables are convenient for CI/CD and agentic workflows.

#### Profiles

Configure API access using profiles.
A profile can authenticate with either a long-lived static auth token or via OAuth 2.0 + PKCE (browser-based login).
All other profile fields are optional at creation time and can be supplied later via `config profiles update` or overridden at runtime with [environment variables or CLI flags](#common-settings).

Browser-based OAuth login (recommended for human users):

```bash
dash0 config profiles create dev --oauth \
    --api-url https://api.us-west-2.aws.dash0.com
```

```bash
dash0 login
```

`dash0 login` opens the system browser, completes the OAuth flow, and saves the access and refresh tokens into the profile.
Access tokens are refreshed automatically as long as the refresh token is valid; `dash0 logout` revokes and clears them.

Static-token profile (suited to CI/CD and agent workflows):

```bash
dash0 config profiles create dev \
    --api-url https://api.us-west-2.aws.dash0.com \
    --otlp-url https://ingress.us-west-2.aws.dash0.com \
    --auth-token auth_xxx
```

Manage and inspect profiles:

```bash
dash0 config profiles list
```

```bash
dash0 config profiles select dev
```

```bash
dash0 config show
```

You can find the API endpoint for your organization on the [Endpoints](https://app.dash0.com/settings/endpoints) page, under the `API` entry, and the OTLP HTTP endpoint under the `OTLP via HTTP` entry.
Currently only HTTP OTLP endpoints are supported.

#### Configuration storage

Profiles and the active-profile selection are stored on disk in `~/.dash0/`:

| File | Content |
|------|---------|
| `profiles.json` | All configured profiles (name, URLs, auth token, dataset, OAuth state) |
| `activeProfile` | Name of the currently active profile |
| `oauth-clients.json` | Cached OAuth dynamic client registrations, keyed by API URL |

The directory is created automatically when you create your first profile.

To store configuration elsewhere, set the `DASH0_CONFIG_DIR` environment variable:

```bash
export DASH0_CONFIG_DIR=~/.local/dash0
dash0 config profiles create dev --api-url https://api.us-west-2.aws.dash0.com
```

### Agent mode

Agent mode optimizes every aspect of the CLI for machine consumption.
Enable it explicitly with `--agent-mode` or `DASH0_AGENT_MODE=true`, or let it auto-activate when a known AI agent environment variable is detected:

| Agent | Environment variables |
|-------|----------------------|
| [Aider](https://aider.chat/) | `AIDER` |
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | `CLAUDE_CODE`, `CLAUDECODE` |
| [Cline](https://cline.bot/) | `CLINE`, `CLINE_TASK_ID` |
| [Cursor](https://www.cursor.com/) | `CURSOR_AGENT`, `CURSOR_SESSION_ID` |
| [GitHub Copilot](https://github.com/features/copilot) | `GITHUB_COPILOT` |
| [MCP](https://modelcontextprotocol.io/) servers | `MCP_SESSION_ID` |
| [OpenAI Codex](https://openai.com/index/introducing-codex/) | `CODEX`, `OPENAI_CODEX` |
| [Windsurf](https://windsurf.com/) | `WINDSURF_AGENT`, `WINDSURF_SESSION_ID` |

When agent mode is active, the CLI:

- **Defaults output to JSON** — all data retrieval commands (`list`, `get`, `query`, `config show`, `metrics instant`) output JSON instead of tables, without needing `-o json`.
- **Returns `--help` as structured JSON** — flags, subcommands, aliases, and metadata are machine-parseable.
- **Emits errors as JSON on stderr** — `{"error": "...", "hint": "..."}` instead of colored text.
- **Skips confirmation prompts** — destructive operations (`delete`, `remove`) proceed without asking, equivalent to `--force`.
- **Disables colored output** — no ANSI escape codes in any output.

To explicitly disable agent mode (for example, when running inside an agent environment but wanting human-readable output), set `DASH0_AGENT_MODE=0` or `DASH0_AGENT_MODE=false`.
This overrides all other activation methods.

See the [agent mode specification](docs/commands.md#agent-mode) for the full priority order and details.

### Applying assets

Apply asset definitions from a file, directory, or stdin.
The input may contain multiple YAML documents separated by `---`.
Supported asset types: `Dashboard`, `PersesDashboard`, `CheckRule`, `PrometheusRule` (alerting and recording rules), `SyntheticCheck`, `View`, `Dash0SpamFilter`, and `Dash0NotificationChannel`.

From a single file:

```bash
dash0 apply -f assets.yaml
```

From a directory (recursive):

```bash
dash0 apply -f dashboards/
```

From stdin:

```bash
cat assets.yaml | dash0 apply -f -
```

Validate without applying:

```bash
dash0 apply -f assets.yaml --dry-run
```

**Note:** In Dash0, dashboards, views, synthetic checks and check rules are called "assets", rather than the more common "resources".
The reason for this is that the word "resource" is overloaded in OpenTelemetry, where it describes "where telemetry comes from".

### Asset CRUD commands

Every asset type (`dashboards`, `check-rules`, `synthetic-checks`, `views`, `recording-rules`, `notification-channels`, `spam-filters`) supports the same five subcommands.
Substitute the asset noun in the examples below.

List all assets of a type:

```bash
dash0 dashboards list
```

Get one by ID (in table form):

```bash
dash0 dashboards get <id>
```

Get one by ID (in a re-appliable YAML form):

```bash
dash0 dashboards get <id> -o yaml
```

Create from a YAML file:

```bash
dash0 dashboards create -f dashboard.yaml
```

Update from a YAML file (the ID is read from the file when not passed explicitly):

```bash
dash0 dashboards update -f dashboard.yaml
```

Delete by ID (use `--force` to skip the confirmation prompt):

```bash
dash0 dashboards delete <id>
```

`dashboards create` and `apply` also accept PersesDashboard CRD files; `check-rules create` and `apply` also accept PrometheusRule CRD files.
See [docs/commands.md](docs/commands.md) for the full per-asset reference and the YAML formats.

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

> [!NOTE]
> The `dash0 logs query` command requires an API URL and auth token configured in the active profile, or via flags or environment variables.

Recent logs (default: last 15 minutes, up to 50):

```bash
dash0 logs query
```

Explicit time range and higher limit:

```bash
dash0 logs query --from now-1h --to now --limit 100
```

Filter by service:

```bash
dash0 logs query --filter "service.name is my-service"
```

Filter by severity range (errors and warnings):

```bash
dash0 logs query --filter "otel.log.severity.range is_one_of ERROR WARN"
```

JSON filter criteria copied from the Dash0 UI:

```bash
dash0 logs query --filter '[{"key":"service.name","operator":"is","value":"api"}]'
```

CSV output for pipelines:

```bash
dash0 logs query -o csv
```

Custom columns:

```bash
dash0 logs query --column time --column service.name --column body
dash0 logs query --precision disabled --filter "test.id is <id>"
```

See the [filter syntax reference](docs/commands.md#filter-syntax) for the full list of operators.
Pass `--precision disabled` to turn off [adaptive sampling](docs/commands.md#precision-mode-adaptive-sampling) when a narrow filter must always return every match.

### Tracing

#### Sending spans to Dash0

> [!NOTE]
> The `dash0 spans send` command requires an OTLP URL configured in the active profile, or via the `--otlp-url` flag or the `DASH0_OTLP_URL` environment variable.

```bash
dash0 spans send --name "GET /api/users" \
    --kind SERVER --status-code OK --duration 100ms \
    --resource-attribute service.name=my-service
```

#### Querying spans from Dash0

> [!NOTE]
> The `dash0 spans query` command requires an API URL and auth token configured in the active profile, or via flags or environment variables.

Recent spans (default: last 15 minutes, up to 50):

```bash
dash0 spans query
```

Explicit time range and higher limit:

```bash
dash0 spans query --from now-1h --to now --limit 100
```

Filter by service:

```bash
dash0 spans query --filter "service.name is my-service"
```

Filter by span status:

```bash
dash0 spans query --filter "otel.span.status.code is ERROR"
```

JSON filter criteria copied from the Dash0 UI:

```bash
dash0 spans query --filter '[{"key":"service.name","operator":"is","value":"api"}]'
```

CSV output:

```bash
dash0 spans query -o csv
```

Custom columns:

```bash
dash0 spans query --column otel.span.start_time --column otel.span.duration --column "span name" --column http.request.method
dash0 spans query --precision disabled --filter "test.id is <id>"
```

See the [filter syntax reference](docs/commands.md#filter-syntax) for the full list of operators.
Pass `--precision disabled` to turn off [adaptive sampling](docs/commands.md#precision-mode-adaptive-sampling) for narrow lookups that must always return every match.

#### Getting a trace from Dash0

> [!NOTE]
> The `dash0 traces get` command requires an API URL and auth token configured in the active profile, or via flags or environment variables.

Get all spans of a trace:

```bash
dash0 traces get <trace-id>
```

Look back further when the trace is older:

```bash
dash0 traces get <trace-id> --from now-2h
```

Follow span links to related traces:

```bash
dash0 traces get <trace-id> --follow-span-links
```

OTLP JSON output:

```bash
dash0 traces get <trace-id> -o json
```

Custom columns:

```bash
dash0 traces get <trace-id> --column otel.span.start_time --column otel.span.duration --column "span name" --column otel.span.status.code
```

`dash0 traces get` always disables [adaptive sampling](docs/commands.md#precision-mode-adaptive-sampling) so every span in the trace is returned.

### Metrics

```bash
# Instant query — current request rate per service
dash0 metrics instant --promql 'sum by (service_name) (rate(http_server_request_duration_seconds_count[5m]))'
```

```bash
# Instant query with filters
dash0 metrics instant --filter 'service.name is my-service'
```

```bash
# Output as CSV with specific columns
dash0 metrics instant --promql 'sum by (service_name) (rate(http_server_request_duration_seconds_count[5m]))' -o csv --column value --column service_name
```

### Alerting

#### Querying failed checks

> [!NOTE]
> The `dash0 failed-checks query` command requires an API URL and auth token configured in the active profile, or via flags or environment variables.

List all currently active (unresolved) issues:

```bash
dash0 failed-checks query --active
```

Filter by status:

```bash
dash0 failed-checks query --status critical,degraded
```

Filter by an arbitrary issue label (priority, owner, …):

```bash
dash0 failed-checks query --filter "priority is_one_of p1 p2" --active
```

Add the `priority` issue label as a column:

```bash
dash0 failed-checks query --active --column "check rule" --column priority --column status --column summary
```

JSON output for pipelines:

```bash
dash0 failed-checks query --active -o json
```

See the [filter syntax reference](docs/commands.md#filter-syntax) for the full list of operators.

### Teams (experimental)

```bash
# List all teams
dash0 -X teams list
```

```bash
# Get team details (members + accessible assets)
dash0 -X teams get <id>
```

```bash
# Create a team
dash0 -X teams create "Backend Team" --color-from "#FF6B6B" --color-to "#4ECDC4"
```

```bash
# Add members to a team
dash0 -X teams add-members <team-id> <member-id-1> <member-id-2>
```

### Members (experimental)

```bash
# List organization members
dash0 -X members list
```

```bash
# Invite a member (default role: basic_member)
dash0 -X members invite user@example.com
```

```bash
# Delete a member
dash0 -X members delete <member-id> --force
```

### Raw HTTP passthrough (experimental)

The `api` command calls any Dash0 API endpoint directly, reusing the active profile's connection settings.
It is useful for endpoints that do not yet have a dedicated subcommand.

```bash
# GET — dataset auto-injected from the active profile
dash0 -X api /api/signal-to-metrics/configs
```

```bash
# POST with a payload from a file
dash0 -X api POST /api/signal-to-metrics/configs -f config.json
```

```bash
# Skip dataset injection for organization-level endpoints
dash0 -X api /api/organization/settings --dataset ""
```

### Local OTLP proxy (experimental)

The `otlp proxy` command runs a long-lived local OTLP forwarder.
It accepts OTLP/HTTP on `127.0.0.1:4318` and OTLP/gRPC on `127.0.0.1:4317` and forwards every batch to Dash0 using the active profile's credentials.
An OpenTelemetry SDK at default endpoint configuration connects without any environment variable change, collapsing the local-dev setup from a YAML-heavy Collector config to a single command.

```bash
# Just run it. SDK defaults already point at the proxy.
dash0 -X otlp proxy

# Print every forwarded record on stdout in collector-debug-exporter style.
dash0 -X otlp proxy --tail

# Pick non-default ports (e.g., when another local Collector holds the defaults).
dash0 -X otlp proxy --http-port 8318 --grpc-port 8317

# Tag every forwarded batch at the resource level so it is filterable in Dash0.
dash0 -X otlp proxy \
    --resource-attribute developer=alice \
    --resource-attribute deployment.environment.name=local
```

The proxy exits on `Ctrl-C` (or `SIGTERM`) after draining in-flight work within a 5-second deadline.
On startup, if either default port is already in use, the proxy exits non-zero with an actionable error that names the holding process.
See [docs/commands.md](docs/commands.md#otlp-proxy-experimental) for the full reference, including the decoration flags (`--scope-attribute`, `--log-attribute`, `--span-attribute`, `--metric-attribute`, `--scope-name`, `--scope-version`), the agent-mode event schema, and the failure-mode classification.

### Common settings

| Flag | Short | Env Variable | Description |
|------|-------|--------------|-------------|
| `--agent-mode` | | `DASH0_AGENT_MODE` | Enable agent mode for AI coding agents. Auto-detected when common agent env vars are set. |
| `--api-url` | | `DASH0_API_URL` | Override API URL from profile. Find yours [here](https://app.dash0.com/goto/settings/endpoints?endpoint_type=api_http). |
| `--otlp-url` | | `DASH0_OTLP_URL` | Override OTLP URL from profile. Find yours [here](https://app.dash0.com/goto/settings/endpoints?endpoint_type=otlp_http). |
| `--auth-token` | | `DASH0_AUTH_TOKEN` | Override auth token from profile. Find yours [here](https://app.dash0.com/goto/settings/auth-tokens). |
| `--color` | | `DASH0_COLOR` | Color mode for output: `semantic` (default) or `none`. Ignored when piping output. |
| `--dataset` | | `DASH0_DATASET` | Override dataset from profile. Use the `identifier`, not `Name`. |
| `--experimental` | `-X` | | Enable experimental features (required for commands marked `[experimental]`) |
| `--profile` | | `DASH0_PROFILE` | Use a named profile for this invocation without changing the active profile. |
| `--file` | `-f` | | Input file path (use `-` for stdin) |
| `--output` | `-o` | | Output format: `table`, `wide`, `json`, `yaml`, `csv` |
| | | `DASH0_CONFIG_DIR` | Override the configuration directory (default: `~/.dash0`) |
| | | `DASH0_OTLP_PROXY_GRPC_PORT` | Override `dash0 otlp proxy --grpc-port` |
| | | `DASH0_OTLP_PROXY_HTTP_PORT` | Override `dash0 otlp proxy --http-port` |
| `--max-retries` | | `DASH0_MAX_RETRIES` | Max retries for failed API requests (default: `3`, max: `5`; `0` to disable) |
| `--no-skill-hint` | | `DASH0_NO_SKILL_HINT` | Suppress the agent-mode error hint pointing at `dash0 skill install`. |

### Output formats

The `list` and `get` commands for assets support multiple output formats via `-o`:

- **`table`** (default): Compact view with essential columns (name and ID)
- **`wide`**: Similar to `table`, with additional columns (dataset, origin, and URL)
- **`json`**: Full asset data in JSON format
- **`yaml`**: Full asset data in YAML format
- **`csv`**: Comma-separated values with the same columns as `wide`, suitable for piping and automation

The `update` and `apply` commands show a unified diff of changes.

The `logs query`, `spans query`, and `traces get` commands support a different set of formats via `-o`:

- **`table`** (default): Columnar output (`logs query` shows timestamp, severity, and body; `spans query` shows timestamp, duration, name, status, service, and trace ID; `traces get` shows a hierarchical span tree)
- **`json`**: Full OTLP/JSON payload
- **`csv`**: Comma-separated values

In [agent mode](#agent-mode), all data retrieval commands default to JSON without needing `-o json`.

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
