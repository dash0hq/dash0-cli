# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

<!-- next version -->

## 1.16.1


### New Components


- `skill`: Add `dash0 skill install` and `dash0 skill show`, which distribute an Agent Skill teaching AI coding agents (Claude Code, Codex, Cursor, GitHub Copilot) this CLI's command surface without spending turns on `--help` exploration. (#212)
  `skill install` detects the current agent host (Claude Code, Codex, Cursor, or GitHub Copilot) and installs a
  SKILL.md plus reference docs into that host's conventional directory in the current project; `skill show [topic]`
  prints the same content to stdout for CI or ephemeral sessions. Neither command calls the Dash0 API or requires
  `--experimental`. In agent mode, a failing command now carries a follow-up hint pointing at `dash0 skill install`
  when the skill isn't installed yet, or at `dash0 skill show` / `dash0 --agent-mode --help` when it is — so an
  agent that mis-invokes has a concrete next step. Suppress with `--no-skill-hint` or `DASH0_NO_SKILL_HINT=1`;
  explicit `--no-skill-hint=false` or `DASH0_NO_SKILL_HINT=0` re-enables the hint on demand.
  

## 1.16.0


### Breaking Changes


- `teams`: `teams get -o json` and `teams get -o yaml` now emit the `TeamDefinitionV1Alpha1` envelope only, without the enriched `members` / `dashboards` / `checkRules` / `syntheticChecks` / `views` / `datasets` arrays (#200)
  Prior versions emitted the enriched `GetTeamResponse` wrapper (`{team, members, dashboards, ...}`). The new output is the CRD envelope directly (`{apiVersion, kind, metadata, spec}`), so `dash0 apply -f` can round-trip the exported document without post-processing. `spec.members` renders as email addresses rather than internal user ids.
  Scripts that read `.team.metadata.labels["dash0.com/id"]` or `.team.spec.display.name` need to drop the `.team.` prefix — e.g. `.metadata.labels["dash0.com/id"]`, `.spec.display.name`. Scripts that consumed the enriched arrays (`.dashboards[]`, `.members[].spec.display.email`, etc.) should use `dash0 teams get <id>` (the default table output) for a human-readable view, or `dash0 teams list-members <id>` for programmatic access to the member list.
  The `dash0 teams *` command tree is gated behind `--experimental` and does not carry a stability guarantee, but this shape change is nevertheless disruptive enough to warrant an explicit callout.
  


### Enhancements


- `teams`: Manage teams declaratively via `teams create -f` and `apply -f` on `Dash0Team` YAML (#200)
  `dash0 --experimental teams create -f <file>` accepts a `TeamDefinitionV1Alpha1` document. When the document carries a `metadata.labels["dash0.com/origin"]` label, the CLI upserts by origin (PUT, idempotent); otherwise it creates a new team (POST) and the server assigns id and origin.
  `dash0 apply` recognizes `kind: Dash0Team` and routes it through the same upsert path, so team definitions round-trip through the standard export-edit-reapply GitOps workflow alongside other assets.
  `spec.members` accepts either email addresses or internal member ids; the server resolves emails during reconciliation and rejects unresolvable ones with a single 400 listing every offender. The CLI renders team membership as emails on `teams get`, `teams list-members`, and the `apply` diff so review artifacts stay legible.
  The existing imperative commands (`teams create <name>`, `teams add-members`, `teams remove-members`, `teams update`) are preserved and now accept emails or ids for member arguments.
  

## 1.15.4


### Enhancements


- `check-rules`: Name check rules imported from a PrometheusRule CRD as `<group name> - <alert name>`, matching the Dash0 Kubernetes operator and Terraform provider (#182)
  Previously the CLI named such check rules after the alert only, so the same CRD produced a different name than the operator and Terraform provider.
  This affects `check-rules create`, `check-rules update`, and `apply`.
  Check rule identity for upsert is the `dash0.com/id` / `id`, not the name, so for definitions that pin an identifier this is a non-destructive in-place rename surfaced as a one-line diff on the next `apply`.
  

## 1.15.3


### Enhancements


- `build`: Add Nix flake packaging so the CLI can be installed and run on Nix and NixOS (#174)
  The flake builds the CLI with `buildGoModule`, installs Bash, Zsh, and Fish
  completions, and exposes `nix run`, `nix profile install`, an `overlays.default`
  for NixOS and Home Manager, and a `nix develop` shell. Non-flake `default.nix`
  and `shell.nix` shims are provided for systems without flakes enabled. A
  `homeManagerModules.default` declares profiles under `~/.dash0`: static tokens
  are read from `authTokenFile` at activation time and OAuth profiles are seeded
  for `dash0 login`, with runtime-acquired tokens preserved across rebuilds.
  A pre-built binary is published to a Nix User Repository for a compile-free
  install on small or non-x86_64 machines.
  

## 1.15.2


### Bug Fixes


- `homebrew`: Resume publishing the Homebrew cask to the dedicated tap on every release (#175)
  Tagged releases failed to update `Casks/dash0.rb` in `dash0hq/homebrew-dash0-cli`
  because the release pipeline could not write to the tap repository, leaving cask
  users pinned to a stale version. The cask is now published again on each release,
  so `brew upgrade --cask dash0` tracks the latest version.
  

## 1.15.1


### Bug Fixes


- `homebrew`: Fix `brew install --cask dash0hq/dash0-cli/dash0` failing on Linux (#172)
  The generated Homebrew cask ran an unconditional `postflight` hook invoking the
  macOS-only `/usr/bin/xattr` to strip the Gatekeeper quarantine attribute. On Linux
  that tool does not exist, so the install aborted at the postflight step even though
  the cask shipped correct Linux binaries. The hook is now guarded with `if OS.mac?`.
  

## 1.15.0


### New Components


- `failed-checks`: Add `dash0 failed-checks query` command to query active and historical alerting issues (#92)
  Supports filtering by `--status critical,degraded`, `--active` (unresolved only),
  and generic `--filter` expressions using the same syntax as `logs query` and `spans query`.
  Output formats: table (with semantic coloring for status), json, csv.
  Customize the displayed columns with `--column`; any issue label (such as `priority` or `owner`)
  can be used directly as a column name.
  The `list` and `ls` aliases are available for backwards compatibility.
  

## 1.14.1


### Bug Fixes


- `spam-filters`: Accept group-prefixed apiVersion values (e.g. `operator.dash0.com/v1alpha1`, `dash0.com/v1alpha1`) in spam filter YAML input (#169)
  The CLI rejected spam filter documents exported from the Dash0 Kubernetes operator because they
  carry a group-prefixed apiVersion. The CLI now normalizes these to the bare version before
  dispatching, so operator-exported and canonical-form YAML files work with create, update, and apply.
  

## 1.14.0


### New Components


- `auth`: Add `dash0 login` and `dash0 logout` for browser-based OAuth 2.0 + PKCE authentication (#27)
  Profiles now have an explicit auth mode: static (long-lived `auth_*` token) or OAuth (browser flow).
  Mark a new profile as OAuth at creation time with `dash0 config profiles create <name> --oauth --api-url <url>`,
  then run `dash0 login` to obtain access and refresh tokens.
  Access tokens are refreshed transparently before they expire; `dash0 logout` revokes them best-effort
  and clears the tokens from the profile while keeping the profile shell for re-login.
  Switch a profile between static and OAuth at any time with `dash0 config profiles update <name> --oauth[=true|=false]`
  (the transition is destructive and prompts for confirmation unless `--force` is passed).
  `dash0 config profiles list` gains an `AUTH` column showing the auth mode and remaining token lifetime,
  and `dash0 config show` annotates the `Auth Token:` line with OAuth state.
  `dash0 login` requires an interactive terminal — agent mode and non-TTY environments still use static tokens
  via `DASH0_AUTH_TOKEN` or `dash0 config profiles create --auth-token`.
  `dash0 logout` requires `--force` when invoked in agent mode (or when auto-detected via env vars like
  `CLAUDE_CODE`), so an AI agent cannot silently revoke a profile's refresh token.
  Telemetry send commands (`dash0 logs send`, `dash0 spans send`) refuse upfront when the active profile
  is OAuth-typed, because the Dash0 OTLP ingress does not (yet) accept OAuth access tokens — only static
  `auth_*` tokens. The error names the workarounds: pass `--auth-token auth_<...>` for the invocation,
  set `DASH0_AUTH_TOKEN`, or convert the profile with `dash0 config profiles update <name> --oauth=false`.
  Older `dash0` CLI binaries from before OAuth support shipped (e.g. v1.12.0) do not understand
  OAuth-typed profiles. `dash0 config show` and `dash0 config profiles list` silently render the
  access token as if it were a static one (no `(OAuth, ...)` annotation), and any command that calls
  the API fails with `auth token must start with 'auth_'` when the api-client library rejects the
  `dash0_at_*` prefix. If you have multiple `dash0` binaries installed (for example a homebrew-installed
  `dash0` alongside a development build) make sure every binary reading the profile is from this
  release or later. Static-token profiles are unaffected.
  

- `otlp`: Add `dash0 otlp proxy`, a long-running local OTLP forwarder that accepts OTLP/HTTP and OTLP/gRPC traffic and forwards every batch to Dash0 using the active profile's credentials. (#159)
  The proxy binds 127.0.0.1:4318 (OTLP/HTTP) and 127.0.0.1:4317 (OTLP/gRPC) by default — an OpenTelemetry SDK at default endpoint configuration connects with no environment variable change.
  It is gated behind `-X` (experimental) and is not intended as a replacement for the OpenTelemetry Collector.
  See `docs/commands.md` for the full reference including the agent-mode event schema and failure-mode classification.
  


### Enhancements


- `logs, spans, traces`: Add `--precision <adaptive|disabled>` to `logs query` and `spans query` to select the API sampling mode for the request. `traces get` always disables adaptive sampling so the complete trace is returned. (#167)
  Without the flag the API defaults to adaptive sampling, which samples telemetry data during query execution to keep queries fast on large datasets while returning statistically representative results.
  Pass `--precision disabled` for the API equivalent of the Precision toggle in the Dash0 UI: every matching record is returned, at the cost of higher query latency on wide time ranges.
  Use this for narrow lookups (e.g. `test.id is <uuid>`, `trace.id is <hex>`) that must be deterministic.
  `traces get` always disables adaptive sampling — retrieving a trace must return every span in the trace regardless of query window, so the flag is not exposed on that command.
  

- `homebrew`: Homebrew distribution is moving to a dedicated tap at `dash0hq/homebrew-dash0-cli` and switching from a formula to a cask. (#82, #162)
  Starting with the next release, new users install with:
  `brew install --cask dash0hq/dash0-cli/dash0` — no `brew tap`, no `brew trust`.
  Existing tap users will see a deprecation warning on `brew upgrade dash0`
  pointing at the new install path. The formula-to-cask switch follows
  Homebrew's convention for pre-built binary CLIs and aligns with
  goreleaser's deprecation of its `brews:` configuration. See
  `docs/brew-tap-migration-2026-06.md` for the full migration guide.
  

## 1.13.1


### Enhancements


- `notification-channels`: surface the Dash0 web app deep link in `notification-channels list` and `notification-channels get` (#147)
  The `list` table and CSV output now include a `URL` column with a direct link
  to the channel in the Dash0 web app. The `get` table output prints the same
  URL as a new `URL:` field.
  

## 1.13.0


### Breaking Changes


- `config`: Rename `DASH0_RETRY_COUNT` to `DASH0_MAX_RETRIES` and add `--max-retries` global flag (#141)
  The environment variable `DASH0_RETRY_COUNT` has been renamed to `DASH0_MAX_RETRIES` for consistency with the Terraform provider.
  A new `--max-retries` global flag is now available on all commands as an alternative to the environment variable.
  Resolution order: `--max-retries` flag, then `DASH0_MAX_RETRIES` env var, then default (3).
  


### Bug Fixes


- `send-log-event action`: Fix `send-log-event` action ignoring connection inputs when a profile already exists (#143)
  When a profile already existed (e.g., from the setup action), the action skipped profile creation
  and ignored any `otlp-url`, `auth-token`, or `dataset` inputs. The action now updates the existing
  profile with the provided values.
  

## 1.12.2


### Enhancements


- `apply`: Accept `Dash0NotificationChannel` and recording-rule `PrometheusRule` documents in `dash0 apply` (#137)
  `dash0 apply` now accepts `Dash0NotificationChannel` documents and dispatches them to the
  organization-level notification-channels endpoint without a dataset query parameter.
  PrometheusRule CRDs containing recording rules (entries with `record:`) are now dispatched to the
  recording-rule endpoint as a single PrometheusRule CRD; mixed CRDs that contain both alerting and
  recording rules are dispatched to both endpoints in one apply. Previously, recording rules in a
  PrometheusRule CRD were silently dropped during apply. A CRD with no rules of either kind now
  fails validation up front with a clear error. The unified diff that apply prints for notification
  channels and recording rules now strips server-managed fields (`dash0.com/created-at`,
  `dash0.com/updated-at`, and similar) so the second apply on unchanged input renders as
  "no changes".
  

- `spam-filters`: Accept v1alpha2 schema in spam-filters create/update/get and in apply (#136)
  The CLI now detects the `apiVersion` on spam filter documents (`v1alpha1` or `v1alpha2`) and routes to the
  matching API endpoint. `v1alpha2` uses `spec.context` (a single signal type) instead of `spec.contexts`
  (an array). A missing `apiVersion` defaults to `v1alpha1`. An unknown value is rejected with a clear
  error listing the supported versions. `dash0 apply` accepts `Dash0SpamFilter` documents in either
  schema and rejects unknown `apiVersion` values during validation, before any document is applied.
  


### Bug Fixes


- `client`: include the parsed server message, HTTP status, and trace ID in every API error printed by the CLI (#139)
  Errors now show the human-readable message extracted from the Dash0 API response (instead of dumping the raw JSON body).
  404 responses now also surface the HTTP status and trace ID so support can correlate to server-side logs.
  

## 1.12.1


### Bug Fixes


- `spam-filters`: Fix spam filter FilterCriteria format in fixtures and tests to match the Dash0 API schema (#134)
  The filter criteria now use the correct flat format with `operator` and `value` fields instead of the nested `stringValue` format.
  

## 1.12.0


### New Components


- `spam-filters`: Add spam-filters command with list, get, create, update, and delete subcommands (#132)
  Manage dataset-scoped spam filters via `dash0 spam-filters <subcommand>`.
  Supports file-based input (`-f`), dry-run validation, and all standard output formats (table, wide, json, yaml, csv).
  

## 1.11.1


### Bug Fixes


- `recording-rules`: Fix recording rules create and update not passing the dataset as a query parameter (#130)
  The `--dataset` flag and profile dataset were silently ignored for `recording-rules create` and `recording-rules update`.
  Updated `dash0-api-client-go` to v1.11.1 which passes the dataset as a query parameter.
  

## 1.11.0


### New Components


- `recording-rules`: Add recording rules commands (create, list, get, update, delete) for managing Prometheus recording rules via the PrometheusRule CRD format. (#126)


### Enhancements


- `config`: Add `--profile` global flag and `DASH0_PROFILE` environment variable to select a profile per invocation. (#127)
  The selector overrides the active profile on disk for the current invocation
  without modifying `~/.dash0/`. Precedence is `--profile` flag → `DASH0_PROFILE`
  → the active profile. A non-existent profile fails before any API call with a
  message listing the available profiles. Passing an empty value falls through
  to the next step. `config show` annotates the `Profile:` line with the source
  when a selector is in effect.
  

## 1.10.0


### New Components


- `api`: Add experimental `api` command for raw HTTP calls to the Dash0 API. (#122)
  The `dash0 -X api` command performs a raw HTTP request against any Dash0 API endpoint,
  reusing the active profile's API URL, auth token, and (by default) dataset.
  It is useful for endpoints that do not yet have a dedicated subcommand.
  


### Enhancements


- `metrics`: Rework `metrics instant` with new flags and output formats (#45)
  Add `--promql` flag (replacing the deprecated `--query`), `--filter` for PromQL label matcher generation,
  `--from` (replacing the deprecated `--time`), `-o csv` output format, `--skip-header`, and `--column` for
  columnar table and CSV output. The `timestamp` and `value` columns are always included automatically.
  Deprecated flags (`--query`, `--time`) remain functional for backwards compatibility.
  

- `logs, spans, traces`: Promote `logs query`, `spans query`, `spans send`, and `traces get` commands to stable (#124)
  These commands no longer require the `--experimental` (`-X`) flag.
  

## 1.9.0


### New Components


- `notification-channels`: Add notification-channels command for managing notification channels (list, get, create, update, delete) (#119)
  Notification channels are organization-level resources (no --dataset flag).
  The command uses CRD-enveloped NotificationChannelDefinition types with file-based input.
  All subcommands are experimental and require the -X flag.
  

## 1.8.1


### Bug Fixes


- `check-rules`: `check-rules update` now accepts PrometheusRule CRD files (#110)
  Previously, `check-rules update -f` failed with "no check rule ID provided" when given a PrometheusRule CRD file,
  because it did not detect the kind and route to the PrometheusRule parser. Both `apply` and `check-rules create`
  already handled this correctly.
  

- `dashboards`: `dashboards update` now accepts PersesDashboard CRD files (#111)
  Previously, `dashboards update -f` did not detect PersesDashboard CRD files, so the CRD-specific conversion
  (v1alpha1/v1alpha2 normalization, ID extraction from labels, annotation mapping) was skipped.
  Both `apply` and `dashboards create` already handled this correctly.
  

## 1.8.0


### New Components


- `agent-mode`: Add agent mode for AI coding agents (#68)
  When active, agent mode defaults output to JSON, returns errors as JSON on
  stderr, emits --help as structured JSON, skips confirmation prompts, and
  disables colored output. Agent mode is activated via --agent-mode, the
  DASH0_AGENT_MODE environment variable, or auto-detection of known AI agent
  environment variables (CLAUDE_CODE, MCP_SESSION_ID, CURSOR_SESSION_ID, etc.).
  

## 1.7.3


### Bug Fixes


- `dashboards`: Clear dashboard ID from the request body before update calls to avoid server-side rejection (#101)
  When updating a dashboard whose user-defined ID is a UUID, the server rejects the request if the
  same ID appears in both the URL path parameter and the request body. The CLI now strips the ID from
  the body before sending the update, since it is already passed as the URL path parameter.
  

- `dashboards`: Fix PersesDashboard annotations (folder-path, sharing, source) being silently dropped during conversion (#103)
  User-settable annotations (`dash0.com/folder-path`, `dash0.com/sharing`, `dash0.com/source`) on PersesDashboard CRDs
  were not carried over when converting to a Dashboard definition, affecting both `apply` and `dashboards create`.
  

## 1.7.2


### Enhancements


- `query`: `--filter` now accepts JSON filter criteria copied from the Dash0 UI (#96)
  The --filter flag on logs query and spans query now accepts JSON arrays and objects
  as produced by the Dash0 UI "copy filter criteria" feature, in addition to the
  existing text-based filter syntax. JSON and text filters can be mixed freely.
  


### Bug Fixes


- `apply`: Preserve user-settable annotations and permissions on asset round-trips (#99)
  `dash0.com/folder-path`, `dash0.com/source`, and `dash0.com/sharing` annotations were silently dropped during `apply`, `<asset> create` and `<asset> update`.
  `spec.permissions` on views and synthetic checks was also stripped.
  

## 1.7.1


### Enhancements


- `apply`: Migrate asset create/update from Import APIs to standard CRUD APIs (#90)
  The `apply` command and individual asset `create`/`update` subcommands now use the standard
  Create and Update APIs instead of the Import APIs, which are intended for one-time migrations.
  


### Bug Fixes


- `apply`: When an asset definition includes a user-defined ID, `apply` now always upserts, making repeated applies idempotent and preventing duplicate assets from being created. (#94)

## 1.7.0


### Enhancements


- `output`: The `update` and `apply` commands now show a unified diff of changes (#66)

- `dashboards`: `<asset> list -o yaml` and `-o json` now output full asset definitions instead of summary list items (#67)
  YAML output is a multi-document stream (separated by `---`) that can be piped directly to `dash0 apply -f -`.
  JSON output is an array of full asset definitions.
  This applies to all four asset types: dashboards, check-rules, views, and synthetic-checks.
  

## 1.6.0


### Enhancements


- `dashboards`: Accept PersesDashboard CRD files in `apply` and `dashboards create` (#85)
  PersesDashboard CRDs (perses.dev/v1alpha1 and perses.dev/v1alpha2) are now accepted as input.
  

## 1.5.4


### Enhancements


- `logs, spans, traces`: Add explorer deep link URL as the first line of output for logs query, spans query, and traces get (#71)
  The URL is printed after the table output and links to the corresponding Dash0 explorer view.
  

## 1.5.3


### Bug Fixes


- `synthetic-checks, views`: Display the human-readable name instead of the CRD name for synthetic checks and views (#80)
  The get, create, and update commands for synthetic checks and views now read from
  spec.display.name instead of metadata.name, consistent with dashboards.
  

## 1.5.2


### Enhancements


- `errors`: Include API response body in error messages when the backend does not return a structured error (#75)
  Previously, errors like 400 Bad Request only showed the HTTP status code and trace ID.
  Now, the full response body is displayed so that users can see the reason for the failure.
  

- `dashboards`: Make the `<id>` argument of `dash0 <asset> update` optional (#76)
  When the `<id>` argument is omitted, the ID is extracted from the file content.
  This applies to all asset types: dashboards, check rules, synthetic checks, and views.
  


### Bug Fixes


- `dashboards`: Fix `dashboards update` overwriting `dash0Extensions.id` with the origin string (#77)
  Unify ID and name extraction between `apply` and per-asset CRUD commands by
  delegating to shared `Extract*` functions in `internal/asset/`.
  

- `dashboards`: Stop force-setting origin in asset update commands (#78)
  The `synthetic-checks update`, `views update`, and `check-rules update` commands
  were force-setting the origin to `"dash0-cli"`, causing 400 errors when updating
  assets originally created with a different origin.
  

## 1.5.1


### Bug Fixes


- `views`: Fix view deeplink URLs to use the correct path for each view type (#72)
  Previously, all view deeplinks used `/goto/logs` regardless of view type.
  Now each view type maps to its correct deeplink path (e.g., `/goto/traces/explorer` for span views, `/goto/metrics/explorer` for metric views).
  The `views list` output also includes a new TYPE column.
  

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
