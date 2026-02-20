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
- `/internal/logs`: The `logs` command group (`send` and `query` subcommands)
- `/internal/metrics`: Commands and utilities to retrieve metrics from Dash0
- `/internal/output`: Output format parsing and formatting (table, wide, JSON, YAML)
- `/internal/query`: Shared query utilities (filter parsing, timestamp normalization) used by query commands across signal types (e.g., `logs query`)
- `/internal/severity`: OpenTelemetry log severity range constants and number-to-range mapping
- `/internal/testutil`: Test helpers — mock HTTP server, fixture constants
- `/internal/version`: Build version (set at build time via linker flags)

Logic that is shared between `apply` and CRUD commands (import with existence check, PrometheusRule conversion, kind display names, file I/O) must live in `internal/asset/`, not be duplicated across packages.
The per-asset packages and `apply` import from `internal/asset`, never from each other.

Logic that is shared across query commands for different signal types (filter parsing, timestamp normalization) must live in `internal/query/`, not be duplicated across per-signal packages like `internal/logs`.

Organize code by domain, make interfaces for testability, and follow standard Go package layout.
