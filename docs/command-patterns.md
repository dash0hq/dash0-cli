# Command patterns

This document describes the architecture and conventions of the `dash0` CLI's command system.
It is a reference for understanding existing commands, reviewing PRs, and adding new ones.

For the user-facing command reference (flags, outputs, examples), see @docs/commands.md.
For naming conventions and aliases, see @docs/cli-naming-conventions.md.
For package responsibilities and directory layout, see @docs/project-structure.md.

## Command taxonomy

Every command in the CLI falls into one of four categories.
Each category has distinct patterns for flags, output, testing, and shared infrastructure.

### Asset CRUD commands

**What they do:** create, list, get, update, and delete Dash0 assets (dashboards, views, check rules, synthetic checks).

**Packages:** `internal/dashboards/`, `internal/views/`, `internal/checkrules/`, `internal/syntheticchecks/`

**Characteristics:**
- Five standard subcommands: `list` (`ls`), `get`, `create` (`add`), `update`, `delete` (`remove`).
- File-based input for `create` and `update` (`-f <file>`), with `--dry-run` support.
- List flags: `--limit` / `-l`, `--all` / `-a` (fetch all pages), `--skip-header`.
- Output formats: `table`, `wide`, `json`, `yaml`, `csv`.
- Shared flag structs from `internal/asset/flags.go` (`ListFlags`, `GetFlags`, `FileInputFlags`, `DeleteFlags`).
- Shared import logic from `internal/asset/` for create-or-update semantics.
- Confirmation prompt for `delete` via `confirmation.ConfirmDestructiveOperation`.

**Reference implementation:** `internal/dashboards/` — the most complete example.

### Query commands

**What they do:** search and retrieve telemetry signals (logs, spans, traces, metrics).

**Packages:** `internal/logging/` (`logs query`), `internal/tracing/` (`spans query`, `traces get`), `internal/metrics/` (`metrics instant`)

**Characteristics:**
- Time range flags: `--from`, `--to` (relative expressions like `now-1h` or absolute ISO 8601).
- Filter flag: `--filter` with the standard `key [operator] value` syntax from `internal/query/filter.go`.
- Column flag: `--column` for customizing table/CSV output, resolved via `internal/query/columns.go`.
- Pagination: `--limit` (no `--all` flag, unlike asset commands).
- Output formats: `table`, `json`, `csv` (no `wide` or `yaml`).
- Gated behind `--experimental` (`-X`).

**Reference implementation:** `internal/logging/query.go` — established the filter syntax and column patterns.

### Send commands

**What they do:** send telemetry data to Dash0 via OTLP.

**Packages:** `internal/logging/` (`logs send`), `internal/tracing/` (`spans send`)

**Characteristics:**
- Positional argument for the primary content (e.g., `logs send <body>`, `spans send --name <name>`).
- Repeatable attribute flags: `--resource-attribute`, `--log-attribute`/`--span-attribute`, `--scope-attribute`.
- OTLP-specific flags: `--trace-id`, `--span-id`, `--scope-name`, `--scope-version`.
- Uses `client.NewOtlpClientFromContext` instead of the standard API client.
- Attribute parsing via `internal/otlp/parse.go`.

**Reference implementation:** `internal/logging/send.go`.

### Organizational commands

**What they do:** manage organizational entities (teams, members) that are not dataset-scoped assets.

**Packages:** `internal/teams/`, `internal/members/`

**Characteristics:**
- Flag-based input (not file-based) — no `-f`, no `--dry-run`, no `apply` integration.
- No `--dataset` flag — these operate at the organization level.
- Custom subcommands beyond CRUD (e.g., `teams add-members`, `teams list-members`).
- Gated behind `--experimental` (`-X`).

**Reference implementation:** `internal/teams/` — the most complete org-level command group.

## Command registration

All commands are registered in `cmd/dash0/main.go` in the `init()` function.
Each package exports a single `New<CommandName>Cmd()` factory function that returns a `*cobra.Command`.
Group commands add their subcommands inside the factory:

```go
func NewDashboardsCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "dashboards",
        Short: "Manage Dash0 dashboards",
        Long:  `Create, list, update, and delete dashboards in Dash0`,
    }
    cmd.AddCommand(newListCmd())
    cmd.AddCommand(newGetCmd())
    cmd.AddCommand(newCreateCmd())
    cmd.AddCommand(newUpdateCmd())
    cmd.AddCommand(newDeleteCmd())
    return cmd
}
```

