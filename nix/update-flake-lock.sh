#!/usr/bin/env bash
#
# Update flake.lock to the latest nixpkgs/flake-utils revisions, then validate
# that dash0 still builds against the new pin before leaving it in place.
#
# If the update produces a revision that fails to build, the attempt is
# discarded and flake.lock is restored to its original content -- a broken
# upstream nixpkgs revision must never be the thing that leaves flake.lock in
# a bad state. Idempotent: when there is nothing newer to pin, flake.lock is
# left unchanged.
#
# Run from the repository root (or via `make update-flake-lock`). Requires Nix
# with flakes enabled. Shared by .github/workflows/nix-flake-update.yml (weekly
# schedule) and .github/workflows/prepare-release.yml (validated update right
# before cutting a release), so neither path can ship a broken nixpkgs pin.
set -euo pipefail

if [ ! -f "flake.lock" ] || [ ! -f "flake.nix" ]; then
  echo "error: run this from the repository root (cannot find flake.lock / flake.nix)" >&2
  exit 1
fi

if ! command -v nix >/dev/null 2>&1; then
  echo "error: nix is not installed or not on PATH" >&2
  exit 1
fi

backup="$(mktemp)"
trap 'rm -f "$backup"' EXIT
cp flake.lock "$backup"

nix flake update

if [ "$(sha256sum <flake.lock)" = "$(sha256sum <"$backup")" ]; then
  echo "flake.lock already up to date."
  exit 0
fi

if nix build .#dash0 --print-build-logs && nix flake check --print-build-logs; then
  echo "flake.lock updated and validated."
else
  echo "::warning::New nixpkgs/flake-utils revision failed to build dash0 or failed flake checks; keeping the previous flake.lock pin." >&2
  cp "$backup" flake.lock
fi
