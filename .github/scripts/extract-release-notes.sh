#!/usr/bin/env bash
set -euo pipefail

# Extracts the changelog section for a given version and adjusts heading
# levels from ### to # for use in GitHub release descriptions.
#
# Usage: extract-release-notes.sh <version> [changelog-file]
#   version:        semver without "v" prefix (e.g., 1.3.0)
#   changelog-file: path to CHANGELOG.md (default: CHANGELOG.md)

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

VERSION="${1:?Usage: extract-release-notes.sh <version> [changelog-file]}"
CHANGELOG="${2:-$REPO_ROOT/CHANGELOG.md}"

awk -v ver="$VERSION" '
  /^## / { if (found) exit; if ($2 == ver) found=1; next }
  found { sub(/[[:space:]]+$/, ""); if ($0 != "") print }
' "$CHANGELOG" \
  | sed 's/^###/#/'
