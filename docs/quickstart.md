# Quickstart

This walkthrough takes you from a freshly installed `dash0` CLI to running your first commands against a Dash0 environment in about five minutes.
It assumes you have already [installed the CLI](installation.md).

## 1. Log in

Authenticate against your Dash0 environment:

```bash
dash0 login --profile default --api-url https://api.eu-west-1.aws.dash0.com
```

Replace the URL with the Dash0 region your organization uses — see the [Dash0 API endpoints settings page](https://app.dash0.com/goto/settings/endpoints?endpoint_type=api_http) for the exact URL.

If the `default` profile does not exist yet, the command prompts to create it and marks it as OAuth-authenticated.
It then opens your browser, walks through OAuth 2.0 with PKCE, and saves the access token to that profile.
Once you have one profile, subsequent `dash0 login` calls without `--profile` refresh the active profile.

If you cannot use an interactive login (for example in a CI runner or an AI agent session), set `DASH0_AUTH_TOKEN` to a static `auth_*` token instead — see the [`login` reference](commands.md#login) for the full authentication story.

## 2. Confirm the active profile

```bash
dash0 config show
```

The output shows the resolved API URL, OTLP URL, dataset, and auth-token metadata for the profile the CLI will use.

## 3. List some assets

Try listing the dashboards you can see:

```bash
dash0 dashboards list
```

The same pattern works for every other asset type — `views`, `check-rules`, `synthetic-checks`, `recording-rules`, `notification-channels`, `spam-filters`.

For structured output (useful in scripts and agent workflows):

```bash
dash0 dashboards list -o json
```

## 4. Query telemetry

Run a query for the most recent logs:

```bash
dash0 logs query --from now-15m
```

Narrow to a specific service:

```bash
dash0 logs query --from now-15m --filter "service.name is my-service"
```

The same time-range, filter, and column flags apply to `dash0 spans query`, `dash0 metrics instant`, and `dash0 failed-checks query`.

## 5. Send a deployment event

Emit a log event announcing that a deployment happened:

```bash
dash0 logs send "Deployment v1.0 completed" \
    --event-name dash0.deployment \
    --severity-number 9 \
    --resource-attribute service.name=my-service \
    --resource-attribute deployment.environment.name=production \
    --log-attribute deployment.status=succeeded
```

> [!NOTE]
> `dash0 logs send` — like every OTLP send command — needs a static `auth_*` token; the Dash0 OTLP ingress does not accept OAuth access tokens.
> If you logged in with `dash0 login`, get an [auth token](https://app.dash0.com/goto/settings/auth-tokens) and either pass `--auth-token auth_<...>` for this call or set `DASH0_AUTH_TOKEN=auth_<...>` in your environment.
> See [Send commands](commands.md#send-commands) for the full story.

The event surfaces in the Dash0 UI's deployment stream and can be correlated with the traces and logs that follow it.

## Where to next

- [Command Reference](commands.md) — full syntax and examples for every command.
- [GitHub Actions](github-actions.md) — run the CLI from CI/CD workflows.
