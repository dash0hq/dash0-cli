# GitHub Actions

The Dash0 CLI ships three composite GitHub Actions in this repository:

- [`setup`](../.github/actions/setup/README.md) — installs and configures the Dash0 CLI in CI workflows.
- [`send-log-event`](../.github/actions/send-log-event/README.md) — emits log events (typically deployment markers) to Dash0 via the CLI, without any bespoke shell scripting.
- [`sync-assets`](../.github/actions/sync-assets/README.md) — syncs a directory of Dash0 asset YAML files, deleting assets removed since the last push, without hand-writing `apply --since`'s GitHub event-payload gating.

Each action's full reference — inputs, outputs, and quick-start snippets — lives on its own page.
