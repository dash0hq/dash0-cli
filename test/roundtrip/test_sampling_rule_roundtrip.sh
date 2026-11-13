#!/usr/bin/env bash
set -euo pipefail

# Exercises the full CRUD cycle for sampling rules against a real Dash0
# environment: create, get, list, update, export/reapply, delete.
#
# The fixture's origin is replaced with a unique one on every run, because
# origins are unique per organization and the API upserts by PUT. A fixed
# origin would overwrite a real rule of the same name. The rule is disabled,
# so it never turns sampling on for the dataset. Every write requires the
# organization admin role.

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ORIGIN="roundtrip-sampling-$(uuidgen | tr '[:upper:]' '[:lower:]')"
YAML_FILE="${TMPDIR}/rule.yaml"

echo "=== Sampling rule round-trip ==="
echo "Origin: $ORIGIN"

ORIGIN="$ORIGIN" yq '
  .metadata.name = env(ORIGIN) |
  .metadata.labels."dash0.com/origin" = env(ORIGIN) |
  .spec.display.name = env(ORIGIN)
' "${FIXTURES}/sampling-rule.yaml" > "$YAML_FILE"

cleanup() {
  "$DASH0" sampling-rules delete "$ORIGIN" --force > /dev/null 2>&1 || true
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

echo "--- Step 1: A document without spec.enabled fails before any API call ---"
NO_ENABLED_FILE="${TMPDIR}/rule-no-enabled.yaml"
yq 'del(.spec.enabled)' "$YAML_FILE" > "$NO_ENABLED_FILE"
if NO_ENABLED_OUT=$("$DASH0" sampling-rules create -f "$NO_ENABLED_FILE" 2>&1); then
  echo "FAIL: expected create to reject a document without spec.enabled, got:"
  echo "$NO_ENABLED_OUT"
  exit 1
fi
echo "$NO_ENABLED_OUT"
if "$DASH0" sampling-rules get "$ORIGIN" > /dev/null 2>&1; then
  echo "FAIL: a rejected document must not create the rule"
  exit 1
fi

echo "--- Step 2: Create ---"
CREATE_OUT=$("$DASH0" sampling-rules create -f "$YAML_FILE")
echo "$CREATE_OUT"
if ! echo "$CREATE_OUT" | grep -q "created"; then
  echo "FAIL: expected 'created' in create output"
  exit 1
fi

echo "--- Step 3: Get by origin ---"
GET_OUT=$("$DASH0" sampling-rules get "$ORIGIN")
echo "$GET_OUT"
if ! echo "$GET_OUT" | grep -q "Origin: ${ORIGIN}"; then
  echo "FAIL: get did not return the expected origin"
  exit 1
fi
if ! echo "$GET_OUT" | grep -q "Enabled: false"; then
  echo "FAIL: get did not show the rule as disabled"
  exit 1
fi
if ! echo "$GET_OUT" | grep -q "Conditions: and(ottl, error)"; then
  echo "FAIL: get did not summarize the conditions"
  exit 1
fi
if ! echo "$GET_OUT" | grep -q "Rate limit: 100 traces per minute"; then
  echo "FAIL: get did not show the rate limit"
  exit 1
fi

ID=$("$DASH0" sampling-rules get "$ORIGIN" -o json | jq -r '.metadata.labels["dash0.com/id"]')
if [ -z "$ID" ] || [ "$ID" = "null" ]; then
  echo "FAIL: could not resolve the server-assigned ID"
  exit 1
fi
echo "Server-assigned ID: $ID"

echo "--- Step 4: Get by ID resolves the same rule ---"
BY_ID_ORIGIN=$("$DASH0" sampling-rules get "$ID" -o json | jq -r '.metadata.labels["dash0.com/origin"]')
if [ "$BY_ID_ORIGIN" != "$ORIGIN" ]; then
  echo "FAIL: get by ID returned origin '$BY_ID_ORIGIN', expected '$ORIGIN'"
  exit 1
fi

echo "--- Step 5: List contains exactly this rule ---"
COUNT=$("$DASH0" sampling-rules list --all -o json | jq --arg o "$ORIGIN" '[.[] | select(.metadata.labels["dash0.com/origin"] == $o)] | length')
if [ "$COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 record with origin '$ORIGIN', got $COUNT"
  exit 1
fi

echo "--- Step 6: Update changes the rate limit and shows a diff ---"
CHANGED_FILE="${TMPDIR}/rule-changed.yaml"
yq '.spec.rateLimit.rate = 50' "$YAML_FILE" > "$CHANGED_FILE"
UPDATE_OUT=$("$DASH0" sampling-rules update -f "$CHANGED_FILE")
echo "$UPDATE_OUT"
if ! echo "$UPDATE_OUT" | grep -q -- "-    rate: 100"; then
  echo "FAIL: diff did not show the old rate limit"
  exit 1
fi
if ! echo "$UPDATE_OUT" | grep -q -- "+    rate: 50"; then
  echo "FAIL: diff did not show the new rate limit"
  exit 1
fi

echo "--- Step 7: Re-updating the same content reports no changes ---"
NOOP_OUT=$("$DASH0" sampling-rules update -f "$CHANGED_FILE")
echo "$NOOP_OUT"
if ! echo "$NOOP_OUT" | grep -q "no changes"; then
  echo "FAIL: expected 'no changes' when re-updating identical content"
  exit 1
fi

echo "--- Step 8: Export and reapply round-trips cleanly ---"
EXPORT_FILE="${TMPDIR}/rule-export.yaml"
"$DASH0" sampling-rules get "$ORIGIN" -o yaml > "$EXPORT_FILE"
REAPPLY_OUT=$("$DASH0" apply -f "$EXPORT_FILE")
echo "$REAPPLY_OUT"
if ! echo "$REAPPLY_OUT" | grep -q "no changes"; then
  echo "FAIL: reapplying an exported definition must be a no-op, got:"
  echo "$REAPPLY_OUT"
  exit 1
fi

echo "--- Step 9: Delete ---"
DELETE_OUT=$("$DASH0" sampling-rules delete "$ORIGIN" --force)
echo "$DELETE_OUT"
if ! echo "$DELETE_OUT" | grep -q "deleted"; then
  echo "FAIL: expected 'deleted' in delete output"
  exit 1
fi

echo "--- Step 10: Deleting again with --force succeeds ---"
if ! "$DASH0" sampling-rules delete "$ORIGIN" --force; then
  echo "FAIL: delete --force of an already-deleted rule must exit 0"
  exit 1
fi

echo "--- Step 11: The delete is permanent, so get fails ---"
if GET_AFTER=$("$DASH0" sampling-rules get "$ORIGIN" 2>&1); then
  echo "FAIL: expected get to fail after delete, got:"
  echo "$GET_AFTER"
  exit 1
fi
COUNT_AFTER=$("$DASH0" sampling-rules list --all -o json | jq --arg o "$ORIGIN" '[.[] | select(.metadata.labels["dash0.com/origin"] == $o)] | length')
if [ "$COUNT_AFTER" != "0" ]; then
  echo "FAIL: expected 0 records with origin '$ORIGIN' after delete, got $COUNT_AFTER"
  exit 1
fi

echo "=== Sampling rule round-trip test PASSED ==="
