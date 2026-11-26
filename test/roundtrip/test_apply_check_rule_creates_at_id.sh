#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ID="$(uuidgen | tr '[:upper:]' '[:lower:]')"
YAML_FILE="${TMPDIR}/check-rule.yaml"

echo "=== Check rule apply with an unknown id test ==="
echo "ID: $ID"

ID="$ID" yq '.id = env(ID)' "$FIXTURES/check-rule.yaml" > "$YAML_FILE"

echo "--- Step 1: First apply (expect: created at the id) ---"
"$DASH0" apply -f "$YAML_FILE"
if ! "$DASH0" check-rules get "$ID" > /dev/null; then
  echo "FAIL: check-rules get '$ID' failed, so the rule was not created at its id"
  exit 1
fi

echo "--- Step 2: Second apply (expect: no changes) ---"
APPLY2=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY2"
if ! grep -q "no changes" <<< "$APPLY2"; then
  echo "FAIL: expected 'no changes' on second apply"
  exit 1
fi

"$DASH0" check-rules delete "$ID" --force

echo "=== Check rule apply with an unknown id test PASSED ==="
