#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
FIXTURE="${FIXTURES}/synthetic-check.yaml"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ASSET_NAME=$(yq '.metadata.name' "$FIXTURE")

echo "=== Synthetic check round-trip test ==="
echo "Asset name: $ASSET_NAME"

# Step 1: Create from fixture
echo "--- Step 1: Create synthetic check from fixture ---"
CREATE_OUTPUT=$("$DASH0" synthetic-checks create -f "$FIXTURE")
if [ $? -ne 0 ]; then
  echo "FAIL: synthetic-checks create failed"
  exit 1
fi
echo "$CREATE_OUTPUT"
if ! echo "$CREATE_OUTPUT" | grep -q "$ASSET_NAME"; then
  echo "FAIL: create output does not mention asset name '$ASSET_NAME'"
  exit 1
fi

# Step 2: List synthetic checks and find the created asset by name
echo "--- Step 2: List synthetic checks and find created asset ---"
LIST_JSON=$("$DASH0" synthetic-checks list -o json)
if [ $? -ne 0 ]; then
  echo "FAIL: synthetic-checks list -o json failed"
  exit 1
fi
ID=$(echo "$LIST_JSON" | jq -r --arg name "$ASSET_NAME" '[.[] | select(.name == $name)][0].id // empty')
if [ -z "$ID" ]; then
  echo "FAIL: Could not find created synthetic check '$ASSET_NAME' in list"
  exit 1
fi
echo "Created synthetic check ID: $ID"

# Step 3: Get by ID
echo "--- Step 3: Get synthetic check by ID ---"
if ! "$DASH0" synthetic-checks get "$ID"; then
  echo "FAIL: synthetic-checks get failed"
  exit 1
fi

# Step 4: Export to YAML
echo "--- Step 4: Export synthetic check to YAML ---"
if ! "$DASH0" synthetic-checks get "$ID" -o yaml > "${TMPDIR}/exported.yaml"; then
  echo "FAIL: synthetic-checks get -o yaml failed"
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
echo "--- Step 6: Delete synthetic check ---"
if ! "$DASH0" synthetic-checks delete "$ID" --force; then
  echo "FAIL: synthetic-checks delete failed"
  exit 1
fi

# Step 7: Verify deletion
echo "--- Step 7: Verify deletion ---"
LIST_JSON=$("$DASH0" synthetic-checks list -o json)
if [ $? -ne 0 ]; then
  echo "FAIL: synthetic-checks list -o json failed"
  exit 1
fi
if echo "$LIST_JSON" | jq -e --arg id "$ID" '.[] | select(.id == $id)' > /dev/null 2>&1; then
  echo "FAIL: Synthetic check '$ID' still exists after deletion"
  exit 1
fi

echo "=== Synthetic check round-trip test PASSED ==="
