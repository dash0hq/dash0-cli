#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# SLO IDs (dash0.com/id) are server-assigned (e.g. "slo_01k...") and cannot be
# chosen by the client. The client-settable stable upsert key is
# dash0.com/origin — that is what `apply` uses to PUT create-or-replace, so
# origin is the key that makes apply idempotent (see ImportSLO).
ORIGIN="cli-roundtrip-$(uuidgen | tr '[:upper:]' '[:lower:]')"
YAML_FILE="${TMPDIR}/slo.yaml"

echo "=== SLO apply idempotency test ==="
echo "Origin: $ORIGIN"

# Inject the origin into the fixture (the client-settable upsert key).
ORIGIN="$ORIGIN" yq '.metadata.labels."dash0.com/origin" = env(ORIGIN)' "$FIXTURES/slo.yaml" > "$YAML_FILE"

# Step 1: First apply — should create the SLO. The printed id is the
# server-assigned slo_... id, not the origin, so we do not assert on it here.
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

# Step 3: Verify exactly one SLO exists with the injected origin (no duplicates),
# and that it is reachable by origin (GET/DELETE accept origin-or-id).
echo "--- Step 3: Verify a single SLO with the expected origin ---"
if ! "$DASH0" slos get "$ORIGIN" > /dev/null; then
  echo "FAIL: slos get '$ORIGIN' failed after second apply"
  exit 1
fi
COUNT=$("$DASH0" slos list --all -o json | jq --arg o "$ORIGIN" '[.[] | select(.metadata.labels["dash0.com/origin"] == $o)] | length')
echo "Matching SLOs: $COUNT"
if [ "$COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 SLO with origin '$ORIGIN', found $COUNT (duplicate created)"
  exit 1
fi

# Step 4: Delete the SLO (by origin).
echo "--- Step 4: Delete ---"
DELETE4=$("$DASH0" slos delete "$ORIGIN" --force)
echo "$DELETE4"
if ! echo "$DELETE4" | grep -q "deleted"; then
  echo "FAIL: expected 'deleted' in delete output"
  exit 1
fi

# Step 5: Apply again after deletion — the asset is restored under the same origin.
# The API soft-deletes assets, so apply (PUT upsert) restores the record.
echo "--- Step 5: Apply after delete (expect: asset restored) ---"
APPLY5=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY5"

# Step 6: Verify the restored asset is active (reachable by origin and listed once).
echo "--- Step 6: Verify restored asset is active ---"
if ! "$DASH0" slos get "$ORIGIN" > /dev/null; then
  echo "FAIL: slos get '$ORIGIN' failed after re-apply"
  exit 1
fi
COUNT=$("$DASH0" slos list --all -o json | jq --arg o "$ORIGIN" '[.[] | select(.metadata.labels["dash0.com/origin"] == $o)] | length')
if [ "$COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 SLO with origin '$ORIGIN' after re-apply, found $COUNT"
  exit 1
fi

# Cleanup.
CLEANUP=$("$DASH0" slos delete "$ORIGIN" --force)
echo "$CLEANUP"
if ! echo "$CLEANUP" | grep -q "deleted"; then
  echo "FAIL: expected 'deleted' in cleanup output"
  exit 1
fi

echo "=== SLO apply idempotency test PASSED ==="
