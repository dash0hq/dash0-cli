# Project Structure

- `/cmd/dash0`: Main entrypoint
- `/docs`: Development guidelines referenced from `CLAUDE.md`
- `/internal/apply`: The `apply` command — orchestration only, delegates asset-specific logic to `internal/asset`
- `/internal/asset`: Shared asset logic (types, import functions, display helpers) used by both `apply` and the per-asset CRUD commands
- `/internal/checkrules`, `/internal/dashboards`, `/internal/syntheticchecks`, `/internal/views`: Per-asset CRUD commands — delegate asset-specific logic to `internal/asset`
- `/internal/client`: API client factory and error handling
- `/internal/color`: Severity-aware color formatting for terminal output
- `/internal/config`: Configuration management (profiles, resolution)
- `/internal/experimental`: Gate for commands behind the `--experimental` (`-X`) flag
- `/internal/logging`: The `logs` command group (`send` and `query` subcommands)
- `/internal/members`: The `members` command group — `members list`, `members invite`, `members remove` (experimental, org-wide, no dataset)
- `/internal/metrics`: Commands and utilities to retrieve metrics from Dash0
- `/internal/otlp`: Shared OTLP utilities (key-value pairs, trace/span ID parsing, scope defaults, log severity range constants and number-to-range mapping) used by send and query commands across signal types
- `/internal/output`: Output format parsing and formatting (table, wide, JSON, YAML)
- `/internal/query`: Shared query utilities (filter parsing, timestamp normalization, timestamp formatting) used by query commands across signal types (e.g., `logs query`, `spans query`)
- `/internal/teams`: The `teams` command group — `teams list`, `teams get`, `teams create`, `teams update`, `teams delete`, `teams add-members`, `teams remove-members` (experimental, org-wide, no dataset)
- `/internal/testutil`: Test helpers — mock HTTP server, fixture constants
- `/internal/tracing`: The `spans` and `traces` command groups — `spans send`, `spans query`, `traces get` — plus shared span helpers (kind/status conversions, duration formatting/parsing)
- `/internal/version`: Build version (set at build time via linker flags)

Logic that is shared between `apply` and CRUD commands (import with existence check, PrometheusRule conversion, kind display names, file I/O) must live in `internal/asset/`, not be duplicated across packages.
The per-asset packages and `apply` import from `internal/asset`, never from each other.

Logic that is shared across query commands for different signal types (filter parsing, timestamp normalization) must live in `internal/query/`, not be duplicated across per-signal packages like `internal/logs`.

Logic that is shared across send commands for different signal types (key-value parsing, trace/span ID parsing, scope defaults) must live in `internal/otlp/`, not be duplicated across per-signal packages.

Span and trace commands live together in `internal/tracing/` since they share helpers, types, and the `extractServiceName` utility.

Organize code by domain, make interfaces for testability, and follow standard Go package layout.
