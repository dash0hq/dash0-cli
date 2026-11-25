#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# The inferred origin hashes metadata.name, so a unique name per run keeps
# runs from updating each other's view.
NAME="cli-roundtrip-$(uuidgen | tr '[:upper:]' '[:lower:]')"
YAML_FILE="${TMPDIR}/view.yaml"

echo "=== View apply without identifier test ==="
echo "Name: $NAME"

NAME="$NAME" yq '.metadata.name = env(NAME) | .spec.display.name = env(NAME)' "$FIXTURES/view.yaml" > "$YAML_FILE"

echo "--- Step 1: Apply twice ---"
"$DASH0" apply -f "$YAML_FILE"
"$DASH0" apply -f "$YAML_FILE"

echo "--- Step 2: Exactly one view with that name, at an inferred origin ---"
LIST=$("$DASH0" views list --all -o csv --skip-header 2>/dev/null)
MATCHES=$(grep -F "$NAME," <<< "$LIST" || true)
COUNT=$(grep -c . <<< "$MATCHES" || true)
if [ "$COUNT" -ne 1 ]; then
  echo "FAIL: expected 1 view named '$NAME', found $COUNT"
  echo "$MATCHES"
  exit 1
fi
ORIGIN=$(cut -d, -f4 <<< "$MATCHES")
if [[ "$ORIGIN" != dash0-cli_* ]]; then
  echo "FAIL: expected an inferred dash0-cli_ origin, got '$ORIGIN'"
  exit 1
fi
if ! "$DASH0" views get "$ORIGIN" > /dev/null; then
  echo "FAIL: views get '$ORIGIN' failed"
  exit 1
fi

echo "--- Step 3: diff reports no differences ---"
if ! "$DASH0" -X diff -f "$YAML_FILE"; then
  echo "FAIL: expected diff to exit 0"
  exit 1
fi

"$DASH0" views delete "$ORIGIN" --force

echo "=== View apply without identifier test PASSED ==="
