#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "=== Dashboard round-trip test ==="

# Step 1: Create from fixture
echo "--- Step 1: Create dashboard from fixture ---"
"$DASH0" dashboards create -f "${FIXTURES}/dashboard.yaml"
# Extract ID from list
echo "--- Step 2: List dashboards and find created asset ---"
"$DASH0" dashboards listID=$("$DASH0" dashboards list -o json | jq -r '.[0].id // empty')
if [ -z "$ID" ]; then
  echo "FAIL: Could not find created dashboard in list"
  exit 1
fi
echo "Created dashboard ID: $ID"

# Step 3: Get by ID
echo "--- Step 3: Get dashboard by ID ---"
"$DASH0" dashboards get "$ID"
# Step 4: Export to YAML
echo "--- Step 4: Export dashboard to YAML ---"
"$DASH0" dashboards get "$ID" -o yaml > "${TMPDIR}/exported.yaml"
echo "Exported to ${TMPDIR}/exported.yaml"

# Step 5: Re-import via apply (round-trip)
echo "--- Step 5: Re-import exported YAML via apply ---"
"$DASH0" apply -f "${TMPDIR}/exported.yaml"
# Step 6: Delete
echo "--- Step 6: Delete dashboard ---"
"$DASH0" dashboards delete "$ID" --force

# Step 7: Verify deletion
echo "--- Step 7: Verify deletion ---"
REMAINING=$("$DASH0" dashboards list -o json | jq 'length')
if [ "$REMAINING" -ne 0 ]; then
  echo "FAIL: Dashboard still exists after deletion"
  exit 1
fi

echo "=== Dashboard round-trip test PASSED ==="
