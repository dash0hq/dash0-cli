# Command reference

This document is a comprehensive reference for the `dash0` CLI, aimed at enabling AI agents and automation workflows to use it effectively.

For every command, this reference lists the exact syntax, all flags, expected outputs, and concrete examples.

## Command taxonomy

Every command falls into one of eight categories.
Each category has distinct patterns for flags, output, and behavior.

| Category | Commands | Characteristics |
|----------|----------|-----------------|
| [Authentication](#authentication) | `login`, `logout` | Browser-based OAuth 2.0 + PKCE; per-profile |
| [Configuration](#configuration) | `config profiles`, `config show` | Profile management, no API calls |
| [Asset CRUD](#asset-crud-commands) | `dashboards`, `views`, `check-rules`, `synthetic-checks`, `recording-rules`, `notification-channels`, `spam-filters`, `apply` | File-based input, `--dry-run`, five standard subcommands |
| [Query](#query-commands) | `logs query`, `spans query`, `traces get`, `metrics instant`, `failed-checks query` | Time range, filters |
| [Send](#send-commands) | `logs send`, `spans send` | OTLP-based, repeatable attribute flags |
| [Daemon](#daemon-commands) | `otlp proxy` | Long-running, signal-driven shutdown, experimental |
| [Organizational](#organizational-commands) | `teams`, `members`, `notification-channels` | Flag-based input, no dataset, experimental |
| [Raw HTTP](#raw-http-command) | `api` | Passthrough to any Dash0 API endpoint, experimental |
| [Agent tooling](#agent-tooling-commands) | `skill install`, `skill show` | No API calls; local filesystem only; not gated by `--experimental`; no `-o`/JSON output — content is always plain markdown |

**Authentication commands** populate or revoke the OAuth tokens of a profile.
A profile can be in one of three auth states: **static** (holds a long-lived `auth_*` token), **OAuth-active** (holds a `dash0_at_*` access token and a refresh token; auto-refreshes), or **OAuth-empty** (marked as OAuth but not yet logged in).
`dash0 login` requires an interactive terminal and never silently mutates a static profile.
`dash0 logout` clears the OAuth tokens from a profile but keeps the profile shell for re-login.

**Asset CRUD commands** create, list, get, update, and delete dataset-scoped assets (dashboards, views, check rules, synthetic checks, recording rules).
They use file-based input (`-f`), support `--dry-run`, and offer five output formats (`table`, `wide`, `json`, `yaml`, `csv`).
The `apply` command provides create-or-update semantics across all asset types.

**Query commands** search and retrieve telemetry signals.
They accept time range flags (`--from`, `--to`), a repeatable `--filter` flag with the standard [filter syntax](#filter-syntax), and customizable columns via `--column`.
Output formats are `table`, `json`, and `csv`.

**Send commands** transmit telemetry data to Dash0 via OTLP.
They require `otlp-url` (not `api-url`) and use repeatable attribute flags (`--resource-attribute`, `--scope-attribute`, and a signal-specific attribute flag).

**Organizational commands** manage entities (teams, members, notification channels) scoped to the organization, not to a dataset.
They use flag-based input (no `-f`, no `--dry-run`, no `apply` integration) and are all experimental.
Notification channels are an exception: they use file-based input (`-f`) and support `--dry-run`, but are still organization-level (no `--dataset`).

**Raw HTTP command** (`api`) is an escape hatch for endpoints that do not yet have a dedicated subcommand.
It reuses the active profile's `api-url`, `auth-token`, and (by default) `dataset`, and emits a plain HTTP request.
It is experimental.

**Agent tooling commands** (`skill install`, `skill show`) distribute the dash0-cli Agent Skill — a bundle that teaches AI coding agents this CLI's command surface without spending turns on `--help` exploration.
They never call the Dash0 API, so they take no `--api-url`, `--auth-token`, or `--dataset`, and they are not gated behind `--experimental` since the entire point is frictionless discovery.
Unlike every other command, their output is always plain markdown, in both human and agent mode — see [Agent tooling commands](#agent-tooling-commands) for why.

## Prerequisites

Every command that talks to the Dash0 API or OTLP endpoint needs credentials.
The CLI resolves each individual setting (`api-url`, `otlp-url`, `auth-token`, `dataset`) in this order (first match wins):

1. Environment variables (`DASH0_API_URL`, `DASH0_OTLP_URL`, `DASH0_AUTH_TOKEN`, `DASH0_DATASET`)
2. CLI flags (`--api-url`, `--otlp-url`, `--auth-token`, `--dataset`)
3. The selected profile (see below)

The profile itself is selected in this order (first match wins):

1. `--profile <name>` flag
2. `DASH0_PROFILE` environment variable
3. The active profile recorded on disk (set via `config profiles select`, stored in `~/.dash0/`)

Using `--profile` or `DASH0_PROFILE` does not modify the active profile on disk — it only changes which profile is read for the current invocation.
Passing `--profile ""` or `DASH0_PROFILE=""` is treated as "not set" and falls through to the next step.
If the selected profile does not exist, the command fails before making any API call with a message listing the available profile names.

Commands that read from the API (asset CRUD, `logs query`, `spans query`, `traces get`, `metrics instant`) require `api-url` and `auth-token`.
Commands that write via OTLP (`logs send`, `spans send`) require `otlp-url` and `auth-token`.

## Global flags

These flags are available on every command:

| Flag | Short | Env variable | Description |
|------|-------|--------------|-------------|
| `--api-url` | | `DASH0_API_URL` | API endpoint URL |
| `--otlp-url` | | `DASH0_OTLP_URL` | OTLP HTTP endpoint URL |
| `--auth-token` | | `DASH0_AUTH_TOKEN` | Authentication token |
| `--dataset` | | `DASH0_DATASET` | Dataset identifier (not display name) |
| `--profile` | | `DASH0_PROFILE` | Profile to use for this invocation; overrides the active profile on disk |
| `--agent-mode` | | `DASH0_AGENT_MODE` | Enable agent mode for AI coding agents (see below) |
| `--color` | | `DASH0_COLOR` | `semantic` (default) or `none` |
| `--experimental` | `-X` | | Enable experimental commands |
| | | `DASH0_CONFIG_DIR` | Override config directory (default: `~/.dash0`) |
| `--max-retries` | | `DASH0_MAX_RETRIES` | Maximum number of retries for failed API requests (default: `3`, max: `5`; set to `0` to disable retries) |
| `--no-skill-hint` | | `DASH0_NO_SKILL_HINT` | Suppress the agent-mode error hint pointing at `dash0 skill install` (see [Agent tooling commands](#agent-tooling-commands)) |

### Agent mode

Agent mode optimizes the CLI for consumption by AI coding agents.
When active, the CLI:

1. Defaults output format to JSON instead of tables.
2. Returns `--help` as structured JSON with flags, subcommands, and metadata.
3. Emits errors as JSON objects on stderr.
4. Skips confirmation prompts automatically (same as `--force`).
5. Disables colored output.

Agent mode is resolved in this priority order (first match wins):

1. `DASH0_AGENT_MODE=0|false` — explicitly disabled, overrides everything.
2. `--agent-mode` flag — explicitly enabled.
3. `DASH0_AGENT_MODE=1|true` — explicitly enabled via environment variable.
4. Auto-detection via known AI agent environment variables: `AIDER`, `CLAUDE_CODE`, `CLAUDECODE`, `CLINE`, `CLINE_TASK_ID`, `CODEX`, `CURSOR_AGENT`, `CURSOR_SESSION_ID`, `GITHUB_COPILOT`, `MCP_SESSION_ID`, `OPENAI_CODEX`, `WINDSURF_AGENT`, `WINDSURF_SESSION_ID`.

## Authentication

`dash0 login` and `dash0 logout` manage the OAuth tokens of a profile.
Both commands operate on the active profile by default; pass `--profile <name>` to target a specific one.

### Profile auth states

A profile is in exactly one of three auth states:

| State | `Auth Token:` in `config show` | How to get there |
|-------|--------------------------------|------------------|
| **Static** | masked `...auth_*` | `dash0 config profiles create <name> --auth-token auth_<...>` |
| **OAuth-empty** | `(OAuth, not logged in)` | `dash0 config profiles create <name> --oauth`, `dash0 config profiles update <name> --oauth`, or `dash0 logout` from OAuth-active |
| **OAuth-active** | masked `...dash0_at_*` with `(OAuth, expires in …)` | `dash0 login` on an OAuth-empty or OAuth-active profile |

OAuth-empty profiles cannot serve API calls — running a CLI command against one prints `the active profile is OAuth-typed but not authenticated. Hint: Run \`dash0 login\` to log in.` (or `profile "<name>" is OAuth-typed but not authenticated.` when `--profile` was passed).

In agent mode (`--agent-mode` or `DASH0_AGENT_MODE=1`), the hint is rewritten because `dash0 login` requires an interactive terminal.
Agents are routed to two escape hatches: either set `DASH0_AUTH_TOKEN` to a static `auth_*` token, or convert the profile back to static auth with `dash0 config profiles update <name> --oauth=false --auth-token auth_<...> --force`.
The same agent-mode hint appears when an OAuth refresh fails at the API layer (token revoked or expired server-side).

### `login`

Authenticate to Dash0 via OAuth 2.0 with PKCE.
Opens the system browser, listens on a localhost TCP port for the callback, exchanges the authorization code for tokens, and saves them in the target profile.

```bash
dash0 login [--profile <name>] [--api-url <url>] [--port <n>] [--timeout <duration>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--profile` | active profile | Profile to save tokens under |
| `--api-url` | active profile's URL / `DASH0_API_URL` | Dash0 API URL to authenticate against |
| `--port` | OS-assigned ephemeral | Local TCP port for the OAuth callback listener |
| `--timeout` | `2m` | How long to wait for the browser callback before aborting |

State machine:

- **OAuth-empty / OAuth-active target** — proceeds silently.
  Re-login on an OAuth-active profile revokes the prior refresh token best-effort.
- **Static target** — prompts: `<profile> uses a static auth token. Convert it to an OAuth profile (the existing auth token will be discarded)? [y/N]:`.
  Aborts cleanly on `n`.
- **No-auth target** — prompts: `<profile> has no auth token configured. Mark it as OAuth and log in? [y/N]:`.
  Aborts cleanly on `n`.
- **Missing target** — prompts to create the profile as OAuth before proceeding.
- **No active profile and no `--profile`** — fails immediately with a hint pointing at `dash0 config profiles create` and `dash0 login --profile <name>`.

The command requires an interactive terminal.
In agent mode or when stdout is not a TTY, it exits with an error pointing at the static-token fallback (`DASH0_AUTH_TOKEN` or `dash0 config profiles create --auth-token`).

Log in to the active profile:

```bash
$ dash0 login
Opening your browser to log in to https://api.eu-west-1.aws.dash0.com
If the browser does not open automatically, paste this URL:
  https://api.eu-west-1.aws.dash0.com/oauth/authorize?...
Logged in as profile "prod" (access token expires in 1h0m0s).
```

Log in to a specific profile:

```bash
dash0 login --profile staging
```

Log in to a Dash0 region different from the active profile:

```bash
dash0 login --profile eu --api-url https://api.eu-west-1.aws.dash0.com
```

The OAuth client registration (RFC 7591) is cached per API URL in `~/.dash0/oauth-clients.json`, so re-running `login` against the same Dash0 region does not re-register a new client every time.
The cache is invalidated automatically when the server reports `invalid_client` during token exchange.

OAuth access tokens are organization-scoped: each token is bound to whichever Dash0 organization the user picks during the browser consent step.
Profiles for different organizations must each go through their own `dash0 login`.

### `logout`

Clear the OAuth tokens of a profile and best-effort revoke them server-side.
The profile shell is kept so a future `dash0 login` can re-fill it.

```bash
dash0 logout [--profile <name>] [--force]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--profile` | active profile | Profile to log out of |
| `--force` | `false` | Skip the confirmation prompt |

Refuses to operate on static-token profiles and points the user at `dash0 config profiles delete` instead.
Logging out of an OAuth-empty profile is a no-op and exits 0.

In agent mode (`--agent-mode` or `DASH0_AGENT_MODE=1`, also auto-detected from agent env vars like `CLAUDE_CODE`), `logout` requires an explicit `--force` and otherwise refuses with an error.
This mirrors `dash0 login`'s blanket refusal to run in agent mode: both directions of the OAuth session transition require deliberate user intent, so an AI agent invoking `dash0 logout` cannot silently revoke the refresh token of whichever profile the env points at.

Log out of the active profile:

```bash
$ dash0 logout
Log out of the active profile 'prod' (revoking its OAuth refresh token)? [y/N]: y
Logged out.
```

Log out of a named profile without confirmation:

```bash
$ dash0 logout --profile staging --force
Logged out of profile "staging".
```

> [!NOTE]
> To leave OAuth behind entirely (not just log out), use `dash0 config profiles update <name> --oauth=false`.
> That clears the OAuth marker and lets the profile take a static token again.

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
    [--dataset <dataset>] \
    [--oauth]
```

Pass `--oauth` to mark the profile as OAuth-authenticated.
An OAuth profile is created in the **OAuth-empty** state — `--api-url` is required (so `dash0 login` knows where to authenticate), and `--oauth` is mutually exclusive with `--auth-token`.
Run `dash0 login` afterwards to obtain the access and refresh tokens.

Example — static-token profile:

```bash
$ dash0 config profiles create dev \
    --api-url https://api.us-west-2.aws.dash0.com \
    --otlp-url https://ingress.us-west-2.aws.dash0.com \
    --auth-token auth_xxx
Profile "dev" added
```

Example — OAuth profile:

```bash
$ dash0 config profiles create dev --oauth \
    --api-url https://api.us-west-2.aws.dash0.com
Profile "dev" created (OAuth).
Hint: Run `dash0 login` to authenticate.
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
    [--dataset <dataset>] \
    [--oauth[=true|=false]] \
    [--force]
```

`--oauth` is the off-ramp for switching a profile between **static** and **OAuth** authentication.
Both transitions are destructive and prompt for confirmation unless `--force` is set:

- `--oauth` (or `--oauth=true`) on a static profile discards the existing static token and marks the profile as OAuth-empty (run `dash0 login` next).
- `--oauth=false` on an OAuth profile revokes its refresh token (best-effort) and clears the OAuth block.
  Pair it with `--auth-token auth_<...>` to land in a static profile in one command; otherwise the profile is left token-less and `dash0 config profiles update` must be used again to set one.

`--oauth=true` and `--auth-token` are mutually exclusive in the same invocation.
Setting `--auth-token` on an OAuth-active profile without also passing `--oauth=false` is rejected — the library cannot interpret a profile that holds both a static token and an OAuth block.

Update the API URL of a profile:

```bash
$ dash0 config profiles update prod --api-url https://api.us-east-1.aws.dash0.com
Profile "prod" updated
```

Convert a static-token profile to OAuth (prompts unless `--force`):

```bash
$ dash0 config profiles update prod --oauth --force
Profile "prod" updated
Hint: Run `dash0 login` to authenticate.
```

Convert an OAuth profile back to static in one command:

```bash
$ dash0 config profiles update prod --oauth=false --auth-token auth_xxx --force
Profile "prod" updated
```

Convert an OAuth profile back to no-auth (the user can set a token later):

```bash
$ dash0 config profiles update prod --oauth=false --force
Profile "prod" updated
Hint: This profile has no auth token configured. Set one with `dash0 config profiles update prod --auth-token auth_<...>`.
```

Remove a field by passing an empty string:

```bash
$ dash0 config profiles update prod --dataset ''
Profile "prod" updated
```

### `config profiles list`

List all profiles.
The active profile is marked with `*` in table output and with `"active": true` in JSON output.

```bash
dash0 config profiles list [-o <format>] [--skip-header]
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--output` | `-o` | `table` | Output format: `table` or `json` |
| `--skip-header` | | `false` | Omit the header row from `table` output |

Example output:

```
  NAME  API URL                              OTLP URL                                    DATASET  AUTH
* dev   https://api.us-west-2.aws.dash0.com  https://ingress.us-west-2.aws.dash0.com     default  oauth ...dash0_at_xx (47m)
  prod  https://api.eu-west-1.aws.dash0.com  https://ingress.eu-west-1.aws.dash0.com     default  static ...uth_yyy
  cold  https://api.eu-west-1.aws.dash0.com                                              default  oauth (not logged in)
```

The `AUTH` column carries the auth state per profile:

- `static <masked-token>` for a profile with a long-lived `auth_*` token.
- `oauth <masked-token> (<remaining-lifetime>)` for an OAuth-active profile.
- `oauth (not logged in)` for an OAuth-empty profile (created with `--oauth` but `dash0 login` not yet run, or just logged out).
- `(none)` when no token is configured.

JSON output keeps the existing `authToken` field and adds a sibling `authKind` field with values `static`, `oauth-active`, `oauth-empty`, or `none`.

Use `-o json` to get structured output (the default in agent mode):

```bash
$ dash0 config profiles list -o json
```

Use `--skip-header` to omit the header row from table output:

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

Aliases: `activate`

### `config profiles delete`

Delete a profile.

```bash
dash0 config profiles delete <name>
```

Aliases: `remove`

### `config show`

Display the resolved configuration (selected profile + environment variable overrides).

```bash
dash0 config show
```

Example output for a static-token profile:

```
Profile:    prod
API URL:    https://api.eu-west-1.aws.dash0.com
OTLP URL:   https://ingress.eu-west-1.aws.dash0.com
Dataset:    default
Auth Token: ...uth_yyy
```

When the profile is OAuth, the `Auth Token:` line is annotated:

```
Profile:    dev
API URL:    https://api.us-west-2.aws.dash0.com
OTLP URL:   (not set)
Dataset:    default
Auth Token: ...dash0_at_xx    (OAuth, expires in 47m23s)
```

When the profile is OAuth but not yet logged in (OAuth-empty), the line points the user at `dash0 login`:

```
Profile:    dev
API URL:    https://api.us-west-2.aws.dash0.com
OTLP URL:   (not set)
Dataset:    default
Auth Token: (OAuth, not logged in)
            Hint: Run `dash0 login` to authenticate.
```

The hint elides `--profile <name>` when the displayed profile is already the active one.

`-o json` (the default in agent mode) emits the same fields plus an `oauth` object on OAuth profiles.
For OAuth-active profiles the object contains `clientId`, `expiresAt` (RFC 3339), and `"authenticated": true`.
For OAuth-empty profiles the object contains `"authenticated": false` plus a `hint` field naming the agent-mode recovery path (`DASH0_AUTH_TOKEN` or `dash0 config profiles update <name> --oauth=false --auth-token auth_<...> --force`).
The `oauth` object is omitted entirely when `DASH0_AUTH_TOKEN` is set, because the static token shadows the OAuth state.

```bash
$ dash0 config show -o json
{
  "profile": {"value": "dev"},
  "apiUrl": {"value": "https://api.us-west-2.aws.dash0.com"},
  ...
  "oauth": {
    "authenticated": false,
    "hint": "set DASH0_AUTH_TOKEN to a static `auth_*` token, or convert the profile with `dash0 config profiles update dev --oauth=false --auth-token auth_<...> --force`"
  }
}
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

When a profile is selected via `--profile` or `DASH0_PROFILE`, the `Profile:` line is annotated with the source:

```bash
$ dash0 --profile prod config show
Profile:    prod    (from --profile flag)
...

$ DASH0_PROFILE=prod dash0 config show
Profile:    prod    (from DASH0_PROFILE environment variable)
...
```

## Asset CRUD commands

Asset CRUD commands create, list, get, update, and delete Dash0 assets.
Dash0 calls dashboards, views, synthetic checks, and check rules "assets" (not "resources", which is an overloaded term in OpenTelemetry).

All seven asset types (`dashboards`, `check-rules`, `synthetic-checks`, `views`, `recording-rules`, `notification-channels`, `spam-filters`) share the same CRUD subcommands.
The examples below use `dashboards`, but the same patterns apply to every asset type.

### `list`

List all assets in the dataset.

```bash
dash0 dashboards list [--limit <n>] [-o <format>] [--skip-header]
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--limit` | `-l` | 50 | Maximum number of results |
| `--output` | `-o` | `table` | `table`, `wide`, `json`, `yaml`, or `csv` |
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

Use `-o json` or `-o yaml` to get the full asset definitions, suitable for backup or re-applying with `apply -f -`.
The YAML output is a multi-document stream (documents separated by `---`) so it can be piped directly to `dash0 apply -f -`.
Use `-o csv` for a pipe-friendly, machine-readable format with the same columns as `wide`:

```bash
$ dash0 dashboards list -o csv
name,id,dataset,origin,url
Production Overview,a1b2c3d4-5678-90ab-cdef-1234567890ab,default,gitops/prod,https://app.dash0.com/goto/dashboards?dashboard_id=a1b2c3d4-...
```

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
Dashboard "My Dashboard" created
```

`dashboards create` also accepts PersesDashboard CRD files (`perses.dev/v1alpha1` and `perses.dev/v1alpha2`).

```bash
$ dash0 dashboards create -f persesdashboard.yaml
Dashboard "My Perses Dashboard" created
```

`check-rules create` also accepts PrometheusRule CRD files.
Each alerting rule in the CRD is created as a separate check rule (recording rules are skipped).
The check rule name is composed as `<group name> - <alert name>`, matching the name produced by the Dash0 Kubernetes operator and the Terraform provider for the same CRD:

```bash
$ dash0 check-rules create -f prometheus-rules.yaml
Check rule "High Error Rate Alert" created
```

Aliases: `add`

### `update`

Update an existing asset from a YAML or JSON file.
If the ID argument is omitted, the ID is extracted from the file content.
The output shows a unified diff of what changed.
The `--dry-run` flag shows the diff without applying the update.

```bash
dash0 dashboards update [id] -f <file> [--dry-run]
```

Update a dashboard from a file:

```bash
$ dash0 dashboards update <id> -f dashboard.yaml
--- Dashboard (before)
+++ Dashboard (after)
@@ -2,7 +2,7 @@
 spec:
   display:
-    name: Old Dashboard Name
+    name: New Dashboard Name
```

Preview changes without applying:

```bash
$ dash0 dashboards update -f dashboard.yaml --dry-run
--- Dashboard (before)
+++ Dashboard (after)
@@ -2,7 +2,7 @@
 spec:
   display:
-    name: Old Dashboard Name
+    name: New Dashboard Name
```

When nothing changed:

```bash
$ dash0 dashboards update -f dashboard.yaml
Dashboard "My Dashboard": no changes
```

### `delete`

Delete an asset by ID.
Prompts for confirmation unless `--force` is passed.

```bash
dash0 dashboards delete <id> [--force]
```

Interactive deletion (prompts for confirmation):

```bash
$ dash0 dashboards delete a1b2c3d4-5678-90ab-cdef-1234567890ab
Are you sure you want to delete dashboard "a1b2c3d4-..."? [y/N]: y
Dashboard "a1b2c3d4-..." deleted
```

Non-interactive deletion (`--force` skips the prompt):

```bash
$ dash0 dashboards delete a1b2c3d4-5678-90ab-cdef-1234567890ab --force
Dashboard "a1b2c3d4-..." deleted
```

With `--force`, an already-deleted asset is treated as success (exit 0) — a short "already deleted" line is printed to stderr and the command returns without error.
This makes `delete --force` safe to run from CI/CD and agent-driven pipelines where the asset may have been removed concurrently by another actor:

```bash
$ dash0 dashboards delete a1b2c3d4-5678-90ab-cdef-1234567890ab --force
Dashboard "a1b2c3d4-5678-90ab-cdef-1234567890ab" was already deleted
$ echo $?
0
```

Without `--force`, a missing asset surfaces as a non-zero exit with a "not found" error.
The idempotent behavior applies uniformly to every `delete` subcommand (`dashboards`, `check-rules`, `synthetic-checks`, `views`, `recording-rules`, `notification-channels`, `spam-filters`, `teams`) as well as the `remove`-shaped variants (`members remove`, `teams remove-members`).
In the multi-target `remove` variants the check runs per member, so one concurrently-removed member does not fail the whole `--force` call — the loop keeps going.
See also the [Non-interactive deletion (for automation)](#non-interactive-deletion-for-automation) workflow.

Aliases: `remove`

### Asset type quick reference

| Asset type | Command | Notes |
|------------|---------|-------|
| Dashboards | `dash0 dashboards <subcommand>` | `create` also accepts PersesDashboard CRD files |
| Check rules | `dash0 check-rules <subcommand>` | `create` also accepts PrometheusRule CRD files |
| Synthetic checks | `dash0 synthetic-checks <subcommand>` | |
| Views | `dash0 views <subcommand>` | |
| Recording rules | `dash0 recording-rules <subcommand>` | Uses PrometheusRule CRD format |
| Notification channels | `dash0 notification-channels <subcommand>` | Organization-level (no `--dataset`) |
| Spam filters | `dash0 spam-filters <subcommand>` | Dataset-scoped; `create`/`update` accept v1alpha1 (`spec.contexts`) and v1alpha2 (`spec.context`) |
| Teams | `dash0 --experimental teams <subcommand>` | Organization-level (no `--dataset`). Experimental. `create` accepts `-f <file>` for a declarative `TeamDefinitionV1Alpha1` document, or a positional `<name>` for the imperative form. `spec.members` and `--member` accept either an email address or an internal member id; the server resolves emails. |

### Asset identifiers and idempotent upsert

Every asset type accepts a user-defined identifier in its YAML/JSON document.
When the identifier is present, both `create` and `apply` perform an **upsert** (PUT) against that identifier: the asset is created on the first call and replaced on every subsequent call.
When the identifier is absent, both commands perform a plain create (POST) and the server assigns a fresh ID on each invocation — repeated runs produce duplicate assets.

Pinning a stable identifier in the source document is the supported way to make `apply` and `create` idempotent, which is what GitOps and CI/CD workflows need.

The identifier field location varies by asset kind:

| Kind | Identifier field | Notes |
|------|------------------|-------|
| `Dashboard` | `metadata.dash0Extensions.id` | |
| `PersesDashboard` | `metadata.labels["dash0.com/id"]` | |
| `CheckRule` | top-level `id` | |
| `PrometheusRule` (alerting rules) | `metadata.labels["dash0.com/id"]` | The CRD-level label is applied to every alerting rule converted from the CRD, so a CRD with multiple alerts shares one identifier — pin a unique label per CRD, or split multi-alert CRDs into one CRD per alert |
| `PrometheusRule` (recording rules) | `metadata.labels["dash0.com/id"]` | |
| `SyntheticCheck` | `metadata.labels["dash0.com/id"]` | |
| `View` | `metadata.labels["dash0.com/id"]` | |
| `Dash0SpamFilter` (v1alpha1 and v1alpha2) | `metadata.labels["dash0.com/id"]` | `metadata.labels["dash0.com/origin"]` is preferred over the ID when both are present; an ID-only filter is not fully idempotent because the server reassigns the ID on the first PUT |
| `Dash0NotificationChannel` | `metadata.labels["dash0.com/origin"]` | There is no user-settable ID field for notification channels — the origin label is the upsert key. A document without it creates a new channel on every apply |
| `Dash0Team` | `metadata.labels["dash0.com/origin"]` | Organization-level. Same origin-based-upsert semantics as notification channels: a document with `dash0.com/origin` upserts by that origin (PUT), a document without it creates a new team on every apply (POST). `spec.members` accepts email addresses or internal member ids interchangeably |

The `dash0.com/id` label is the user-defined external identifier and is distinct from `dash0.com/origin`, which records the system of record (`dash0-cli`, `terraform`, `ui`).
The CLI strips `dash0.com/origin` from outbound payloads for the asset types where the server treats origin as provenance metadata (dashboards, views, check rules, synthetic checks), so do not use origin as the upsert key for those kinds.
Notification channels and spam filters are the two exceptions: their server APIs key on origin, and the CLI preserves it accordingly.

When `list -o yaml` or `get -o yaml` exports an existing asset, the server-assigned ID is rendered into the correct field, so the export-edit-reapply workflow round-trips through the identifier automatically.

### `apply`

Apply asset definitions from a file, directory, or stdin.
If an asset already exists (matched by ID), it is updated; otherwise it is created.

```bash
dash0 apply -f <file|directory> [--dry-run]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--file` | `-f` | Path to a YAML/JSON file, a directory, or `-` for stdin |
| `--dry-run` | | Validate without applying |

For assets that are updated, a unified diff of the changes is shown.
Assets that are created show the standard creation message.

When a directory is specified, all `.yaml` and `.yml` files are discovered recursively.
Hidden files and directories (starting with `.`) are skipped.
All documents are validated before any are applied.
If any document fails validation, no changes are made.

Supported `kind` values: `Dashboard`, `PersesDashboard`, `CheckRule`, `PrometheusRule`, `SyntheticCheck`, `View`, `Dash0SpamFilter`, `Dash0NotificationChannel`, `Dash0Team`.
A single file may contain multiple documents separated by `---`.

For `Dash0SpamFilter`, the `apiVersion` field on the document selects the schema (`v1alpha1` or `v1alpha2`); a missing value defaults to `v1alpha1`.
An unknown value fails validation up front, before any document is applied.

For `PrometheusRule`, `apply` inspects each rule entry and dispatches by type.
Alerting rules (entries with `alert:`) are sent to the check-rule endpoint as one check rule per alert, named `<group name> - <alert name>` to match the Dash0 Kubernetes operator and the Terraform provider.
Recording rules (entries with `record:`) are sent to the recording-rule endpoint as a single PrometheusRule CRD with the alerting rules removed.
A CRD that mixes both kinds is dispatched to both endpoints in a single apply.
A CRD that contains no alerting and no recording rules fails validation up front.

`Dash0NotificationChannel` documents are dispatched to the organization-level notification-channels endpoint and are not associated with a dataset.
The `dash0.com/origin` label is the upsert key when present; otherwise the server assigns a fresh ID on each apply.

`Dash0Team` documents are dispatched to the organization-level teams endpoint (also not associated with a dataset).
The `dash0.com/origin` label is the upsert key when present; otherwise the server creates a new team on every apply.
`spec.members` accepts either email addresses (recommended for GitOps) or internal member ids; the server resolves emails during reconciliation and rejects unresolvable ones with a single 400 listing every offender.
Requires `--experimental`.

> [!NOTE]
> The `-f` flag accepts a single path.
> Do not use shell glob patterns like `-f assets/*` — the shell expands the glob into multiple arguments and only the first file is passed to `-f`.
> Use `-f assets/` (the directory) instead.

Apply a single file:

```bash
$ dash0 apply -f dashboard.yaml
Dashboard "Production Overview" (a1b2c3d4-...) created
```

Apply a directory recursively:

```bash
$ dash0 apply -f assets/
assets/dashboard.yaml: Dashboard "Production Overview" (a1b2c3d4-...) created
assets/rule.yaml: Check rule "High Error Rate" (b2c3d4e5-...) updated
...
```

Apply from stdin (the `cat … | dash0 apply -f -` pipeline counts as one command):

```bash
$ cat assets.yaml | dash0 apply -f -
Dashboard "Production Overview" (a1b2c3d4-...) created
...
```

Dry-run validation:

```bash
$ dash0 apply -f assets.yaml --dry-run
Dry run: 1 document validated
  1. Dashboard "Production Overview" (a1b2c3d4-5678-90ab-cdef-1234567890ab)
```

### Asset YAML formats

Dashboard:

```yaml
kind: Dashboard
metadata:
  name: a1b2c3d4-5678-90ab-cdef-1234567890ab
  dash0Extensions:
    id: a1b2c3d4-5678-90ab-cdef-1234567890ab
spec:
  display:
    name: Production Overview
```

The user-defined identifier lives in `metadata.dash0Extensions.id` (see [asset identifiers](#asset-identifiers-and-idempotent-upsert)).
`metadata.name` is the CRD-style metadata name and is independent of `spec.display.name`, which is the title shown in the UI.
Omitting `dash0Extensions.id` causes every `create` / `apply` to allocate a new server-assigned ID, breaking idempotency.

PersesDashboard (Perses CRD, converted to a Dashboard on import):

```yaml
apiVersion: perses.dev/v1alpha1
kind: PersesDashboard
metadata:
  name: my-perses-dashboard
  labels:
    dash0.com/id: a1b2c3d4-5678-90ab-cdef-1234567890ab
spec:
  display:
    name: My Perses Dashboard
  duration: 5m
  panels: {}
```

For PersesDashboards the user-defined identifier is the `dash0.com/id` metadata label (see [asset identifiers](#asset-identifiers-and-idempotent-upsert)).

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

Recording rule (PrometheusRule CRD format):

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: My Recording Rules
  labels:
    dash0.com/id: e5f6a7b8-9012-34ef-0123-567890abcdef
spec:
  groups:
    - name: cpu-averages
      interval: 1m
      rules:
        - record: instance:cpu_usage:avg5m
          expr: avg without(cpu) (rate(node_cpu_seconds_total{mode!="idle"}[5m]))
```

Spam filter (v1alpha1 — `spec.contexts` is an array of signal types):

```yaml
apiVersion: v1alpha1
kind: Dash0SpamFilter
metadata:
  name: Drop noisy health checks
  labels:
    dash0.com/id: a6b7c8d9-0123-45a6-7890-cdef01234567
spec:
  contexts:
    - log
    - span
  filter:
    - key: http.target
      operator: ends_with
      value: /healthz
```

Spam filter (v1alpha2 — `spec.context` is a single signal type):

```yaml
apiVersion: v1alpha2
kind: Dash0SpamFilter
metadata:
  name: Drop debug logs
  labels:
    dash0.com/id: b7c8d9e0-1234-56b7-8901-def012345678
spec:
  context: log
  filter:
    - key: otel.log.severity.range
      operator: is
      value: DEBUG
```

The CLI detects the apiVersion from the document and routes to the corresponding endpoint.
The `list` endpoint returns v1alpha1 definitions only; use `spam-filters get <id>` to retrieve a filter in its native apiVersion.
The `delete` endpoint is version-agnostic.

Notification channel (organization-level, no `--dataset`):

```yaml
kind: Dash0NotificationChannel
metadata:
  name: Slack Alerts
  labels:
    dash0.com/origin: my-slack-channel
spec:
  type: slack
  config:
    url: https://hooks.slack.com/services/T00/B00/XXX
```

Team (organization-level, no `--dataset`, requires `--experimental`):

```yaml
kind: Dash0Team
metadata:
  name: backend-team
  labels:
    dash0.com/origin: backend-team
spec:
  display:
    name: Backend Team
    color:
      from: "#FF6B6B"
      to: "#4ECDC4"
  members:
    - alice@example.com
    - bob@example.com
```

`spec.members` entries can be email addresses (recommended for GitOps) or internal member ids; the server resolves emails during reconciliation.

Mixed PrometheusRule (one alerting rule and one recording rule in the same CRD):

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: Mixed Rules
  labels:
    dash0.com/id: c8d9e0f1-2345-67c8-9012-ef0123456789
spec:
  groups:
    - name: errors-and-cpu
      interval: 1m
      rules:
        - alert: HighErrorRate
          expr: sum(rate(errors[5m])) > 0.1
          for: 5m
          labels:
            severity: critical
        - record: instance:cpu_usage:avg5m
          expr: avg without(cpu) (rate(node_cpu_seconds_total{mode!="idle"}[5m]))
```

The alerting rule is created as a check rule under `/api/alerting/check-rules`, and a recording-rule CRD with only the recording rule is created under `/api/recording-rules`.

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

## Query commands

Query commands search and retrieve telemetry signals and alerting issues from Dash0.
They share a common set of characteristics:
- Time range flags: `--from` and `--to` (relative expressions like `now-1h` or absolute ISO 8601 timestamps).
- Filter flag: `--filter` with the standard `key [operator] value` syntax (see [filter syntax](#filter-syntax)).
- Column flag: `--column` for customizing table/CSV output (see [custom columns](#custom-columns)).
- Pagination: `--limit`.
- Output formats: `table`, `json`, `csv` (no `wide` or `yaml`).
- Sampling flag: `--precision` to disable [adaptive sampling](#precision-mode-adaptive-sampling) on `logs query` and `spans query` (`traces get` always disables it; `metrics instant` and `failed-checks query` do not honor it).

### `logs query`

Query log records from Dash0.
Requires `api-url` and `auth-token`.

```bash
dash0 logs query [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--from` | `now-15m` | Start of time range |
| `--to` | `now` | End of time range |
| `--limit` | 50 | Maximum number of records |
| `--filter` | | Filter expression (repeatable); accepts text (`key [operator] value`) or JSON from the Dash0 UI |
| `-o` | `table` | Output format: `table`, `json` (OTLP/JSON), or `csv` |
| `--skip-header` | `false` | Omit the header row from `table` and `csv` output |
| `--column` | | Column to display (repeatable; `table` and `csv` only); see [custom columns](#custom-columns) |
| `--precision` | | Sampling mode for the query: `adaptive` (server default) or `disabled` (return every match). See [Precision Mode](#precision-mode-adaptive-sampling) |

Both `--from` and `--to` accept relative expressions like `now-1h` or absolute ISO 8601 timestamps.
Absolute timestamps are normalized to millisecond precision, so `2024-01-25T10:00:00Z` and `2024-01-25` are both accepted.

Query recent logs (last 15 minutes, up to 50 records):

```bash
$ dash0 logs query
TIMESTAMP                     SEVERITY    BODY
2026-02-16T09:12:03.456Z      INFO        Application started successfully
2026-02-16T09:12:04.789Z      ERROR       Connection timeout
...
```

Query with a time range:

```bash
dash0 logs query --from now-1h --to now --limit 100
```

Filter by service:

```bash
dash0 logs query --filter "service.name is my-service"
```

Filter by severity (errors and above):

```bash
dash0 logs query --filter "otel.log.severity.number gte 17"
```

Multiple filters (AND logic):

```bash
dash0 logs query \
    --filter "service.name is my-service" \
    --filter "otel.log.severity.range is_one_of ERROR WARN"
```

Use JSON filter criteria copied from the Dash0 UI:

```bash
dash0 logs query \
    --filter '[{"key":"service.name","operator":"is","value":"api"}]'
```

Output as JSON (full OTLP payload):

```bash
dash0 logs query -o json
```

Output as CSV (pipe-friendly):

```bash
$ dash0 logs query -o csv
otel.log.time,otel.log.severity.range,otel.log.body
2026-02-16T09:12:03.456Z,INFO,Application started successfully
...
```

CSV without header:

```bash
dash0 logs query -o csv --skip-header
```

Disable adaptive sampling so a narrow filter always returns every match:

```bash
$ dash0 logs query --precision disabled --filter "test.id is <id>"
```

### `spans query`

Query spans from Dash0.
Requires `api-url` and `auth-token`.

```bash
dash0 spans query [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--from` | `now-15m` | Start of time range |
| `--to` | `now` | End of time range |
| `--limit` | 50 | Maximum number of spans |
| `--filter` | | Filter expression (repeatable); accepts text (`key [operator] value`) or JSON from the Dash0 UI |
| `-o` | `table` | Output format: `table`, `json` (OTLP/JSON), or `csv` |
| `--skip-header` | `false` | Omit the header row from `table` and `csv` output |
| `--column` | | Column to display (repeatable; `table` and `csv` only); see [custom columns](#custom-columns) |
| `--precision` | | Sampling mode for the request: `adaptive` (server default) or `disabled` (return every matching span). See [Precision Mode](#precision-mode-adaptive-sampling) |

Both `--from` and `--to` accept relative expressions like `now-1h` or absolute ISO 8601 timestamps.

Query recent spans (last 15 minutes, up to 50 spans):

```bash
$ dash0 spans query
TIMESTAMP                     DURATION    SPAN NAME                                 STATUS    SERVICE NAME                    PARENT ID         TRACE ID                          SPAN LINKS
2026-02-16T09:12:03.456Z      150ms       GET /api/users                            OK        my-service                                        0af76519...
2026-02-16T09:12:04.789Z      500ms       POST /api/orders                          ERROR     api-gateway                     b7ad6b7169203331  3d3d3d3d...
...
```

Query with a time range:

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

Use JSON filter criteria copied from the Dash0 UI:

```bash
dash0 spans query \
    --filter '[{"key":"service.name","operator":"is","value":"api"}]'
```

Output as CSV:

```bash
$ dash0 spans query -o csv
otel.span.start_time,otel.span.duration,otel.span.name,otel.span.status.code,service.name,otel.parent.id,otel.trace.id,otel.span.links
2026-02-16T09:12:03.456Z,150ms,GET /api/users,OK,my-service,,0af76519...,
...
```

Output as OTLP JSON:

```bash
dash0 spans query -o json --limit 10
```

The `--filter` flag uses the same [filter syntax](#filter-syntax) as `logs query`.
Common span attribute keys: `service.name`, `otel.span.status.code`, `otel.trace.id`, `otel.span.name`.

### `traces get`

Retrieve all spans belonging to a trace from Dash0.
Requires `api-url` and `auth-token`.

`traces get` always disables [adaptive sampling](#precision-mode-adaptive-sampling) so every span in the trace is returned.
The command does not accept the `--precision` flag.

```bash
dash0 traces get <trace-id> [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--from` | `now-1h` | Start of time range |
| `--to` | `now` | End of time range |
| `-o` | `table` | Output format: `table`, `json` (OTLP/JSON), or `csv` |
| `--skip-header` | `false` | Omit the header row from `table` and `csv` output |
| `--follow-span-links` | | Follow span links to related traces; optional value sets the lookback period (default: `1h`) |
| `--column` | | Column to display (repeatable; `table` and `csv` only); see [custom columns](#custom-columns) |

The `<trace-id>` argument must be 32 hex characters.

In table format, spans are displayed as a hierarchical tree with child spans indented under their parents.

Get all spans in a trace:

```bash
$ dash0 traces get <trace-id>
TIMESTAMP                     DURATION    TRACE ID                          SPAN ID           PARENT ID         SPAN NAME                                   STATUS    SERVICE NAME                    SPAN LINKS
2026-02-16T09:12:03.456Z      200ms       0af7651916cd43dd8448eb211c80319c  b7ad6b7169203331                    GET /api/users                              OK        frontend
2026-02-16T09:12:03.486Z      150ms       0af7651916cd43dd8448eb211c80319c  00f067aa0ba902b7  b7ad6b7169203331  SELECT * FROM users                         UNSET     frontend
2026-02-16T09:12:03.641Z      10ms        0af7651916cd43dd8448eb211c80319c  123456789abcdef0  b7ad6b7169203331  serialize response                          UNSET     frontend
```

Get a trace with a specific time range:

```bash
dash0 traces get <trace-id> --from now-2h
```

Follow span links to related traces:

```bash
dash0 traces get <trace-id> --follow-span-links
```

Follow span links with a custom lookback period:

```bash
dash0 traces get <trace-id> --follow-span-links 2h
```

Output as OTLP JSON:

```bash
dash0 traces get <trace-id> -o json
```

Output as CSV:

```bash
$ dash0 traces get <trace-id> -o csv
otel.trace.id,otel.span.start_time,otel.span.duration,otel.span.id,otel.parent.id,otel.span.name,otel.span.status.code,service.name,otel.span.links
0af7651916cd43dd8448eb211c80319c,2026-02-16T09:12:03.456Z,200ms,b7ad6b7169203331,,GET /api/users,OK,frontend,
...
```

When `--follow-span-links` is used, linked traces are displayed after the primary trace, separated by a header line showing the linked trace ID.
The command follows links recursively up to a maximum of 20 traces.

### `metrics instant`

Run an instant PromQL query against the Dash0 API, returning a single datapoint per time series.

With a raw PromQL expression:

```bash
dash0 metrics instant --promql <promql> [--from <timestamp>] [--dataset <dataset>] [-o <format>]
```

With one or more `--filter` expressions (translated to PromQL label matchers):

```bash
dash0 metrics instant --filter <filter> [--from <timestamp>] [--dataset <dataset>] [-o <format>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--promql` | | PromQL query expression; mutually exclusive with `--filter` |
| `--filter` | | Filter as `key [operator] value`, translated to PromQL label matchers (repeatable); mutually exclusive with `--promql` |
| `--from` | `now` | Evaluation timestamp; supports relative expressions like `now-1h` or absolute ISO 8601 timestamps |
| `--dataset` | | Dataset to query |
| `-o` | `table` | Output format: `table`, `json`, or `csv` (default: `json` in agent mode) |
| `--skip-header` | `false` | Omit the header row from `table` and `csv` output |
| `--column` | | Column to display (repeatable; `table` and `csv` only); see below |

At least one of `--promql` or `--filter` must be specified.

The `--query` flag (alias for `--promql`) and `--time` flag (alias for `--from`) are deprecated but still accepted for backwards compatibility.

#### Output formats

The default `table` output uses a verbose label-per-line format showing all labels for each metric result.
Use `--column` to switch to a columnar table format with specific columns.

The `csv` output defaults to columns: `timestamp`, `__name__`, `value`.
Use `--column` to override the default columns.

When no `--column` is specified, a hint listing the available label keys is printed to stderr.

The `--column` flag accepts any Prometheus label key from the result (e.g., `__name__`, `service_name`, `job`, `instance`).
Built-in columns `timestamp` and `value` are always available.
The `--column` flag is not supported with `-o json`.

> [!NOTE]
> The column `otel_metric_name` is not available in Prometheus query results.
> Use `__name__` for the Prometheus-normalized metric name.

#### Filter syntax

The `--filter` flag uses the same [filter syntax](#filter-syntax) as `logs query`.
Filters are translated to PromQL label matchers.
For example, `--filter "service.name is my-service"` becomes `{service_name="my-service"}`.

OTel attribute keys are normalized to Prometheus label names (dots replaced with underscores).

#### Examples

Query the current request rate per service:

```bash
dash0 metrics instant --promql 'sum by (service_name) (rate(http_server_request_duration_seconds_count[5m]))'
```

Query against a specific dataset:

```bash
dash0 metrics instant --promql 'sum(rate(http_server_request_duration_seconds_count[5m]))' --dataset production
```

Query at a specific evaluation time:

```bash
dash0 metrics instant --promql 'sum by (service_name) (rate(http_server_request_duration_seconds_count[5m]))' --from 2024-01-25T10:00:00Z
```

Query with filters instead of PromQL:

```bash
dash0 metrics instant --filter 'service.name is my-service'
```

Output as CSV:

```bash
dash0 metrics instant --promql 'sum by (service_name) (rate(http_server_request_duration_seconds_count[5m]))' -o csv
```

Output as CSV without header:

```bash
dash0 metrics instant --promql 'sum by (service_name) (rate(http_server_request_duration_seconds_count[5m]))' -o csv --skip-header
```

Select specific columns:

```bash
dash0 metrics instant --promql 'sum by (service_name) (rate(http_server_request_duration_seconds_count[5m]))' --column value --column service_name
```

Output as JSON:

```bash
dash0 metrics instant --promql 'sum by (service_name) (rate(http_server_request_duration_seconds_count[5m]))' -o json
```

### `failed-checks query`

Query failed check instances (active or recently resolved issues raised by check rules) from Dash0 alerting.
Requires `api-url` and `auth-token`.

```bash
dash0 failed-checks query [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--from` | `now-15m` | Start of time range |
| `--to` | `now` | End of time range |
| `--limit` | 50 | Maximum number of failed checks |
| `--filter` | | Filter expression (repeatable); accepts text (`key [operator] value`) or JSON from the Dash0 UI |
| `--status` | | Filter by instance status: comma-separated list of `critical`, `degraded`, `healthy`, `inactive`, `pending` |
| `--active` | `false` | Only show currently active (unresolved) issues |
| `-o` | `table` | Output format: `table`, `json`, or `csv` |
| `--skip-header` | `false` | Omit the header row from `table` and `csv` output |
| `--column` | | Column to display (repeatable; `table` and `csv` only); see [custom columns](#custom-columns) |

The `--status` and `--active` flags are convenience shortcuts that translate into the equivalent `--filter` expressions on the well-known issue attributes the alerting API exposes (`dash0.issue.status` and `dash0.issue.end_time`).
Custom labels — for example a team-defined `priority` or `owner` label — must be filtered through `--filter`.

Aliases: `list`, `ls`.

List currently active (unresolved) issues:

```bash
$ dash0 failed-checks query --active
CHECK RULE                                     STATUS      STARTED              SUMMARY
BLOCK DEPLOYMENTS - please update reason ...   critical    2026-06-19 13:10:54  BLOCK DEPLOYMENTS due to XYZ
...
```

Filter by instance status:

```bash
dash0 failed-checks query --status critical,degraded
```

Filter by an arbitrary issue label (`priority`, `owner`, or any other label set on the originating check rule):

```bash
dash0 failed-checks query --filter "priority is_one_of p1 p2" --active
```

Issues from the last hour (active and resolved):

```bash
dash0 failed-checks query --filter "priority is p1" --from now-1h
```

Use JSON filter criteria copied from the Dash0 UI:

```bash
dash0 failed-checks query \
    --filter '[{"key":"priority","operator":"is","value":"p1"}]'
```

Output as JSON:

```bash
dash0 failed-checks query --active -o json
```

Output as CSV:

```bash
$ dash0 failed-checks query --status critical --active -o csv
dash0.issue.check_rule_name,dash0.issue.status,dash0.issue.start_time,dash0.issue.summary
BLOCK DEPLOYMENTS - please update reason ...,critical,2026-06-19 13:10:54,BLOCK DEPLOYMENTS due to XYZ
...
```

Show only specific columns:

```bash
dash0 failed-checks query \
    --column "check rule" --column status --column summary
```

Any issue label key (for example `priority` or `owner`) can be used directly as a column name.
Add a `PRIORITY` column derived from the `priority` issue label:

```bash
$ dash0 failed-checks query --active --column "check rule" --column priority --column status --column summary
CHECK RULE                                     priority  STATUS      SUMMARY
BLOCK DEPLOYMENTS - please update reason ...   p1        critical    BLOCK DEPLOYMENTS due to XYZ
...
```

The header for a label-based column defaults to the label key as-is.
To override it, combine `--column` with `--skip-header` and render the header separately, or use the JSON output and post-process.

Column aliases (case-insensitive): `check rule`/`rule` → `dash0.issue.check_rule_name`, `status` → `dash0.issue.status`, `started`/`start` → `dash0.issue.start_time`, `ended`/`end` → `dash0.issue.end_time`, `summary` → `dash0.issue.summary`, `description` → `dash0.issue.description`, `id` → `dash0.issue.id`, `identifier` → `dash0.issue.identifier`, `check rule id`/`rule id` → `dash0.issue.check_rule_id`.

### Filter syntax

The `--filter` flag accepts expressions in the form `key [operator] value`.
When the operator is omitted, `is` (exact match) is assumed.

The flag also accepts JSON filter criteria as produced by the Dash0 UI "copy filter criteria" feature.
A JSON array of filter objects is expanded into multiple filters; a single JSON object is treated as one filter.
For example:

```bash
# Paste a JSON array copied from the Dash0 UI
$ dash0 logs query --filter '[
  {"key": "service.name", "operator": "is", "value": "api"},
  {"key": "otel.log.severity.range", "operator": "is_one_of", "values": ["ERROR", "WARN"]}
]'
```

JSON filters and text filters can be mixed freely across multiple `--filter` flags.

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

### Precision mode (adaptive sampling)

By default the Dash0 API applies [adaptive sampling](https://dash0.com/docs/dash0/miscellaneous/glossary/adaptive-sampling) to log and span queries.
The sampler intelligently samples telemetry data during query execution to keep queries fast on large datasets, while returning statistically representative results.
All telemetry data is stored and available; adaptive sampling only affects how queries are executed, not what data is kept.

The representative-but-incomplete trade-off is fine for exploration, but for narrow lookups that expect every match — scripted lookups by trace ID, request ID, or any other near-unique attribute — a single matching log or span can be omitted from the response.

Pass `--precision disabled` to `logs query` or `spans query` to switch the request to **Precision mode**, the API equivalent of the [Precision toggle](https://dash0.com/docs/dash0/miscellaneous/glossary/adaptive-sampling) in the Dash0 UI.
Precision mode keeps queries deterministic at the cost of higher latency on large windows.

The flag accepts either value of the API's sampling mode:

- `--precision disabled` — disable adaptive sampling and return every match.
- `--precision adaptive` — explicit form of the default; useful when overriding a profile or environment-set default in scripts.

When the flag is omitted, the request leaves the sampling field unset and the server applies its default (currently `adaptive`).

```bash
# Narrow lookup: always return every matching log
dash0 logs query --precision disabled --filter "test.id is <id>"
```

```bash
# Narrow lookup: always return every matching span
dash0 spans query --precision disabled --filter "test.id is <id>"
```

`traces get` does not accept `--precision` — it always disables adaptive sampling so the complete trace, including any spans that would otherwise be sampled out, is returned.

The flag is not available on `metrics instant`: the Prometheus-compatible API the command uses does not honor the sampling field.

### Custom columns

The `--column` flag lets you choose which columns appear in `table` and `csv` output.
It is repeatable: pass one `--column` per column.
When used, the flag replaces the default column set entirely.

Show only timestamp and body:

```bash
dash0 logs query --column time --column body
```

Include an arbitrary attribute column:

```bash
dash0 spans query \
    --column timestamp --column duration \
    --column "span name" --column http.request.method
```

Include an arbitrary attribute column by key:

```bash
dash0 logs query --column time --column service.name --column body
```

Each command has predefined columns with short aliases.
Aliases are matched case-insensitively.
Any OTLP attribute key (resource, scope, or record/span level) can also be used as a column; its header defaults to the attribute key as-is.

**Log query aliases:**

| Alias | Attribute key |
|-------|---------------|
| `time`, `timestamp` | `otel.log.time` |
| `severity` | `otel.log.severity.range` |
| `body` | `otel.log.body` |

**Span query aliases:**

| Alias | Attribute key |
|-------|---------------|
| `timestamp`, `start time`, `time` | `otel.span.start_time` |
| `duration` | `otel.span.duration` |
| `span name`, `name` | `otel.span.name` |
| `status`, `status code` | `otel.span.status.code` |
| `service name`, `service` | `service.name` |
| `parent id` | `otel.parent.id` |
| `trace id` | `otel.trace.id` |
| `span id` | `otel.span.id` |
| `span links`, `links` | `otel.span.links` |

**Trace get aliases** share the span query aliases.

**Failed-checks query aliases:**

| Alias | Attribute key |
|-------|---------------|
| `check rule`, `rule` | `dash0.issue.check_rule_name` |
| `status` | `dash0.issue.status` |
| `started`, `start`, `start time` | `dash0.issue.start_time` |
| `ended`, `end`, `end time` | `dash0.issue.end_time` |
| `summary` | `dash0.issue.summary` |
| `description` | `dash0.issue.description` |
| `id` | `dash0.issue.id` |
| `identifier` | `dash0.issue.identifier` |
| `check rule id`, `rule id` | `dash0.issue.check_rule_id` |

Any issue label key (e.g. `priority`, `owner`, or any custom label set on the originating check rule) can be used directly as a column name.
For example, `--column priority` renders a column whose values come from the `priority` issue label.

Aliases that contain spaces must be quoted: `--column "start time"`.

The `--column` flag is not supported with JSON output.
Using `--column` with `-o json` returns an error.

## Send commands

Send commands transmit telemetry data to Dash0 via OTLP.
They share a common set of characteristics:
- Require `otlp-url` and `auth-token` (not `api-url`).
- Repeatable attribute flags: `--resource-attribute`, `--scope-attribute`, and a signal-specific attribute flag.
- OTLP scope flags: `--scope-name` (default: `dash0-cli`), `--scope-version` (default: CLI version).

> [!IMPORTANT]
> The Dash0 OTLP ingress does not accept OAuth access tokens — it only honors static `auth_*` tokens.
> Send commands running against an OAuth-typed profile fail upfront with a clear error instead of producing a generic 401 from the server.
> Workarounds, in order of least-invasive:
> 1. Pass `--auth-token auth_<...>` for the single invocation.
> 2. Set `DASH0_AUTH_TOKEN=auth_<...>` in the environment (shadows the profile for any one command).
> 3. Convert the profile to a static token with `dash0 config profiles update <name> --oauth=false --auth-token auth_<...> --force`.

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

Simple log message:

```bash
$ dash0 logs send "Application started"
Log record sent
```

Log with severity and attributes:

```bash
$ dash0 logs send "Application started" \
    --resource-attribute service.name=my-service \
    --log-attribute user.id=12345 \
    --severity-text INFO --severity-number 9
Log record sent
```

Deployment event with event name:

```bash
$ dash0 logs send "Deployment completed" \
    --event-name dash0.deployment \
    --severity-number 9 \
    --resource-attribute service.name=my-service \
    --resource-attribute deployment.environment.name=production \
    --log-attribute deployment.status=succeeded
Log record sent
```

Using environment variables for connection (env-var prefix counts as part of a single command per the [code block rules](documentation.md#code-blocks)):

```bash
$ DASH0_OTLP_URL=https://ingress.us-west-2.aws.dash0.com \
  DASH0_AUTH_TOKEN=auth_xxx \
  dash0 logs send "Health check passed" \
    --severity-number 9 --severity-text INFO
Log record sent
```

### `spans send`

Send a span to Dash0 via OTLP.
Requires `otlp-url` and `auth-token`.

```bash
dash0 spans send --name <name> [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--name` | | Span name (required) |
| `--kind` | `INTERNAL` | Span kind: `INTERNAL`, `SERVER`, `CLIENT`, `PRODUCER`, `CONSUMER` |
| `--status-code` | `UNSET` | Status code: `UNSET`, `OK`, `ERROR` |
| `--status-message` | | Status message (typically for ERROR status) |
| `--start-time` | now | Start timestamp in RFC3339 format |
| `--end-time` | | End timestamp in RFC3339 format; mutually exclusive with `--duration` |
| `--duration` | | Span duration (e.g., `100ms`, `1.5s`); mutually exclusive with `--end-time` |
| `--trace-id` | auto | Trace ID (32 hex characters); auto-generated if omitted |
| `--span-id` | auto | Span ID (16 hex characters); auto-generated if omitted |
| `--parent-span-id` | | Parent span ID (16 hex characters) |
| `--resource-attribute` | | Resource attribute as `key=value` (repeatable) |
| `--span-attribute` | | Span attribute as `key=value` (repeatable) |
| `--span-link` | | Span link as `trace-id:span-id[,key=value,...]` (repeatable) |
| `--scope-name` | `dash0-cli` | Instrumentation scope name |
| `--scope-version` | CLI version | Instrumentation scope version |
| `--scope-attribute` | | Instrumentation scope attribute as `key=value` (repeatable) |

Send a simple span:

```bash
$ dash0 spans send --name "my-operation"
Span sent (trace-id: 0af7651916cd43dd8448eb211c80319c, span-id: b7ad6b7169203331)
```

Send a server span with duration:

```bash
$ dash0 spans send --name "GET /api/users" \
    --kind SERVER --status-code OK --duration 100ms \
    --resource-attribute service.name=my-service
Span sent (trace-id: ..., span-id: ...)
```

Send a span with a link to another trace:

```bash
dash0 spans send --name "process-message" \
    --kind CONSUMER \
    --span-link 0af7651916cd43dd8448eb211c80319c:b7ad6b7169203331
```

Send a child span with explicit parent:

```bash
dash0 spans send --name "db-query" \
    --kind CLIENT \
    --trace-id 0af7651916cd43dd8448eb211c80319c \
    --parent-span-id b7ad6b7169203331
```

## Daemon commands

Daemon commands run as long-lived foreground processes and exit on `SIGINT` or `SIGTERM` rather than after a single operation.
They write a startup banner to stderr, accept traffic until signaled, then drain in-flight work within a bounded deadline before exiting.

### `otlp proxy` (experimental)

Run a local OTLP forwarder that accepts OTLP/HTTP and OTLP/gRPC traffic on `127.0.0.1` and forwards every batch to Dash0 using the active profile's credentials.
Requires the `-X` (or `--experimental`) flag.

The proxy is a local-dev shortcut for the common "I want telemetry in Dash0 right now while iterating on my code" loop.
An OpenTelemetry SDK at default endpoint configuration (`OTEL_EXPORTER_OTLP_ENDPOINT` unset or pointing at `127.0.0.1:4318` / `4317`) connects with no extra setup.

> [!NOTE]
> The proxy is not a replacement for the OpenTelemetry Collector.
> It does not buffer outbound on Dash0 outages.
> Backpressure surfaces to SDKs as HTTP 503 or gRPC `UNAVAILABLE` with `Retry-After` honored by the SDK.

```bash
dash0 -X otlp proxy [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--http-port` | 4318 | TCP port for the OTLP/HTTP listener |
| `--grpc-port` | 4317 | TCP port for the OTLP/gRPC listener |
| `--tail` | false | Print every forwarded record on stdout in collector-debug-exporter style (incompatible with `--agent-mode`) |
| `--resource-attribute` | | Resource attribute as `key=value` to upsert into every forwarded batch (repeatable) |
| `--scope-attribute` | | Instrumentation-scope attribute as `key=value` to upsert into every forwarded batch (repeatable) |
| `--scope-name` | | Instrumentation-scope name to set on every forwarded batch (default: preserve the SDK's value) |
| `--scope-version` | | Instrumentation-scope version to set on every forwarded batch (default: preserve the SDK's value) |
| `--log-attribute` | | Attribute as `key=value` to upsert on every forwarded log record (repeatable) |
| `--span-attribute` | | Attribute as `key=value` to upsert on every forwarded span (repeatable) |
| `--metric-attribute` | | Attribute as `key=value` to upsert on every forwarded metric data point (repeatable) |

#### Outbound decoration

The decoration flags upsert values onto every forwarded pdata batch at three levels:

- **Resource level** (`--resource-attribute`) — applied to each `ResourceLogs`, `ResourceSpans`, or `ResourceMetrics` resource. This is the level Dash0 indexes for filtering.
- **Scope level** (`--scope-attribute`, `--scope-name`, `--scope-version`) — applied to each instrumentation scope.
- **Record level** (`--log-attribute`, `--span-attribute`, `--metric-attribute`) — applied to every individual log record, span, or metric data point of the corresponding signal type. Per-signal flags don't cross signal boundaries: a `--log-attribute` never lands on a span.

The flag shape mirrors `dash0 logs send` and `dash0 spans send` so the user experience is consistent across signal-authoring and signal-forwarding workflows.

When a key collides with one the inbound SDK already set (e.g., `--resource-attribute service.name=foo` against an SDK-set `service.name`), the flag value wins.
Unlike the send commands, `--scope-name` and `--scope-version` default to empty — the proxy preserves the SDK's instrumentation-library identity unless the user explicitly overrides it.

The default ports follow the OTLP specification.
If either port is already in use, the proxy exits non-zero with an actionable error that names the holding process (resolved via `lsof` on Unix) and points at the `--http-port` / `--grpc-port` override:

```
HTTP port 4318 is already in use by "otelcol-contrib" (PID 12345)
  Stop that process, or pass --http-port <N> to use another port (cause: …)
```

A silent fallback to an OS-assigned port was considered but discarded — when an SDK is still pointed at the original default, the proxy starting on a different port produces an invisible "no telemetry" failure that's painful to debug.

#### Environment variables

| Variable | Description |
|----------|-------------|
| `DASH0_OTLP_PROXY_HTTP_PORT` | Override for `--http-port` |
| `DASH0_OTLP_PROXY_GRPC_PORT` | Override for `--grpc-port` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Parsed; routed to HTTP or gRPC by `OTEL_EXPORTER_OTLP_PROTOCOL` |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `grpc`, `http/protobuf`, or `http/json`; disambiguates the endpoint |

Precedence per port flag (high to low):

1. Explicit `--http-port` or `--grpc-port` on the command line
2. `DASH0_OTLP_PROXY_*`
3. `OTEL_EXPORTER_OTLP_ENDPOINT` (parsed; routed by `OTEL_EXPORTER_OTLP_PROTOCOL`)
4. Built-in default (4318 HTTP, 4317 gRPC)

#### Start banner

On startup the proxy writes a single banner line to stderr listing both endpoints, the active profile name, and the resolved dataset.

```
dash0 otlp proxy listening — http://127.0.0.1:4318 (OTLP/HTTP), 127.0.0.1:4317 (OTLP/gRPC) — profile: dev (dataset: default)
```

In TTY mode the banner is followed by a live per-signal stats block on stderr that updates once per second:

```
   logs:    42/s ▁▂▄▆▇ 1234 total
  spans:    18/s ▁▂▃▄▅  540 total
metrics:     0/s ▁▁▁▁▁    0 total
```

Each signal occupies its own line; labels, rates, sparklines, and totals all right-align so the eye can scan vertically.
The sparkline width adapts to the terminal: wider terminals show more of the 30-sample rolling history.
The block redraws in place via ANSI cursor controls.
When stderr is not a TTY (piped to a file or another process), the block is suppressed but lifecycle messages still print as plain lines.

#### Failure modes

The proxy implements **async-forward** semantics per the OTLP specification: an HTTP 200 / gRPC `OK` to the SDK means "accepted at this node" — the actual upstream forward happens asynchronously on the worker pool.

When the per-signal queue saturates (128 deep), the receiver returns HTTP 503 (or gRPC `UNAVAILABLE`) and the SDK retries with exponential backoff.
On upstream errors, the worker pool classifies the outcome:

| Classification | Triggers |
|----------------|----------|
| `upstream_unreachable` | Network errors, DNS failures, no response |
| `upstream_5xx` | Dash0 returns 500-599 |
| `upstream_4xx_auth` | Dash0 returns 401 or 403; surfaces a throttled stderr warning |
| `upstream_4xx_other` | Dash0 returns 400, 404, 422, etc. |
| `internal_panic` | A worker panicked (caught and emitted; the worker restarts) |

The first 401 or 403 from upstream writes a one-shot stderr warning ("authentication to Dash0 failed; check your profile").
Subsequent auth failures within 30 seconds are suppressed to avoid filling the terminal.

#### Agent mode

When `--agent-mode` is active, the proxy emits NDJSON OTLP/JSON event records on stdout instead of human-readable output.
Each record is one log record with a `dash0.cli.otlp_proxy.*` event name.
Stats redraw on stderr is suppressed and `--tail` is rejected with an error (agents already see batches through the structured event stream).

Event names and key attributes:

| `event_name` | Attributes |
|--------------|-----------|
| `dash0.cli.otlp_proxy.started` | `endpoint.http`, `endpoint.grpc`, `dataset`, `profile.name` |
| `dash0.cli.otlp_proxy.forwarded` | `signal` (`logs`, `spans`, `metrics`), `count`, `bytes` |
| `dash0.cli.otlp_proxy.stats` | `logs.rate`, `logs.total`, `logs.failed`, `spans.rate`, `spans.total`, `spans.failed`, `metrics.rate`, `metrics.total`, `metrics.failed` |
| `dash0.cli.otlp_proxy.error` | `error.kind` (per the failure-modes table), `reason`, `code` (HTTP status when available) |
| `dash0.cli.otlp_proxy.shutdown` | `reason` (`signal` or `deadline`), `final_total.logs`, `final_total.spans`, `final_total.metrics` |

Every event record carries the resource attributes `service.name="dash0-cli"` and `service.instance.id=<uuid>` so multiple proxy instances are distinguishable in the event stream.

#### Shutdown

The proxy listens for `SIGINT` (Ctrl-C) and `SIGTERM`.
On signal, it stops accepting new traffic, drains in-flight RPCs and worker queues within a 5-second deadline, emits the `shutdown` event with final cumulative totals, and exits zero.
A drain that hits the deadline still exits zero — the `reason` attribute on the `shutdown` event distinguishes the two cases.

Startup failures (missing profile, both listener ports unavailable) exit non-zero before the banner.

#### Examples

##### Just run it

SDK defaults already point at `127.0.0.1:4318` (HTTP) and `127.0.0.1:4317` (gRPC) so no environment variable change is required on the SDK side.

```bash
$ dash0 -X otlp proxy
dash0 otlp proxy listening — http://127.0.0.1:4318 (OTLP/HTTP), 127.0.0.1:4317 (OTLP/gRPC) — profile: dev (dataset: default)
   logs:    42/s ▁▂▄▆▇ 1234 total
  spans:    18/s ▁▂▃▄▅  540 total
metrics:     0/s ▁▁▁▁▁    0 total
```

##### Override the listener ports

```bash
# Move the HTTP listener; keep the gRPC default.
$ dash0 -X otlp proxy --http-port 8318

# Move both, e.g. when another local Collector is on 4317/4318.
$ dash0 -X otlp proxy --http-port 8318 --grpc-port 8317
```

If a port is already in use, the proxy fails with an actionable error that names the holder (resolved via `lsof` on Unix):

```
HTTP port 4318 is already in use by "otelcol-contrib" (PID 12345)
  Stop that process, or pass --http-port <N> to use another port (cause: …)
```

##### Watch the forwarded data

The `--tail` flag prints every forwarded record on stdout in the same style as the OpenTelemetry Collector's `debug` exporter — useful for verifying SDK output before going to the Dash0 UI.

```bash
$ dash0 -X otlp proxy --tail
dash0 otlp proxy listening — http://127.0.0.1:4318 (OTLP/HTTP), 127.0.0.1:4317 (OTLP/gRPC) — profile: dev (dataset: default)
ResourceLogs #0
Resource attributes:
  service.name: Str("frontend")
ScopeLogs #0
ScopeLogs SchemaURL:
InstrumentationScope dash0-cli v1
LogRecord #0
ObservedTimestamp: 2026-06-12T10:08:42.123Z
Timestamp: 2026-06-12T10:08:42.123Z
SeverityText: INFO
SeverityNumber: 9 (INFO)
Body: Str("Application started successfully")
...
```

##### Tag forwarded data for filtering

The decoration flags upsert attributes onto every forwarded batch at three levels: resource (filterable in Dash0), scope, and per-record.

```bash
# Add a developer-specific tag at the resource level so each developer's
# data is filterable in the Dash0 UI even on a shared backend.
$ dash0 -X otlp proxy \
    --resource-attribute developer=alice \
    --resource-attribute deployment.environment.name=local

# Tag the environment at the resource level (filterable) and mark every
# individual span and log as having flowed through your proxy.
$ dash0 -X otlp proxy \
    --resource-attribute deployment.environment.name=local \
    --span-attribute proxy.tagged=true \
    --log-attribute proxy.tagged=true \
    --metric-attribute proxy.tagged=true

# Override the instrumentation-scope identity on every forwarded batch.
# By default the proxy preserves the SDK's scope name and version; these
# flags explicitly overwrite them.
$ dash0 -X otlp proxy \
    --scope-name dash0-cli-otlp-proxy \
    --scope-version v1
```

Resource attribute keys collide with values the SDK already set; the flag's value wins:

```bash
# The SDK has set service.name=frontend; this overrides it to "frontend-staging".
$ dash0 -X otlp proxy --resource-attribute service.name=frontend-staging
```

##### Agent mode

When `--agent-mode` is active, the proxy emits one NDJSON OTLP/JSON event record per line on stdout. The stats redraw on stderr is suppressed; `--tail` is incompatible (agents already see batches through the structured event stream).

```bash
$ dash0 --agent-mode -X otlp proxy
{"resourceLogs":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"dash0-cli"}},...]},"scopeLogs":[{"scope":{"name":"dash0.cli.otlp_proxy"},"logRecords":[{"timeUnixNano":"...","attributes":[{"key":"endpoint.http","value":{"stringValue":"127.0.0.1:4318"}},...],"eventName":"dash0.cli.otlp_proxy.started"}]}]}]}
{"resourceLogs":[{...,"eventName":"dash0.cli.otlp_proxy.forwarded","attributes":[{"key":"signal","value":{"stringValue":"logs"}},{"key":"count","value":{"intValue":"3"}},...]}]}
...
```

##### Use with `telemetrygen` for a quick smoke test

[`telemetrygen`](https://github.com/open-telemetry/opentelemetry-collector-contrib/tree/main/cmd/telemetrygen) generates synthetic OTLP data, which is convenient for verifying the proxy without running a real SDK.

```bash
# In one terminal: start the proxy.
$ dash0 -X otlp proxy

# In another: send 10 traces over gRPC, 20 logs over HTTP, and 5 metrics over gRPC.
$ telemetrygen traces  --otlp-insecure                  --otlp-endpoint 127.0.0.1:4317 --traces 10
$ telemetrygen logs    --otlp-insecure --otlp-http      --otlp-endpoint 127.0.0.1:4318 --logs 20
$ telemetrygen metrics --otlp-insecure                  --otlp-endpoint 127.0.0.1:4317 --metrics 5
```

The proxy's stats block updates in place as the data flows through, then the records appear in the Dash0 UI under the active profile's dataset.

## Organizational commands

Organizational commands manage entities (teams, members, notification channels) that are scoped to the organization, not to a dataset.
They share a common set of characteristics:
- No `--dataset` flag.
- All are experimental and require the `-X` flag.
- Use `api-url` and `auth-token`.

Most organizational commands use flag-based input.
Notification channels are an exception: they use file-based input (`-f`) and support `--dry-run`.

### `notification-channels list` (experimental)

List all notification channels in the organization.

```bash
dash0 -X notification-channels list [-o <format>] [--skip-header] [--column <col>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-o` | `table` | Output format: `table`, `json`, `yaml`, or `csv` |
| `--skip-header` | `false` | Omit the header row from `table` and `csv` output |
| `--column` | | Column to display (repeatable; `table` and `csv` only) |

Example:

```bash
$ dash0 -X notification-channels list
NAME                  TYPE         ID                  ORIGIN       URL
Slack Alerts          slack        abc-123-def-456     my-origin    https://app.dash0.com/goto/settings/notifications?channel_id=abc-123-def-456
PagerDuty On-Call     pagerduty    def-456-ghi-789     -            https://app.dash0.com/goto/settings/notifications?channel_id=def-456-ghi-789
Email Digest          email_v2     ghi-789-jkl-012     -            https://app.dash0.com/goto/settings/notifications?channel_id=ghi-789-jkl-012
```

Column aliases: `name` / `channel name`, `type` / `channel type`, `id` / `channel id`, `origin`, `url`.

Aliases: `ls`

### `notification-channels get` (experimental)

Get a notification channel definition by ID or origin.

```bash
dash0 -X notification-channels get <id> [-o <format>]
```

The `<id>` argument can be a notification channel ID or origin.

Example:

```bash
$ dash0 -X notification-channels get <id>
Kind:  Dash0NotificationChannel
Name:  Slack Alerts
Type:  slack
ID:    abc-123-def-456
Origin: my-origin
URL:   https://app.dash0.com/goto/settings/notifications?channel_id=abc-123-def-456
```

Use `-o yaml` to get the full CRD definition:

```bash
$ dash0 -X notification-channels get <id> -o yaml
kind: Dash0NotificationChannel
metadata:
  name: Slack Alerts
  labels:
    dash0.com/id: abc-123-def-456
    dash0.com/origin: my-origin
spec:
  type: slack
  config: ...
```

### `notification-channels create` (experimental)

Create a notification channel from a YAML or JSON definition file.
If the definition contains a `dash0.com/origin` label, the channel is created or replaced (PUT).
Otherwise, a new channel is created (POST) and the server assigns an ID.

```bash
dash0 -X notification-channels create -f <file> [--dry-run]
```

| Flag | Description |
|------|-------------|
| `-f` | Path to YAML or JSON definition file (use `-` for stdin) |
| `--dry-run` | Validate without creating |

Create from a YAML file:

```bash
$ dash0 -X notification-channels create -f channel.yaml
Notification channel "Slack Alerts" created (id: abc-123).
```

Create from stdin (the `cat | dash0 …` pipeline is a single command):

```bash
cat channel.yaml | dash0 -X notification-channels create -f -
```

Validate without creating:

```bash
$ dash0 -X notification-channels create -f channel.yaml --dry-run
Dry run: notification channel definition is valid
```

Aliases: `add`

### `notification-channels update` (experimental)

Update an existing notification channel from a YAML or JSON definition file.
If the ID argument is omitted, the ID is extracted from the file content.

```bash
dash0 -X notification-channels update [id] -f <file> [--dry-run]
```

Update a notification channel from a file:

```bash
dash0 -X notification-channels update <id> -f channel.yaml
```

Update using the ID embedded in the file content:

```bash
dash0 -X notification-channels update -f channel.yaml
```

### `notification-channels delete` (experimental)

Delete a notification channel by ID or origin.
Prompts for confirmation unless `--force` is passed.

```bash
dash0 -X notification-channels delete <id> [--force]
```

Example:

```bash
$ dash0 -X notification-channels delete <id> --force
Notification channel deleted (id: <id>).
```

Aliases: `remove`

### `teams list` (experimental)

List all teams in the organization.

```bash
dash0 -X teams list [-o <format>] [--skip-header] [--column <col>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-o` | `table` | Output format: `table`, `json`, or `csv` |
| `--skip-header` | `false` | Omit the header row from `table` and `csv` output |
| `--column` | | Column to display (repeatable; `table` and `csv` only) |

Example:

```bash
$ dash0 -X teams list
NAME            ID          MEMBERS  ORIGIN       URL
Backend Team    a1b2c3d4    3        dash0-cli    https://app.dash0.com/goto/settings/teams?team_id=a1b2c3d4
Frontend Team   b2c3d4e5    2                     https://app.dash0.com/goto/settings/teams?team_id=b2c3d4e5
```

Column aliases: `name` / `team name`, `id` / `team id`, `members` / `member count`, `origin`, `url`.

Aliases: `ls`

### `teams get` (experimental)

Get detailed information about a team, including members and accessible assets.

```bash
dash0 -X teams get <id> [-o <format>]
```

The `<id>` argument can be a team ID or origin.

Example:

```bash
$ dash0 -X teams get <id>
Kind:    Team
Name:    Backend Team
ID:      a1b2c3d4-5678-90ab-cdef-1234567890ab
Origin:  dash0-cli
Color:   #FF6B6B -> #4ECDC4
Members: 2
...
```

### `teams create` (experimental)

Create a new team.

```bash
dash0 -X teams create <name> [--color-from <hex>] [--color-to <hex>] [--member <id>]
```

| Flag | Description |
|------|-------------|
| `--color-from` | Gradient start color (e.g. `"#FF0000"`) |
| `--color-to` | Gradient end color (e.g. `"#00FF00"`) |
| `--member` | Member ID to add to the team (repeatable) |

Example:

```bash
$ dash0 -X teams create "Backend Team" --color-from "#FF6B6B" --color-to "#4ECDC4"
Team "Backend Team" created (id: a1b2c3d4-...)
```

Aliases: `add`

### `teams update` (experimental)

Update the display settings of a team (name, color).

```bash
dash0 -X teams update <id> [--name <name>] [--color-from <hex>] [--color-to <hex>]
```

Example:

```bash
$ dash0 -X teams update <id> --name "New Team Name"
Team "<id>" updated
```

### `teams delete` (experimental)

Delete a team.
Prompts for confirmation unless `--force` is passed.

```bash
dash0 -X teams delete <id> [--force]
```

Example:

```bash
$ dash0 -X teams delete <id> --force
Team "<id>" deleted
```

Aliases: `remove`

### `teams list-members` (experimental)

List all members of a team.

```bash
dash0 -X teams list-members <team-id> [-o <format>] [--skip-header] [--column <col>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-o` | `table` | Output format: `table`, `json`, or `csv` |
| `--skip-header` | `false` | Omit the header row from `table` and `csv` output |
| `--column` | | Column to display (repeatable; `table` and `csv` only) |

Example:

```bash
$ dash0 -X teams list-members <team-id>
NAME              EMAIL                  ID
Alice Smith       alice@example.com      m1-0000-0000-0000-000000000001
Bob Jones         bob@example.com        m2-0000-0000-0000-000000000002
```

Column aliases are the same as for `members list`: `name` / `member name`, `email`, `id` / `member id`.

### `teams add-members` (experimental)

Add one or more existing organization members to a team.
Members can be specified by ID or email address.
When an email address is provided, it is resolved to a member ID via the members list API.

```bash
dash0 -X teams add-members <team-id> <member-id-or-email> [<member-id-or-email>...]
```

Add members by ID:

```bash
$ dash0 -X teams add-members <team-id> <member-id-1> <member-id-2>
2 members added to team "<team-id>"
```

Add a member by email address:

```bash
$ dash0 -X teams add-members <team-id> <email-address>
1 member added to team "<team-id>"
```

Mix of IDs and email addresses (a single invocation; each positional arg is one member):

```bash
$ dash0 -X teams add-members <team-id> <member-id> <email-address>
2 members added to team "<team-id>"
```

### `teams remove-members` (experimental)

Remove one or more members from a team.
Members can be specified by ID or email address.
Prompts for confirmation unless `--force` is passed.

```bash
dash0 -X teams remove-members <team-id> <member-id-or-email> [<member-id-or-email>...] [--force]
```

Remove a member by ID:

```bash
$ dash0 -X teams remove-members <team-id> <member-id> --force
1 member removed from team "<team-id>"
```

Remove a member by email address:

```bash
$ dash0 -X teams remove-members <team-id> <email-address> --force
1 member removed from team "<team-id>"
```

### `members list` (experimental)

List all members of the organization.

```bash
dash0 -X members list [-o <format>] [--skip-header] [--column <col>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-o` | `table` | Output format: `table`, `json`, or `csv` |
| `--skip-header` | `false` | Omit the header row from `table` and `csv` output |
| `--column` | | Column to display (repeatable; `table` and `csv` only) |

Example:

```bash
$ dash0 -X members list
NAME              EMAIL                  ID
Alice Smith       alice@example.com      m1-0000-0000-0000-000000000001
Bob Jones         bob@example.com        m2-0000-0000-0000-000000000002
...
```

Column aliases: `name` / `member name`, `email`, `id` / `member id`.

Aliases: `ls`

### `members invite` (experimental)

Invite one or more members to the organization by email address.

```bash
dash0 -X members invite <email> [<email>...] [--role <role>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--role` | `basic_member` | Role to assign: `basic_member` or `admin` |

Invite a single user:

```bash
$ dash0 -X members invite user@example.com
Invitation sent to user@example.com
```

Invite multiple users in one call with an explicit role:

```bash
$ dash0 -X members invite user1@example.com user2@example.com --role admin
Invitations sent to 2 email addresses
```

Aliases: `add`

### `members remove` (experimental)

Remove one or more members from the organization.
Members can be specified by ID or email address.
Prompts for confirmation unless `--force` is passed.

```bash
dash0 -X members remove <member-id-or-email> [<member-id-or-email>...] [--force]
```

Remove a member by ID:

```bash
$ dash0 -X members remove <member-id> --force
Member "<member-id>" removed
```

Remove a member by email address (resolved to an ID server-side):

```bash
$ dash0 -X members remove <email-address> --force
Member "<member-id>" removed
```

Aliases: `delete`

## Raw HTTP command

### `api` (experimental)

Call any Dash0 API endpoint directly, reusing the active profile's API URL, authentication token, and (by default) dataset.
Useful for endpoints that do not yet have a dedicated subcommand.
Requires the `-X` (or `--experimental`) flag.

```bash
dash0 -X api [METHOD] <path> [flags]
```

The command is deliberately flag-driven — no positional grammar for headers, query parameters, or body fields.
Request bodies always come from a file (or stdin) via `-f`.
Headers are set with `-H`.
Query parameters are baked into the path.

#### Path

Relative paths must start with `/api/` and are resolved against the profile's `api-url`:

- `dash0 -X api /api/signal-to-metrics/configs` → `<api-url>/api/signal-to-metrics/configs`

Absolute URLs (`http://` or `https://`) are passed through verbatim.
Query parameters are part of the path:

- `dash0 -X api "/api/signal-to-metrics/configs?limit=50"`

#### Method

The method is a positional argument before the path.
It is optional, case-insensitive, and defaults to `GET`.

- `dash0 -X api /api/foo` uses `GET`.
- `dash0 -X api POST /api/foo -f body.json` uses `POST`.
- `dash0 -X api delete /api/foo/<id>` uses `DELETE`.

Combining `GET` with `-f` is a usage error — use an explicit body-bearing method (`POST`, `PUT`, `PATCH`) when sending a payload.

#### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--file` | `-f` | Request body from a file, or `-` for stdin. Mutually exclusive with `GET`. |
| `--header` | `-H` | Request header as `Key: Value` (repeatable). |
| `--verbose` | `-v` | Print request and response details to stderr (with the `Authorization` header redacted). |
| `--dataset` | | Dataset to inject as a query parameter. Pass `""` to skip injection. |

#### Authentication

Authentication is managed by the active profile.
Setting `Authorization` via `-H` is a hard error — remove the header and let the CLI handle it.

#### Dataset injection

The dataset is auto-injected as a `dataset=<value>` query parameter, resolved from the standard precedence chain (flag → environment variable → active profile).
Pass `--dataset ""` to opt out for endpoints that do not accept a dataset, such as organization-level APIs.

If the path already contains a `dataset=` query parameter and `--dataset` resolves to a non-empty value, the command errors out — remove the value from the path or pass `--dataset ""`.

#### Content-Type

The `Content-Type` header defaults to `application/json` when a body is present.
Override it via `-H 'Content-Type: <value>'`.

#### Output and errors

The response body is streamed to stdout unchanged.
Non-2xx responses return a non-zero exit code.
The response body is still printed so the caller can inspect the error payload.

Use `-v` to see the full request line, outbound headers, request body, response status, and response headers on stderr.

#### Examples

GET — dataset auto-injected from the active profile:

```bash
dash0 -X api /api/signal-to-metrics/configs
```

GET against an organization-level endpoint that does not take a dataset:

```bash
dash0 -X api /api/organization/settings --dataset ""
```

GET with query parameters baked into the path:

```bash
dash0 -X api "/api/signal-to-metrics/configs?limit=50&enabled=true"
```

POST with a payload from a file:

```bash
dash0 -X api POST /api/signal-to-metrics/configs -f config.json
```

POST with a payload from stdin and a custom header (the `echo | dash0 …` pipeline is a single command):

```bash
echo '{"name":"my-config","enabled":true}' \
  | dash0 -X api POST /api/signal-to-metrics/configs -f - -H 'X-Request-Id: abc123'
```

DELETE with an explicit dataset override:

```bash
dash0 -X api delete /api/signal-to-metrics/configs/<id> --dataset production
```

Debug a failing request:

```bash
dash0 -X api POST /api/signal-to-metrics/configs -f config.json -v
```

## Agent tooling commands

Agent tooling commands distribute the dash0-cli Agent Skill — a `SKILL.md` plus reference docs following the open [Agent Skills specification](https://github.com/anthropics/skills) — so AI coding agents (Claude Code, Cursor, Codex, GitHub Copilot, and others) can discover this CLI's command surface without spending turns on `--help` exploration.

They differ from every other command category:

- No `--api-url`, `--auth-token`, or `--dataset` — they never call the Dash0 API.
- Not gated behind `--experimental` — the whole point is frictionless discovery, so requiring a flag agents don't yet know about would be circular.
- No `-o`/`--output` flag, in either human or agent mode. A skill's content is markdown prose meant to be read directly; JSON-wrapping it would only force an agent to unescape it back into the prose it already reads natively, with no compensating structure gained. Errors from these commands still get the standard `hint`-carrying JSON treatment in agent mode (see [Agent mode](#agent-mode)), since that's a mechanism independent of a command's own `-o` flag.

### `skill install`

Detect which supported AI coding agent (Claude Code, Codex, Cursor, or GitHub Copilot) is driving the current session, and install the dash0-cli Agent Skill into that agent's conventional skills directory in the current project.

```bash
dash0 skill install [--dir <path>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--dir` | current directory | Directory to install into |

Detection is environment-variable based (the same markers [Agent mode](#agent-mode) auto-detects), and maps to one directory per host, relative to the target directory:

| Host | Directory |
|------|-----------|
| Claude Code | `.claude/skills/dash0-cli` |
| Codex | `.agents/skills/dash0-cli` |
| Cursor | `.cursor/skills/dash0-cli` |
| GitHub Copilot | `.github/skills/dash0-cli` (the project-level convention; `~/.copilot/skills/` is for personal, home-directory-level skills) |

`install` writes only to the one directory matching the detected host — it does not spray files into every known convention.
Re-running it always overwrites, since the bundle is regenerable, CLI-managed content, not user data.

If no supported host can be detected, `install` fails rather than guessing, and points at the standards-based alternative:

```bash
$ dash0 skill install
Error: could not detect a supported agent host in this environment (checked: Claude Code, Codex, Cursor, GitHub Copilot)
Hint: install the dash0-cli skill instead with `npx skills add dash0hq/dash0-cli` or `gh skill install dash0hq/dash0-cli`
```

Install the skill for the detected agent host:

```bash
$ dash0 skill install
Installed dash0-cli skill (20 files) to .claude/skills/dash0-cli (detected: claude-code)
```

Install into a different directory than the current one:

```bash
dash0 skill install --dir ./my-project
```

### `skill show`

Print the dash0-cli Agent Skill bundle to stdout without writing any files — the disk-free path for CI or ephemeral agent sessions, or for environments where the agent host can't be auto-detected.

```bash
dash0 skill show [topic]
```

With no argument, prints `SKILL.md`, the entry point, which includes an index of every available topic.
With a topic argument, prints that topic's reference content raw.
Topics match the actual top-level `dash0 <command>` name (`dashboards`, `logs`, `teams`, and so on), not an internal taxonomy category.

Print the entry-point `SKILL.md`, including the topic index:

```bash
dash0 skill show
```

Print a specific topic's reference content:

```bash
dash0 skill show dashboards
```

An unknown topic fails with an error listing the valid topic names, so an agent can self-correct in one more call:

```bash
$ dash0 skill show bogus-topic
Error: unknown skill topic "bogus-topic"
Hint: valid topics are: apply, api, check-rules, config, dashboards, failed-checks, login, logs, members, metrics, notification-channels, otlp, recording-rules, spam-filters, spans, synthetic-checks, teams, traces, views
```

## Common workflows for AI agents

### Set up credentials from environment variables

When environment variables are already set, no profile is needed.
Export the four connection variables once in your shell:

```bash
export DASH0_API_URL=https://api.us-west-2.aws.dash0.com DASH0_OTLP_URL=https://ingress.us-west-2.aws.dash0.com DASH0_AUTH_TOKEN=auth_xxx DASH0_DATASET=default
```

Then any subsequent command picks them up without `--api-url`/`--auth-token` flags, for example:

```bash
dash0 dashboards list
```

```bash
dash0 logs query --from now-1h
```

### Export an asset, modify it, and re-apply

Export the asset to YAML:

```bash
dash0 dashboards get <id> -o yaml > dashboard.yaml
```

Edit the file, then update — the ID is read from the file content so no positional arg is needed:

```bash
dash0 dashboards update -f dashboard.yaml
```

Alternatively, use `apply`, which auto-detects create vs update from the asset's ID:

```bash
dash0 apply -f dashboard.yaml
```

### Bulk export all assets of one type

The YAML output contains full asset definitions as a multi-document stream, ready to be re-applied.

Export all dashboards to a file:

```bash
dash0 dashboards list -o yaml > all-dashboards.yaml
```

Re-apply them later (the same file can be replayed across environments):

```bash
dash0 apply -f all-dashboards.yaml
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
dash0 logs query \
    --from now-1h \
    --filter "otel.log.severity.range is_one_of ERROR WARN" \
    --limit 200
```

### Non-interactive deletion (for automation)

Always pass `--force` to skip the confirmation prompt:

```bash
dash0 dashboards delete a1b2c3d4-... --force
```

With `--force`, an asset that is already gone is treated as success: the command exits 0 and prints an "already deleted" line to stderr.
This is safe to invoke from GitOps or agent-driven pipelines even when another actor may have deleted the asset concurrently.

### Validate assets before applying

Use `--dry-run` to check for errors without making changes:

```bash
dash0 apply -f assets/ --dry-run
```
