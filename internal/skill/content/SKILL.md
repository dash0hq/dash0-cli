---
name: dash0-cli
description: Use when working with Dash0 observability data or configuration via the dash0 CLI (the `dash0` binary) — querying logs, spans, traces, metrics, or failed checks; managing dashboards, views, check rules, synthetic checks, recording rules, notification channels, or spam filters; sending OTLP telemetry or deployment events; or managing teams, members, and profiles. Trigger on "Dash0", "dash0 CLI", or any of these operations.
---

<!-- This file is packaged with the dash0-cli distribution. Installed copies are overwritten by `dash0 skill install` / `make skill-bundle`; edit the hand-curated source at internal/skill/content/SKILL.md in the dash0hq/dash0-cli repository instead. -->

# dash0-cli

`dash0` is a command-line interface for the [Dash0](https://www.dash0.com) observability platform. It manages Dash0 assets (dashboards, views, check rules, synthetic checks, recording rules, notification channels, spam filters), queries telemetry (logs, spans, traces, metrics, failed checks), sends telemetry via OTLP, and manages organization entities (teams, members, profiles).

**Prefer `dash0 --agent-mode <command> --help` over guessing flags.** Every command's exact, always-current flag list, aliases, and examples are available as structured JSON via `--agent-mode <command> --help` (e.g. `dash0 --agent-mode dashboards list --help`). This bundle deliberately does not duplicate flag tables — they'd go stale the moment a flag is added or renamed. Use the topics below for concepts, YAML formats, and workflows that `--help` output can't express; use `--agent-mode <command> --help` for the exact flags to pass.

## Command taxonomy

| Category | Commands | Characteristics |
|----------|----------|-----------------|
| Authentication | `login`, `logout` | Browser-based OAuth 2.0 + PKCE; per-profile |
| Configuration | `config profiles`, `config show` | Profile management, no API calls |
| Asset CRUD | `dashboards`, `views`, `check-rules`, `synthetic-checks`, `recording-rules`, `notification-channels`, `spam-filters`, `apply` | File-based input, `--dry-run`, five standard subcommands |
| Query | `logs query`, `spans query`, `traces get`, `metrics instant`, `failed-checks query` | Time range, filters |
| Send | `logs send`, `spans send` | OTLP-based, repeatable attribute flags |
| Daemon | `otlp proxy` | Long-running, signal-driven shutdown, experimental |
| Organizational | `teams`, `members`, `notification-channels` | Flag-based input, no dataset, experimental |
| Raw HTTP | `api` | Passthrough to any Dash0 API endpoint, experimental |
| Agent tooling | `skill install`, `skill show` | No API calls; local filesystem only; not gated by `--experimental`; no `-o`/JSON output |

## Prerequisites

Every command that talks to the Dash0 API or OTLP endpoint needs credentials, resolved in this order (first match wins): environment variables (`DASH0_API_URL`, `DASH0_OTLP_URL`, `DASH0_AUTH_TOKEN`, `DASH0_DATASET`), CLI flags (`--api-url`, `--otlp-url`, `--auth-token`, `--dataset`), then the selected profile (`--profile` flag → `DASH0_PROFILE` env var → the active profile on disk). See the `config` and `login` topics for profile management and OAuth authentication.

## Global flags

`--api-url`, `--otlp-url`, `--auth-token`, `--dataset`, `--profile`, `--agent-mode` (env: `DASH0_AGENT_MODE`), `--color` (`semantic` or `none`), `--experimental`/`-X`, `--max-retries`, `--no-skill-hint` (env: `DASH0_NO_SKILL_HINT` — suppress the "run `dash0 skill install`" hint stitched onto errors when this bundle isn't installed in the current project). Run `dash0 --agent-mode --help` for the full, current list.

### Agent mode

Agent mode optimizes the CLI for AI agents: JSON output by default, structured `--help`, JSON errors, no confirmation prompts, no color. It auto-activates when a known AI agent environment variable is detected (`CLAUDE_CODE`, `CURSOR_AGENT`, `CODEX`, `GITHUB_COPILOT`, and others), or can be forced with `--agent-mode` / `DASH0_AGENT_MODE=1`.

## How asset commands work

All seven asset types (`dashboards`, `check-rules`, `synthetic-checks`, `views`, `recording-rules`, `notification-channels`, `spam-filters`) share the same five subcommands: `list`, `get`, `create` (alias `add`), `update`, `delete` (alias `remove`). Output formats are `table`, `wide`, `json`, `yaml`, `csv` (query commands use `table`/`json`/`csv` only). `create`/`update` accept `-f <file>` (or `-f -` for stdin) and `--dry-run`.

`dash0 apply -f <file|directory>` provides create-or-update semantics across all asset types in one command — see the `apply` topic.

### Asset identifiers and idempotent upsert

Every asset type accepts a user-defined identifier in its YAML/JSON document. When present, both `create` and `apply` perform an **upsert** (PUT) against that identifier — the asset is created on the first call and replaced on every subsequent call, which is what makes GitOps/CI workflows idempotent. When absent, both perform a plain create (POST) and the server assigns a fresh ID on each invocation, so repeated runs produce duplicates. The identifier field location varies by kind:

| Kind | Identifier field |
|------|------------------|
| `Dashboard` | `metadata.dash0Extensions.id` |
| `PersesDashboard` | `metadata.labels["dash0.com/id"]` |
| `CheckRule` | top-level `id` |
| `PrometheusRule` (alerting or recording) | `metadata.labels["dash0.com/id"]` |
| `SyntheticCheck` | `metadata.labels["dash0.com/id"]` |
| `View` | `metadata.labels["dash0.com/id"]` |
| `Dash0SpamFilter` | `metadata.labels["dash0.com/id"]` (`dash0.com/origin` takes precedence when both are present) |
| `Dash0NotificationChannel` | no ID field — `metadata.labels["dash0.com/origin"]` is the upsert key |

**Origin vs ID — do not conflate them.** *Origin* (`dash0.com/origin` label) identifies which system is the authoritative source of truth for an asset (`dash0-cli`, `terraform`, `ui`) — it's provenance metadata, not a lookup key, and the CLI strips it before sending most asset types to the API so it doesn't claim ownership of assets managed elsewhere. *ID* is the user-defined external identifier used for upsert, described above. Notification channels and spam filters are the two exceptions where origin (not ID) is the upsert key.

When `list -o yaml` or `get -o yaml` exports an existing asset, the server-assigned ID is rendered into the correct field, so an export → edit → `apply` (or `update`) round-trips through the identifier automatically.

## Filter syntax

Query commands (`logs query`, `spans query`, `metrics instant --filter`, `failed-checks query`) accept `--filter` expressions in the form `key [operator] value` (operator defaults to `is` when omitted), or JSON filter criteria copied from the Dash0 UI. Multiple `--filter` flags combine with AND logic.

| Operator | Alias | Description |
|----------|-------|-------------|
| `is` | `=` | Exact match (default) |
| `is_not` | `!=` | Not equal |
| `contains` / `does_not_contain` | | Substring match / negation |
| `starts_with` / `does_not_start_with` | | Prefix match / negation |
| `ends_with` / `does_not_end_with` | | Suffix match / negation |
| `matches` / `does_not_match` | `~` / `!~` | Regular expression match / negation |
| `gt` / `gte` / `lt` / `lte` | `>` / `>=` / `<` / `<=` | Numeric comparison |
| `is_set` / `is_not_set` | | Attribute present / absent |
| `is_one_of` / `is_not_one_of` | | Matches any / none of space-separated values |
| `is_any` | | Matches any value |

Common attribute keys: `service.name`, `otel.log.severity.number`, `otel.log.severity.range`, `otel.log.body`, `otel.span.status.code`, `otel.trace.id`. Valid `otel.log.severity.range` values: `TRACE`, `DEBUG`, `INFO`, `WARN`, `ERROR`, `FATAL`, `UNKNOWN`.

## Custom columns

`--column` (repeatable) selects which columns appear in `table`/`csv` output, replacing the default set entirely. Each command has short aliases (e.g. `time`, `severity`, `body` for `logs query`; `duration`, `status`, `service` for `spans query`) — see each topic for its alias table — and any OTLP attribute key can also be used directly as a column. Not supported with `-o json`.

## Precision mode (adaptive sampling)

By default the API applies adaptive sampling to log and span queries for speed on large datasets. Pass `--precision disabled` to `logs query` or `spans query` for deterministic, complete results (higher latency) on narrow lookups like a specific trace or request ID; `--precision adaptive` is the explicit default. `traces get` always disables sampling and doesn't accept the flag; `metrics instant` and `failed-checks query` don't honor it.

## Common workflows for AI agents

### Set up credentials from environment variables

```bash
export DASH0_API_URL=https://api.us-west-2.aws.dash0.com DASH0_OTLP_URL=https://ingress.us-west-2.aws.dash0.com DASH0_AUTH_TOKEN=auth_xxx DASH0_DATASET=default
```
Then any command picks them up without flags, e.g. `dash0 dashboards list` or `dash0 logs query --from now-1h`.

### Export an asset, modify it, and re-apply

```bash
dash0 dashboards get <id> -o yaml > dashboard.yaml
```
Edit the file, then either `dash0 dashboards update -f dashboard.yaml` (ID read from the file) or `dash0 apply -f dashboard.yaml` (auto-detects create vs. update).

### Bulk export and re-apply all assets of one type

```bash
dash0 dashboards list -o yaml > all-dashboards.yaml
dash0 apply -f all-dashboards.yaml
```
The YAML output is a multi-document stream, ready to replay across environments.

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
dash0 logs query --from now-1h --filter "otel.log.severity.range is_one_of ERROR WARN" --limit 200
```

### Non-interactive deletion (for automation)

Always pass `--force` to skip the confirmation prompt: `dash0 dashboards delete <id> --force`.

### Validate assets before applying

```bash
dash0 apply -f assets/ --dry-run
```

## Topics

Run `dash0 skill show <topic>` for the reference content below, or read `references/<topic>.md` directly if the skill is installed on disk.

| Topic | Covers |
|-------|--------|
| `apply` | Create-or-update asset definitions from files, directories, or stdin |
| `api` | Raw HTTP passthrough to any Dash0 API endpoint |
| `check-rules` | Check rule (alerting rule) CRUD, including PrometheusRule CRD import |
| `config` | Profile management (create/update/list/select/delete) and `config show` |
| `dashboards` | Dashboard CRUD, including PersesDashboard CRD import |
| `failed-checks` | Query active and historical alerting issues |
| `login` | OAuth 2.0 login/logout and profile authentication states |
| `logs` | Query and send log records |
| `members` | Organization membership management |
| `metrics` | Instant PromQL queries |
| `notification-channels` | Notification channel CRUD (organization-level, no dataset) |
| `otlp` | Local OTLP forwarding proxy (`otlp proxy`) |
| `recording-rules` | Recording rule CRUD (PrometheusRule CRD format) |
| `spam-filters` | Spam filter CRUD (v1alpha1 and v1alpha2) |
| `spans` | Query and send spans |
| `synthetic-checks` | Synthetic check CRUD |
| `teams` | Team management and membership |
| `traces` | Retrieve every span belonging to a trace |
| `views` | View CRUD |

## Keeping this skill up to date

If the dash0-cli version this skill shipped with feels stale (missing a command you can see in `dash0 --help`), run `dash0 skill install` again to refresh it from the currently installed binary.
