# Dash0 CLI

A command-line interface for interacting with Dash0 services.

## Installation

### From Source

Requires Go 1.22 or higher.

```bash
# Clone the repository
git clone https://github.com/dash0hq/dash0-cli.git
cd dash0-cli

# Build and install
make install
```

## Usage

### Configuration

Configure API access using profiles.

```bash
$ dash0ctl config profile add --name dev --api-url https://api.eu-west-1.aws.dash0.com --auth-token auth_xxx
Profile "dev" added and set as active

$ dash0ctl config profile list
  NAME  API URL                                  AUTH TOKEN
* dev   https://api.eu-west-1.aws.dash0-dev.com  ...ULSzVkM

$ dash0ctl config show
Profile:    dev
API URL:    https://api.eu-west-1.aws.dash0-dev.com
Auth Token: ...ULSzVkM
```

The last seven digits of the auth token are displayed, matching the format shown in Dash0 as the `dash0.auth.token` attribute.

The API URL and the authentication tokens can be overridden using the `DASH0_API_URL` and `DASH0_AUTH_TOKEN` environment variables:

```bash
$ DASH0_API_URL='http://test' dash0ctl config show
Profile:    dev
API URL:    http://test    (from DASH0_API_URL environment variable)
Auth Token: ...ULSzVkM

$ DASH0_AUTH_TOKEN='my_auth_test_token' dash0ctl config show
Profile:    dev
API URL:    https://api.eu-west-1.aws.dash0-dev.com
Auth Token: ...t_token    (from DASH0_AUTH_TOKEN environment variable)
```

### Applying Resources

Apply resource definitions from a file. The file may contain multiple YAML documents separated by `---`:

```bash
$ dash0ctl apply -f resources.yaml
Dashboard "Production Overview" applied successfully
CheckRule "High Error Rate" applied successfully
View "Error Logs" applied successfully

$ dash0ctl apply -f dashboard.yaml --dry-run
Dry run: 1 document(s) validated successfully
  1. Dashboard
```

Supported resource types: `Dashboard`, `CheckRule` (both the plain Prometheus YAML and the PrometheusRule CRD), `PrometheusRule`, `SyntheticCheck`, `View`

### Dashboards

```bash
$ dash0ctl dashboards list
NAME                                      ID
Production Overview                       a1b2c3d4-5678-90ab-cdef-1234567890ab
...

$ dash0ctl dashboards get a1b2c3d4-5678-90ab-cdef-1234567890ab
Kind: Dashboard
Name: Production Overview
Created: 2026-01-15 10:30:00
Updated: 2026-01-20 14:45:00

$ dash0ctl dashboards get a1b2c3d4-5678-90ab-cdef-1234567890ab -o yaml
kind: Dashboard
metadata:
  name: a1b2c3d4-5678-90ab-cdef-1234567890ab
  ...
spec:
  display:
    name: Production Overview
  ...

$ dash0ctl dashboards create -f dashboard.yaml
Dashboard "My Dashboard" created successfully

$ dash0ctl dashboards update a1b2c3d4-5678-90ab-cdef-1234567890ab -f dashboard.yaml
Dashboard "My Dashboard" updated successfully

$ dash0ctl dashboards delete a1b2c3d4-5678-90ab-cdef-1234567890ab
Are you sure you want to delete dashboard "a1b2c3d4-..."? [y/N]: y
Dashboard "a1b2c3d4-..." deleted successfully

$ dash0ctl dashboards export a1b2c3d4-5678-90ab-cdef-1234567890ab -f dashboard.yaml
Dashboard exported to dashboard.yaml
```

### Check Rules

```bash
$ dash0ctl check-rules list
NAME                                      ID
High Error Rate Alert                     a1b2c3d4-5678-90ab-cdef-1234567890ab
...

$ dash0ctl check-rules get a1b2c3d4-5678-90ab-cdef-1234567890ab
Name: High Error Rate Alert
Expression: sum(rate(errors[5m])) > 0.1
Enabled: true
Description: Alert when error rate exceeds threshold

$ dash0ctl check-rules create -f rule.yaml
Check rule "High Error Rate Alert" created successfully

$ dash0ctl check-rules update a1b2c3d4-5678-90ab-cdef-1234567890ab -f rule.yaml
Check rule "High Error Rate Alert" updated successfully

$ dash0ctl check-rules delete a1b2c3d4-5678-90ab-cdef-1234567890ab --force
Check rule "a1b2c3d4-..." deleted successfully

$ dash0ctl check-rules export a1b2c3d4-5678-90ab-cdef-1234567890ab -f rule.yaml
Check rule exported to rule.yaml
```

