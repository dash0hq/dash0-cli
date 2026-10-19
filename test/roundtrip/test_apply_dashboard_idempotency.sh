#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

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

# Step 5: The API soft-deletes dashboards, so GET still returns the record
# after delete. `dashboards get` must show it as deleted.
echo "--- Step 5: Get after delete (expect: Deleted line) ---"
GET5=$("$DASH0" dashboards get "$ID")
echo "$GET5"
if ! echo "$GET5" | grep -q "^Deleted: "; then
  echo "FAIL: expected a 'Deleted:' line in dashboards get output after delete"
  exit 1
fi

# Step 6: Apply again after deletion. Apply calls PUT (upsert), which restores
# the soft-deleted dashboard and reports it as "restored".
echo "--- Step 6: Apply after delete (expect: restored) ---"
APPLY6=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY6"
if ! echo "$APPLY6" | grep -qE 'restored[[:space:]]*$'; then
  echo "FAIL: expected 'restored' in apply output after delete"
  exit 1
fi
if ! echo "$APPLY6" | grep -q "$ID"; then
  echo "FAIL: expected ID '$ID' in apply output after delete"
  exit 1
fi

# Step 7: Verify the restored asset is active (reachable and not marked deleted).
# Note: dashboards list shows server-assigned primary IDs, not user-defined origins,
# so we verify via GET by origin instead.
echo "--- Step 7: Verify restored asset is active ---"
if ! GET7=$("$DASH0" dashboards get "$ID"); then
  echo "FAIL: dashboards get '$ID' failed after re-apply"
  exit 1
fi
if echo "$GET7" | grep -q "^Deleted: "; then
  echo "FAIL: dashboards get still shows '$ID' as deleted after re-apply"
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
