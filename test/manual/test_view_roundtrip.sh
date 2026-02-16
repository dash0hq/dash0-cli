#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
FIXTURE="${FIXTURES}/view.yaml"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ASSET_NAME=$(yq '.metadata.name' "$FIXTURE")

echo "=== View round-trip test ==="
echo "Asset name: $ASSET_NAME"

# Step 1: Create from fixture
echo "--- Step 1: Create view from fixture ---"
CREATE_OUTPUT=$("$DASH0" views create -f "$FIXTURE")
if [ $? -ne 0 ]; then
  echo "FAIL: views create failed"
  exit 1
fi
echo "$CREATE_OUTPUT"
if ! echo "$CREATE_OUTPUT" | grep -q "$ASSET_NAME"; then
  echo "FAIL: create output does not mention asset name '$ASSET_NAME'"
  exit 1
fi

# Step 2: List views and find the created asset by name
echo "--- Step 2: List views and find created asset ---"
LIST_JSON=$("$DASH0" views list -o json)
if [ $? -ne 0 ]; then
  echo "FAIL: views list -o json failed"
  exit 1
fi
ID=$(echo "$LIST_JSON" | jq -r --arg name "$ASSET_NAME" '[.[] | select(.name == $name)][0].id // empty')
if [ -z "$ID" ]; then
  echo "FAIL: Could not find created view '$ASSET_NAME' in list"
  exit 1
fi
echo "Created view ID: $ID"

# Step 3: Get by ID
echo "--- Step 3: Get view by ID ---"
if ! "$DASH0" views get "$ID"; then
  echo "FAIL: views get failed"
  exit 1
fi

# Step 4: Export to YAML
echo "--- Step 4: Export view to YAML ---"
if ! "$DASH0" views get "$ID" -o yaml > "${TMPDIR}/exported.yaml"; then
  echo "FAIL: views get -o yaml failed"
  exit 1
fi
echo "Exported to ${TMPDIR}/exported.yaml"

# Step 5: Re-import via apply (round-trip)
echo "--- Step 5: Re-import exported YAML via apply ---"
if ! "$DASH0" apply -f "${TMPDIR}/exported.yaml"; then
  echo "FAIL: apply failed"
  exit 1
fi

# Step 6: Delete
echo "--- Step 6: Delete view ---"
if ! "$DASH0" views delete "$ID" --force; then
  echo "FAIL: views delete failed"
  exit 1
fi

# Step 7: Verify deletion
echo "--- Step 7: Verify deletion ---"
LIST_JSON=$("$DASH0" views list -o json)
if [ $? -ne 0 ]; then
  echo "FAIL: views list -o json failed"
  exit 1
fi
if echo "$LIST_JSON" | jq -e --arg id "$ID" '.[] | select(.id == $id)' > /dev/null 2>&1; then
  echo "FAIL: View '$ID' still exists after deletion"
  exit 1
fi

echo "=== View round-trip test PASSED ==="
