#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${DASH0_BIN:-${SCRIPT_DIR}/../../build/dash0}"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

if [ ! -x "$DASH0" ]; then
  echo "FAIL: build the CLI with make build, or set DASH0_BIN to a locally built binary"
  exit 1
fi

DATASET="${DASH0_TTR_DATASET:-}"
if [ -z "$DATASET" ]; then
  DATASET=$("$DASH0" config show -o json | jq -r '.dataset.value // "default"')
fi
if ! PROBE=$("$DASH0" telemetry-transformation-rules list --dataset "$DATASET" -o json 2>&1); then
  if echo "$PROBE" | grep -qE '\(status: 403[,)]'; then
    echo "=== SKIP: telemetry transformation rules require admin write permissions (403) ==="
    exit 77
  fi
  echo "FAIL: could not list telemetry transformation rules:"
  echo "$PROBE"
  exit 1
fi
if [ -z "$DATASET" ] || [ "$DATASET" = "default" ]; then
  echo "FAIL: set DASH0_TTR_DATASET to an existing non-default test dataset, or configure one in the active profile"
  exit 1
fi

ORIGIN="roundtrip-ttr-$(uuidgen | tr '[:upper:]' '[:lower:]')"
MISSING_ORIGIN="${ORIGIN}-missing"
YAML_FILE="${TMPDIR}/rule.yaml"
ORIGIN="$ORIGIN" yq '
  .metadata.name = env(ORIGIN) |
  .metadata.labels."dash0.com/origin" = env(ORIGIN) |
  .spec.display.name = env(ORIGIN)
' "${FIXTURES}/telemetry-transformation-rule.yaml" > "$YAML_FILE"

