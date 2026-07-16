# Installation

The Dash0 CLI is available for macOS, Linux, and Windows through multiple channels.

## Homebrew (macOS and Linux)

```bash
brew install --cask dash0hq/dash0-cli/dash0
```

No `brew tap` or extra ceremony required — the qualified install path taps the [`dash0hq/homebrew-dash0-cli`](https://github.com/dash0hq/homebrew-dash0-cli) repository on first use.

> [!NOTE]
> If you previously installed `dash0` from the legacy formula (`brew install dash0` after `brew tap dash0hq/dash0-cli <URL>`), see the [Homebrew Tap Migration (June 2026)](brew-tap-migration-2026-06.md) for the one-time switch to the new cask.

## GitHub Releases

Download pre-built binaries for your platform from the [releases page](https://github.com/dash0hq/dash0-cli/releases).
Archives are available for Linux, macOS, and Windows across multiple architectures.

## Docker

```bash
docker run ghcr.io/dash0hq/cli:latest [command]
```

Multi-architecture images (`linux/amd64`, `linux/arm64`) are published to the GitHub Container Registry.

## Nix / NixOS

The repository is published as a Nix flake.

Run without installing:

```bash
nix run github:dash0hq/dash0-cli -- dashboards list
```

Install into your Nix profile:

```bash
nix profile install github:dash0hq/dash0-cli
```

A pre-built binary is also published to the Dash0 Nix User Repository (NUR), which skips compilation and is useful on small or non-`x86_64` machines:

```bash
nix profile install github:dash0hq/nur#dash0
```

For Home Manager integration with declarative Dash0 profiles, and for consuming the flake's `overlays.default` from a NixOS or Home Manager configuration, see the [full Nix documentation on GitHub](https://github.com/dash0hq/dash0-cli/blob/main/README.md#nix--nixos).

## From source

Requires Go 1.22 or higher.

```bash
git clone https://github.com/dash0hq/dash0-cli.git
cd dash0-cli
make install
```

## GitHub Actions

To use the CLI from GitHub Actions workflows, see the [GitHub Actions guide](github-actions.md).
