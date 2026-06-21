#!/usr/bin/env bash
#
# Point nix/package-bin.nix at a released version: set the pinned `version` and
# refresh the four platform tarball hashes from the published GitHub release.
#
# Run this AFTER the release artifacts are published (i.e. after the Release
# workflow finishes for the tag), since it downloads the release tarballs.
# Requires Nix.
#
# Usage: nix/update-bin-hashes.sh <version>     # e.g. 1.16.0
set -euo pipefail

version="${1:-}"
if [ -z "$version" ]; then
  echo "usage: $0 <version>   (e.g. 1.16.0)" >&2
  exit 1
fi

pkg="nix/package-bin.nix"
if [ ! -f "$pkg" ]; then
  echo "error: run this from the repository root (cannot find $pkg)" >&2
  exit 1
fi
if ! command -v nix >/dev/null 2>&1; then
  echo "error: nix is not installed or not on PATH" >&2
  exit 1
fi

base="https://github.com/dash0hq/dash0-cli/releases/download/v${version}"

# Nix system -> GoReleaser asset suffix (darwin builds are named "macos").
systems=(x86_64-linux aarch64-linux x86_64-darwin aarch64-darwin)
suffixes=(linux_amd64 linux_arm64 macos_amd64 macos_arm64)

for i in "${!systems[@]}"; do
  suffix="${suffixes[$i]}"
  url="${base}/dash0_${version}_${suffix}.tar.gz"
  echo "prefetching ${suffix}..." >&2
  hash="$(nix store prefetch-file --json "$url" | sed -n 's/.*"hash":"\([^"]*\)".*/\1/p')"
  if [ -z "$hash" ]; then
    echo "error: could not prefetch hash for $url" >&2
    exit 1
  fi
  # Replace the hash on the line immediately following this suffix's line.
  sed -i "/suffix = \"${suffix}\";/{n;s|hash = \"[^\"]*\";|hash = \"${hash}\";|}" "$pkg"
done

# Bump the pinned default version.
sed -i "s/version ? \"[0-9]*\.[0-9]*\.[0-9]*\"/version ? \"${version}\"/" "$pkg"

echo "updated $pkg to v${version}"
