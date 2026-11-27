---
name: dash0-cli
description: Use when working with Dash0 observability data or configuration via the dash0 CLI (the `dash0` binary) — querying logs, spans, traces, metrics, or failed checks; managing dashboards, views, check rules, synthetic checks, service level objectives (SLOs), recording rules, notification channels, spam filters, time series aggregations, signal-to-metrics rules, or sampling rules; sending OTLP telemetry or deployment events; or managing teams, members, and profiles. Trigger on "Dash0", "dash0 CLI", or any of these operations.
---

<!-- This file is packaged with the dash0-cli distribution. Installed copies are overwritten by `dash0 skill install` / `make skill-bundle`; edit the hand-curated source at internal/skill/content/SKILL.md in the dash0hq/dash0-cli repository instead. -->

# dash0-cli

`dash0` is a command-line interface for the [Dash0](https://www.dash0.com) observability platform. It manages Dash0 assets (dashboards, views, check rules, synthetic checks, SLOs, recording rules, notification channels, spam filters, time series aggregations, signal-to-metrics rules, sampling rules), queries telemetry (logs, spans, traces, metrics, failed checks), sends telemetry via OTLP, and manages organization entities (teams, members, profiles).

**Prefer `dash0 --agent-mode <command> --help` over guessing flags.** Every command's exact, always-current flag list, aliases, and examples are available as structured JSON via `--agent-mode <command> --help` (e.g. `dash0 --agent-mode dashboards list --help`). This bundle deliberately does not duplicate flag tables — they'd go stale the moment a flag is added or renamed. Use the topics below for concepts, YAML formats, and workflows that `--help` output can't express; use `--agent-mode <command> --help` for the exact flags to pass.

## Command taxonomy

| Category | Commands | Characteristics |
|----------|----------|-----------------|
| Authentication | `login`, `logout` | Browser-based OAuth 2.0 + PKCE; per-profile |
| Configuration | `config profiles`, `config show` | Profile management, no API calls |
| Asset CRUD | `dashboards`, `views`, `check-rules`, `synthetic-checks`, `slos`, `evals`, `recording-rules`, `notification-channels`, `spam-filters`, `time-series-aggregations`, `signal-to-metrics-rules`, `sampling-rules`, `apply`, `diff` | File-based input, `--dry-run`, five standard subcommands |
| Query | `logs query`, `spans query`, `traces get`, `metrics instant`, `failed-checks query` | Time range, filters |
| Send | `logs send`, `spans send`, `debuginfo upload` | OTLP-based, repeatable attribute flags; `debuginfo upload` is experimental |
| Daemon | `otlp proxy` | Long-running, signal-driven shutdown, experimental |
| Interactive | `k8s pods watch`, `k8s deployments watch`, and the other kinds | Full-screen terminal page, refreshes on a timer, experimental |
| Organizational | `teams`, `members`, `notification-channels`, `datasets`, `auth-tokens`, `agent0 skills`, `aws-metric-scrape-configs` | Flag-based input, no dataset, experimental |
| Raw HTTP | `api` | Passthrough to any Dash0 API endpoint, experimental |
| Agent tooling | `skill install`, `skill show` | No API calls; local filesystem only; not gated by `--experimental`; no `-o`/JSON output |

## Prerequisites

Every command that talks to the Dash0 API or OTLP endpoint needs credentials, resolved in this order (first match wins): CLI flags (`--api-url`, `--otlp-url`, `--auth-token` or `--auth-token-file <path>`, `--dataset`), environment variables (`DASH0_API_URL`, `DASH0_OTLP_URL`, `DASH0_AUTH_TOKEN`, `DASH0_DATASET`), then the selected profile (`--profile` flag → `DASH0_PROFILE` env var → the active profile on disk). See the `config` and `login` topics for profile management and OAuth authentication.

## Global flags

`--api-url`, `--otlp-url`, `--auth-token`, `--auth-token-file` (read the token from a file that contains only the token, e.g. a mounted Kubernetes or Docker secret), `--dataset`, `--profile`, `--config-dir` (env: `DASH0_CONFIG_DIR`; default `~/.dash0`), `--agent-mode` (env: `DASH0_AGENT_MODE`), `--color` (`semantic` or `none`), `--experimental`/`-X`, `--max-retries`, `--no-skill-hint` (env: `DASH0_NO_SKILL_HINT` — suppress the "start with `dash0 skill show` … or run `dash0 skill install`" hint stitched onto errors; `dash0 skill show` (no arg) prints the entry point with a full topic index, `dash0 skill show <topic>` drills into one topic without writing files, and `dash0 skill install` adds a persistent bundle to the project). Run `dash0 --agent-mode --help` for the full, current list.

### Agent mode

Agent mode optimizes the CLI for AI agents: JSON output by default, structured `--help`, JSON errors, no confirmation prompts, no color. It auto-activates when a known AI agent environment variable is detected (`CLAUDE_CODE`, `CURSOR_AGENT`, `CODEX`, `GITHUB_COPILOT`, and others), or can be forced with `--agent-mode` / `DASH0_AGENT_MODE=1`.

## How asset commands work

All twelve asset types (`dashboards`, `check-rules`, `synthetic-checks`, `slos`, `evals`, `views`, `recording-rules`, `notification-channels`, `spam-filters`, `time-series-aggregations`, `signal-to-metrics-rules`, `sampling-rules`) share the same five subcommands: `list`, `get`, `create` (alias `add`), `update`, `delete` (alias `remove`). Output formats are `table`, `wide`, `json`, `yaml`, `csv` (query commands use `table`/`json`/`csv` only). `create`/`update` accept `-f <file>` (or `-f -` for stdin) and `--dry-run`. `update`'s diff (and `diff`'s) is computed semantically: cosmetic differences (key order, duration formatting, int-vs-float, API-populated defaults) never produce a spurious "changed" result.

