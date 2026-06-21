#!/usr/bin/env bash
#
# Recompute the buildGoModule vendorHash in nix/package.nix for the current
# go.mod / go.sum and rewrite it in place. Idempotent: when the hash is already
# correct the file is left unchanged.
#
# Run from the repository root (or via `make update-vendor-hash`). Requires Nix
# with flakes enabled. Used by .github/workflows/nix-vendor-hash.yml to keep
# dependency-bump PRs (Dependabot or human) green automatically.
set -euo pipefail

pkg="nix/package.nix"
# A deliberately wrong placeholder so the fixed-output module build reports the
# real hash in its "got:" line, which we parse back out.
fake="sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

if [ ! -f "$pkg" ] || [ ! -f "flake.nix" ]; then
  echo "error: run this from the repository root (cannot find $pkg / flake.nix)" >&2
  exit 1
fi

if ! command -v nix >/dev/null 2>&1; then
  echo "error: nix is not installed or not on PATH" >&2
  exit 1
fi

current="$(sed -n 's/.*vendorHash = "\(sha256-[^"]*\)".*/\1/p' "$pkg" | head -n1)"
if [ -z "$current" ]; then
  echo "error: could not find a vendorHash literal in $pkg" >&2
  exit 1
fi

# Force the wrong hash, build the module set, capture the reported "got:" hash.
sed -i "s|vendorHash = \"${current}\"|vendorHash = \"${fake}\"|" "$pkg"

got="$(nix build '.#dash0.goModules' --no-link 2>&1 | awk '/got:/ { print $NF; exit }' || true)"

if [ -z "$got" ]; then
  # Restore the original rather than leaving the placeholder in place.
  sed -i "s|vendorHash = \"${fake}\"|vendorHash = \"${current}\"|" "$pkg"
  echo "error: could not determine vendorHash from the nix build output" >&2
  exit 1
fi

sed -i "s|vendorHash = \"${fake}\"|vendorHash = \"${got}\"|" "$pkg"

if [ "$got" = "$current" ]; then
  echo "vendorHash already up to date: ${got}"
else
  echo "vendorHash updated: ${current} -> ${got}"
fi
