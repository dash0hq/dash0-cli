#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
FIXTURE="${FIXTURES}/perses-dashboard.yaml"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ASSET_NAME=$(yq '.spec.display.name' "$FIXTURE")

echo "=== PersesDashboard CRD round-trip test ==="
echo "Asset name: $ASSET_NAME"

# Step 1: Create via apply (PersesDashboard CRD).
echo "--- Step 1: Apply PersesDashboard CRD ---"
APPLY_OUTPUT=$("$DASH0" apply -f "$FIXTURE")
echo "$APPLY_OUTPUT"
if ! echo "$APPLY_OUTPUT" | grep -q "$ASSET_NAME"; then
  echo "FAIL: apply output does not mention asset name '$ASSET_NAME'"
  exit 1
fi

# Step 2: List dashboards and find the created asset by name.
echo "--- Step 2: List dashboards and find created asset ---"
LIST_JSON=$("$DASH0" dashboards list -o json)
ID=$(echo "$LIST_JSON" | jq -r --arg name "$ASSET_NAME" '[.[] | select(.spec.display.name == $name)][0].metadata.dash0Extensions.id // empty')
if [ -z "$ID" ]; then
  echo "FAIL: Could not find created dashboard '$ASSET_NAME' in list"
  exit 1
fi
echo "Created dashboard ID: $ID"

# Step 3: Get by ID and verify the conversion produced a valid dashboard.
echo "--- Step 3: Verify converted dashboard ---"
GET_YAML=$("$DASH0" dashboards get "$ID" -o yaml)
echo "$GET_YAML"

ACTUAL_NAME=$(echo "$GET_YAML" | yq '.spec.display.name')
if [ "$ACTUAL_NAME" != "$ASSET_NAME" ]; then
  echo "FAIL: expected display name '$ASSET_NAME', got '$ACTUAL_NAME'"
  exit 1
fi

ACTUAL_KIND=$(echo "$GET_YAML" | yq '.kind')
if [ "$ACTUAL_KIND" != "Dashboard" ]; then
  echo "FAIL: expected kind 'Dashboard', got '$ACTUAL_KIND'"
  exit 1
fi
echo "kind: $ACTUAL_KIND"

# Step 4: Export to YAML and re-import via apply (round-trip).
echo "--- Step 4: Export and re-apply (round-trip) ---"
"$DASH0" dashboards get "$ID" -o yaml > "${TMPDIR}/exported.yaml"
REAPPLY_OUTPUT=$("$DASH0" apply -f "${TMPDIR}/exported.yaml")
echo "$REAPPLY_OUTPUT"

# Step 5: Also test dashboards create with the CRD file (parity check).
echo "--- Step 5: Create via dashboards create (parity check) ---"
CREATE_OUTPUT=$("$DASH0" dashboards create -f "$FIXTURE")
echo "$CREATE_OUTPUT"
if ! echo "$CREATE_OUTPUT" | grep -q "$ASSET_NAME"; then
  echo "FAIL: dashboards create output does not mention asset name '$ASSET_NAME'"
  exit 1
fi

# Find the second copy and clean it up.
LIST_JSON2=$("$DASH0" dashboards list -o json)
ID2=$(echo "$LIST_JSON2" | jq -r --arg name "$ASSET_NAME" --arg id "$ID" '[.[] | select(.spec.display.name == $name and .metadata.dash0Extensions.id != $id)][0].metadata.dash0Extensions.id // empty')

# Cleanup.
echo "--- Cleanup ---"
"$DASH0" dashboards delete "$ID" --force
if [ -n "$ID2" ]; then
  "$DASH0" dashboards delete "$ID2" --force
fi
echo "=== PersesDashboard CRD round-trip test PASSED ==="
