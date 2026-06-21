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
- `overlays.default` — exposes the source package as `pkgs.dash0`.
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

### Binary distribution (NUR)

The pre-built binary is **not** packaged in this repository.
A binary derivation that pins release artifact hashes cannot live in the source tree: the hashes depend on artifacts built *from* a tag, so the tagged commit could never contain the correct hashes for its own release (the same reason the Homebrew cask lives in its own repo).

Instead, the GoReleaser `nix` publisher (see [`.goreleaser.yaml`](.goreleaser.yaml)) writes `pkgs/dash0/default.nix` to a separate Nix User Repository, [`dash0hq/nur`](https://github.com/dash0hq/nur), after each release — the direct counterpart to the `homebrew_casks` publisher.
Because the manifest is generated post-build in its own repo, every released version is exactly reproducible, with no per-tag lag and no in-source hash upkeep.
The release binary is built `CGO_ENABLED=0` (see [`.goreleaser.yaml`](.goreleaser.yaml)), so it is statically linked and runs on NixOS without `autoPatchelfHook`.

The [`dash0hq/nur`](https://github.com/dash0hq/nur) repo is already set up: it holds a hand-maintained `flake.nix` that exposes packages from `pkgs/*`, and a committed placeholder `pkgs/dash0/default.nix` that GoReleaser overwrites on each release. Its `flake.nix` looks like:

```nix
{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  inputs.flake-utils.url = "github:numtide/flake-utils";
  outputs = { nixpkgs, flake-utils, ... }:
    flake-utils.lib.eachDefaultSystem (system: {
      packages.dash0 = nixpkgs.legacyPackages.${system}.callPackage ./pkgs/dash0/default.nix { };
    });
}
```

The GoReleaser `GITHUB_TOKEN` (`REPOSITORY_FULL_ACCESS_GITHUB_TOKEN`) must have write access to that repo.
Until the first release publishes through the `nix` block, `github:dash0hq/nur#dash0` builds the committed placeholder, which prints a "no release published yet" message and exits non-zero; the first release replaces it with the real binary.

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
The release automation keeps it in step (see [Releasing](#releasing)) — you do not bump it by hand.

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
3. Choose the version bump type (`major`, `minor`, or `patch`); the next version is computed from the highest existing tag.
4. Click "Run workflow".

The workflow automatically:
1. Updates `CHANGELOG.md` with entries from `.chloggen/`.
2. Removes the processed `.chloggen/*.yaml` files.
3. Bumps the `version` in [`flake.nix`](flake.nix) (the source Nix package).
4. Commits the changes to `main`.
5. Creates and pushes the version tag.
6. Triggers the Release workflow, which builds and publishes the release (GitHub Release, Docker images, Homebrew cask, and the NUR binary package).

The `flake.nix` bump lands in the tagged commit, so `nix build github:dash0hq/dash0-cli/v<version>` reports the right version.
The binary package is published by GoReleaser to [`dash0hq/nur`](https://github.com/dash0hq/nur) (see [Binary distribution](#binary-distribution-nur)).

### Version numbering

Follow [Semantic Versioning](https://semver.org/):
- **MAJOR**: Breaking changes
- **MINOR**: New features (backward compatible)
- **PATCH**: Bug fixes (backward compatible)