cleanup() {
  "$DASH0" telemetry-transformation-rules delete "$ORIGIN" --dataset "$DATASET" --force > /dev/null 2>&1 || true
  "$DASH0" telemetry-transformation-rules delete "$MISSING_ORIGIN" --dataset "$DATASET" --force > /dev/null 2>&1 || true
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

expect_status() {
  local status="$1"
  shift
  local out
  if out=$("$@" 2>&1); then
    echo "FAIL: expected HTTP $status, but the command succeeded: $out"
    exit 1
  fi
  echo "$out"
  if ! echo "$out" | grep -qE "\\(status: ${status}[,)]"; then
    echo "FAIL: expected HTTP $status"
    exit 1
  fi
}

spec_of() {
  jq -S '.spec | if (.examples | length) == 0 then del(.examples) else . end |
    if (.conditions | length) == 0 then del(.conditions) else . end'
}

assert_spec() {
  local expected actual
  expected=$(yq -o json '.' "$1" | spec_of)
  actual=$("$DASH0" telemetry-transformation-rules get "$ORIGIN" --dataset "$DATASET" -o json | spec_of)
  if [ "$actual" != "$expected" ]; then
    echo "FAIL: stored spec differs from $1"
    diff -u <(echo "$expected") <(echo "$actual") || true
    exit 1
  fi
}

echo "=== Telemetry transformation rule round-trip ==="
echo "Origin: $ORIGIN"
echo "Dataset: $DATASET"

echo "--- Step 1: Create without apiVersion ---"
CREATE_OUT=$("$DASH0" telemetry-transformation-rules create -f "$YAML_FILE" --dataset "$DATASET")
echo "$CREATE_OUT"
if ! echo "$CREATE_OUT" | grep -qE 'created \(id: [^)]+\)[[:space:]]*$'; then
  echo "FAIL: expected 'created' in create output"
  exit 1
fi
assert_spec "$YAML_FILE"

echo "--- Step 2: Get by origin includes examples and server metadata ---"
"$DASH0" telemetry-transformation-rules get "$ORIGIN" --dataset "$DATASET"
GET_JSON=$("$DASH0" telemetry-transformation-rules get "$ORIGIN" --dataset "$DATASET" -o json)
ID=$(echo "$GET_JSON" | jq -er '.metadata.labels["dash0.com/id"] | select(length > 0)')
echo "$GET_JSON" | jq -e --arg origin "$ORIGIN" --arg dataset "$DATASET" '
  .kind == "Dash0TelemetryTransformationRule" and
  .metadata.labels["dash0.com/origin"] == $origin and
  .metadata.labels["dash0.com/dataset"] == $dataset and
  (.metadata.annotations["dash0.com/updated-at"] | length) > 0 and
  (.spec.examples | length) == 1
' > /dev/null

echo "--- Step 3: Get by ID resolves the same rule ---"
BY_ID=$("$DASH0" telemetry-transformation-rules get "$ID" --dataset "$DATASET" -o json)
if [ "$(echo "$BY_ID" | jq -S '.')" != "$(echo "$GET_JSON" | jq -S '.')" ]; then
  echo "FAIL: get by ID differs from get by origin"
  exit 1
fi

echo "--- Step 4: List contains exactly one rule, omits examples, and respects dataset ---"
LIST_JSON=$("$DASH0" telemetry-transformation-rules list --all --dataset "$DATASET" -o json)
echo "$LIST_JSON" | jq -e --arg origin "$ORIGIN" '
  [.[] | select(.metadata.labels["dash0.com/origin"] == $origin)] |
  length == 1 and (.[0].spec | has("examples") | not)
' > /dev/null
DEFAULT_JSON=$("$DASH0" telemetry-transformation-rules list --all --dataset default -o json)
echo "$DEFAULT_JSON" | jq -e --arg origin "$ORIGIN" '
  [.[] | select(.metadata.labels["dash0.com/origin"] == $origin)] | length == 0
' > /dev/null

echo "--- Step 5: Create with the same origin conflicts instead of upserting ---"
CONFLICT_FILE="${TMPDIR}/conflict.yaml"
yq '.spec.priority = 400' "$YAML_FILE" > "$CONFLICT_FILE"
expect_status 409 "$DASH0" telemetry-transformation-rules create -f "$CONFLICT_FILE" --dataset "$DATASET"
assert_spec "$YAML_FILE"

echo "--- Step 6: Update of an unknown origin returns 404 ---"
MISSING_FILE="${TMPDIR}/missing.yaml"
ORIGIN="$MISSING_ORIGIN" yq '
  .metadata.name = env(ORIGIN) |
  .metadata.labels."dash0.com/origin" = env(ORIGIN)
' "$YAML_FILE" > "$MISSING_FILE"
expect_status 404 "$DASH0" telemetry-transformation-rules update -f "$MISSING_FILE" --dataset "$DATASET"
expect_status 404 "$DASH0" telemetry-transformation-rules get "$MISSING_ORIGIN" --dataset "$DATASET" -o json

echo "--- Step 7: Fresh export, edit, and full replacement update ---"
FRESH_FILE="${TMPDIR}/fresh.yaml"
CHANGED_FILE="${TMPDIR}/changed.yaml"
"$DASH0" telemetry-transformation-rules get "$ORIGIN" --dataset "$DATASET" -o yaml > "$FRESH_FILE"
OLD_TOKEN=$(yq '.metadata.annotations."dash0.com/updated-at"' "$FRESH_FILE")
yq '.spec.priority = 300 | del(.spec.conditions) |
  .spec.transformations = [.spec.transformations[0]]' "$FRESH_FILE" > "$CHANGED_FILE"
UPDATE_OUT=$("$DASH0" telemetry-transformation-rules update "$ID" -f "$CHANGED_FILE" --dataset "$DATASET")
echo "$UPDATE_OUT"
if ! echo "$UPDATE_OUT" | grep -qE '^[-+]'; then
  echo "FAIL: expected a diff for the edited rule"
  exit 1
fi
assert_spec "$CHANGED_FILE"
NEW_TOKEN=$("$DASH0" telemetry-transformation-rules get "$ORIGIN" --dataset "$DATASET" -o json | jq -er '.metadata.annotations["dash0.com/updated-at"]')
if [ "$NEW_TOKEN" = "$OLD_TOKEN" ]; then
  echo "FAIL: updated-at did not change after update"
  exit 1
fi

echo "--- Step 8: A stale updated-at returns 400 without changing the rule ---"
STALE_FILE="${TMPDIR}/stale.yaml"
yq '.spec.priority = 400' "$FRESH_FILE" > "$STALE_FILE"
expect_status 400 "$DASH0" telemetry-transformation-rules update -f "$STALE_FILE" --dataset "$DATASET"
assert_spec "$CHANGED_FILE"

echo "--- Step 9: An example-only update persists nested OTLP ---"
"$DASH0" telemetry-transformation-rules get "$ORIGIN" --dataset "$DATASET" -o yaml > "$FRESH_FILE"
EXAMPLE_FILE="${TMPDIR}/example.yaml"
yq '.spec.examples[0].name = "Edited synthetic example" |
  .spec.examples[0].otlp.resourceLogs[0].scopeLogs[0].logRecords[0].body.stringValue = "Edited synthetic log"' \
  "$FRESH_FILE" > "$EXAMPLE_FILE"
EXAMPLE_OUT=$("$DASH0" telemetry-transformation-rules update -f "$EXAMPLE_FILE" --dataset "$DATASET")
echo "$EXAMPLE_OUT"
if ! echo "$EXAMPLE_OUT" | grep -qE '^[-+]'; then
  echo "FAIL: expected a diff for an example-only update"
  exit 1
fi
assert_spec "$EXAMPLE_FILE"

echo "--- Step 10: Deliberately omitting examples removes them ---"
"$DASH0" telemetry-transformation-rules get "$ORIGIN" --dataset "$DATASET" -o yaml > "$FRESH_FILE"
NO_EXAMPLES_FILE="${TMPDIR}/no-examples.yaml"
yq 'del(.spec.examples)' "$FRESH_FILE" > "$NO_EXAMPLES_FILE"
"$DASH0" telemetry-transformation-rules update -f "$NO_EXAMPLES_FILE" --dataset "$DATASET"
assert_spec "$NO_EXAMPLES_FILE"

echo "--- Step 11: Fresh export, identical update, and apply are no-ops ---"
"$DASH0" telemetry-transformation-rules get "$ORIGIN" --dataset "$DATASET" -o yaml > "$FRESH_FILE"
NOOP_OUT=$("$DASH0" telemetry-transformation-rules update -f "$FRESH_FILE" --dataset "$DATASET")
echo "$NOOP_OUT"
if ! echo "$NOOP_OUT" | grep -q 'no changes'; then
  echo "FAIL: expected no changes for an identical update"
  exit 1
fi
"$DASH0" telemetry-transformation-rules get "$ORIGIN" --dataset "$DATASET" -o yaml > "$FRESH_FILE"
REAPPLY_OUT=$("$DASH0" apply -f "$FRESH_FILE" --dataset "$DATASET")
echo "$REAPPLY_OUT"
if ! echo "$REAPPLY_OUT" | grep -q 'no changes'; then
  echo "FAIL: reapplying an exported rule must be a no-op"
  exit 1
fi

echo "--- Step 12: Delete and forced repeat delete ---"
"$DASH0" telemetry-transformation-rules delete "$ID" --dataset "$DATASET" --force
expect_status 404 "$DASH0" telemetry-transformation-rules get "$ORIGIN" --dataset "$DATASET" -o json
"$DASH0" telemetry-transformation-rules delete "$ID" --dataset "$DATASET" --force
AFTER_JSON=$("$DASH0" telemetry-transformation-rules list --all --dataset "$DATASET" -o json)
echo "$AFTER_JSON" | jq -e --arg origin "$ORIGIN" '
  [.[] | select(.metadata.labels["dash0.com/origin"] == $origin)] | length == 0
' > /dev/null

echo "=== Telemetry transformation rule round-trip test PASSED ==="
