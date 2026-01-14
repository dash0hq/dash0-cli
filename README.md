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

The CLI supports multiple named configuration profiles, similar to the AWS CLI.

```bash
# Show current configuration
dash0 config show

# Add a new configuration profile
dash0 config profile add --name dev --api-url https://api.eu-west-1.aws.dash0-dev.com --auth-token your-token

# List available profiles
dash0 config profile list

# Select a profile
dash0 config profile select --name dev

# Remove a profile
dash0 config profile remove --name dev
```

Configuration precedence:
1. Environment variables: `DASH0_API_URL` and `DASH0_AUTH_TOKEN`
2. Active configuration profile

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