The exported factory (`NewDashboardsCmd`) creates the parent command.
The unexported factories (`newListCmd`, `newGetCmd`, etc.) create individual subcommands.

## Key patterns

For general Go style, error handling, and Cobra conventions (help text, examples, `RunE`), see @docs/code-style.md.
For agent mode behavior and the checklist for new commands, see the agent mode section of @docs/code-style.md.
This section covers patterns specific to command implementation that are not covered elsewhere.

### Output format flag default

Always register the `-o` flag with an empty-string default:

```go
cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table, json, csv ...")
```

Never hardcode `"table"` as the default.
The empty string is resolved at runtime by the format parser, which checks `agentmode.Enabled` to choose between JSON and table.
Flags are registered during `init()`, before agent mode is resolved in `main()` — a hardcoded default would bake in the wrong value.

### Format parsers for non-asset commands

Asset commands use `output.ParseFormat()`, which handles agent mode automatically.
Non-asset commands (teams, members, query commands) define their own format parsers because they support a different set of formats.
These local parsers must handle the empty-string case:

```go
func parseQueryFormat(s string) (queryFormat, error) {
    switch strings.ToLower(s) {
    case "":
        if agentmode.Enabled {
            return queryFormatJSON, nil
        }
        return queryFormatTable, nil
    // ...
    }
}
```

### Repeatable flags

Use `StringArrayVar` (not `StringSliceVar`, which splits on commas):

```go
cmd.Flags().StringArrayVar(&flags.Filter, "filter", nil, "Filter expression (repeatable)")
```

### API client creation and error handling

All commands that talk to the Dash0 API use `client.NewClientFromContext` (or `NewOtlpClientFromContext` for send commands) and `client.HandleAPIError` for consistent error messages.
See `internal/client/client.go` for details.

## Anti-patterns

These are concrete mistakes to avoid, drawn from real code review feedback.

### Hardcoding the output format default

```go
// WRONG: bakes in "table", ignoring agent mode
cmd.Flags().StringVarP(&flags.Output, "output", "o", "table", "Output format")

// CORRECT: empty default, resolved at runtime
cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format")
```

### Inventing a new filter syntax

All `--filter` flags must use the standard `key [operator] value` syntax from `internal/query/filter.go`.
This syntax also accepts JSON filter criteria copied from the Dash0 UI.

Do not create a command-specific filter mechanism (e.g., substring matching, regex-only, custom operators).
If the existing syntax does not fit, discuss the extension with the team before implementing.

### Using the wrong verb

The CLI has specific semantics for each subcommand verb:

| Verb | Semantics | Has filters | Example |
|------|-----------|-------------|---------|
| `list` | Enumerate all assets in a dataset | No | `dashboards list` |
| `get` | Retrieve by ID or with query parameters | Depends on type | `traces get <trace-id>` |
| `query` | Search/filter telemetry signals | Yes | `logs query --filter ...` |
| `create` | Create from a file | N/A | `dashboards create -f ...` |
| `update` | Update from a file | N/A | `dashboards update -f ...` |
| `delete` | Delete by ID | N/A | `dashboards delete <id>` |

`list` commands do not have filters.
If a command needs filters, it is either a `query` or a `get` — not a `list`.

### Duplicating shared logic

Logic that is used by both `apply` and per-asset CRUD commands belongs in `internal/asset/`.
Logic shared across query commands belongs in `internal/query/`.
Logic shared across send commands belongs in `internal/otlp/`.

Never copy-paste shared logic into a per-command package.
If a pattern is repeated in two packages, extract it to the appropriate shared package.

## Adding a new command

### Decision tree

1. **Does it manage a dataset-scoped asset (create, read, update, delete)?** → Asset CRUD command.
   Follow `internal/dashboards/` as the reference.

2. **Does it search or retrieve telemetry data (logs, spans, metrics)?** → Query command.
   Follow `internal/logging/query.go` as the reference.

3. **Does it send telemetry data via OTLP?** → Send command.
   Follow `internal/logging/send.go` as the reference.