Check rules are exported in Prometheus Operator `PrometheusRule` CRD format:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: High Error Rate Alert
  labels:
    dash0.com/dataset: default
    dash0.com/id: a1b2c3d4-5678-90ab-cdef-1234567890ab
spec:
  groups:
    - name: High Error Rate Alert
      interval: 1m0s
      rules:
        - alert: High Error Rate Alert
          expr: sum(rate(errors[5m])) > 0.1
          for: 5m
          labels:
            severity: critical
          annotations:
            description: Alert when error rate exceeds threshold
            summary: High error rate detected
```

You can also apply `PrometheusRule` CRD files directly:

```bash
$ dash0ctl apply -f prometheus-rules.yaml
PrometheusRule "High Error Rate Alert" applied successfully
```

### Synthetic Checks

```bash
$ dash0ctl synthetic-checks list
NAME                                      ID
API Health Check                          a1b2c3d4-5678-90ab-cdef-1234567890ab
...

$ dash0ctl synthetic-checks get a1b2c3d4-5678-90ab-cdef-1234567890ab
Kind: Dash0SyntheticCheck
Name: API Health Check
Description: Checks API endpoint availability

$ dash0ctl synthetic-checks create -f check.yaml
Synthetic check "API Health Check" created successfully

$ dash0ctl synthetic-checks update a1b2c3d4-5678-90ab-cdef-1234567890ab -f check.yaml
Synthetic check "API Health Check" updated successfully

$ dash0ctl synthetic-checks delete a1b2c3d4-5678-90ab-cdef-1234567890ab
Are you sure you want to delete synthetic check "a1b2c3d4-..."? [y/N]: y
Synthetic check "a1b2c3d4-..." deleted successfully

$ dash0ctl synthetic-checks export a1b2c3d4-5678-90ab-cdef-1234567890ab -f check.yaml
Synthetic check exported to check.yaml
```

### Views

```bash
$ dash0ctl views list
NAME                                      ID
Error Logs View                           a1b2c3d4-5678-90ab-cdef-1234567890ab
...

$ dash0ctl views get a1b2c3d4-5678-90ab-cdef-1234567890ab
Kind: Dash0View
Name: Error Logs View

$ dash0ctl views create -f view.yaml
View "Error Logs View" created successfully

$ dash0ctl views update a1b2c3d4-5678-90ab-cdef-1234567890ab -f view.yaml
View "Error Logs View" updated successfully

$ dash0ctl views delete a1b2c3d4-5678-90ab-cdef-1234567890ab --force
View "a1b2c3d4-..." deleted successfully

$ dash0ctl views export a1b2c3d4-5678-90ab-cdef-1234567890ab -f view.yaml
View exported to view.yaml
```

### Common Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--api-url` | | Override API URL from profile |
| `--auth-token` | | Override auth token from profile |
| `--dataset` | `-d` | Specify dataset to operate on |
| `--output` | `-o` | Output format: `table`, `json`, `yaml` |

### Shell Completions

Enable tab completion for your shell:

**Bash** (requires `bash-completion`):
```bash
# Current session
source <(dash0ctl completion bash)

# Permanent (Linux)
dash0ctl completion bash > /etc/bash_completion.d/dash0ctl

# Permanent (macOS with Homebrew)
dash0ctl completion bash > $(brew --prefix)/etc/bash_completion.d/dash0ctl
```

**Zsh**:
```bash
# Current session
source <(dash0ctl completion zsh)

# Permanent (Linux)
dash0ctl completion zsh > "${fpath[1]}/_dash0ctl"

# Permanent (macOS with Homebrew)
dash0ctl completion zsh > $(brew --prefix)/share/zsh/site-functions/_dash0ctl
```

**Fish**:
```bash
# Current session
dash0ctl completion fish | source

# Permanent
dash0ctl completion fish > ~/.config/fish/completions/dash0ctl.fish
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development instructions.
