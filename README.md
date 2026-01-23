# Dash0 CLI

A command-line interface for interacting with Dash0 services.

## Installation

### From Source

Requires Go 1.22 or higher.

```bash
# Clone the repository
git clone https://github.com/dash0hq/dash0-cli.git
cd dash0-cli/go-cli

# Build and install
make install
```

### Using Docker

```bash
docker pull dash0/cli
docker run --rm dash0/cli --help
```

## Usage

### Configuration

Configure API access using profiles.

```bash
$ dash0ctl config profile add --name dev --api-url https://api.eu-west-1.aws.dash0.com --auth-token auth_xxx
Profile "dev" added and set as active

$ dash0ctl config profile list
  NAME  API URL
* dev   https://api.eu-west-1.aws.dash0.com

$ dash0ctl config show
  Profile:    dev
  API URL:    https://api.eu-west-1.aws.dash0-dev.com
  Auth Token: ...ULSzVkM # Last seven digits of the auth token, same as displayed in Dash0 as the `dash0.auth.token` attribute

$ dash0ctl config show
  Profile:    dev
  API URL:    https://api.eu-west-1.aws.dash0-dev.com
  Auth Token: ...ULSzVkM # Last seven digits of the auth token, same as displayed in Dash0 as the `dash0.auth.token` attribute
```

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

### Dashboards

```bash
$ dash0ctl dashboards list
ID                                    NAME
a1b2c3d4-5678-90ab-cdef-1234567890ab  Production Overview
...

$ dash0ctl dashboards get a1b2c3d4-5678-90ab-cdef-1234567890ab -o yaml
kind: Dashboard
metadata:
  name: Production Overview
...

$ dash0ctl dashboards create -f dashboard.yaml
Dashboard "My Dashboard" created successfully

$ dash0ctl dashboards update a1b2c3d4-5678-90ab-cdef-1234567890ab -f dashboard.yaml
Dashboard "My Dashboard" updated successfully

$ dash0ctl dashboards delete a1b2c3d4-5678-90ab-cdef-1234567890ab
Are you sure you want to delete dashboard "a1b2c3d4-..."? [y/N]: y
Dashboard "a1b2c3d4-..." deleted successfully

$ dash0ctl dashboards apply -f dashboard.yaml
Dashboard "My Dashboard" applied successfully

$ dash0ctl dashboards export a1b2c3d4-5678-90ab-cdef-1234567890ab -f dashboard.yaml
Dashboard exported to dashboard.yaml
```

### Check Rules

```bash
$ dash0ctl check-rules list
ID                                    NAME
high-error-rate                       High Error Rate Alert
...

$ dash0ctl check-rules get high-error-rate -o json
{"name":"High Error Rate Alert","expression":"sum(rate(errors[5m])) > 0.1",...}

$ dash0ctl check-rules create -f rule.yaml
Check rule "High Error Rate Alert" created successfully

$ dash0ctl check-rules update high-error-rate -f rule.yaml
Check rule "High Error Rate Alert" updated successfully

$ dash0ctl check-rules delete high-error-rate --force
Check rule "high-error-rate" deleted successfully

$ dash0ctl check-rules apply -f rule.yaml
Check rule "High Error Rate Alert" applied successfully

$ dash0ctl check-rules export high-error-rate -f rule.yaml
Check rule exported to rule.yaml
```

### Synthetic Checks

```bash
$ dash0ctl synthetic-checks list
ID                                    NAME
api-health-check                      API Health Check
...

$ dash0ctl synthetic-checks get api-health-check -o yaml
kind: SyntheticCheck
metadata:
  name: API Health Check
...

$ dash0ctl synthetic-checks create -f check.yaml
Synthetic check "API Health Check" created successfully

$ dash0ctl synthetic-checks update api-health-check -f check.yaml
Synthetic check "API Health Check" updated successfully

$ dash0ctl synthetic-checks delete api-health-check
Are you sure you want to delete synthetic check "api-health-check"? [y/N]: y
Synthetic check "api-health-check" deleted successfully

$ dash0ctl synthetic-checks apply -f check.yaml
Synthetic check "API Health Check" applied successfully

$ dash0ctl synthetic-checks export api-health-check -f check.yaml
Synthetic check exported to check.yaml
```

### Views

```bash
$ dash0ctl views list
ID                                    NAME
errors-view                           Error Logs View
...

$ dash0ctl views get errors-view -o yaml
kind: View
metadata:
  name: Error Logs View
...

$ dash0ctl views create -f view.yaml
View "Error Logs View" created successfully

$ dash0ctl views update errors-view -f view.yaml
View "Error Logs View" updated successfully

$ dash0ctl views delete errors-view --force
View "errors-view" deleted successfully

$ dash0ctl views apply -f view.yaml
View "Error Logs View" applied successfully

$ dash0ctl views export errors-view -f view.yaml
View exported to view.yaml
```

### Common Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--api-url` | | Override API URL from profile |
| `--auth-token` | | Override auth token from profile |
| `--dataset` | `-d` | Specify dataset to operate on |
| `--output` | `-o` | Output format: `table`, `json`, `yaml` |

## Development

### Build

```bash
make build
```

### Test

```bash
make test
```

### Build for multiple platforms

```bash
make build-all
```
