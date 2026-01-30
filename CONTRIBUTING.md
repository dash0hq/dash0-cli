# Contributing to Dash0 CLI

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

## Changelog

This project uses the [chloggen](https://github.com/open-telemetry/opentelemetry-go-build-tools/tree/main/chloggen) tool to manage changelog entries. Every pull request that affects end users must include a changelog entry.

### Adding a Changelog Entry

1. Create a new changelog entry file:
   ```bash
   make chlog-new
   ```
   This creates a new YAML file in `.chloggen/` named after your current branch.

2. Edit the generated file and fill in the required fields:
   - `change_type`: One of `breaking`, `deprecation`, `new_component`, `enhancement`, `bug_fix`
   - `component`: The affected component (e.g., `dashboards`, `config`, `apply`)
   - `note`: A brief description of the change
   - `issues`: Related issue or PR numbers

3. Validate your entry:
   ```bash
   make chlog-validate
   ```

4. Preview the changelog:
   ```bash
   make chlog-preview
   ```

### Skipping Changelog

If your change doesn't affect end users (e.g., CI changes, internal refactoring), you can skip the changelog requirement by either:
- Starting your PR title with `chore`
- Adding the `Skip Changelog` label to the PR

### Example Changelog Entry

```yaml
change_type: enhancement
component: dashboards
note: Add support for JSON output format in list command
issues: [42]
subtext: |
  The new `--output json` flag enables scripting and automation workflows.
```
