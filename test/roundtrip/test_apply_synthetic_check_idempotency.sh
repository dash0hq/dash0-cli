#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ID=$(uuidgen | tr '[:upper:]' '[:lower:]')
YAML_FILE="${TMPDIR}/synthetic-check.yaml"

echo "=== Synthetic check apply idempotency test ==="
echo "Generated ID: $ID"

# Inject the generated ID into the fixture.
ID="$ID" yq '.metadata.labels."dash0.com/id" = env(ID)' "$FIXTURES/synthetic-check.yaml" > "$YAML_FILE"

# Step 1: First apply — should create the synthetic check with the generated ID.
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
if ! "$DASH0" synthetic-checks get "$ID" > /dev/null; then
  echo "FAIL: synthetic-checks get '$ID' failed after second apply"
  exit 1
fi

# Step 4: Delete the synthetic check.
echo "--- Step 4: Delete ---"
DELETE4=$("$DASH0" synthetic-checks delete "$ID" --force)
echo "$DELETE4"
if ! echo "$DELETE4" | grep -q "deleted"; then
  echo "FAIL: expected 'deleted' in delete output"
  exit 1
fi

# Step 5: The API soft-deletes synthetic checks, so GET still returns the record
# after delete. `synthetic-checks get` must show it as deleted.
echo "--- Step 5: Get after delete (expect: Deleted line) ---"
GET5=$("$DASH0" synthetic-checks get "$ID")
echo "$GET5"
if ! echo "$GET5" | grep -q "^Deleted: "; then
  echo "FAIL: expected a 'Deleted:' line in synthetic-checks get output after delete"
  exit 1
fi

# Step 6: Apply again after deletion. Apply calls PUT (upsert), which restores
# the soft-deleted synthetic check and reports it as "restored".
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

# Step 7: Verify the restored asset is active (not marked deleted, and listed).
echo "--- Step 7: Verify restored asset is active ---"
if ! GET7=$("$DASH0" synthetic-checks get "$ID"); then
  echo "FAIL: synthetic-checks get '$ID' failed after re-apply"
  exit 1
fi
if echo "$GET7" | grep -q "^Deleted: "; then
  echo "FAIL: synthetic-checks get still shows '$ID' as deleted after re-apply"
  exit 1
fi
# Capture the list before searching it: with pipefail, `list | grep -q` fails
# when grep exits on an early match and the CLI dies on the closed pipe.
if ! LIST7=$("$DASH0" synthetic-checks list --all 2>/dev/null) || ! grep -q "$ID" <<< "$LIST7"; then
  echo "FAIL: synthetic-checks list does not contain '$ID' after re-apply"
  exit 1
fi

# Cleanup.
CLEANUP=$("$DASH0" synthetic-checks delete "$ID" --force)
echo "$CLEANUP"
if ! echo "$CLEANUP" | grep -q "deleted"; then
  echo "FAIL: expected 'deleted' in cleanup output"
  exit 1
fi

echo "=== Synthetic check apply idempotency test PASSED ==="
