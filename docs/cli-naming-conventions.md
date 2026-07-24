# CLI Naming Conventions

In Dash0, dashboards, views, synthetic checks and check rules are called "assets", rather than the more common "resources".
The reason for this is that the word "resource" is overloaded in OpenTelemetry, where it describes "where telemetry comes from".
Use the word "asset" consistently where appropriate.

## Top-level Asset Commands
- Use **plural form**: `dashboards`, `views`, `check-rules`, `synthetic-checks`, `recording-rules`, `notification-channels`, `spam-filters`
- Use **kebab-case** for multi-word names: `check-rules`, `synthetic-checks`, `recording-rules`, `notification-channels`, `spam-filters`
- Group related functionality: `config profiles` for profile management

## Standard CRUD Subcommands for Assets
All asset commands (`dashboards`, `check-rules`, `views`, `synthetic-checks`, `recording-rules`, `notification-channels`, `spam-filters`) use these subcommands:

| Subcommand | Alias    | Description                          |
|------------|----------|--------------------------------------|
| `list`     | `ls`     | List all assets                      |
| `get`      | -        | Get a single asset by ID             |
| `create`   | `add`    | Create a new asset from a file       |
| `update`   | -        | Update an existing asset from a file |
| `delete`   | `remove` | Delete an asset by ID                |

### Idempotent `delete --force`
Every `delete` subcommand — including `remove`-shaped variants like `members remove` and `teams remove-members` — must treat a 404 from the server as success when the caller passed `--force`.
The desired end-state ("asset is gone") already holds regardless of who removed it, so returning a non-zero exit code punishes CI/CD and agent-driven pipelines for concurrency they cannot always avoid.
Without `--force`, a 404 must surface as a clean "asset not found" error (via `client.HandleAPIError` with an `ErrorContext`), not a raw HTTP dump.

The shared helper is `client.IsAlreadyDeleted(err, force, ectx)` in `internal/client/client.go`.
Every delete path must call it before falling through to `client.HandleAPIError`; the helper prints an "already deleted" line to stderr and returns `true` when force is set and the error is a 404.
This is a hard rule, not a guideline — adding a new delete command without this wiring is incomplete work and will be flagged in review.

## Config Profiles Subcommands
The `config profiles` command uses:

| Subcommand | Alias      | Description                |
|------------|------------|----------------------------|
| `list`     | `ls`       | List all profiles          |
| `create`   | `add`      | Create a new profile       |
| `update`   | -          | Update an existing profile |
| `delete`   | `remove`   | Delete a profile           |
| `select`   | `activate` | Set the active profile     |

## Teams Subcommands (experimental)
The `teams` command manages organizational teams (not assets — no dataset, no YAML input, no `apply`):

| Subcommand       | Alias    | Description                                     |
|------------------|----------|-------------------------------------------------|
| `list`           | `ls`     | List all teams                                  |
| `get`            | -        | Get team details (members + accessible assets)  |
| `create`         | `add`    | Create a team (flag-based, not file-based)      |
| `update`         | -        | Update team display settings                    |
| `delete`         | `remove` | Delete a team                                   |
| `list-members`   | -        | List members of a team                          |
| `add-members`    | -        | Add members to a team                           |
| `remove-members` | -        | Remove members from a team                      |

## Members Subcommands (experimental)
The `members` command manages organization membership:

| Subcommand | Alias    | Description                          |
|------------|----------|--------------------------------------|
| `list`     | `ls`     | List all organization members        |
| `invite`   | `add`    | Invite members by email              |
| `remove`   | `delete` | Remove members from the organization |

## Skill Subcommands
The `skill` command distributes the dash0-cli Agent Skill to AI coding agents.
Unlike the plural-noun asset commands, `skill` is deliberately singular — there is exactly one skill this CLI ships, and pluralizing it would falsely imply a collection to enumerate.

| Subcommand | Alias | Description                                                             |
|------------|-------|-------------------------------------------------------------------------|
| `install`  | -     | Install the skill into the detected agent host's conventional directory |
| `show`     | -     | Print the skill content (SKILL.md or a topic) to stdout                 |

