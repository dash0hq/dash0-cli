# Command patterns

This document describes the architecture and conventions of the `dash0` CLI's command system.
It is a reference for understanding existing commands, reviewing PRs, and adding new ones.

For the user-facing command reference (flags, outputs, examples), see @docs/commands.md.
For naming conventions and aliases, see @docs/cli-naming-conventions.md.

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

## Package structure

### Command registration

All commands are registered in `cmd/dash0/main.go` in the `init()` function:

```go
func init() {
    rootCmd.AddCommand(dashboards.NewDashboardsCmd())
    rootCmd.AddCommand(logging.NewLogsCmd())
    rootCmd.AddCommand(teams.NewTeamsCmd())
    // ...
}
```

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

### File layout per command type

**Asset CRUD package** (e.g., `internal/dashboards/`):

| File | Responsibility |
|------|----------------|
| `dashboards_cmd.go` | Parent command factory; registers subcommands |
| `list.go` | List with pagination, multiple output formats |
| `get.go` | Single asset retrieval |
| `create.go` | File-based creation with dry-run |
| `update.go` | File-based update with diff output |
| `delete.go` | Deletion with confirmation prompt |
| `integration_test.go` | Integration tests for all subcommands |

**Query package** (e.g., `internal/logging/`):

| File | Responsibility |
|------|----------------|
| `logs_cmd.go` | Parent command factory |
| `query.go` | Query implementation with filter/column/format support |
| `send.go` | Send implementation with OTLP attribute handling |
| `query_test.go` | Unit tests for format/column parsing |
| `integration_test.go` | Integration tests with mock server |

**Organizational package** (e.g., `internal/teams/`):

| File | Responsibility |
|------|----------------|
| `teams_cmd.go` | Parent command factory; registers CRUD + member subcommands |
| `list.go`, `get.go`, `create.go`, `update.go`, `delete.go` | Standard CRUD |
| `list_members.go`, `add_members.go`, `remove_members.go` | Relationship management |
| `integration_test.go` | Integration tests |

## Shared infrastructure

### `internal/asset/` — asset operations

Used by asset CRUD commands and `apply`.

| Export | Purpose |
|--------|---------|
| `CommonFlags`, `ListFlags`, `GetFlags`, `FileInputFlags`, `DeleteFlags` | Reusable flag structs with `Register*Flags()` helpers |
| `ImportDashboard()`, `ImportCheckRule()`, `ImportView()`, `ImportSyntheticCheck()` | Create-or-update logic (check if ID exists; update if so, create otherwise) |
| `ReadRawInput()`, `ReadDefinition()`, `ReadDefinitionFile()` | YAML/JSON file I/O (reading from stdin, files, and with unmarshalling) |
| `KindDisplayName()` | Human-readable names for asset kinds (e.g., `"CheckRule"` → `"Check rule"`) |
| `ImportResult`, `ImportAction` | Result types for import operations |

Logic shared between `apply` and the per-asset CRUD commands must live here, not be duplicated.

### `internal/query/` — query operations

Used by `logs query`, `spans query`, and `traces get`.

| Export | Purpose |
|--------|---------|
| `ParseFilters()` | Parses `--filter` values (text `key [operator] value` or JSON from the Dash0 UI) |
| `ColumnDef`, `ColumnSpec`, `ResolveColumns()` | Column alias resolution for `--column`; unknown keys become arbitrary attribute columns |
| `NormalizeTimestamp()` | Parses relative (`now-1h`) and absolute (ISO 8601) timestamps |

