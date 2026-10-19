#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

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

# Step 5: The API soft-deletes check rules, so GET still returns the record
# after delete. `check-rules get` must show it as deleted.
echo "--- Step 5: Get after delete (expect: Deleted line) ---"
GET5=$("$DASH0" check-rules get "$ID")
echo "$GET5"
if ! echo "$GET5" | grep -q "^Deleted: "; then
  echo "FAIL: expected a 'Deleted:' line in check-rules get output after delete"
  exit 1
fi

# Step 6: Apply again after deletion. Apply calls PUT (upsert), which restores
# the soft-deleted check rule under the same ID and reports it as "restored".
echo "--- Step 6: Apply after delete (expect: restored, ID in output) ---"
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

# Step 7: Verify the restored asset is active (not marked deleted, and listed).
echo "--- Step 7: Verify restored asset is active ---"
if ! GET7=$("$DASH0" check-rules get "$ID"); then
  echo "FAIL: check-rules get '$ID' failed after re-apply"
  exit 1
fi
if echo "$GET7" | grep -q "^Deleted: "; then
  echo "FAIL: check-rules get still shows '$ID' as deleted after re-apply"
  exit 1
fi
if ! "$DASH0" check-rules list --all 2>/dev/null | grep -q "$ID"; then
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
