# GitHub Actions

The Dash0 CLI ships two composite GitHub Actions in this repository:

- [`setup`](https://github.com/dash0hq/dash0-cli/tree/main/.github/actions/setup) — installs and configures the Dash0 CLI in CI workflows.
- [`send-log-event`](https://github.com/dash0hq/dash0-cli/tree/main/.github/actions/send-log-event) — emits log events (typically deployment markers) to Dash0 via the CLI, without any bespoke shell scripting.

Each action's full reference — inputs, outputs, and quick-start snippets — lives on its own page.
