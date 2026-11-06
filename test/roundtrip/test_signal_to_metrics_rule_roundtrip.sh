#!/usr/bin/env bash
set -euo pipefail

# Exercises the full CRUD cycle for signal-to-metrics rules against a real
# Dash0 environment: test, create, get, list, update, export/reapply, delete.
#
# The fixture's origin is replaced with a unique one on every run, because
# origins are unique per organization and the API upserts by PUT. A fixed
# origin would overwrite a real rule of the same name. The rule is disabled,
# so a leaked one produces no metric.

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ORIGIN="roundtrip-s2m-$(uuidgen | tr '[:upper:]' '[:lower:]')"
YAML_FILE="${TMPDIR}/rule.yaml"

echo "=== Signal-to-metrics rule round-trip ==="
echo "Origin: $ORIGIN"

ORIGIN="$ORIGIN" yq '
  .metadata.name = env(ORIGIN) |
  .metadata.labels."dash0.com/origin" = env(ORIGIN) |
  .spec.display.name = env(ORIGIN)
' "${FIXTURES}/signal-to-metrics-rule.yaml" > "$YAML_FILE"

cleanup() {
  "$DASH0" s2m delete "$ORIGIN" --force > /dev/null 2>&1 || true
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

echo "--- Step 1: Test the definition (creates nothing) ---"
TEST_OUT=$("$DASH0" s2m test -f "$YAML_FILE" --from now-1h)
echo "$TEST_OUT"
if ! echo "$TEST_OUT" | grep -qE "matched [0-9]+ logs"; then
  echo "FAIL: expected a matched count in test output"
  exit 1
fi
if "$DASH0" s2m get "$ORIGIN" > /dev/null 2>&1; then
  echo "FAIL: test must not create the rule"
  exit 1
fi

echo "--- Step 2: An invalid definition fails the test ---"
INVALID_FILE="${TMPDIR}/rule-invalid.yaml"
yq '.spec.output.type = "gauge"' "$YAML_FILE" > "$INVALID_FILE"
if INVALID_OUT=$("$DASH0" s2m test -f "$INVALID_FILE" 2>&1); then
  echo "FAIL: expected test to reject a typed output without a value, got:"
  echo "$INVALID_OUT"
  exit 1
fi
echo "$INVALID_OUT"
if ! echo "$INVALID_OUT" | grep -q "is not valid"; then
  echo "FAIL: the error did not say the definition is not valid"
  exit 1
fi

echo "--- Step 3: Create ---"
CREATE_OUT=$("$DASH0" s2m create -f "$YAML_FILE")
echo "$CREATE_OUT"
if ! echo "$CREATE_OUT" | grep -q "created"; then
  echo "FAIL: expected 'created' in create output"
  exit 1
fi

echo "--- Step 4: Get by origin ---"
GET_OUT=$("$DASH0" s2m get "$ORIGIN")
echo "$GET_OUT"
if ! echo "$GET_OUT" | grep -q "Origin: ${ORIGIN}"; then
  echo "FAIL: get did not return the expected origin"
  exit 1
fi
if ! echo "$GET_OUT" | grep -q "Interval: 5m"; then
  echo "FAIL: get did not return the expected interval"
  exit 1
fi

ID=$("$DASH0" s2m get "$ORIGIN" -o json | jq -r '.metadata.labels["dash0.com/id"]')
if [ -z "$ID" ] || [ "$ID" = "null" ]; then
  echo "FAIL: could not resolve the server-assigned ID"
  exit 1
fi
echo "Server-assigned ID: $ID"

echo "--- Step 5: Get by ID resolves the same rule ---"
BY_ID_ORIGIN=$("$DASH0" s2m get "$ID" -o json | jq -r '.metadata.labels["dash0.com/origin"]')
if [ "$BY_ID_ORIGIN" != "$ORIGIN" ]; then
  echo "FAIL: get by ID returned origin '$BY_ID_ORIGIN', expected '$ORIGIN'"
  exit 1
fi

echo "--- Step 6: Filtered list contains exactly this rule ---"
COUNT=$("$DASH0" s2m list --all --origin-prefix "$ORIGIN" --signal logs --enabled=false -o json | jq --arg o "$ORIGIN" '[.[] | select(.metadata.labels["dash0.com/origin"] == $o)] | length')
if [ "$COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 record with origin '$ORIGIN', got $COUNT"
  exit 1
fi
ENABLED_COUNT=$("$DASH0" s2m list --all --origin-prefix "$ORIGIN" --enabled -o json | jq 'length')
if [ "$ENABLED_COUNT" != "0" ]; then
  echo "FAIL: --enabled listed the disabled rule"
  exit 1
fi

echo "--- Step 7: Update changes the interval and shows a diff ---"
CHANGED_FILE="${TMPDIR}/rule-changed.yaml"
yq '.spec.output.interval = "1m"' "$YAML_FILE" > "$CHANGED_FILE"
UPDATE_OUT=$("$DASH0" s2m update -f "$CHANGED_FILE")
echo "$UPDATE_OUT"
if ! echo "$UPDATE_OUT" | grep -q -- "-    interval: 5m"; then
  echo "FAIL: diff did not show the old interval"
  exit 1
fi
if ! echo "$UPDATE_OUT" | grep -q -- "+    interval: 1m"; then
  echo "FAIL: diff did not show the new interval"
  exit 1
fi

echo "--- Step 8: Re-updating the same content reports no changes ---"
NOOP_OUT=$("$DASH0" s2m update -f "$CHANGED_FILE")
echo "$NOOP_OUT"
if ! echo "$NOOP_OUT" | grep -q "no changes"; then
  echo "FAIL: expected 'no changes' when re-updating identical content"
  exit 1
fi

echo "--- Step 9: Export and reapply round-trips cleanly ---"
EXPORT_FILE="${TMPDIR}/rule-export.yaml"
"$DASH0" s2m get "$ORIGIN" -o yaml > "$EXPORT_FILE"
REAPPLY_OUT=$("$DASH0" apply -f "$EXPORT_FILE")
echo "$REAPPLY_OUT"
if ! echo "$REAPPLY_OUT" | grep -q "no changes"; then
  echo "FAIL: reapplying an exported definition must be a no-op, got:"
  echo "$REAPPLY_OUT"
  exit 1
fi

echo "--- Step 10: Delete ---"
DELETE_OUT=$("$DASH0" s2m delete "$ORIGIN" --force)
echo "$DELETE_OUT"
if ! echo "$DELETE_OUT" | grep -q "deleted"; then
  echo "FAIL: expected 'deleted' in delete output"
  exit 1
fi

echo "--- Step 11: Deleting again with --force succeeds ---"
if ! "$DASH0" s2m delete "$ORIGIN" --force; then
  echo "FAIL: delete --force of an already-deleted rule must exit 0"
  exit 1
fi

echo "--- Step 12: The rule is not listed ---"
COUNT_AFTER=$("$DASH0" s2m list --all --origin-prefix "$ORIGIN" -o json | jq 'length')
if [ "$COUNT_AFTER" != "0" ]; then
  echo "FAIL: expected 0 records with origin '$ORIGIN' after delete, got $COUNT_AFTER"
  exit 1
fi

echo "=== Signal-to-metrics rule round-trip test PASSED ==="
