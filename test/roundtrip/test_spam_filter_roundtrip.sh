#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
FIXTURE="${FIXTURES}/spam-filter.yaml"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ASSET_NAME=$(yq '.metadata.name' "$FIXTURE")

echo "=== Spam filter round-trip test ==="
echo "Asset name: $ASSET_NAME"

# Step 1: Create from fixture
echo "--- Step 1: Create spam filter from fixture ---"
if ! CREATE_OUTPUT=$("$DASH0" --experimental spam-filters create -f "$FIXTURE"); then
  echo "FAIL: spam-filters create failed"
  exit 1
fi
echo "$CREATE_OUTPUT"
if ! echo "$CREATE_OUTPUT" | grep -q "$ASSET_NAME"; then
  echo "FAIL: create output does not mention asset name '$ASSET_NAME'"
  exit 1
fi

# Step 2: List spam filters and find the created asset by name
echo "--- Step 2: List spam filters and find created asset ---"
if ! LIST_JSON=$("$DASH0" --experimental spam-filters list --all -o json); then
  echo "FAIL: spam-filters list -o json failed"
  exit 1
fi
ID=$(echo "$LIST_JSON" | jq -r --arg name "$ASSET_NAME" '[.[] | select(.metadata.name == $name)][0].metadata.labels["dash0.com/id"] // empty')
if [ -z "$ID" ]; then
  echo "FAIL: Could not find created spam filter '$ASSET_NAME' in list"
  exit 1
fi
echo "Created spam filter ID: $ID"

# Step 3: Get by ID
echo "--- Step 3: Get spam filter by ID ---"
if ! "$DASH0" --experimental spam-filters get "$ID"; then
  echo "FAIL: spam-filters get failed"
  exit 1
fi

# Step 4: Export to YAML
echo "--- Step 4: Export spam filter to YAML ---"
if ! "$DASH0" --experimental spam-filters get "$ID" -o yaml > "${TMPDIR}/exported.yaml"; then
  echo "FAIL: spam-filters get -o yaml failed"
  exit 1
fi
echo "Exported to ${TMPDIR}/exported.yaml"

# Step 5: Re-import via update (round-trip)
echo "--- Step 5: Re-import exported YAML via update ---"
if ! "$DASH0" --experimental spam-filters update "$ID" -f "${TMPDIR}/exported.yaml"; then
  echo "FAIL: spam-filters update failed"
  exit 1
fi

# Step 6: Delete
echo "--- Step 6: Delete spam filter ---"
if ! "$DASH0" --experimental spam-filters delete "$ID" --force; then
  echo "FAIL: spam-filters delete failed"
  exit 1
fi

# Step 7: Verify deletion
echo "--- Step 7: Verify deletion ---"
if ! LIST_JSON=$("$DASH0" --experimental spam-filters list --all -o json); then
  echo "FAIL: spam-filters list -o json failed"
  exit 1
fi
if echo "$LIST_JSON" | jq -e --arg id "$ID" '.[] | select(.metadata.labels["dash0.com/id"] == $id)' > /dev/null 2>&1; then
  echo "FAIL: Spam filter '$ID' still exists after deletion"
  exit 1
fi

echo "=== Spam filter round-trip test PASSED ==="
