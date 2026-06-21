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

## Nix packaging

The repository is a Nix flake ([`flake.nix`](flake.nix)) that packages the CLI and a Home Manager module.
End-user instructions live in the [README](README.md#nix--nixos); this section is for maintaining the packaging itself.

### Flake outputs

- `packages.<system>.dash0` (also `.default`) — the CLI built from source with `buildGoModule` ([`nix/package.nix`](nix/package.nix)). This is canonical: it builds the current tree, so it validates the repo under Nix, supports unreleased revisions, and is the form a future nixpkgs submission would take.
- `packages.<system>.dash0-bin` — the pre-built GoReleaser release binary repackaged from GitHub Releases ([`nix/package-bin.nix`](nix/package-bin.nix)). Compile-free, useful on small or non-`x86_64` machines.
- `overlays.default` — exposes both as `pkgs.dash0` and `pkgs.dash0-bin`.
- `homeManagerModules.default` — declarative `programs.dash0` profiles (see below).
- `checks.<system>.{hm-assertions,hm-merge}` — unit tests for the module logic, run by `nix flake check`.
- `devShells.<system>.default` — Go toolchain plus the lint and changelog tooling.

The non-flake [`default.nix`](default.nix) and [`shell.nix`](shell.nix) are thin shims for systems without flakes enabled.

### Source package and `vendorHash`

`buildGoModule` fetches Go dependencies in a fixed-output derivation whose content is pinned by `vendorHash` in [`nix/package.nix`](nix/package.nix).
That hash is derived from `go.mod`/`go.sum`, so it only changes when dependencies change — editing Go source does not affect it.
When `go.mod`/`go.sum` change, refresh it:

```bash
make update-vendor-hash
```

On pull requests this is automated: the [`Nix vendorHash`](.github/workflows/nix-vendor-hash.yml) workflow recomputes the hash and commits the fix back to the branch, so Dependabot and other dependency PRs heal themselves.
Note that CI builds the PR's *merge commit*, so a dependency bump landing on `main` can make an unrelated open PR's Nix build go red until its branch is updated.
The `dash0-bin` package has no `vendorHash` and is unaffected.

### Binary package

[`nix/package-bin.nix`](nix/package-bin.nix) pins a release `version` and the SRI hash of each platform's release tarball.
To bump it, update `version` and refresh the four hashes:

```bash
nix store prefetch-file --json https://github.com/dash0hq/dash0-cli/releases/download/v<version>/dash0_<version>_linux_amd64.tar.gz
```

The release binary is built `CGO_ENABLED=0` (see [`.goreleaser.yaml`](.goreleaser.yaml)), so it is statically linked and runs on NixOS without `autoPatchelfHook`.
The asset naming (`macos` for darwin builds) mirrors the GoReleaser archive template.

### Home Manager module

[`nix/hm-module.nix`](nix/hm-module.nix) renders `~/.dash0/profiles.json` from declared profiles.
Because the CLI rewrites that file at runtime (OAuth refresh, `login`, `logout`), the module cannot own it outright: an activation script *merges* declared profiles into the live file rather than replacing it.
The merge logic is factored out so it can be unit-tested:

- [`nix/merge-profiles.jq`](nix/merge-profiles.jq) — the merge program. It upserts declared profiles, preserves runtime-acquired OAuth state for profiles the user has logged into, seeds `oauth: {}` for new OAuth profiles, and injects static tokens read from `authTokenFile` at activation time. Tested by `checks.<system>.hm-merge`.
- [`nix/assertions.nix`](nix/assertions.nix) — a pure `lib -> cfg -> [assertion]` function (profile validation, including the `activeProfile` guard under `pruneUndeclared`). Tested by `checks.<system>.hm-assertions`.

Run the checks with `nix flake check`.
After changing the module's logic, add or update a case in [`nix/tests/`](nix/tests/).
The `nix flake check` warning `unknown flake output 'homeManagerModules'` is benign — that output name is a Home Manager convention the Nix CLI does not validate.

### Versioning

The source package version is pinned in [`flake.nix`](flake.nix) (the `version` binding) and feeds the `-X main.version` ldflag.
Keep it in step with releases, the same way `.goreleaser.yaml` and the Homebrew cask are.

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
