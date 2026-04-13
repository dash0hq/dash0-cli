#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ID=$(uuidgen | tr '[:upper:]' '[:lower:]')
YAML_FILE="${TMPDIR}/view.yaml"
FOLDER_PATH="/views/test/annotations"

echo "=== View annotations and permissions round-trip test ==="
echo "Generated ID: $ID"

# Inject the generated ID, folder-path annotation, and permissions into the fixture.
ID="$ID" FOLDER_PATH="$FOLDER_PATH" \
  yq '
    .metadata.labels."dash0.com/id" = env(ID) |
    .metadata.annotations."dash0.com/folder-path" = env(FOLDER_PATH) |
    .spec.permissions = [{"actions": ["views:read", "views:write"], "role": "admin"}]
  ' "$FIXTURES/view.yaml" > "$YAML_FILE"

# Step 1: Create via apply.
echo "--- Step 1: Apply view with annotations and permissions ---"
APPLY_OUTPUT=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY_OUTPUT"
if ! echo "$APPLY_OUTPUT" | grep -q "$ID"; then
  echo "FAIL: apply output does not contain ID '$ID'"
  exit 1
fi

# Step 2: Get the view and verify annotations and permissions are present.
echo "--- Step 2: Verify annotations and permissions on created view ---"
GET_YAML=$("$DASH0" views get "$ID" -o yaml)
echo "$GET_YAML"

ACTUAL_FOLDER=$(echo "$GET_YAML" | yq '.metadata.annotations."dash0.com/folder-path"')
if [ "$ACTUAL_FOLDER" != "$FOLDER_PATH" ]; then
  echo "FAIL: expected folder-path '$FOLDER_PATH', got '$ACTUAL_FOLDER'"
  exit 1
fi
echo "folder-path: $ACTUAL_FOLDER"

PERM_COUNT=$(echo "$GET_YAML" | yq '.spec.permissions | length')
if [ "$PERM_COUNT" -lt 1 ]; then
  echo "FAIL: expected at least 1 permission entry, got $PERM_COUNT"
  exit 1
fi
echo "permissions count: $PERM_COUNT"

# Step 3: Export and re-apply (round-trip). Annotations and permissions must survive.
echo "--- Step 3: Export and re-apply (round-trip) ---"
"$DASH0" views get "$ID" -o yaml > "${TMPDIR}/exported.yaml"
REAPPLY_OUTPUT=$("$DASH0" apply -f "${TMPDIR}/exported.yaml")
echo "$REAPPLY_OUTPUT"

# Step 4: Verify annotations and permissions survived the round-trip.
echo "--- Step 4: Verify annotations and permissions after round-trip ---"
GET_YAML2=$("$DASH0" views get "$ID" -o yaml)

ACTUAL_FOLDER2=$(echo "$GET_YAML2" | yq '.metadata.annotations."dash0.com/folder-path"')
if [ "$ACTUAL_FOLDER2" != "$FOLDER_PATH" ]; then
  echo "FAIL: folder-path changed after round-trip: expected '$FOLDER_PATH', got '$ACTUAL_FOLDER2'"
  exit 1
fi
echo "folder-path after round-trip: $ACTUAL_FOLDER2"

PERM_COUNT2=$(echo "$GET_YAML2" | yq '.spec.permissions | length')
if [ "$PERM_COUNT2" -lt 1 ]; then
  echo "FAIL: permissions lost after round-trip: expected at least 1, got $PERM_COUNT2"
  exit 1
fi
echo "permissions count after round-trip: $PERM_COUNT2"

# Cleanup.
echo "--- Cleanup ---"
"$DASH0" views delete "$ID" --force
echo "=== View annotations and permissions round-trip test PASSED ==="