4. **Does it manage organizational entities (not dataset-scoped)?** → Organizational command.
   Follow `internal/teams/` as the reference.

5. **None of the above?** → Discuss the pattern with the team before implementing.
   Gate it behind `--experimental` so the interface can evolve without breaking changes.

### Step-by-step checklist

This checklist applies to all command types.
Items marked with a command type in parentheses only apply to that type.

#### 1. Create the package

- [ ] Create `internal/<command>/` with a file layout matching the reference implementation for your command type.
- [ ] Export a `New<CommandName>Cmd()` factory function that returns `*cobra.Command`.
- [ ] Add subcommands inside the factory.

#### 2. Register the command

- [ ] Add `rootCmd.AddCommand(<package>.New<CommandName>Cmd())` in `cmd/dash0/main.go`'s `init()` function.
- [ ] Add the import for the new package.

#### 3. Implement flags

- [ ] (Asset) Use the flag structs from `internal/asset/flags.go`.
- [ ] (Query) Define a custom flags struct with `--from`, `--to`, `--filter`, `--limit`, `--skip-header`, `--column`.
- [ ] (Send) Define a custom flags struct with `--otlp-url`, `--auth-token`, `--resource-attribute`, and signal-specific attributes.
- [ ] (Org) Define a custom flags struct without `--dataset`.
- [ ] Register `-o` with an empty-string default (see [key patterns](#output-format-flag-default)).
- [ ] (Destructive) Add `--force` and use `confirmation.ConfirmDestructiveOperation`.

#### 4. Implement the command logic

- [ ] Use `RunE` (not `Run`) and return errors — see @docs/code-style.md.
- [ ] (Experimental) Call `experimental.RequireExperimental(cmd)` at the start of `RunE`.
- [ ] Create the API client via `client.NewClientFromContext` or `client.NewOtlpClientFromContext`.
- [ ] Handle API errors via `client.HandleAPIError` with appropriate `ErrorContext`.
- [ ] (Asset) Use `asset.Import<AssetType>()` for create/update operations.
- [ ] (Query) Use `query.ParseFilters()` and `query.ResolveColumns()`.
- [ ] (Send) Use `otlp.ParseKeyValuePairs()` and `otlp.ResolveScopeDefaults()`.

#### 5. Implement agent mode

- [ ] (Asset) Handled automatically via `output.ParseFormat`.
- [ ] (Non-asset) Write a local `parse*Format` function that defaults to JSON when `agentmode.Enabled`.
- [ ] Confirmation prompts and errors are handled globally — no per-command logic needed.
- [ ] See the agent mode checklist in @docs/code-style.md.

#### 6. Add examples and help text

- [ ] Add `Example` field — see @docs/code-style.md for formatting rules.
- [ ] Add `Long` description ending with `internal.CONFIG_HINT` if the command needs credentials.
- [ ] Add standard aliases per @docs/cli-naming-conventions.md.

#### 7. Write tests

- [ ] Add integration tests — see @docs/testing.md for the mock server, fixtures, and test structure.
- [ ] Test success, empty result, auth error, not found, and all output formats.
- [ ] Run `make test-integration` to verify.

#### 8. Update documentation

- [ ] Add the command to @docs/commands.md with flags, outputs, and examples.
- [ ] Update @README.md if the command adds new user-facing functionality.
- [ ] Add to @docs/cli-naming-conventions.md if introducing new subcommand patterns.
- [ ] Create a changelog entry — see @docs/changelog-maintenance.md.

#### 9. Verify

- [ ] `make build` succeeds.
- [ ] `make test` passes (unit + integration).
- [ ] `make lint` passes.
- [ ] `./dash0 <command> --help` shows correct help text.
- [ ] `./dash0 --agent-mode <command> --help` shows JSON help.

## When to deviate

The patterns in this document cover the established command types.
New command types may need different patterns — that is fine, as long as the deviation is intentional.

**Before deviating:**
1. Gate the new command behind `--experimental` (`-X`) so the interface can evolve.
2. Discuss the proposed pattern with the team.
3. Document the new pattern in this file once it stabilizes.

**Common reasons to deviate:**
- The command needs interactive input (e.g., a TUI or streaming output).
- The command does not talk to the Dash0 API (e.g., local file transformation).
- The command bridges two systems with different conventions.
