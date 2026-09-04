#!/usr/bin/env bash
set -euo pipefail

# Exercises the full CRUD cycle for time series aggregations against a real
# Dash0 environment: create, get, list, update, export/reapply, delete.
#
# Every time series aggregation endpoint requires the organization admin role,
# which is stricter than any other asset type, so this script skips rather
# than fails when the resolved token lacks it. A CI matrix that runs the
# round-trip suite under both a static and an OAuth token would otherwise fail
# half its jobs for a reason that has nothing to do with the CLI.
#
# The fixture's origin is replaced with a unique one on every run. Origins are
# unique per organization and the API upserts by PUT, so a fixed origin would
# silently overwrite a real aggregation of the same name in whatever
# organization the test runs against.

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# Probe the admin role before doing anything else, and skip cleanly if the
# token does not have it.
if ! PROBE=$("$DASH0" tsa list -o json 2>&1); then
  if echo "$PROBE" | grep -qi "admin role"; then
    echo "=== SKIP: time series aggregations require the organization admin role ==="
    echo "The resolved token does not have it. Use a static admin token to run this test."
    exit 0
  fi
  echo "FAIL: could not list time series aggregations:"
  echo "$PROBE"
  exit 1
fi

ORIGIN="roundtrip-tsa-$(uuidgen | tr '[:upper:]' '[:lower:]')"
YAML_FILE="${TMPDIR}/aggregation.yaml"

echo "=== Time series aggregation round-trip ==="
echo "Origin: $ORIGIN"

ORIGIN="$ORIGIN" yq '
  .metadata.name = env(ORIGIN) |
  .metadata.labels."dash0.com/origin" = env(ORIGIN) |
  .spec.display.name = env(ORIGIN)
' "${FIXTURES}/time-series-aggregation.yaml" > "$YAML_FILE"

cleanup() {
  "$DASH0" tsa delete "$ORIGIN" --force > /dev/null 2>&1 || true
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

echo "--- Step 1: Create ---"
CREATE_OUT=$("$DASH0" tsa create -f "$YAML_FILE")
echo "$CREATE_OUT"
if ! echo "$CREATE_OUT" | grep -q "created"; then
  echo "FAIL: expected 'created' in create output"
  exit 1
fi

echo "--- Step 2: Get by origin ---"
GET_OUT=$("$DASH0" tsa get "$ORIGIN")
echo "$GET_OUT"
if ! echo "$GET_OUT" | grep -q "Origin: ${ORIGIN}"; then
  echo "FAIL: get did not return the expected origin"
  exit 1
fi
if ! echo "$GET_OUT" | grep -q "Interval: 5m"; then
  echo "FAIL: get did not return the expected interval"
  exit 1
fi

ID=$("$DASH0" tsa get "$ORIGIN" -o json | jq -r '.metadata.labels["dash0.com/id"]')
if [ -z "$ID" ] || [ "$ID" = "null" ]; then
  echo "FAIL: could not resolve the server-assigned ID"
  exit 1
fi
echo "Server-assigned ID: $ID"

echo "--- Step 3: Get by ID resolves the same aggregation ---"
BY_ID_ORIGIN=$("$DASH0" tsa get "$ID" -o json | jq -r '.metadata.labels["dash0.com/origin"]')
if [ "$BY_ID_ORIGIN" != "$ORIGIN" ]; then
  echo "FAIL: get by ID returned origin '$BY_ID_ORIGIN', expected '$ORIGIN'"
  exit 1
fi

echo "--- Step 4: List contains exactly one record with this origin ---"
COUNT=$("$DASH0" tsa list --all -o json | jq --arg o "$ORIGIN" '[.[] | select(.metadata.labels["dash0.com/origin"] == $o)] | length')
if [ "$COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 record with origin '$ORIGIN', got $COUNT"
  exit 1
fi

echo "--- Step 5: Update changes the interval and shows a diff ---"
CHANGED_FILE="${TMPDIR}/aggregation-changed.yaml"
yq '.spec.sample.interval = "1m"' "$YAML_FILE" > "$CHANGED_FILE"
UPDATE_OUT=$("$DASH0" tsa update -f "$CHANGED_FILE")
echo "$UPDATE_OUT"
if ! echo "$UPDATE_OUT" | grep -q -- "-    interval: 5m"; then
  echo "FAIL: diff did not show the old interval"
  exit 1
fi
if ! echo "$UPDATE_OUT" | grep -q -- "+    interval: 1m"; then
  echo "FAIL: diff did not show the new interval"
  exit 1
fi

echo "--- Step 6: Re-updating the same content reports no changes ---"
NOOP_OUT=$("$DASH0" tsa update -f "$CHANGED_FILE")
echo "$NOOP_OUT"
if ! echo "$NOOP_OUT" | grep -q "no changes"; then
  echo "FAIL: expected 'no changes' when re-updating identical content"
  exit 1
fi

echo "--- Step 7: Export and reapply round-trips cleanly ---"
EXPORT_FILE="${TMPDIR}/aggregation-export.yaml"
"$DASH0" tsa get "$ORIGIN" -o yaml > "$EXPORT_FILE"
REAPPLY_OUT=$("$DASH0" apply -f "$EXPORT_FILE")
echo "$REAPPLY_OUT"
if ! echo "$REAPPLY_OUT" | grep -q "no changes"; then
  echo "FAIL: reapplying an exported definition must be a no-op, got:"
  echo "$REAPPLY_OUT"
  exit 1
fi

echo "--- Step 8: Delete ---"
DELETE_OUT=$("$DASH0" tsa delete "$ORIGIN" --force)
echo "$DELETE_OUT"
if ! echo "$DELETE_OUT" | grep -q "deleted"; then
  echo "FAIL: expected 'deleted' in delete output"
  exit 1
fi

echo "--- Step 9: The aggregation is gone ---"
COUNT_AFTER=$("$DASH0" tsa list --all -o json | jq --arg o "$ORIGIN" '[.[] | select(.metadata.labels["dash0.com/origin"] == $o)] | length')
if [ "$COUNT_AFTER" != "0" ]; then
  echo "FAIL: expected 0 records with origin '$ORIGIN' after delete, got $COUNT_AFTER"
  exit 1
fi

echo "=== Time series aggregation round-trip test PASSED ==="
