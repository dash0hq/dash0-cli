#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "=== Synthetic check round-trip test ==="

# Step 1: Create from fixture
echo "--- Step 1: Create synthetic check from fixture ---"
"$DASH0" synthetic-checks create -f "${FIXTURES}/synthetic-check.yaml"
# Extract ID from list
echo "--- Step 2: List synthetic checks and find created asset ---"
"$DASH0" synthetic-checks listID=$("$DASH0" synthetic-checks list -o json | jq -r '.[0].id // empty')
if [ -z "$ID" ]; then
  echo "FAIL: Could not find created synthetic check in list"
  exit 1
fi
echo "Created synthetic check ID: $ID"

# Step 3: Get by ID
echo "--- Step 3: Get synthetic check by ID ---"
"$DASH0" synthetic-checks get "$ID"
# Step 4: Export to YAML
echo "--- Step 4: Export synthetic check to YAML ---"
"$DASH0" synthetic-checks get "$ID" -o yaml > "${TMPDIR}/exported.yaml"
echo "Exported to ${TMPDIR}/exported.yaml"

# Step 5: Re-import via apply (round-trip)
echo "--- Step 5: Re-import exported YAML via apply ---"
"$DASH0" apply -f "${TMPDIR}/exported.yaml"
# Step 6: Delete
echo "--- Step 6: Delete synthetic check ---"
"$DASH0" synthetic-checks delete "$ID" --force

# Step 7: Verify deletion
echo "--- Step 7: Verify deletion ---"
REMAINING=$("$DASH0" synthetic-checks list -o json | jq 'length')
if [ "$REMAINING" -ne 0 ]; then
  echo "FAIL: Synthetic check still exists after deletion"
  exit 1
fi

echo "=== Synthetic check round-trip test PASSED ==="
