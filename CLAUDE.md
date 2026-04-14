# Dash0 CLI Development Guide

This repository provides a CLI utility to perform various tasks related with Dash0, enabling users via terminal,
AI agents and CI/CD workflows to perform tasks like creating, updating and deleting a number of assets in Dash0 like dashboards, alerting rules, views and more.

## Commands
- Build: `make build`
- Test all: `make test`
- Test unit only: `make test-unit`
- Test integration only: `make test-integration`
- Roundtrip tests only: `make test-roundtrip`
- Test specific: `go test -v ./internal/config -run TestServiceAddContext`
- Run locally: `./dash0 [command]`

## Development guidelines

Detailed guidelines are split into focused documents:

- @docs/adding-commands.md — step-by-step guide for adding a new command
- @docs/commands.md — full command reference with flags, outputs, and examples
- @docs/cli-naming-conventions.md — command naming, aliases, asset kind display names
- @docs/code-style.md — Go style, dependencies, error handling
- @docs/project-structure.md — directory layout, package responsibilities
- @docs/documentation.md — prose rules, attribute keys in examples, validation
- @docs/testing.md — test strategies, integration tests, fixtures, mock server, roundtrip tests
- @docs/github-actions.md — how to create GitHub actions based on the `dash0` CLI, existing actions and their maintenance
- @docs/changelog-maintenance.md — when and how to create changelog entries

## Key Dash0 API concepts

### Origin vs ID

**Origin** and **ID** are distinct concepts — do not conflate them.

- **Origin** identifies which system is the authoritative source of truth for an asset (e.g., `dash0-cli`, `terraform`, `ui`).
  It is provenance/ownership metadata, not a lookup key.
  It is stored as the label `dash0.com/origin` on check rules, synthetic checks, and views.
  The CLI strips it before sending assets to the API (`StripXxxServerFields`), because the CLI should not claim ownership when the asset may be managed by another system.

- **ID** is the user-defined external identifier used for upsert (create-or-replace) operations.
  When an asset has a user-defined ID, `apply` and the individual import functions always use PUT (which has upsert semantics) instead of POST.
  The ID field varies by asset type:
  - Check rules: top-level `id` field (`PrometheusAlertRule.Id`)
  - Views: `metadata.labels["dash0.com/id"]` (`ViewLabels.Dash0Comid`)
  - Synthetic checks: `metadata.labels["dash0.com/id"]`
  - Dashboards: `metadata.dash0extensions.id` (`DashboardMetadataExtensions.Id`)

The `ORIGIN` column in `list -o wide` output shows the `origin` provenance field, not the ID.

For dashboards, the ID must be cleared from the request body before PUT (update) calls.
The ID is already passed as the URL path parameter (`originOrId`); sending it in both places causes the server to reject the request when the ID is a UUID that matches the dashboard's own server-assigned ID.
Use `dash0api.ClearDashboardID()` before calling `UpdateDashboard`.
This applies to both `ImportDashboard` (used by `apply`) and `dashboards update`.

### Asset annotations

Assets (dashboards, views, synthetic checks) have typed annotation structs with these fields:

| Annotation | User-settable | Behavior |
|------------|---------------|----------|
| `dash0.com/folder-path` | Yes | Fully user-controlled, round-trips through the API unchanged |
| `dash0.com/source` | Yes (dashboards only) | User-settable, but the server overrides it based on the API path used (e.g., `api`, `ui`) |
| `dash0.com/sharing` | Write-only | The API parses it into typed `Permissions` objects in `spec.permissions`, then strips the annotation before persisting. It is never returned in GET responses. Format: `"role:basic_member,team:team_123,user:alice@example.com"` |
| `dash0.com/deleted-at` | No | Server-managed soft-delete timestamp |

**`Strip*ServerFields` functions** (`dash0-api-client-go`) remove server-managed noise before sending assets to the API and for diff rendering.
They must only strip truly server-managed fields (`deleted-at`, `createdAt`, `updatedAt`, `version`, `dataset`, `origin`).
User-settable annotations (`folder-path`, `sharing`, `source`) and `spec.permissions` must be preserved.

**`spec.permissions`** on views and synthetic checks holds typed permission objects (role + actions).
These are user-provided and must not be stripped.

## GitHub issues

When creating GitHub issues, describe **what** and **why** — not **how**.
Issues should only contain the problem statement, user-facing behavior, and acceptance criteria.