`dash0 apply -f <file|directory>` provides create-or-update semantics across all asset types in one command, and (experimentally, via `--since <ref>`) delete semantics based on git history — see the `apply` topic. `dash0 --experimental diff -f <file|directory>` previews what `apply` would do, fetching each document's current state from Dash0 first so it can tell a create from an update, without mutating anything; see the `diff` topic.

File-based writes (`apply`, `create -f`, and `update -f`) warn on stderr about unsupported metadata labels and annotations before writing, including during `--dry-run`.
Warnings identify fields without printing their values, preserve normal stdout and exit codes, and leave ordinary exported server metadata quiet.

### Asset identifiers and idempotent upsert

Every asset type accepts a user-defined identifier in its YAML/JSON document. When present, both `create` and `apply` perform an **upsert** (PUT) against that identifier — the asset is created on the first call and replaced on every subsequent call, which is what makes GitOps/CI workflows idempotent. When absent, the kinds below that follow the routing rule are created by an inferred origin, so repeated runs update the same asset. The identifier field location varies by kind:

| Kind | Identifier field |
|------|------------------|
| `Dashboard`, `EphemeralDashboard` | Routing rule: `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]` (or `metadata.dash0Extensions.id`, which must equal it when both are set) |
| `PersesDashboard` | Routing rule: `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]`. The inferred origin hashes kind `Dashboard`, because `apply` converts the CRD first |
| `CheckRule` | Routing rule: top-level `id`, then top-level `labels["dash0.com/origin"]`. The inferred origin hashes the top-level `name` |
| `PrometheusRule` (alerting or recording) | Routing rule: `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]`, per alert. A CRD with 2+ alerting rules derives a distinct id and origin per alert: `<label>--<slug of "<group> - <alert>">`. Without an origin, each alert hashes `metadata.name` and `<group> - <alert>`; the recording rules are one asset that hashes `metadata.name` |
| `SyntheticCheck` | Routing rule: `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]` |
| `View` | Routing rule: `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]` |
| `SLO` | Routing rule: `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]`. SLO IDs are server-assigned, so pin origin in a hand-written document |
| `Dash0SpamFilter` | Routing rule: `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]` |
| `Dash0NotificationChannel` | Routing rule: `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]` |
| `Dash0Dataset` | `spec.slug`, or `metadata.labels["dash0.com/id"]` when `spec.slug` is absent (the dataset slug — required and immutable; `spec.slug` wins when both are set); `dash0.com/origin` is optional |
| `Dash0Team` | Routing rule: `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]` |
| `Dash0AwsMetricScrapeConfig` | `metadata.labels["dash0.com/id"]`, the AWS integration id — **required**. One config per integration; `apply` replaces it (PUT), there is no create. `spec.namespaces` is required (`[]` stops all scraping). The first config of an integration, and the first after a clear, sends `dash0.com/origin` (the file's, or one inferred from the integration id); later writes do not, and a stored origin that differs from the file's fails |
| `Dash0Skill` | Routing rule: `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]`. The inferred origin hashes the skill name from the `SKILL.md` frontmatter, not `metadata.name` |
| `Dash0SignalToMetrics` | Routing rule: `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]`. Origins are unique per organization while each rule belongs to one dataset, so one document with an explicit origin cannot serve two datasets |
| `Dash0Sampling` | Routing rule: `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]`. Origins are unique per organization while each rule belongs to one dataset, so one document with an explicit origin cannot serve two datasets |
| `Dash0TimeSeriesAggregation` | Routing rule: `metadata.labels["dash0.com/id"]`, then `metadata.labels["dash0.com/origin"]`. Origins are unique per organization while each aggregation belongs to one dataset, so one document with an explicit origin cannot serve two datasets |

**Origin vs ID — do not conflate them.** *ID* is the user-defined external identifier described above. *Origin* (`dash0.com/origin` label) records which system is the authoritative source of truth for an asset (`dash0-cli`, `terraform`, `ui`, `operator`). Which of the two is the upsert key depends on the kind, and the split follows the shape of the underlying API rather than being a list of exceptions:

- **The routing rule** serves every kind: dashboards, ephemeral dashboards, PersesDashboards, check rules, PrometheusRules, synthetic checks, views, spam filters, time series aggregations, notification channels, teams, SLOs, evals, signal-to-metrics rules, sampling rules, Agent0 skills, and datasets. A dataset's id is its slug; it is required, so its inferred origin hashes the slug, and a document whose origin belongs to a dataset with another slug fails, because the slug cannot change. AWS metric scrape configs have their own rule: the integration id is the only key, and an unknown integration fails. With `dash0.com/id`, the CLI reads the asset by id and updates it there; it fails when the stored origin differs from an explicit origin in the file. When the id is not found, or there is none, it PUTs by `dash0.com/origin`, or by an inferred origin: `dash0-cli_` plus a hash of the kind, `metadata.name` (the `SKILL.md` name for an Agent0 skill), and the dataset. Check rules and PrometheusRule alerting rules are the exception: an id that is not found creates the check rule at that id, so the file keeps its id. These kinds are never created by POST, and an update never sends an origin other than the stored one. An asset that an earlier CLI version created by POST, from a document without an id or an origin, sits at a server-assigned origin, so the next apply creates a second asset; set `dash0.com/id` or `dash0.com/origin` in the file to the stored value to update the existing one (`list -o wide` shows it); for evals and Agent0 skills that apply fails with a name collision until you do. Origins are unique per organization, so a document with an explicit origin applies to one dataset only, for every dataset-scoped kind except spam filters; leave the origin out to get one inferred per dataset. Renaming `metadata.name` without an explicit origin creates a new asset. Two documents without an id that would write the same origin fail validation (two inferred origins, or an explicit origin equal to another document's inferred one), and a document whose id is not found fails when another document of the run already writes its inferred origin.
- **An update never changes `dash0.com/origin`**, so an `apply` never claims ownership of an asset managed by Terraform or the Kubernetes operator.
- **Dashboards** follow the routing rule. Never use the deprecated `metadata.dash0Extensions.origin` field.

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

