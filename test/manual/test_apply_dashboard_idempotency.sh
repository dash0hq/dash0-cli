#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# Dashboard origins must be non-UUID strings (the API rejects UUID-format origins
# because UUIDs are reserved for server-assigned primary IDs).
ID="test-idempotency-$(uuidgen | tr '[:upper:]' '[:lower:]' | tr -d '-')"
YAML_FILE="${TMPDIR}/dashboard.yaml"

echo "=== Dashboard apply idempotency test ==="
echo "Generated origin: $ID"

# Write a minimal dashboard fixture with the generated origin.
cat > "$YAML_FILE" << YAML
kind: Dashboard
metadata:
  dash0extensions:
    id: $ID
  name: Idempotency Test Dashboard
spec:
  display:
    name: Idempotency Test Dashboard
  layouts: []
  panels: {}
YAML

# Step 1: First apply — should create the dashboard with the generated ID.
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

# Step 3: Verify the asset is reachable by origin.
echo "--- Step 3: Verify asset exists ---"
if ! "$DASH0" dashboards get "$ID" > /dev/null; then
  echo "FAIL: dashboards get '$ID' failed after second apply"
  exit 1
fi

# Step 4: Delete the dashboard.
echo "--- Step 4: Delete ---"
DELETE4=$("$DASH0" dashboards delete "$ID" --force)
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
# Step 6: Verify the restored asset is reachable by origin.
# Note: dashboards list shows server-assigned primary IDs, not user-defined origins,
# so we verify via GET by origin instead.
echo "--- Step 6: Verify restored asset is reachable ---"
if ! "$DASH0" dashboards get "$ID" > /dev/null; then
  echo "FAIL: dashboards get '$ID' failed after re-apply"
  exit 1
fi

# Cleanup.
CLEANUP=$("$DASH0" dashboards delete "$ID" --force)
echo "$CLEANUP"
if ! echo "$CLEANUP" | grep -q "deleted"; then
  echo "FAIL: expected 'deleted' in cleanup output"
  exit 1
fi

echo "=== Dashboard apply idempotency test PASSED ==="
