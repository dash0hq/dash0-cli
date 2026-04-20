# Promoting an experimental command to stable

This document is a step-by-step guide for removing the `--experimental` (`-X`) gate from a command.
The goal is to make the command available without `-X` while preserving backward compatibility for users who still pass it.

## 1. Remove the experimental gate

In the command's source file (e.g., `internal/tracing/spans_query.go`):

1. Remove the `experimental.RequireExperimental(cmd)` call from `RunE`.
2. Remove the `[experimental]` prefix from the `Short` field.
3. Remove `--experimental` / `-X` from all lines in the `Example` field.
4. Remove the `"github.com/dash0hq/dash0-cli/internal/experimental"` import if it is no longer used.

The `--experimental` flag is a persistent flag on the root command, so it is always accepted.
Commands that no longer check its value simply ignore it, which preserves backward compatibility.

## 2. Update unit tests

- Delete the `Test*RequiresExperimentalFlag` test (e.g., `TestQueryRequiresExperimentalFlag`, `TestSendRequiresExperimentalFlag`).
- Remove `-X` from `SetArgs` calls in remaining unit tests.

## 3. Update integration tests

- Remove `-X` from all `SetArgs` calls.
- Add a backward-compatibility test that passes `-X` and asserts the command still succeeds.
  Name it `Test<Command>_BackwardCompatWithExperimentalFlag`.
  This test should exercise the same mock server fixture as the main success test.

Example:

```go
func TestQueryLogs_BackwardCompatWithExperimentalFlag(t *testing.T) {
    testutil.SetupTestEnv(t)

    server := testutil.NewMockServer(t, testutil.FixturesDir())
    server.On(http.MethodPost, apiPathLogs, testutil.MockResponse{
        StatusCode: http.StatusOK,
        BodyFile:   fixtureQuerySuccess,
        Validator:  testutil.RequireHeaders,
    })

    cmd := newExperimentalLogsCmd()
    cmd.SetArgs([]string{"-X", "logs", "query", "--api-url", server.URL, "--auth-token", testLogsAuthToken})

    var err error
    output := testutil.CaptureStdout(t, func() {
        err = cmd.Execute()
    })

    require.NoError(t, err)
    assert.Contains(t, output, "Application started successfully")
}
```

## 4. Update roundtrip tests

Remove `-X` from all invocations of the promoted command in `test/roundtrip/test_*.sh` scripts.

## 5. Update documentation

### `docs/commands.md`

- Remove `(experimental)` from the section header (e.g., `### \`logs query\` (experimental)` → `### \`logs query\``).
- Remove the `Requires the -X (or --experimental) flag, plus` prefix from the description (keep the remaining requirements like `api-url` and `auth-token`).
- Remove `-X` from all example invocations in the section.
- Update the command taxonomy table if it mentions the command as experimental.

### `README.md`

- Remove the `> [!WARNING]` block about the command being experimental.
- Remove `-X` from all example invocations.

## 6. Create a changelog entry

Run `make chlog-new` and fill in the entry:

- `change_type`: `enhancement`
- `component`: the affected area (e.g., `logs, spans, traces`)
- `note`: describe which commands were promoted
- `subtext`: mention that `-X` is no longer required

Validate with `make chlog-validate`.

## 7. Verify

1. `make build` succeeds.
2. `make test` passes (unit + integration).
3. `make lint` passes.
4. `./dash0 <command> --help` shows help without `[experimental]` prefix.
5. `./dash0 -X <command> --help` still works (backward compatibility).
