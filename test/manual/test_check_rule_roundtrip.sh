#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "=== Check rule round-trip test ==="

# Step 1: Create from fixture
echo "--- Step 1: Create check rule from fixture ---"
"$DASH0" check-rules create -f "${FIXTURES}/check-rule.yaml"
# Extract ID from list
echo "--- Step 2: List check rules and find created asset ---"
"$DASH0" check-rules listID=$("$DASH0" check-rules list -o json | jq -r '.[0].id // empty')
if [ -z "$ID" ]; then
  echo "FAIL: Could not find created check rule in list"
  exit 1
fi
echo "Created check rule ID: $ID"

# Step 3: Get by ID
echo "--- Step 3: Get check rule by ID ---"
"$DASH0" check-rules get "$ID"
# Step 4: Export to YAML
echo "--- Step 4: Export check rule to YAML ---"
"$DASH0" check-rules get "$ID" -o yaml > "${TMPDIR}/exported.yaml"
echo "Exported to ${TMPDIR}/exported.yaml"

# Step 5: Re-import via apply (round-trip)
echo "--- Step 5: Re-import exported YAML via apply ---"
"$DASH0" apply -f "${TMPDIR}/exported.yaml"
# Step 6: Delete
echo "--- Step 6: Delete check rule ---"
"$DASH0" check-rules delete "$ID" --force

# Step 7: Verify deletion
echo "--- Step 7: Verify deletion ---"
REMAINING=$("$DASH0" check-rules list -o json | jq 'length')
if [ "$REMAINING" -ne 0 ]; then
  echo "FAIL: Check rule still exists after deletion"
  exit 1
fi

echo "=== Check rule round-trip test PASSED ==="
