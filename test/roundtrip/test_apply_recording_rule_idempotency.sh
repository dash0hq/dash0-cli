#!/usr/bin/env bash
set -euo pipefail

# Exercises `dash0 apply` for PrometheusRule CRDs that contain recording rules.
# Confirms that:
#   - apply -f <record-only CRD> creates a recording rule (not a check rule);
#   - apply is idempotent on a second run when dash0.com/id is set;
#   - apply -f <mixed CRD> creates both a check rule and a recording rule;
#   - apply -f <empty CRD> fails validation before any API call.

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# Stage 1: Recording-only CRD — apply creates, then upserts.
# The recording-rules API assigns its own server-side ID; the user-provided
# `dash0.com/id` label is honored as the upsert lookup key, but the canonical
# stored ID is server-assigned (so the value echoed in the apply output differs
# from the label). The roundtrip identifies the stored record by metadata.name,
# which is preserved verbatim. The unique suffix on the name keeps this test
# isolated from any other recording rules in the dataset.
RECORDING_ID=$(uuidgen | tr '[:upper:]' '[:lower:]')
RECORDING_NAME="Apply Roundtrip Recording Rule $(uuidgen | tr '[:upper:]' '[:lower:]')"
RECORDING_YAML="${TMPDIR}/recording-rule.yaml"
cat > "$RECORDING_YAML" <<EOF
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: ${RECORDING_NAME}
  labels:
    dash0.com/id: ${RECORDING_ID}
spec:
  groups:
    - name: apply-roundtrip-group
      interval: 1m
      rules:
        - record: apply_roundtrip:cpu_usage:avg5m
          expr: avg without(cpu) (rate(node_cpu_seconds_total{mode!="idle"}[5m]))
EOF

echo "=== Stage 1: Recording-only PrometheusRule apply idempotency ==="
echo "Recording rule name: $RECORDING_NAME"

cleanup_recording() {
  local list
  list=$("$DASH0" recording-rules list --all -o json 2>/dev/null || echo "[]")
  local ids
  ids=$(echo "$list" | jq -r --arg n "$RECORDING_NAME" '.[] | select(.metadata.name == $n) | .metadata.labels["dash0.com/id"]')
  for id in $ids; do
    "$DASH0" recording-rules delete "$id" --force > /dev/null 2>&1 || true
  done
}
trap 'cleanup_recording; rm -rf "$TMPDIR"' EXIT

echo "--- Step 1.1: First apply (expect: Recording rule created) ---"
APPLY1=$("$DASH0" apply -f "$RECORDING_YAML")
echo "$APPLY1"
if ! echo "$APPLY1" | grep -q "Recording rule"; then
  echo "FAIL: expected 'Recording rule' in first apply output"
  exit 1
fi
if ! echo "$APPLY1" | grep -E -q '\) created$'; then
  echo "FAIL: expected '<...> created' line in first apply output"
  exit 1
fi
if echo "$APPLY1" | grep -q "Check rule"; then
  echo "FAIL: unexpected 'Check rule' in record-only apply output"
  exit 1
fi