Neither subcommand has an alias.
The Agent Tooling command category is a deliberate exception to several conventions elsewhere in this document:

- **Not gated behind `--experimental`.** The whole point is frictionless discovery for agents that have not yet learned about `-X`; requiring the flag first would be circular.
- **No `-o` / `--output` flag.** Content is markdown prose meant to be read directly, not structured data to reshape.
- **No `--api-url`, `--auth-token`, `--dataset`.** Neither subcommand talks to the Dash0 API; they read embedded content and, for `install`, write to the local filesystem.

See [docs/adding-commands.md](adding-commands.md#1-determine-the-command-type) for the reference implementation (`internal/skill/`) and [docs/agent-skill-maintenance.md](agent-skill-maintenance.md) for the bundle-generation flow.

## Authentication Commands
`login` and `logout` are top-level (not under `config`) so they read like the equivalent commands in `kubectl`, `gh`, and `terraform`:

| Command  | Alias | Description                                                       |
|----------|-------|-------------------------------------------------------------------|
| `login`  | -     | Open the browser to authenticate via OAuth 2.0 + PKCE             |
| `logout` | -     | Clear (and best-effort revoke) the OAuth tokens of a profile      |

Neither command takes a positional argument; the target profile is the active profile by default and can be overridden with `--profile <name>`.
`logout` keeps the profile shell so subsequent `login` can re-fill it; to leave OAuth behind entirely use `dash0 config profiles update <name> --oauth=false`.

## Aliases
- `activate` → `select`
- `add` → `create` / `invite`
- `delete` → `remove`
- `remove` → `delete`
- `ls` → `list`

## Asset Kind Display Names
In user-facing output (success messages, dry-run listings, error messages), use human-readable names for asset kinds — **not** PascalCase identifiers:

| Kind identifier             | Display name         |
|-----------------------------|----------------------|
| `Dashboard`                 | Dashboard            |
| `CheckRule`                 | Check rule           |
| `SyntheticCheck`            | Synthetic check      |
| `View`                      | View                 |
| `PrometheusRule`            | PrometheusRule       |
| `PersesDashboard`           | PersesDashboard      |
| `Dash0NotificationChannel`  | Notification channel |
| `Dash0SpamFilter`           | Spam filter          |

For example: `Check rule "High Error Rate" created`, not `CheckRule "High Error Rate" created`.

## Naming Rules
1. **Prefer verbs** for actions: `create`, `delete`, `list`, `get`, `update`, `select`
2. **Use plural** for asset type commands: `dashboards` not `dashboard`
3. **Use kebab-case** for multi-word commands: `check-rules` not `checkRules`
4. **Provide aliases** when renaming commands for backwards compatibility
5. **Be consistent** across all asset types - if `dashboards` has `create`, all assets should have `create`

## Parity Between `apply` and CRUD Commands
The `apply` command and the individual CRUD subcommands (e.g., `check-rules create`, `dashboards create`) must have the same expressiveness.
Any asset format accepted by `apply` must also be accepted by the corresponding `create` command, and vice versa.
For example, `dash0 apply -f prometheusrule.yaml` and `dash0 check-rules create -f prometheusrule.yaml` both accept PrometheusRule CRD files.
Similarly, `dash0 apply -f persesdashboard.yaml` and `dash0 dashboards create -f persesdashboard.yaml` both accept PersesDashboard CRD files.
Shared parsing and import logic lives in `internal/asset/` so that both code paths stay in sync.

**This is a hard rule, not a guideline.**
Adding a new Asset CRUD command without wiring its `kind` into `internal/apply/apply.go` is incomplete work and must be flagged in review.
Adding a new schema version (e.g. `apiVersion: v1alpha2`) to an existing asset without teaching `apply` to detect and route on it is the same kind of incomplete work.
See [docs/adding-commands.md, step 4](adding-commands.md#4-wire-asset-crud-commands-into-apply) for the concrete wiring steps and the [verification step](adding-commands.md#11-verify) that proves the wiring is in place.
