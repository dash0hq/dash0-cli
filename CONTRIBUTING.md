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

## Releasing

Releases are automated via GitHub Actions when a version tag is pushed.

### Release Steps

1. **Ensure changelog entries exist**

   Verify that all merged PRs have corresponding changelog entries in `.chloggen/`:
   ```bash
   make chlog-validate
   make chlog-preview
   ```

2. **Create and push a version tag**

   ```bash
   VERSION=0.x.x
   make "VERSION=${VERSION}" chlog-update
   git add CHANGELOG.md .chloggen
   git add -m "release: v${VERSION}"
   git tag "v${VERSION}"
   git push origin "v${VERSION}"
   ```

3. **Automated release process**

   When a `v*` tag is pushed, GitHub Actions automatically:
   - Updates `CHANGELOG.md` from `.chloggen/` entries
   - Commits the changelog update to `main`
   - Builds binaries for multiple platforms via GoReleaser
   - Creates a GitHub release with release notes
   - Pushes Docker images to `ghcr.io`

### Version Numbering

Follow [Semantic Versioning](https://semver.org/):
- **MAJOR**: Breaking changes
- **MINOR**: New features (backward compatible)
- **PATCH**: Bug fixes (backward compatible)
