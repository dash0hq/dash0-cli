# About the Dash0 CLI

The Dash0 CLI (`dash0`) is a command-line interface for the [Dash0](https://www.dash0.com) observability platform.
It exposes the same primitives as the web UI — dashboards, views, check rules, synthetic checks, teams, notification channels — plus telemetry queries and OTLP send operations, as commands that humans, agentic AIs, and CI/CD workflows can drive.
Humans authenticate interactively via OAuth 2.0 with `dash0 login`; CI/CD and agent workflows use static auth tokens — see the [quickstart](quickstart.md).

## Who it's for

- **Humans** managing Dash0 assets and querying telemetry from the terminal.
- **AI coding agents** (Claude Code, Cursor, Codex, Copilot, and others) that need discoverable, structured commands with predictable output.
  Every command supports JSON output, structured `--help`, and JSON error formatting.
  Agent mode makes those defaults automatic when the CLI detects an agent in the environment.
- **CI/CD pipelines** that keep dashboards, rules, and other assets in sync with git via GitOps-style `apply`, or emit deployment events at release time.

## What it does

- **Manage assets as code.**
  Create, list, get, update, and delete dashboards, views, check rules, recording rules, synthetic checks, notification channels, and spam filters — one command at a time or via `dash0 apply -f <dir>` for GitOps flows.
- **Query telemetry.**
  Search logs, spans, traces, metrics, and failed checks with a common filter syntax and time-range flags.
- **Send telemetry.**
  Emit logs, spans, and deployment events via OTLP directly from your terminal or from GitHub Actions.
- **Manage profiles.**
  Configure multiple Dash0 environments (development, staging, production, or several organizations) with named profiles and either OAuth or static-token authentication.

## Design principles

- **Ergonomic for agents by default.**
  Structured JSON output, JSON help, JSON errors, no interactive prompts, no colored output — all triggered automatically when the CLI is invoked by a known AI coding agent.
  `dash0 skill install` adds a local [Agent Skill](commands.md#agent-tooling-commands) to a project — a `SKILL.md` plus reference docs — so Claude Code, Cursor, Codex, and GitHub Copilot sessions there discover the command surface without spending turns on `--help` exploration.
- **Consistent surface across asset types.**
  Every asset kind uses the same five subcommands (`list`, `get`, `create`, `update`, `delete`), the same output formats (`table`, `wide`, `json`, `yaml`, `csv`), and the same idempotent-upsert semantics.
- **Idempotent by design.**
  `apply` and per-asset `create` perform create-or-replace (PUT) when the source document carries a user-defined identifier — safe to run repeatedly from CI.
- **No secrets on the command line.**
  Authentication and connection settings live in profiles or environment variables; the CLI never asks for a token as a positional argument.

## Next steps

- [Installation](installation.md) — install via Homebrew, Docker, Nix, or from source.
- [Quickstart](quickstart.md) — from login to your first commands in about five minutes.
- [Command Reference](commands.md) — full syntax and examples for every command.
- [GitHub Actions](github-actions.md) — use the CLI from your workflows.
