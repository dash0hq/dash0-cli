# Non-flake entrypoint, so `nix-build` works on systems without flakes enabled.
# Flake users should prefer `nix build .#dash0` (see flake.nix).
{ pkgs ? import <nixpkgs> { } }:

pkgs.callPackage ./nix/package.nix { }
