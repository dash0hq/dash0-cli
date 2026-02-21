# CLI Naming Conventions

In Dash0, dashboards, views, synthetic checks and check rules are called "assets", rather than the more common "resources".
The reason for this is that the word "resource" is overloaded in OpenTelemetry, where it describes "where telemetry comes from".
Use the word "asset" consistently where appropriate.

## Top-level Asset Commands
- Use **plural form**: `dashboards`, `views`, `check-rules`, `synthetic-checks`
- Use **kebab-case** for multi-word names: `check-rules`, `synthetic-checks`
- Group related functionality: `config profiles` for profile management

## Standard CRUD Subcommands for Assets
All asset commands (`dashboards`, `check-rules`, `views`, `synthetic-checks`) use these subcommands:

| Subcommand | Alias    | Description |
|------------|----------|-------------|
| `list`     | `ls`     | List all assets |
| `get`      | -        | Get a single asset by ID |
| `create`   | `add`    | Create a new asset from a file |
| `update`   | -        | Update an existing asset from a file |
| `delete`   | `remove` | Delete an asset by ID |

## Config Profiles Subcommands
The `config profiles` command uses:

| Subcommand | Alias    | Description |
|------------|----------|-------------|
| `list`     | `ls`     | List all profiles |
| `create`   | `add`    | Create a new profile |
| `update`   | -        | Update an existing profile |
| `delete`   | `remove` | Delete a profile |
| `select`   | `activate` | Set the active profile |

## Aliases
- `activate` → `select`
- `add` → `create`
- `remove` → `delete`
- `ls` → `list`

## Asset Kind Display Names
In user-facing output (success messages, dry-run listings, error messages), use human-readable names for asset kinds — **not** PascalCase identifiers:

| Kind identifier   | Display name       |
|-------------------|--------------------|
| `Dashboard`       | Dashboard          |
| `CheckRule`       | Check rule         |
| `SyntheticCheck`  | Synthetic check    |
| `View`            | View               |
| `PrometheusRule`  | PrometheusRule     |

For example: `Check rule "High Error Rate" created successfully`, not `CheckRule "High Error Rate" created successfully`.

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
Shared parsing and import logic lives in `internal/asset/` so that both code paths stay in sync.
