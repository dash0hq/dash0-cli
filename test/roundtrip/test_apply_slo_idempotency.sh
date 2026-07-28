#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ID=$(uuidgen | tr '[:upper:]' '[:lower:]')
YAML_FILE="${TMPDIR}/slo.yaml"

echo "=== SLO apply idempotency test ==="
echo "Generated ID: $ID"

# Inject the generated ID into the fixture.
ID="$ID" yq '.metadata.labels."dash0.com/id" = env(ID)' "$FIXTURES/slo.yaml" > "$YAML_FILE"

# Step 1: First apply — should create the SLO with the generated ID.
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

# Step 3: Verify exactly one SLO exists with the expected ID (no duplicates).
echo "--- Step 3: Verify a single SLO with the expected ID ---"
if ! "$DASH0" slos get "$ID" > /dev/null; then
  echo "FAIL: slos get '$ID' failed after second apply"
  exit 1
fi
COUNT=$("$DASH0" slos list --all -o json | jq --arg id "$ID" '[.[] | select(.metadata.labels["dash0.com/id"] == $id)] | length')
echo "Matching SLOs: $COUNT"
if [ "$COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 SLO with ID '$ID', found $COUNT (duplicate created)"
  exit 1
fi

# Step 4: Delete the SLO.
echo "--- Step 4: Delete ---"
DELETE4=$("$DASH0" slos delete "$ID" --force)
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
if ! "$DASH0" slos get "$ID" > /dev/null; then
  echo "FAIL: slos get '$ID' failed after re-apply"
  exit 1
fi
if ! "$DASH0" slos list --all 2>/dev/null | grep -q "$ID"; then
  echo "FAIL: slos list does not contain '$ID' after re-apply"
  exit 1
fi

# Cleanup.
CLEANUP=$("$DASH0" slos delete "$ID" --force)
echo "$CLEANUP"
if ! echo "$CLEANUP" | grep -q "deleted"; then
  echo "FAIL: expected 'deleted' in cleanup output"
  exit 1
fi

echo "=== SLO apply idempotency test PASSED ==="
