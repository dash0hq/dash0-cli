# Code Style

## Dependencies

Never add dependencies with licenses incompatible with Apache 2.0 (the project's license).
Acceptable licenses: Apache 2.0, MIT, BSD-2-Clause, BSD-3-Clause, ISC, MPL-2.0.
Reject GPL, LGPL, AGPL, SSPL, and other copyleft licenses.
Always check the license before adding a dependency.
Keep the [Direct production dependencies](#direct-production-dependencies) updated.

### Direct production dependencies

| Module | Purpose |
|--------|---------|
| `github.com/dash0hq/dash0-api-client-go` | API and OTLP client for the Dash0 backend |
| `github.com/fatih/color` | Semantic coloring of terminal output (severity levels, errors) |
| `github.com/google/uuid` | UUID generation for asset imports |
| `github.com/spf13/cobra` | CLI command structure, flag parsing, and routing |
| `go.opentelemetry.io/collector/pdata` | OTLP data structures for logs, traces, and metrics |
| `gopkg.in/yaml.v3` | YAML marshalling/unmarshalling for asset definitions, needed for YAML stream processing |
| `sigs.k8s.io/yaml` | YAML handling that respects JSON struct tags and `omitempty` |

## Style Rules
- Use Go 1.24+ features
- Format with `gofmt`
- Add unit tests for all new functionality
- Use zerolog for structured logging
- Error handling: wrap errors with descriptive messages using `fmt.Errorf("... %w", err)`.
  Never use lazy pluralization like `error(s)` or `file(s)` — use proper singular/plural forms based on the actual count (e.g., "1 error", "3 errors").
  Invest the extra lines of code to give users clear, natural-sounding messages.
  When an error wraps a nested cause, put the cause on a new line indented by 2 spaces relative to its parent, so the hierarchy is visually clear:
  ```
  Error: validation failed with 1 error:
    malformed.yaml: failed to parse YAML:
      yaml: line 6: could not find expected ':'
  ```
- Naming: use camelCase for variable names and PascalCase for exported functions/types
- Never introduce test-specific behavior (env var checks, test flags, etc.) in production code.
  Tests must exercise the real code paths.
  Use proper configuration (profiles via `DASH0_CONFIG_DIR`, environment variables, or CLI flags) to set up the state tests need.