## Explorer deep links

`logs query`, `spans query`, `failed-checks query`, and `traces get` print a link that reopens the same query in the Dash0 web app, below the results, prefixed by `Open this query in Dash0:` (`Open this trace in Dash0:` for `traces get`). The link carries the filters, time range, and dataset, so it is what to hand a human who should look at the same result set.

Where it appears depends on the format. In `json` output — the agent-mode default — it is a top-level `dash0ExplorerUrl` field, a sibling of `resourceLogs`/`resourceSpans`/`issues`, so reading stdout alone is enough. In `table` and `csv` output it goes to stderr instead, keeping the piped result stream clean.

The field is omitted when no link could be built (an `api-url` host with a single label, such as `localhost`, or an IP address). It is emitted even when the query matched nothing. `metrics instant` emits no deep link.

Note that `logs query`, `spans query`, and `traces get` emit OTLP/JSON, and `dash0ExplorerUrl` is a Dash0 extension to it: a strict OpenTelemetry protobuf-JSON parser rejects unknown fields, so strip it with `jq 'del(.dash0ExplorerUrl)'` before feeding that output to one.

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
With `--force`, an already-deleted asset is treated as idempotent success — the command exits 0 and prints a short "already deleted" line to stderr. Without `--force`, a missing asset still surfaces as a non-zero exit with a "not found" error. This is uniform across every `delete` subcommand and every `remove`-shaped variant (`members remove`, `teams remove-members`); in the multi-target variants the check runs per member so one concurrently-removed member does not fail the whole call.

