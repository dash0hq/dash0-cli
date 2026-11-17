#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"

# Earlier runs may have left dashboards with the fixture's name behind, so a
# unique name keeps every lookup by name below on this run's own dashboards.
FIXTURE="${TMPDIR}/perses-dashboard.yaml"
ASSET_NAME="$(yq '.spec.display.name' "${FIXTURES}/perses-dashboard.yaml") $(uuidgen | tr '[:upper:]' '[:lower:]' | cut -c1-8)"
ASSET_NAME="$ASSET_NAME" yq '.spec.display.name = env(ASSET_NAME)' "${FIXTURES}/perses-dashboard.yaml" > "$FIXTURE"

# dashboard_ids prints the IDs of the dashboards this run created.
dashboard_ids() {
  "$DASH0" dashboards list --all -o json \
    | jq -r --arg name "$ASSET_NAME" '.[] | select(.spec.display.name == $name) | .metadata.dash0Extensions.id // empty'
}

# cleanup deletes the dashboards this run created. It runs on exit, so a run
# that fails or is interrupted after a create still cleans up.
cleanup() {
  local ids id
  if ! ids=$(dashboard_ids); then
    echo "WARN: could not list dashboards to delete those named '$ASSET_NAME'" >&2
    return
  fi
  for id in $ids; do
    "$DASH0" dashboards delete "$id" --force || echo "WARN: could not delete dashboard $id" >&2
  done
}
trap 'cleanup; rm -rf "$TMPDIR"' EXIT

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
LIST_JSON=$("$DASH0" dashboards list --all -o json)
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

# Step 5: Update via dashboards update with the CRD file (ID from argument).
echo "--- Step 5: Update via dashboards update with CRD file (ID from arg) ---"
UPDATE_OUTPUT=$("$DASH0" dashboards update "$ID" -f "$FIXTURE")
echo "$UPDATE_OUTPUT"

# Verify the CRD conversion was applied correctly (not silently corrupted).
echo "--- Step 5a: Verify dashboard content after update ---"
POST_UPDATE_NAME=$("$DASH0" dashboards get "$ID" -o yaml | yq '.spec.display.name')
if [ "$POST_UPDATE_NAME" != "$ASSET_NAME" ]; then
  echo "FAIL: after update, expected display name '$ASSET_NAME', got '$POST_UPDATE_NAME'"
  echo "      (this indicates the PersesDashboard CRD was not converted before sending to the API)"
  exit 1
fi
echo "Display name after update: $POST_UPDATE_NAME"

# Step 5b: Update using a CRD file with dash0.com/id (ID from file, no CLI argument).
echo "--- Step 5b: Update via dashboards update with CRD file (ID from file) ---"
yq ".metadata.labels.\"dash0.com/id\" = \"$ID\"" "$FIXTURE" > "${TMPDIR}/perses-with-id.yaml"
UPDATE_OUTPUT2=$("$DASH0" dashboards update -f "${TMPDIR}/perses-with-id.yaml")
echo "$UPDATE_OUTPUT2"

POST_UPDATE_NAME2=$("$DASH0" dashboards get "$ID" -o yaml | yq '.spec.display.name')
if [ "$POST_UPDATE_NAME2" != "$ASSET_NAME" ]; then
  echo "FAIL: after update (ID from file), expected display name '$ASSET_NAME', got '$POST_UPDATE_NAME2'"
  exit 1
fi
echo "Display name after update (ID from file): $POST_UPDATE_NAME2"

# Step 6: Also test dashboards create with the CRD file (parity check).
echo "--- Step 6: Create via dashboards create (parity check) ---"
CREATE_OUTPUT=$("$DASH0" dashboards create -f "$FIXTURE")
echo "$CREATE_OUTPUT"
if ! echo "$CREATE_OUTPUT" | grep -q "$ASSET_NAME"; then
  echo "FAIL: dashboards create output does not mention asset name '$ASSET_NAME'"
  exit 1
fi

if [ "$(dashboard_ids | wc -l | tr -d ' ')" != "2" ]; then
  echo "FAIL: expected 2 dashboards named '$ASSET_NAME' after apply and create, got:"
  dashboard_ids
  exit 1
fi

# Cleanup: delete both copies and check that none is left.
echo "--- Cleanup ---"
cleanup
REMAINING=$(dashboard_ids)
if [ -n "$REMAINING" ]; then
  echo "FAIL: dashboards named '$ASSET_NAME' remain after cleanup: $REMAINING"
  exit 1
fi
trap 'rm -rf "$TMPDIR"' EXIT
echo "=== PersesDashboard CRD round-trip test PASSED ==="
