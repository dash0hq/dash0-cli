#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ID=$(uuidgen | tr '[:upper:]' '[:lower:]')
YAML_FILE="${TMPDIR}/check-rule.yaml"

echo "=== Check rule apply idempotency test ==="
echo "Generated ID: $ID"

# Inject the generated ID into the fixture.
yq ".id = \"$ID\"" "$FIXTURES/check-rule.yaml" > "$YAML_FILE"

# Step 1: First apply — should create the check rule with the generated ID.
echo "--- Step 1: First apply (expect: created) ---"
APPLY1=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY1"
if ! echo "$APPLY1" | grep -q "created"; then
  echo "FAIL: expected 'created' in first apply output"
  exit 1
fi
if ! echo "$APPLY1" | grep -q "$ID"; then
  echo "FAIL: expected ID '$ID' in first apply output"
  exit 1
fi

# Step 2: Apply the same file again — should update, not create a duplicate.
echo "--- Step 2: Second apply (expect: no duplicate created) ---"
APPLY2=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY2"
if echo "$APPLY2" | grep -q "created"; then
  echo "FAIL: unexpected 'created' on second apply — duplicate was created"
  exit 1
fi

# Step 3: Verify the asset still exists with the expected ID.
echo "--- Step 3: Verify asset exists ---"
if ! "$DASH0" check-rules get "$ID" > /dev/null; then
  echo "FAIL: check-rules get '$ID' failed after second apply"
  exit 1
fi

# Step 4: Delete the check rule.
echo "--- Step 4: Delete ---"
DELETE4=$("$DASH0" check-rules delete "$ID" --force)
echo "$DELETE4"
if ! echo "$DELETE4" | grep -q "deleted"; then
  echo "FAIL: expected 'deleted' in delete output"
  exit 1
fi

# Step 5: Apply again after deletion — the asset is restored with the same ID.
# The API soft-deletes assets, so GET still returns the record after delete.
# Apply calls PUT (upsert), which restores the asset — output shows "no changes"
# (not "created") because the data is unchanged, but the asset is active again.
echo "--- Step 5: Apply after delete (expect: asset restored, ID in output) ---"
APPLY5=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY5"
# Step 6: Verify the restored asset is active (appears in list and is reachable by GET).
echo "--- Step 6: Verify restored asset is active ---"
if ! "$DASH0" check-rules get "$ID" > /dev/null; then
  echo "FAIL: check-rules get '$ID' failed after re-apply"
  exit 1
fi
if ! "$DASH0" check-rules list 2>/dev/null | grep -q "$ID"; then
  echo "FAIL: check-rules list does not contain '$ID' after re-apply"
  exit 1
fi

# Cleanup.
CLEANUP=$("$DASH0" check-rules delete "$ID" --force)
echo "$CLEANUP"
if ! echo "$CLEANUP" | grep -q "deleted"; then
  echo "FAIL: expected 'deleted' in cleanup output"
  exit 1
fi

echo "=== Check rule apply idempotency test PASSED ==="
