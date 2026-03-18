# Dash0 CLI Development Guide

This repository provides a CLI utility to perform various tasks related with Dash0, enabling users via terminal,
AI agents and CI/CD workflows to perform tasks like creating, updating and deleting a number of assets in Dash0 like dashboards, alerting rules, views and more.

## Commands
- Build: `make build`
- Test all: `make test`
- Test unit only: `make test-unit`
- Test integration only: `make test-integration`
- Test specific: `go test -v ./internal/config -run TestServiceAddContext`
- Run locally: `./dash0 [command]`

## Development guidelines

Detailed guidelines are split into focused documents:

- @docs/commands.md — full command reference with flags, outputs, and examples
- @docs/cli-naming-conventions.md — command naming, aliases, asset kind display names
- @docs/code-style.md — Go style, dependencies, error handling
- @docs/project-structure.md — directory layout, package responsibilities
- @docs/documentation.md — prose rules, attribute keys in examples, validation
- @docs/testing.md — test strategies, integration tests, fixtures, mock server
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

## GitHub issues

When creating GitHub issues, describe **what** and **why** — not **how**.
Issues should only contain the problem statement, user-facing behavior, and acceptance criteria.
