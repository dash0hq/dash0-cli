# Adding a new command

This document is a step-by-step guide for adding a new command to the `dash0` CLI.
It consolidates references to conventions and patterns documented elsewhere.

## 1. Determine the command type

Every command falls into one of four types.
See the [command taxonomy](commands.md#command-taxonomy) in @docs/commands.md for the user-facing specification, and the [reference implementations](code-style.md#reference-implementations) for the package to use as a template.

| Type | When to use | Reference package |
|------|-------------|-------------------|
| Asset CRUD | Manages a dataset-scoped asset (create, read, update, delete) | `internal/dashboards/` |
| Query | Searches or retrieves telemetry data (logs, spans, metrics) | `internal/logging/query.go` |
| Send | Sends telemetry data via OTLP | `internal/logging/send.go` |
| Organizational | Manages organization-level entities (not dataset-scoped) | `internal/teams/` |

If the command does not fit any type, gate it behind `--experimental` (`-X`) so the interface can evolve, and discuss the pattern with the team before implementing.
When a command is ready for general use, follow the [promoting commands to stable](promoting-commands-to-stable.md) guide to remove the experimental gate.

## 2. Create the package

Create `internal/<command>/` with a file layout matching the reference package for your command type.

Typical file layout for an asset CRUD command:

```
internal/<command>/
  <command>_cmd.go          # Parent command with New<Command>Cmd() factory
  list.go                   # list subcommand
  get.go                    # get subcommand
  create.go                 # create subcommand
  update.go                 # update subcommand
  delete.go                 # delete subcommand
  integration_test.go       # Integration tests
```

Export a single `New<Command>Cmd()` factory function that returns `*cobra.Command`.
Add subcommands inside this factory.
Individual subcommand factories (`newListCmd`, `newGetCmd`, etc.) should be unexported.

## 3. Register the command

In `cmd/dash0/main.go`:
1. Add the import for the new package.
2. Add `rootCmd.AddCommand(<package>.New<Command>Cmd())` in the `init()` function.

## 4. Implement flags

Use the shared flag structs from `internal/asset/flags.go` where applicable:
- `asset.ListFlags` for list commands (`--limit`, `--all`, `--skip-header`, plus common flags).
- `asset.GetFlags` for get commands (common flags only).
- `asset.FileInputFlags` for create/update commands (`--file`, `--dry-run`, plus common flags).
- `asset.DeleteFlags` for delete commands (`--force`, plus common flags).

For non-asset commands, define a custom flags struct with the flags appropriate to the command type:
- **Query:** `--from`, `--to`, `--filter`, `--limit`, `--skip-header`, `--column`.
- **Send:** `--resource-attribute`, `--scope-attribute`, plus a signal-specific attribute flag (e.g., `--log-attribute`).
  Use `StringArrayVar` (not `StringSliceVar`, which splits on commas) for repeatable flags.
- **Organizational:** no `--dataset`, no `-f`, no `--dry-run`.

Register `-o` / `--output` with an empty-string default — never hardcode `"table"`.
See @docs/code-style.md [agent mode](code-style.md#agent-mode) for why.

For destructive commands, add `--force` and use `confirmation.ConfirmDestructiveOperation`.

## 5. Implement the command logic

- Use `RunE` (not `Run`) and return errors.
  See @docs/code-style.md for error handling conventions.
- Gate experimental commands with `experimental.RequireExperimental(cmd)` at the start of `RunE`.
- Create the API client via `client.NewClientFromContext` (or `client.NewOtlpClientFromContext` for send commands).
- Handle API errors via `client.HandleAPIError`.
- Use shared logic from the appropriate package:
  - **Asset CRUD:** `asset.Import<AssetType>()` for create/update, shared flag structs from `internal/asset/flags.go`.
  - **Query:** `query.ParseFilters()`, `query.ResolveColumns()` from `internal/query/`.
  - **Send:** `otlp.ParseKeyValuePairs()`, `otlp.ResolveScopeDefaults()` from `internal/otlp/`.

Do not duplicate shared logic — see @docs/project-structure.md for where shared code belongs.

## 6. Implement agent mode

- **Asset commands:** handled automatically via `output.ParseFormat`.
- **Non-asset commands:** write a local `parse*Format` function that defaults to JSON when `agentmode.Enabled`.
  See the [agent mode checklist](code-style.md#adding-a-new-command-agent-mode-checklist) in @docs/code-style.md.
- Confirmation prompts, error formatting, and help rendering are handled globally — no per-command logic needed.

## 7. Add examples and help text

- Add an `Example` field to every cobra command.
  See @docs/code-style.md for formatting rules (2-space indent, `<id>` placeholders, `#` comments).
- Add a `Long` description.
- Add standard aliases per @docs/cli-naming-conventions.md.

## 8. Write tests

- Add integration tests using the mock server.
  See @docs/testing.md for fixtures, mock server setup, and test structure.
- Test at minimum: success, empty result, auth error, not found, and all output formats.
- Run `make test-integration` to verify.
- Add a roundtrip test if the command creates or modifies assets.
  See the [roundtrip tests](testing.md#roundtrip-tests) section in @docs/testing.md.
- Ensure roundtrip tests work with `make test-roundtrip`

## 9. Update documentation

- Add the command to @docs/commands.md under the appropriate [taxonomy category](commands.md#command-taxonomy), with flags, outputs, and examples.
- Update @README.md if the command adds new user-facing functionality.
- Add to @docs/cli-naming-conventions.md if introducing new subcommand patterns.
- Create a changelog entry — see @docs/changelog-maintenance.md.

## 10. Verify

- `make build` succeeds.
- `make test` passes (unit + integration).
- `make lint` passes.
- `./dash0 <command> --help` shows correct help text.
- `./dash0 --agent-mode <command> --help` shows JSON help.