All query commands must use the same filter syntax.
Inventing a different filter syntax for a new command is an anti-pattern (see [anti-patterns](#anti-patterns)).

### `internal/otlp/` — OTLP utilities

Used by send commands (`logs send`, `spans send`).

| Export | Purpose |
|--------|---------|
| `ParseKeyValuePairs()` | Parses `key=value` attribute flags |
| `ParseTraceID()`, `ParseSpanID()` | Validates and parses hex ID strings |
| `ResolveScopeDefaults()` | Sets scope name/version to defaults if not explicitly provided |
| `FindAttribute()`, `MergeAttributes()` | Attribute lookup and merging utilities for `dash0api.KeyValue` slices |
| `SeverityNumberToRange()` | Maps OpenTelemetry severity numbers to range labels (INFO, WARN, ERROR, etc.) |

### `internal/output/` — output formatting

Used by asset CRUD commands.

| Export | Purpose |
|--------|---------|
| `ParseFormat()` | Parses the `-o` flag value; defaults to JSON in agent mode, table otherwise |
| `Formatter` | Multi-format output (table, wide, json, yaml, csv) with `WithSkipHeader` option |
| `ValidateSkipHeader()` | Rejects `--skip-header` with json/yaml output |

Non-asset commands (teams, members, query commands) define their own format parsers because they support a different set of formats.
These local parsers must still handle the empty-string case for agent mode (see [agent mode](#agent-mode)).

### `internal/client/` — API client and errors

Used by all commands that talk to the Dash0 API.

| Export | Purpose |
|--------|---------|
| `NewClientFromContext()` | Creates an API client from context configuration + flag overrides |
| `NewOtlpClientFromContext()` | Creates an OTLP client for send commands |
| `HandleAPIError()` | Translates API errors into user-friendly messages |
| `ErrorContext` | Struct for asset type/ID/name context in error messages |
| `ResolveDataset()` | Resolves the dataset from context + flag |
| `ResolveApiUrl()` | Resolves the API URL from context + flag |

### `internal/confirmation/` — destructive operation prompts

Used by delete and remove commands.

```go
confirmed, err := confirmation.ConfirmDestructiveOperation(
    fmt.Sprintf("Are you sure you want to delete dashboard %q? [y/N]: ", id),
    flags.Force,
)
```

The prompt is skipped when `--force` is set or agent mode is active.
All destructive commands must use this helper instead of implementing their own prompt logic.

### `internal/experimental/` — experimental feature gate

Used by commands gated behind `--experimental` (`-X`).

```go
RunE: func(cmd *cobra.Command, args []string) error {
    if err := experimental.RequireExperimental(cmd); err != nil {
        return err
    }
    // ...
},
```

Call `experimental.RequireExperimental(cmd)` at the start of `RunE` for experimental commands.

## Standard patterns

### Flag registration

**Output format flag (`-o`):** Always register with an empty-string default:

```go
cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format: table, json, csv ...")
```

Never hardcode `"table"` as the default.
The empty string is resolved at runtime by the format parser, which checks `agentmode.Enabled` to choose between JSON and table.
Flags are registered during `init()`, before agent mode is resolved in `main()` — a hardcoded default would bake in the wrong value.

**Asset commands** reuse the flag structs from `internal/asset/flags.go`:

```go
var flags asset.ListFlags
asset.RegisterListFlags(cmd, &flags)
```

**Query commands** define their own flag structs because they have different fields (`--from`, `--to`, `--filter`, `--column`).

**Repeatable flags** use `StringArrayVar` (not `StringSliceVar`, which splits on commas):

```go
cmd.Flags().StringArrayVar(&flags.Filter, "filter", nil, "Filter expression (repeatable)")
```

### Agent mode

Agent mode (`--agent-mode` / `DASH0_AGENT_MODE`) optimizes the CLI for AI coding agents.
It is resolved once in `main()` and stored in `agentmode.Enabled`.

**What it affects:**

| Concern | Agent mode behavior | Where it is handled |
|---------|--------------------|--------------------|
| Output format default | JSON instead of table | Format parsers (`output.ParseFormat`, local `parse*Format`) |
| Help output | Structured JSON | `help.PrintJSONHelp`, installed in `main()` |
| Error output | JSON on stderr | `agentmode.PrintJSONError`, called from `main()` |
| Confirmation prompts | Auto-skipped | `confirmation.ConfirmDestructiveOperation` |
| Color | Disabled | `dashcolor.NoColor = true` in `main()` |

**For asset commands**, `output.ParseFormat("")` handles the agent mode default automatically.

**For non-asset commands** with their own format types, each local parser must handle the empty case:

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

Errors and help are handled globally in `main()` — individual commands do not need agent-mode-specific logic for these.

### Output formatting

**Asset commands** use `output.Formatter` for all format rendering:

```go
format, err := output.ParseFormat(flags.Output)
formatter := output.NewFormatter(format, os.Stdout, output.WithSkipHeader(flags.SkipHeader))
```

**Query commands** handle formatting directly because their output structure differs from assets (streaming rows vs. structured objects).
They still follow the same format names (`table`, `json`, `csv`).

**Conventions:**
- Table output includes column headers (unless `--skip-header` is set).
- JSON output is the full structured payload (OTLP/JSON for query commands, asset definitions for asset commands).
- YAML output for `list` produces a multi-document stream (separated by `---`) suitable for piping to `apply -f -`.
- CSV output uses the same columns as `wide` (for assets) or the default columns (for queries).
- Confirmation messages avoid the word "successfully" — write `Dashboard "X" created`, not `Dashboard "X" created successfully`.

### Error handling

All API errors flow through `client.HandleAPIError`:

```go
err = apiClient.DeleteDashboard(ctx, id, dataset)
if err != nil {
    return client.HandleAPIError(err, client.ErrorContext{
        AssetType: "dashboard",
        AssetID:   id,
    })
}
```

This provides consistent, user-friendly messages for common HTTP errors (401, 403, 404, 409, 429, 5xx).
Never format API errors manually in command code.

For non-API errors (validation, file I/O), wrap with context using `fmt.Errorf("... %w", err)`.
See @docs/code-style.md for error formatting conventions.

### Cobra command conventions

Every command should have:

- `Use`: the command name with argument placeholders (e.g., `"delete <id>"`).
- `Short`: a one-line description.
- `Long`: a detailed description, ending with `internal.CONFIG_HINT` for commands that need credentials.
- `Example`: indented by 2 spaces, with `#` comments explaining each use case. Use `<id>` as a placeholder — never use actual or fake UUIDs.
- `Aliases`: standard aliases per @docs/cli-naming-conventions.md.
- `Args`: argument validation (e.g., `cobra.ExactArgs(1)`).
- `RunE`: the implementation function (not `Run`) — always return errors instead of calling `os.Exit`.

```go
cmd := &cobra.Command{
    Use:     "delete <id>",
    Aliases: []string{"remove"},
    Short:   "Delete a dashboard",
    Long:    `Delete a dashboard by its ID.` + internal.CONFIG_HINT,
    Example: `  # Delete with confirmation prompt
  dash0 dashboards delete <id>

  # Delete without confirmation
  dash0 dashboards delete <id> --force`,
    Args: cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        return runDelete(cmd.Context(), args[0], &flags)
    },
}
```

## Testing

See @docs/testing.md for the full testing strategy.
This section covers patterns specific to command testing.

### Integration test structure

Integration tests use a mock HTTP server and JSON fixture files.
They are gated behind a build tag:

```go
//go:build integration

package dashboards
```

Name integration test files with the `_integration_test.go` suffix.

### Mock server setup

```go
func TestListDashboards_Success(t *testing.T) {
    testutil.SetupTestEnv(t)

    server := testutil.NewMockServer(t, testutil.FixturesDir())
    server.On(http.MethodGet, "/api/dashboards", testutil.MockResponse{
        StatusCode: http.StatusOK,
        BodyFile:   "dashboards/list_success.json",
        Validator:  testutil.RequireHeaders,
    })

    cmd := NewDashboardsCmd()
    cmd.SetArgs([]string{"list", "--api-url", server.URL, "--auth-token", testAuthToken})

    var err error
    output := testutil.CaptureStdout(t, func() {
        err = cmd.Execute()
    })

    require.NoError(t, err)
    assert.Contains(t, output, "NAME")
}
```

Key patterns:
- Call `testutil.SetupTestEnv(t)` to set required environment variables.
- Use `testutil.NewMockServer(t, testutil.FixturesDir())` to create the mock server.
- Register routes with `server.On()` (exact path) or `server.OnPattern()` (regex).
- Always pass `testutil.RequireHeaders` as the validator to check auth token and user agent.
- Use `testutil.CaptureStdout` to capture command output.
- Use constants from `internal/testutil/mockserver.go` for fixture paths.

### Testing experimental commands

Experimental commands need the `-X` flag, which is a persistent flag on the root command.
Create a wrapper root command in tests:

```go
func newExperimentalTeamsCmd() *cobra.Command {
    root := &cobra.Command{Use: "dash0"}
    root.PersistentFlags().BoolP("experimental", "X", false, "Enable experimental features")
    root.AddCommand(NewTeamsCmd())
    return root
}

// Usage: cmd.SetArgs([]string{"-X", "teams", "list", ...})
```

### Standard test cases

Every command should test at minimum:
- **Success case** — correct output format and content.
- **Empty result** — e.g., "No dashboards found" for list.
- **Authentication error** (401) — verifies error message.
- **Not found** (404) — for get/update/delete.
- **All output formats** — table, json, csv (and wide/yaml for asset commands).

### Fixture organization

Fixtures are stored in `internal/testutil/fixtures/`, organized by asset type:

```
fixtures/
  dashboards/
    list_success.json
    list_empty.json
    get_success.json
    error_not_found.json
    error_unauthorized.json
  checkrules/
    ...
  logs/
    query_success.json
    query_empty.json
  teams/
    ...
```

Use the fixture constants defined in `internal/testutil/mockserver.go` (e.g., `testutil.FixtureDashboardsListSuccess`).

## Anti-patterns

These are concrete mistakes to avoid, drawn from real code review feedback.

### Hardcoding the output format default

```go
// WRONG: bakes in "table", ignoring agent mode
cmd.Flags().StringVarP(&flags.Output, "output", "o", "table", "Output format")

// CORRECT: empty default, resolved at runtime
cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output format")
```

The format parser checks `agentmode.Enabled` to choose the right default.
A hardcoded `"table"` means the parser sees `"table"` instead of `""` and never considers agent mode.

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

### Putting test-specific behavior in production code

Never add environment variable checks, test flags, or conditional logic that only exists to support tests.
Tests must exercise the real code paths.
Use proper configuration (profiles via `DASH0_CONFIG_DIR`, environment variables, or CLI flags) to set up the state tests need.

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

- [ ] Create `internal/<command>/` with the appropriate file layout (see [package structure](#package-structure)).
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
- [ ] Register `-o` with an empty-string default.
- [ ] (Destructive) Add `--force` and use `confirmation.ConfirmDestructiveOperation`.

#### 4. Implement the command logic

- [ ] Use `RunE` (not `Run`) and return errors.
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

#### 6. Add examples and help text

- [ ] Add `Example` field with 2-space indentation and `#` comments.
- [ ] Use `<id>` as a placeholder — no fake UUIDs.
- [ ] Add `Long` description ending with `internal.CONFIG_HINT` if the command needs credentials.
- [ ] Add standard aliases per @docs/cli-naming-conventions.md.

#### 7. Write tests

- [ ] Add integration tests with the `//go:build integration` tag.
- [ ] Create fixture files in `internal/testutil/fixtures/<type>/`.
- [ ] Add fixture constants in `internal/testutil/mockserver.go`.
- [ ] Test success, empty result, auth error, not found, and all output formats.
- [ ] (Experimental) Use the wrapper root command pattern for the `-X` flag.
- [ ] Run `make test-integration` to verify.

#### 8. Update documentation

- [ ] Add the command to @docs/commands.md with flags, outputs, and examples.
- [ ] Update @README.md if the command adds new user-facing functionality.
- [ ] Add to @docs/cli-naming-conventions.md if introducing new subcommand patterns.

#### 9. Create a changelog entry

- [ ] Run `make chlog-new` and fill in the YAML fields.
- [ ] Run `make chlog-validate`.
- [ ] See @docs/changelog-maintenance.md for details.

#### 10. Verify

- [ ] `make build` succeeds.
- [ ] `make test` passes (unit + integration).
- [ ] `make lint` passes.
- [ ] `./dash0 <command> --help` shows correct help text.
- [ ] `./dash0 --agent-mode <command> --help` shows JSON help.
- [ ] Test the command against a real Dash0 environment if possible.

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
