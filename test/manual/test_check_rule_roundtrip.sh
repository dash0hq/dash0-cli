#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
FIXTURE="${FIXTURES}/check-rule.yaml"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ASSET_NAME=$(yq '.name' "$FIXTURE")

echo "=== Check rule round-trip test ==="
echo "Asset name: $ASSET_NAME"

# Step 1: Create from fixture
echo "--- Step 1: Create check rule from fixture ---"
CREATE_OUTPUT=$("$DASH0" check-rules create -f "$FIXTURE")
if [ $? -ne 0 ]; then
  echo "FAIL: check-rules create failed"
  exit 1
fi
echo "$CREATE_OUTPUT"
if ! echo "$CREATE_OUTPUT" | grep -q "$ASSET_NAME"; then
  echo "FAIL: create output does not mention asset name '$ASSET_NAME'"
  exit 1
fi

# Step 2: List check rules and find the created asset by name
echo "--- Step 2: List check rules and find created asset ---"
LIST_JSON=$("$DASH0" check-rules list -o json)
if [ $? -ne 0 ]; then
  echo "FAIL: check-rules list -o json failed"
  exit 1
fi
ID=$(echo "$LIST_JSON" | jq -r --arg name "$ASSET_NAME" '[.[] | select(.name == $name)][0].id // empty')
if [ -z "$ID" ]; then
  echo "FAIL: Could not find created check rule '$ASSET_NAME' in list"
  exit 1
fi
echo "Created check rule ID: $ID"

# Step 3: Get by ID
echo "--- Step 3: Get check rule by ID ---"
if ! "$DASH0" check-rules get "$ID"; then
  echo "FAIL: check-rules get failed"
  exit 1
fi

# Step 4: Export to YAML
echo "--- Step 4: Export check rule to YAML ---"
if ! "$DASH0" check-rules get "$ID" -o yaml > "${TMPDIR}/exported.yaml"; then
  echo "FAIL: check-rules get -o yaml failed"
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
echo "--- Step 6: Delete check rule ---"
if ! "$DASH0" check-rules delete "$ID" --force; then
  echo "FAIL: check-rules delete failed"
  exit 1
fi

# Step 7: Verify deletion
echo "--- Step 7: Verify deletion ---"
LIST_JSON=$("$DASH0" check-rules list -o json)
if [ $? -ne 0 ]; then
  echo "FAIL: check-rules list -o json failed"
  exit 1
fi
if echo "$LIST_JSON" | jq -e --arg id "$ID" '.[] | select(.id == $id)' > /dev/null 2>&1; then
  echo "FAIL: Check rule '$ID' still exists after deletion"
  exit 1
fi

echo "=== Check rule round-trip test PASSED ==="
