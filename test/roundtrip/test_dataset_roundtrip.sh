#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
FIXTURE_TEMPLATE="${FIXTURES}/dataset.yaml"
TMPDIR="$(mktemp -d)"
# Dash0 keeps a deleted dataset for 30 days, so every run uses a fresh slug.
SLUG="cli-rt-$(uuidgen | tr '[:upper:]' '[:lower:]' | cut -c1-8)"
NAME="CLI Roundtrip Dataset ${SLUG}"
UPDATED_NAME="${NAME} (edited)"
FIXTURE="${TMPDIR}/dataset.yaml"
CREATED=0

cleanup() {
  if [ "$CREATED" = "1" ]; then
    echo "--- Cleanup: deleting dataset $SLUG ---"
    "$DASH0" -X datasets delete "$SLUG" --force > /dev/null 2>&1 || true
  fi
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

sed -e "s|__SLUG__|${SLUG}|" -e "s|__NAME__|${NAME}|" "$FIXTURE_TEMPLATE" > "$FIXTURE"

echo "=== Dataset round-trip test ==="
echo "Slug: $SLUG"

# Step 1: Create from fixture
echo "--- Step 1: Create dataset from fixture ---"
if ! CREATE_OUTPUT=$("$DASH0" -X datasets create -f "$FIXTURE"); then
  echo "FAIL: datasets create failed"
  exit 1
fi
CREATED=1
echo "$CREATE_OUTPUT"
if ! echo "$CREATE_OUTPUT" | grep -q "$NAME"; then
  echo "FAIL: create output does not mention dataset name '$NAME'"
  exit 1
fi

# Step 2: List and find the created dataset by slug
echo "--- Step 2: List datasets and find created dataset ---"
if ! LIST_JSON=$("$DASH0" -X datasets list -o json); then
  echo "FAIL: datasets list -o json failed"
  exit 1
fi
FOUND=$(echo "$LIST_JSON" | jq -r --arg slug "$SLUG" '[.[] | select(.metadata.labels["dash0.com/id"] == $slug)][0].metadata.name // empty')
if [ "$FOUND" != "$NAME" ]; then
  echo "FAIL: Could not find created dataset '$SLUG' in list (found name: '$FOUND')"
  exit 1
fi

# Step 3: Get by slug
echo "--- Step 3: Get dataset by slug ---"
if ! GET_OUTPUT=$("$DASH0" -X datasets get "$SLUG"); then
  echo "FAIL: datasets get failed"
  exit 1
fi
echo "$GET_OUTPUT"
if ! echo "$GET_OUTPUT" | grep -q "$NAME"; then
  echo "FAIL: get output does not mention dataset name '$NAME'"
  exit 1
fi

# Step 4: Export to YAML
echo "--- Step 4: Export dataset to YAML ---"
EXPORT_FILE="${TMPDIR}/exported.yaml"
if ! "$DASH0" -X datasets get "$SLUG" -o yaml > "$EXPORT_FILE"; then
  echo "FAIL: datasets get -o yaml failed"
  exit 1
fi

# Step 5: Update the name and verify the change
echo "--- Step 5: Update dataset ---"
yq -i ".metadata.name = \"${UPDATED_NAME}\"" "$EXPORT_FILE"
if ! UPDATE_OUTPUT=$("$DASH0" -X datasets update -f "$EXPORT_FILE"); then
  echo "FAIL: datasets update failed"
  exit 1
fi
echo "$UPDATE_OUTPUT"
UPDATED=$("$DASH0" -X datasets get "$SLUG" -o json | jq -r '.metadata.name')
if [ "$UPDATED" != "$UPDATED_NAME" ]; then
  echo "FAIL: expected name '$UPDATED_NAME' after update, got '$UPDATED'"
  exit 1
fi

# Step 6: Delete
echo "--- Step 6: Delete dataset ---"
if ! "$DASH0" -X datasets delete "$SLUG" --force; then
  echo "FAIL: datasets delete failed"
  exit 1
fi

# Step 7: Verify deletion
echo "--- Step 7: Verify deletion ---"
LIST_JSON=$("$DASH0" -X datasets list -o json)
if echo "$LIST_JSON" | jq -e --arg slug "$SLUG" '.[] | select(.metadata.labels["dash0.com/id"] == $slug)' > /dev/null 2>&1; then
  echo "FAIL: Dataset '$SLUG' still exists after deletion"
  exit 1
fi

# Step 8: Deleting again with --force succeeds (idempotent)
echo "--- Step 8: Delete again with --force (expect: success) ---"
if ! "$DASH0" -X datasets delete "$SLUG" --force; then
  echo "FAIL: second datasets delete --force failed"
  exit 1
fi

echo "=== Dataset round-trip test PASSED ==="
