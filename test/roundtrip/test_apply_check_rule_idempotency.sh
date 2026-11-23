#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# An id that is not found is created at the origin with a server-assigned id,
# so the origin is the key the test sets and addresses the asset by.
ORIGIN="cli-roundtrip-$(uuidgen | tr '[:upper:]' '[:lower:]')"
YAML_FILE="${TMPDIR}/check-rule.yaml"

echo "=== Check rule apply idempotency test ==="
echo "Origin: $ORIGIN"

# Inject the origin into the fixture.
ORIGIN="$ORIGIN" yq '.labels."dash0.com/origin" = env(ORIGIN)' "$FIXTURES/check-rule.yaml" > "$YAML_FILE"

# Step 1: First apply — should create the check rule at the origin.
echo "--- Step 1: First apply (expect: created) ---"
APPLY1=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY1"
if ! echo "$APPLY1" | grep -q "created"; then
  echo "FAIL: expected 'created' in first apply output"
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

# Step 3: Verify the asset still exists at the origin.
echo "--- Step 3: Verify asset exists ---"
if ! "$DASH0" check-rules get "$ORIGIN" > /dev/null; then
  echo "FAIL: check-rules get '$ORIGIN' failed after second apply"
  exit 1
fi

# Step 4: Delete the check rule.
echo "--- Step 4: Delete ---"
DELETE4=$("$DASH0" check-rules delete "$ORIGIN" --force)
echo "$DELETE4"
if ! echo "$DELETE4" | grep -q "deleted"; then
  echo "FAIL: expected 'deleted' in delete output"
  exit 1
fi

# Step 5: The API soft-deletes check rules, so GET still returns the record
# after delete. `check-rules get` must show it as deleted, and `list` must not
# show it. `list | grep -q` can fail under pipefail, so capture the output first.
echo "--- Step 5: Get and list after delete (expect: Deleted line, not listed) ---"
GET5=$("$DASH0" check-rules get "$ORIGIN")
echo "$GET5"
if ! grep -q "^Deleted: " <<< "$GET5"; then
  echo "FAIL: expected a 'Deleted:' line in check-rules get output after delete"
  exit 1
fi
LIST5=$("$DASH0" check-rules list --all -o wide 2>/dev/null)
if grep -q "$ORIGIN" <<< "$LIST5"; then
  echo "FAIL: check-rules list still contains '$ORIGIN' after delete"
  exit 1
fi

# Step 6: Apply again after deletion. Apply calls PUT (upsert), which restores
# the soft-deleted check rule at the same origin and reports it as "restored".
echo "--- Step 6: Apply after delete (expect: restored) ---"
APPLY6=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY6"
if ! grep -qE 'restored[[:space:]]*$' <<< "$APPLY6"; then
  echo "FAIL: expected 'restored' in apply output after delete"
  exit 1
fi

# Step 7: Verify the restored asset is active (not marked deleted, and listed).
echo "--- Step 7: Verify restored asset is active ---"
if ! GET7=$("$DASH0" check-rules get "$ORIGIN"); then
  echo "FAIL: check-rules get '$ORIGIN' failed after re-apply"
  exit 1
fi
if grep -q "^Deleted: " <<< "$GET7"; then
  echo "FAIL: check-rules get still shows '$ORIGIN' as deleted after re-apply"
  exit 1
fi
LIST7=$("$DASH0" check-rules list --all -o wide 2>/dev/null)
if ! grep -q "$ORIGIN" <<< "$LIST7"; then
  echo "FAIL: check-rules list does not contain '$ORIGIN' after re-apply"
  exit 1
fi

# Cleanup.
CLEANUP=$("$DASH0" check-rules delete "$ORIGIN" --force)
echo "$CLEANUP"
if ! echo "$CLEANUP" | grep -q "deleted"; then
  echo "FAIL: expected 'deleted' in cleanup output"
  exit 1
fi

echo "=== Check rule apply idempotency test PASSED ==="
