# Setup Dash0 CLI

Install the [Dash0 CLI](https://github.com/dash0hq/dash0-cli) in your GitHub Actions workflows.

## Quick start

```yaml
steps:
  - uses: actions/checkout@v4

  - name: Setup Dash0 CLI
    uses: dash0hq/dash0-cli/.github/actions/setup@main
    with:
      api-url: ... # E.g. https://api.us-west-2.aws.dash0.com
      otlp-url: ... # E.g. https://ingress.us-west-2.aws.dash0.com
      auth-token: ${{ secrets.DASH0_AUTH_TOKEN }}
      dataset: ... # E.g., `production`; if omitted, the `default` dataset is used

  - run: dash0 dashboards list
```

You can use any git ref: `@main`, `@v1.0.0`, or `@<commit-sha>`.

## Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `version` | No | latest | Dash0 CLI version to install (e.g., `1.0.0`) |
| `cache` | No | `true` | Enable caching of the Dash0 CLI binary |
| `api-url` | No | | Dash0 API URL. Find yours under [Endpoints](https://app.dash0.com/goto/settings/endpoints?endpoint_type=api_http). |
| `otlp-url` | No | | Dash0 OTLP HTTP endpoint URL. Find yours under [Endpoints](https://app.dash0.com/goto/settings/endpoints?endpoint_type=otlp_http). |
| `auth-token` | No | | Dash0 auth token. Get one from [Auth Tokens](https://app.dash0.com/goto/settings/auth-tokens). Store it in a [GitHub secret](https://docs.github.com/en/actions/security-for-github-actions/security-guides/using-secrets-in-github-actions). |
| `dataset` | No | `default` | Dash0 dataset name |

When any of these inputs are provided, the action creates a `default` CLI profile so subsequent `dash0` commands use them automatically.

## Outputs

| Output | Description |
|--------|-------------|
| `version` | Installed Dash0 CLI version (e.g., `v1.0.0`) |

## Configuration via environment variables

Instead of (or in addition to) action inputs, you can configure the CLI via environment variables in individual steps:

```yaml
- name: List dashboards from a different dataset
  env:
    DASH0_DATASET: production
  run: dash0 dashboards list
```

See the [CLI documentation](https://github.com/dash0hq/dash0-cli#common-settings) for all supported environment variables.

## Supported runners

- `ubuntu-latest` (x64)
- `ubuntu-24.04-arm` (arm64)
