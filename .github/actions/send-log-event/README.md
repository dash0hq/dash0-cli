# Send log event

Send a log event to [Dash0](https://www.dash0.com) via the Dash0 CLI.

This action is standalone: it installs the Dash0 CLI automatically if it is not already on `PATH`.
If the [setup](../setup/README.md) action has already run, the existing installation is reused.
Both actions use the same install path (`~/.dash0/bin`) and cache key, so they are fully compatible.

## Quick start

```yaml
steps:
  - name: Send deployment event
    uses: dash0hq/dash0-cli/.github/actions/send-log-event@main
    with:
      otlp-url: ${{ vars.DASH0_OTLP_URL }}
      auth-token: ${{ secrets.DASH0_AUTH_TOKEN }}
      event-name: dash0.deployment
      body: 'Deployment completed'
      severity-number: '9'
      service-name: my-service
      service-version: 1.2.3
      deployment-environment-name: production
      deployment-status: succeeded
      vcs-repository-url: ${{ github.server_url }}/${{ github.repository }}
      vcs-ref-head-revision: ${{ github.sha }}
      vcs-ref-head-name: ${{ github.ref_name }}
```

You can use any git ref: `@main`, `@v1.1.0`, or `@<commit-sha>`.

## Inputs

### CLI installation

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `cli-version` | No | latest | Dash0 CLI version to install (e.g., `1.1.0`). Minimum supported: `1.1.0`. Ignored if the CLI is already on `PATH`. |

### Connection

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `otlp-url` | Yes | | Dash0 OTLP HTTP endpoint URL. Overrides the active CLI profile. Find yours under [Endpoints](https://app.dash0.com/goto/settings/endpoints?endpoint_type=otlp_http). |
| `auth-token` | Yes | | Dash0 auth token. Overrides the active CLI profile. Store it in a [GitHub secret](https://docs.github.com/en/actions/security-for-github-actions/security-guides/using-secrets-in-github-actions). |
| `dataset` | No | `default` | Dash0 dataset name. Overrides the active CLI profile. |

If the [setup](../setup/README.md) action already configured a profile with these values, you do not need to specify them again.

### Log event

Inputs marked as **recommended** are not strictly required but should be set for meaningful log events.

| Input | Required | Description |
|-------|----------|-------------|
| `event-name` | **Yes** | Event name (e.g., `dash0.deployment`). |
| `body` | **Yes** | The log message body. |
| `service-name` | **Yes** | Service name (maps to the `service.name` resource attribute). |
| `service-namespace` | Recommended | Service namespace (maps to the `service.namespace` resource attribute). |
| `service-version` | Recommended | Service version (maps to the `service.version` resource attribute). |
| `deployment-environment-name` | Recommended | Deployment environment name (maps to the `deployment.environment.name` resource attribute). |
| `deployment-name` | Recommended | Deployment name (maps to the `deployment.name` resource attribute). |
| `deployment-id` | Recommended | Deployment ID (maps to the `deployment.id` resource attribute). |
| `deployment-status` | Recommended | Deployment status, e.g., `succeeded` or `failed` (maps to the `deployment.status` log attribute). |
| `severity-number` | Recommended | Severity number (1-24, per the [OpenTelemetry specification](https://opentelemetry.io/docs/specs/otel/logs/data-model/#field-severitynumber)). |
| `vcs-repository-url` | No | VCS repository URL (maps to the `vcs.repository.url.full` log attribute). |
| `vcs-ref-head-revision` | No | VCS head revision, e.g., a commit SHA (maps to the `vcs.ref.head.revision` log attribute). |
| `vcs-ref-head-name` | No | VCS head ref name, e.g., a branch name (maps to the `vcs.ref.head.name` log attribute). |
| `severity-text` | No | Severity text (e.g., `INFO`, `WARN`, `ERROR`). |
| `other-resource-attributes` | No | Additional resource attributes as `key=value` pairs, one per line. |
| `other-log-attributes` | No | Additional log record attributes as `key=value` pairs, one per line. |
| `time` | No | Log record timestamp in RFC3339 format (e.g., `2024-03-15T10:30:00.123456789Z`). Defaults to now. |
| `observed-time` | No | Observed timestamp in RFC3339 format. Defaults to now. |
| `trace-id` | No | Trace ID (32 hex characters). Must be specified together with `span-id`. |
| `span-id` | No | Span ID (16 hex characters). Must be specified together with `trace-id`. |
| `flags` | No | Log record flags. |
| `resource-dropped-attributes-count` | No | Number of dropped resource attributes. |
| `log-dropped-attributes-count` | No | Number of dropped log record attributes. |

The instrumentation scope is set automatically: scope name is `github.com/dash0hq/dash0-cli@send-log-event` and scope version is the installed CLI version.

## Supported runners

- `ubuntu-latest` (x64)
- `ubuntu-24.04-arm` (arm64)
