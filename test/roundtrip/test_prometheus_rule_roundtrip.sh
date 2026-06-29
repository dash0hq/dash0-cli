#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
FIXTURE="${FIXTURES}/prometheus-rule.yaml"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

ALERT_NAME=$(yq '.spec.groups[0].rules[0].alert' "$FIXTURE")
GROUP_NAME=$(yq '.spec.groups[0].name' "$FIXTURE")
# The CLI names check rules imported from a PrometheusRule CRD as
# "<group name> - <alert name>", matching the operator and Terraform provider.
EXPECTED_NAME="${GROUP_NAME} - ${ALERT_NAME}"

echo "=== PrometheusRule CRD round-trip test ==="
echo "Expected check rule name: $EXPECTED_NAME"

# Step 1: Create via apply (PrometheusRule CRD).
echo "--- Step 1: Apply PrometheusRule CRD ---"
APPLY_OUTPUT=$("$DASH0" apply -f "$FIXTURE")
echo "$APPLY_OUTPUT"
if ! echo "$APPLY_OUTPUT" | grep -qF "$EXPECTED_NAME"; then
  echo "FAIL: apply output does not mention check rule name '$EXPECTED_NAME'"
  exit 1
fi

# Step 2: List check rules and find the created rule by name.
echo "--- Step 2: List check rules and find created rule ---"
LIST_JSON=$("$DASH0" check-rules list --all -o json)
ID=$(echo "$LIST_JSON" | jq -r --arg name "$EXPECTED_NAME" '[.[] | select(.name == $name)][0].id // empty')
if [ -z "$ID" ]; then
  echo "FAIL: Could not find created check rule '$EXPECTED_NAME' in list"
  exit 1
fi
echo "Created check rule ID: $ID"

# Step 3: Get by ID and verify key fields survived the conversion.
echo "--- Step 3: Verify converted check rule fields ---"
GET_JSON=$("$DASH0" check-rules get "$ID" -o json)
echo "$GET_JSON"

ACTUAL_NAME=$(echo "$GET_JSON" | jq -r '.name')
if [ "$ACTUAL_NAME" != "$EXPECTED_NAME" ]; then
  echo "FAIL: expected name '$EXPECTED_NAME', got '$ACTUAL_NAME'"
  exit 1
fi

ACTUAL_EXPR=$(echo "$GET_JSON" | jq -r '.expression')
if [ -z "$ACTUAL_EXPR" ]; then
  echo "FAIL: expression is empty"
  exit 1
fi
echo "expression: $ACTUAL_EXPR"

ACTUAL_SUMMARY=$(echo "$GET_JSON" | jq -r '.annotations.summary // empty')
if [ -z "$ACTUAL_SUMMARY" ]; then
  echo "FAIL: annotations.summary is empty (should be preserved from PrometheusRule annotations)"
  exit 1
fi
echo "annotations.summary: $ACTUAL_SUMMARY"

# Step 4: Export to YAML and re-import via apply (round-trip).
echo "--- Step 4: Export and re-apply (round-trip) ---"
"$DASH0" check-rules get "$ID" -o yaml > "${TMPDIR}/exported.yaml"
REAPPLY_OUTPUT=$("$DASH0" apply -f "${TMPDIR}/exported.yaml")
echo "$REAPPLY_OUTPUT"

# Step 5: Update via check-rules update with the CRD file (ID from argument).
echo "--- Step 5: Update via check-rules update with CRD file (ID from arg) ---"
UPDATE_OUTPUT=$("$DASH0" check-rules update "$ID" -f "$FIXTURE")
echo "$UPDATE_OUTPUT"

# Verify the CRD conversion was applied correctly (not silently corrupted).
echo "--- Step 5a: Verify check rule content after update ---"
POST_UPDATE_JSON=$("$DASH0" check-rules get "$ID" -o json)
POST_UPDATE_NAME=$(echo "$POST_UPDATE_JSON" | jq -r '.name')
if [ "$POST_UPDATE_NAME" != "$EXPECTED_NAME" ]; then
  echo "FAIL: after update, expected name '$EXPECTED_NAME', got '$POST_UPDATE_NAME'"
  echo "      (this indicates the PrometheusRule CRD was not converted before sending to the API)"
  exit 1
fi
POST_UPDATE_EXPR=$(echo "$POST_UPDATE_JSON" | jq -r '.expression')
if [ -z "$POST_UPDATE_EXPR" ]; then
  echo "FAIL: after update, expression is empty"
  exit 1
fi
echo "Name after update: $POST_UPDATE_NAME"
echo "Expression after update: $POST_UPDATE_EXPR"

# Step 5b: Update using a CRD file with dash0.com/id (ID from file, no CLI argument).
echo "--- Step 5b: Update via check-rules update with CRD file (ID from file) ---"
yq ".metadata.labels.\"dash0.com/id\" = \"$ID\"" "$FIXTURE" > "${TMPDIR}/prom-rule-with-id.yaml"
UPDATE_OUTPUT2=$("$DASH0" check-rules update -f "${TMPDIR}/prom-rule-with-id.yaml")
echo "$UPDATE_OUTPUT2"

POST_UPDATE_NAME2=$("$DASH0" check-rules get "$ID" -o json | jq -r '.name')
if [ "$POST_UPDATE_NAME2" != "$EXPECTED_NAME" ]; then
  echo "FAIL: after update (ID from file), expected name '$EXPECTED_NAME', got '$POST_UPDATE_NAME2'"
  exit 1
fi
echo "Name after update (ID from file): $POST_UPDATE_NAME2"

# Step 6: Also test check-rules create with the CRD file.
echo "--- Step 6: Create via check-rules create (parity check) ---"
CREATE_OUTPUT=$("$DASH0" check-rules create -f "$FIXTURE")
echo "$CREATE_OUTPUT"
if ! echo "$CREATE_OUTPUT" | grep -qF "$EXPECTED_NAME"; then
  echo "FAIL: check-rules create output does not mention check rule name '$EXPECTED_NAME'"
  exit 1
fi

# Find the second copy and clean it up.
LIST_JSON2=$("$DASH0" check-rules list --all -o json)
ID2=$(echo "$LIST_JSON2" | jq -r --arg name "$EXPECTED_NAME" --arg id "$ID" '[.[] | select(.name == $name and .id != $id)][0].id // empty')

# Cleanup.
echo "--- Cleanup ---"
"$DASH0" check-rules delete "$ID" --force
if [ -n "$ID2" ]; then
  "$DASH0" check-rules delete "$ID2" --force
fi
echo "=== PrometheusRule CRD round-trip test PASSED ==="