echo "--- Step 1.2: Verify exactly one recording rule exists with this name ---"
COUNT=$("$DASH0" recording-rules list --all -o json | jq --arg n "$RECORDING_NAME" '[.[] | select(.metadata.name == $n)] | length')
if [ "$COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 recording rule with name '$RECORDING_NAME' after first apply, got $COUNT"
  exit 1
fi

echo "--- Step 1.3: Second apply (expect: no new 'created' line, still exactly one record) ---"
APPLY2=$("$DASH0" apply -f "$RECORDING_YAML")
echo "$APPLY2"
if echo "$APPLY2" | grep -E -q '\) created$'; then
  echo "FAIL: unexpected '<...> created' on second apply — duplicate was created"
  exit 1
fi
COUNT=$("$DASH0" recording-rules list --all -o json | jq --arg n "$RECORDING_NAME" '[.[] | select(.metadata.name == $n)] | length')
if [ "$COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 recording rule with name '$RECORDING_NAME' after second apply, got $COUNT"
  exit 1
fi

echo "--- Step 1.4: Cleanup ---"
cleanup_recording

# Stage 2: Mixed PrometheusRule CRD — apply hits both endpoints.
# The CRD's `dash0.com/id` label is honored as the check-rule ID directly (check
# rules accept user-defined IDs). The recording-rules API assigns its own
# server-side ID; the user-provided value is used only as an upsert lookup key.
# We therefore identify each side by what the API guarantees to preserve:
#   - check rule: by its `id` (= MIXED_ID, the alert rule's ID echoed from the
#     CRD's dash0.com/id label).
#   - recording rule: by its metadata.name (which is the CRD's metadata.name,
#     suffixed with a UUID to keep this test isolated).
MIXED_ID=$(uuidgen | tr '[:upper:]' '[:lower:]')
MIXED_NAME="Apply Roundtrip Mixed Rule $(uuidgen | tr '[:upper:]' '[:lower:]')"
MIXED_YAML="${TMPDIR}/mixed-rule.yaml"
cat > "$MIXED_YAML" <<EOF
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: ${MIXED_NAME}
  labels:
    dash0.com/id: ${MIXED_ID}
spec:
  groups:
    - name: apply-roundtrip-mixed
      interval: 1m
      rules:
        - alert: ApplyRoundtripHighErrors
          expr: sum(rate(errors[5m])) > 0.1
          for: 5m
          labels:
            severity: critical
          annotations:
            summary: Apply roundtrip mixed alert
        - record: apply_roundtrip_mixed:value
          expr: sum(rate(node_cpu_seconds_total[5m]))
EOF

cleanup_mixed() {
  "$DASH0" check-rules delete "$MIXED_ID" --force > /dev/null 2>&1 || true
  local list
  list=$("$DASH0" recording-rules list --all -o json 2>/dev/null || echo "[]")
  local ids
  ids=$(echo "$list" | jq -r --arg n "$MIXED_NAME" '.[] | select(.metadata.name == $n) | .metadata.labels["dash0.com/id"]')
  for id in $ids; do
    "$DASH0" recording-rules delete "$id" --force > /dev/null 2>&1 || true
  done
}
trap 'cleanup_recording; cleanup_mixed; rm -rf "$TMPDIR"' EXIT

echo "=== Stage 2: Mixed PrometheusRule apply idempotency ==="
echo "Check rule ID: $MIXED_ID"
echo "Recording rule name: $MIXED_NAME"

echo "--- Step 2.1: First apply (expect: Check rule + Recording rule created) ---"
APPLY3=$("$DASH0" apply -f "$MIXED_YAML")
echo "$APPLY3"
if ! echo "$APPLY3" | grep -q "Check rule"; then
  echo "FAIL: expected 'Check rule' in mixed apply output"
  exit 1
fi
if ! echo "$APPLY3" | grep -q "Recording rule"; then
  echo "FAIL: expected 'Recording rule' in mixed apply output"
  exit 1
fi
# Both halves of the mixed CRD must report 'created' on first apply.
CREATED_COUNT=$(echo "$APPLY3" | grep -c -E '\) created$' || true)
if [ "$CREATED_COUNT" -lt 2 ]; then
  echo "FAIL: expected 2 'created' lines (one per endpoint) on first mixed apply, got $CREATED_COUNT"
  exit 1
fi

echo "--- Step 2.2: Verify both assets exist ---"
if ! "$DASH0" check-rules get "$MIXED_ID" > /dev/null; then
  echo "FAIL: check-rules get '$MIXED_ID' failed after mixed apply"
  exit 1
fi
RECORDING_COUNT=$("$DASH0" recording-rules list --all -o json | jq --arg n "$MIXED_NAME" '[.[] | select(.metadata.name == $n)] | length')
if [ "$RECORDING_COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 recording rule with name '$MIXED_NAME' after first apply, got $RECORDING_COUNT"
  exit 1
fi

echo "--- Step 2.3: Second apply (expect: no duplicate created at either endpoint) ---"
APPLY4=$("$DASH0" apply -f "$MIXED_YAML")
echo "$APPLY4"
if echo "$APPLY4" | grep -E -q '\) created$'; then
  echo "FAIL: unexpected '<...> created' on second mixed apply — duplicate was created at one or both endpoints"
  exit 1
fi

echo "--- Step 2.4: Verify exactly one asset at each endpoint ---"
RECORDING_COUNT=$("$DASH0" recording-rules list --all -o json | jq --arg n "$MIXED_NAME" '[.[] | select(.metadata.name == $n)] | length')
if [ "$RECORDING_COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 recording rule with name '$MIXED_NAME' after second apply, got $RECORDING_COUNT"
  exit 1
fi
CHECK_COUNT=$("$DASH0" check-rules list --all -o json | jq --arg id "$MIXED_ID" '[.[] | select(.id == $id)] | length')
if [ "$CHECK_COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 check rule with ID '$MIXED_ID' after second apply, got $CHECK_COUNT"
  exit 1
fi

echo "--- Step 2.5: Cleanup ---"
cleanup_mixed

# Stage 3: Empty PrometheusRule CRD — apply must fail at validation.
echo "=== Stage 3: PrometheusRule with no rules fails validation ==="
EMPTY_YAML="${TMPDIR}/empty-rule.yaml"
cat > "$EMPTY_YAML" <<'EOF'
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: Empty Roundtrip Rule
spec:
  groups:
    - name: empty-group
      rules: []
EOF
if EMPTY_OUT=$("$DASH0" apply -f "$EMPTY_YAML" --dry-run 2>&1); then
  echo "FAIL: expected apply --dry-run to fail for empty PrometheusRule, got:"
  echo "$EMPTY_OUT"
  exit 1
fi
echo "$EMPTY_OUT"
if ! echo "$EMPTY_OUT" | grep -q "no alerting or recording rules"; then
  echo "FAIL: error message did not explain the empty PrometheusRule failure"
  exit 1
fi

echo "=== Recording rule apply idempotency test PASSED ==="
