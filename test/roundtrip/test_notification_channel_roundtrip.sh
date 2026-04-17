#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
FIXTURE="${FIXTURES}/notification-channel.yaml"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ASSET_NAME=$(yq '.metadata.name' "$FIXTURE")

echo "=== Notification channel round-trip test ==="
echo "Asset name: $ASSET_NAME"

# Step 1: Create from fixture
echo "--- Step 1: Create notification channel from fixture ---"
if ! CREATE_OUTPUT=$("$DASH0" -X notification-channels create -f "$FIXTURE"); then
  echo "FAIL: notification-channels create failed"
  exit 1
fi
echo "$CREATE_OUTPUT"
if ! echo "$CREATE_OUTPUT" | grep -q "$ASSET_NAME"; then
  echo "FAIL: create output does not mention asset name '$ASSET_NAME'"
  exit 1
fi

# Step 2: List notification channels and find the created channel by name
echo "--- Step 2: List notification channels and find created channel ---"
if ! LIST_JSON=$("$DASH0" -X notification-channels list -o json); then
  echo "FAIL: notification-channels list -o json failed"
  exit 1
fi
ID=$(echo "$LIST_JSON" | jq -r --arg name "$ASSET_NAME" '[.[] | select(.metadata.name == $name)][0].metadata.labels["dash0.com/id"] // empty')
if [ -z "$ID" ]; then
  echo "FAIL: Could not find created notification channel '$ASSET_NAME' in list"
  exit 1
fi
echo "Created notification channel ID: $ID"

# Step 3: Get by ID
echo "--- Step 3: Get notification channel by ID ---"
if ! "$DASH0" -X notification-channels get "$ID"; then
  echo "FAIL: notification-channels get failed"
  exit 1
fi

# Step 4: Export to YAML
echo "--- Step 4: Export notification channel to YAML ---"
if ! "$DASH0" -X notification-channels get "$ID" -o yaml > "${TMPDIR}/exported.yaml"; then
  echo "FAIL: notification-channels get -o yaml failed"
  exit 1
fi
echo "Exported to ${TMPDIR}/exported.yaml"

# Step 5: Delete
echo "--- Step 5: Delete notification channel ---"
if ! "$DASH0" -X notification-channels delete "$ID" --force; then
  echo "FAIL: notification-channels delete failed"
  exit 1
fi

# Step 6: Verify deletion
echo "--- Step 6: Verify deletion ---"
if ! LIST_JSON=$("$DASH0" -X notification-channels list -o json); then
  echo "FAIL: notification-channels list -o json failed"
  exit 1
fi
if echo "$LIST_JSON" | jq -e --arg id "$ID" '.[] | select(.metadata.labels["dash0.com/id"] == $id)' > /dev/null 2>&1; then
  echo "FAIL: Notification channel '$ID' still exists after deletion"
  exit 1
fi

echo "=== Notification channel round-trip test PASSED ==="
