#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# Dashboard IDs must be non-UUID strings (UUIDs are reserved for server-assigned primary IDs).
ID="test-annotations-$(uuidgen | tr '[:upper:]' '[:lower:]' | tr -d '-')"
YAML_FILE="${TMPDIR}/dashboard.yaml"
FOLDER_PATH="/test/annotations/nested"

echo "=== Dashboard annotations round-trip test ==="
echo "Generated ID: $ID"

# Write a minimal dashboard with user-settable annotations.
cat > "$YAML_FILE" << YAML
kind: Dashboard
metadata:
  annotations:
    dash0.com/folder-path: $FOLDER_PATH
    dash0.com/source: ui
  dash0extensions:
    id: $ID
  name: Annotations Test Dashboard
spec:
  display:
    name: Annotations Test Dashboard
  layouts: []
  panels: {}
YAML

# Step 1: Create via apply.
echo "--- Step 1: Apply dashboard with annotations ---"
APPLY_OUTPUT=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY_OUTPUT"
if ! echo "$APPLY_OUTPUT" | grep -q "$ID"; then
  echo "FAIL: apply output does not contain ID '$ID'"
  exit 1
fi

# Step 2: Get the dashboard and verify annotations are present.
echo "--- Step 2: Verify annotations on created dashboard ---"
GET_YAML=$("$DASH0" dashboards get "$ID" -o yaml)
echo "$GET_YAML"

if ! echo "$GET_YAML" | yq -e '.metadata.annotations."dash0.com/folder-path"' > /dev/null 2>&1; then
  echo "FAIL: dash0.com/folder-path annotation is missing after create"
  exit 1
fi
ACTUAL_FOLDER=$(echo "$GET_YAML" | yq '.metadata.annotations."dash0.com/folder-path"')
if [ "$ACTUAL_FOLDER" != "$FOLDER_PATH" ]; then
  echo "FAIL: expected folder-path '$FOLDER_PATH', got '$ACTUAL_FOLDER'"
  exit 1
fi
echo "folder-path: $ACTUAL_FOLDER"

# dash0.com/source is set by the server based on the API path, so it may differ
# from what we sent. Just verify it is present.
if ! echo "$GET_YAML" | yq -e '.metadata.annotations."dash0.com/source"' > /dev/null 2>&1; then
  echo "FAIL: dash0.com/source annotation is missing after create"
  exit 1
fi
ACTUAL_SOURCE=$(echo "$GET_YAML" | yq '.metadata.annotations."dash0.com/source"')
echo "source: $ACTUAL_SOURCE"

# Step 3: Export and re-apply (round-trip). Annotations must survive.
echo "--- Step 3: Export and re-apply (round-trip) ---"
"$DASH0" dashboards get "$ID" -o yaml > "${TMPDIR}/exported.yaml"
REAPPLY_OUTPUT=$("$DASH0" apply -f "${TMPDIR}/exported.yaml")
echo "$REAPPLY_OUTPUT"

# Step 4: Verify annotations survived the round-trip.
echo "--- Step 4: Verify annotations after round-trip ---"
GET_YAML2=$("$DASH0" dashboards get "$ID" -o yaml)

ACTUAL_FOLDER2=$(echo "$GET_YAML2" | yq '.metadata.annotations."dash0.com/folder-path"')
if [ "$ACTUAL_FOLDER2" != "$FOLDER_PATH" ]; then
  echo "FAIL: folder-path changed after round-trip: expected '$FOLDER_PATH', got '$ACTUAL_FOLDER2'"
  exit 1
fi
echo "folder-path after round-trip: $ACTUAL_FOLDER2"

# Cleanup.
echo "--- Cleanup ---"
"$DASH0" dashboards delete "$ID" --force
echo "=== Dashboard annotations round-trip test PASSED ==="