### Validate assets before applying

```bash
dash0 apply -f assets/ --dry-run
```

### Sync a directory to match its state as of a git ref (experimental)

`apply --since <ref>` deletes assets removed from `-f`'s contents since `<ref>`, in addition to the usual create/update behavior. Requires `--experimental`/`-X` and `-f`'s target to be inside a git repository:

```bash
dash0 --experimental apply -f assets/ --since HEAD~1 --force
```

Preview the plan first with `dash0 --experimental diff -f assets/ --since HEAD~1` — see the `apply` topic for the full reference, including the GitHub Actions invocation pattern and ref-resolution edge cases.

## Topics

Run `dash0 skill show <topic>` for the reference content below, or read `references/<topic>.md` directly if the skill is installed on disk.

| Topic | Covers |
|-------|--------|
| `apply` | Create-or-update asset definitions from files, directories, or stdin |
| `agent0` | Agent0 custom skill CRUD (`agent0 skills`; organization-level, experimental; `kind: Dash0Skill`) |
| `api` | Raw HTTP passthrough to any Dash0 API endpoint |
| `auth-tokens` | Auth token CRUD (organization-level, experimental; `create` needs `--secret-file` outside an interactive terminal; not supported by `apply`) |
| `aws-metric-scrape-configs` | CloudWatch metric scrape config of an AWS integration: `get`, `update` (replace), `delete` (clear) by integration id (organization-level, experimental; also accepted by `apply` as `Dash0AwsMetricScrapeConfig`) |
| `check-rules` | Check rule (alerting rule) CRUD, including PrometheusRule CRD import |
| `config` | Profile management (create/update/list/select/delete) and `config show` |
| `dashboards` | Dashboard CRUD, including PersesDashboard CRD import |
| `debuginfo` | Upload source maps so Dash0 can symbolize Node.js and browser profiles (`debuginfo upload`, experimental) |
| `diff` | Preview what `apply` would do (create, update, delete), without mutating anything |
| `datasets` | Dataset CRUD (organization-level, experimental; deleting keeps its assets for 30 days; telemetry follows its retention) |
| `evals` | Generative AI evaluation (eval) CRUD (dataset-scoped, experimental; `kind: Dash0GenerativeAiEvaluation`; `metadata.name` and the judge output kind cannot change) |
| `failed-checks` | Query active and historical alerting issues |
| `k8s` | A pod's or workload's CPU, memory, requests, and logs as a terminal page or JSON (`k8s pods watch`, `k8s deployments watch`, and the other kinds, experimental; also a k9s plugin) |
| `login` | OAuth 2.0 login/logout and profile authentication states |
| `logs` | Query and send log records |
| `members` | Organization membership management |
| `metrics` | Instant PromQL queries |
| `notification-channels` | Notification channel CRUD (organization-level, no dataset) |
| `otlp` | Local OTLP forwarding proxy (`otlp proxy`) |
| `recording-rules` | Recording rule CRUD (PrometheusRule CRD format) |
| `sampling-rules` | Sampling rule CRUD (decides which traces Dash0 keeps; the first enabled rule of a dataset turns sampling on; admin role required for writes) |
| `signal-to-metrics-rules` | Signal-to-metrics rule CRUD and `test` (alias `s2m`; turns logs or spans into metrics) |
| `slos` | SLO CRUD (`apiVersion: openslo.com/v1`, `kind: SLO` — a supported subset of OpenSLO v1, not upstream-identical) |
| `spam-filters` | Spam filter CRUD (v1alpha1 and v1alpha2) |
| `spans` | Query and send spans |
| `synthetic-checks` | Synthetic check CRUD |
| `teams` | Team management and membership |
| `time-series-aggregations` | Time series aggregation CRUD (alias `tsa`; admin role required) |
| `traces` | Retrieve every span belonging to a trace |
| `views` | View CRUD |

## Keeping this skill up to date

If the dash0-cli version this skill shipped with feels stale (missing a command you can see in `dash0 --help`), run `dash0 skill install` again to refresh it from the currently installed binary.
