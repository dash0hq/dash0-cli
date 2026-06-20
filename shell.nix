# Non-flake development shell: `nix-shell` drops you into a Go toolchain with
# the project's lint and changelog tooling. Flake users should prefer
# `nix develop` (see flake.nix).
{ pkgs ? import <nixpkgs> { } }:

pkgs.mkShell {
  packages = [
    pkgs.go_1_25
    pkgs.gopls
    pkgs.gotools
    pkgs.golangci-lint
    pkgs.shellcheck
  ];
}
