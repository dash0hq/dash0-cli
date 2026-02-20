# Contributing to Dash0 CLI

## Development

### Build and test

```bash
make build
make test
```

For more granular test commands (`make test-unit`, `make test-integration`, running a single test), see the "Commands" section in [CLAUDE.md](CLAUDE.md).

### Guidelines

The development guidelines live in [`docs/`](docs/):

- [Code style](docs/code-style.md) — Go conventions, dependencies, error handling
- [Project structure](docs/project-structure.md) — directory layout, package responsibilities
- [CLI naming conventions](docs/cli-naming-conventions.md) — command names, aliases, asset kind display names
- [Testing](docs/testing.md) — integration tests, fixtures, mock server
- [Documentation](docs/documentation.md) — prose rules, attribute keys in examples
- [GitHub Actions](docs/github-actions.md) — setup action, send-log-event, keeping actions in sync

## Changelog

Every pull request that affects end users must include a changelog entry.
See [docs/changelog-maintenance.md](docs/changelog-maintenance.md) for full instructions on creating, validating, and previewing entries.

Quick start:

```bash
make chlog-new        # create .chloggen/<branch-name>.yaml
# edit the file
make chlog-validate   # check it is well-formed
```

If the change does not affect end users (refactoring, CI, etc.), prefix the PR title with `chore` or add the "Skip Changelog" label.

## Releasing

Releases are fully automated via GitHub Actions.

### Creating a release

1. Go to [Actions > Prepare Release](../../actions/workflows/prepare-release.yml).
2. Click "Run workflow".
3. Enter the version number (without `v` prefix, e.g., `1.0.0`).
4. Click "Run workflow".

The workflow automatically:
1. Updates `CHANGELOG.md` with entries from `.chloggen/`.
2. Removes the processed `.chloggen/*.yaml` files.
3. Commits the changes to `main`.
4. Creates and pushes the version tag.
5. Triggers the Release workflow, which builds and publishes the release.

### Version numbering

Follow [Semantic Versioning](https://semver.org/):
- **MAJOR**: Breaking changes
- **MINOR**: New features (backward compatible)
- **PATCH**: Bug fixes (backward compatible)
