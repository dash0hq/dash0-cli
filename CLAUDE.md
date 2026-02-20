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
