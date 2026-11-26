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

# One origin per run keeps repeated applies on this script's own rule.
ORIGIN="apply-ttr-$(uuidgen | tr '[:upper:]' '[:lower:]')"
YAML_FILE="${TMPDIR}/rule.yaml"
ORIGIN="$ORIGIN" yq '
  .metadata.name = env(ORIGIN) |
  .metadata.labels."dash0.com/origin" = env(ORIGIN) |
  .spec.display.name = env(ORIGIN)
' "${FIXTURES}/telemetry-transformation-rule.yaml" > "$YAML_FILE"

cleanup() {
  "$DASH0" telemetry-transformation-rules delete "$ORIGIN" --dataset "$DATASET" --force > /dev/null 2>&1 || true
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
  jq -S '.spec | if (.examples | length) == 0 then del(.examples) else . end'
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

assert_single_rule() {
  local rules
  rules=$("$DASH0" telemetry-transformation-rules list --all --dataset "$DATASET" -o json)
  echo "$rules" | jq -e --arg origin "$ORIGIN" --arg id "$ID" --arg dataset "$DATASET" '
    [.[] | select(.metadata.labels["dash0.com/origin"] == $origin)] |
    length == 1 and .[0].metadata.labels["dash0.com/id"] == $id and
    .[0].metadata.labels["dash0.com/dataset"] == $dataset and
    (.[0].spec | has("examples") | not)
  ' > /dev/null
}

apply_changed() {
  local out
  out=$("$DASH0" apply -f "$1" --dataset "$DATASET")
  echo "$out"
  if ! echo "$out" | grep -qE '^[-+]'; then
    echo "FAIL: expected a diff for changed configuration"
    exit 1
  fi
  assert_spec "$1"
}

apply_unchanged() {
  local out
  out=$("$DASH0" apply -f "$1" --dataset "$DATASET")
  echo "$out"
  if ! echo "$out" | grep -q 'no changes'; then
    echo "FAIL: expected no changes for identical configuration"
    exit 1
  fi
  if echo "$out" | grep -qE '(created|updated|restored)[[:space:]]*$'; then
    echo "FAIL: unchanged apply reported a write"
    exit 1
  fi
  assert_spec "$1"
  assert_single_rule
}

echo "=== Telemetry transformation rule apply idempotency ==="
echo "Origin: $ORIGIN"
echo "Dataset: $DATASET"

echo "--- Step 1: A genuinely missing origin is created by apply ---"
expect_status 404 "$DASH0" telemetry-transformation-rules get "$ORIGIN" --dataset "$DATASET" -o json
APPLY1=$("$DASH0" apply -f "$YAML_FILE" --dataset "$DATASET")
echo "$APPLY1"
if ! echo "$APPLY1" | grep -qE 'created[[:space:]]*$'; then
  echo "FAIL: expected created on the first apply"
  exit 1
fi
ID=$("$DASH0" telemetry-transformation-rules get "$ORIGIN" --dataset "$DATASET" -o json | jq -er '.metadata.labels["dash0.com/id"] | select(length > 0)')
assert_spec "$YAML_FILE"

echo "--- Step 2: Reapply the same origin without ID or timestamp ---"
apply_unchanged "$YAML_FILE"

echo "--- Step 3: Transformation order alone is a change ---"
ORDER_FILE="${TMPDIR}/transformation-order.yaml"
yq '.spec.transformations |= reverse' "$YAML_FILE" > "$ORDER_FILE"
apply_changed "$ORDER_FILE"
apply_unchanged "$ORDER_FILE"

echo "--- Step 4: Statement order alone is a change ---"
STATEMENTS_FILE="${TMPDIR}/statement-order.yaml"
yq '.spec.transformations[0].statements |= reverse' "$ORDER_FILE" > "$STATEMENTS_FILE"
apply_changed "$STATEMENTS_FILE"
apply_unchanged "$STATEMENTS_FILE"

echo "--- Step 5: Example-only changes are persisted and diffed ---"
EXAMPLE_FILE="${TMPDIR}/example.yaml"
yq '.spec.examples[0].name = "Edited synthetic example" |
  .spec.examples[0].otlp.resourceLogs[0].scopeLogs[0].logRecords[0].body.stringValue = "Edited synthetic log"' \
  "$STATEMENTS_FILE" > "$EXAMPLE_FILE"
apply_changed "$EXAMPLE_FILE"
apply_unchanged "$EXAMPLE_FILE"

echo "--- Step 6: Omitted examples are deliberately removed ---"
NO_EXAMPLES_FILE="${TMPDIR}/no-examples.yaml"
yq 'del(.spec.examples)' "$EXAMPLE_FILE" > "$NO_EXAMPLES_FILE"
apply_changed "$NO_EXAMPLES_FILE"
apply_unchanged "$NO_EXAMPLES_FILE"

echo "--- Step 7: Export and reapply has no server-metadata diff ---"
EXPORT_FILE="${TMPDIR}/export.yaml"
"$DASH0" telemetry-transformation-rules get "$ORIGIN" --dataset "$DATASET" -o yaml > "$EXPORT_FILE"
apply_unchanged "$EXPORT_FILE"

echo "--- Step 8: Restore the synthetic examples before testing deletion ---"
apply_changed "$EXAMPLE_FILE"
assert_single_rule

echo "--- Step 9: Delete hides the tombstone from get and list ---"
"$DASH0" telemetry-transformation-rules delete "$ORIGIN" --dataset "$DATASET" --force
expect_status 404 "$DASH0" telemetry-transformation-rules get "$ORIGIN" --dataset "$DATASET" -o json
expect_status 404 "$DASH0" telemetry-transformation-rules get "$ID" --dataset "$DATASET" -o json
AFTER_JSON=$("$DASH0" telemetry-transformation-rules list --all --dataset "$DATASET" -o json)
echo "$AFTER_JSON" | jq -e --arg origin "$ORIGIN" '
  [.[] | select(.metadata.labels["dash0.com/origin"] == $origin)] | length == 0
' > /dev/null

echo "--- Step 10: Create still conflicts with a hidden tombstone ---"
expect_status 409 "$DASH0" telemetry-transformation-rules create -f "$EXAMPLE_FILE" --dataset "$DATASET"
expect_status 404 "$DASH0" telemetry-transformation-rules get "$ORIGIN" --dataset "$DATASET" -o json

echo "--- Step 11: Apply recovers the same origin after POST 409 and GET 404 ---"
RECOVER_OUT=$("$DASH0" apply -f "$EXAMPLE_FILE" --dataset "$DATASET")
echo "$RECOVER_OUT"
if echo "$RECOVER_OUT" | grep -q 'no changes'; then
  echo "FAIL: recovering a deleted rule must not report no changes"
  exit 1
fi
RESTORED_JSON=$("$DASH0" telemetry-transformation-rules get "$ORIGIN" --dataset "$DATASET" -o json)
echo "$RESTORED_JSON" | jq -e --arg id "$ID" '
  .metadata.labels["dash0.com/id"] == $id and
  (.metadata.annotations["dash0.com/deleted-at"] // "") == ""
' > /dev/null
assert_spec "$EXAMPLE_FILE"
assert_single_rule
apply_unchanged "$EXAMPLE_FILE"

echo "--- Step 12: Delete and forced repeat delete ---"
"$DASH0" telemetry-transformation-rules delete "$ORIGIN" --dataset "$DATASET" --force
"$DASH0" telemetry-transformation-rules delete "$ORIGIN" --dataset "$DATASET" --force
expect_status 404 "$DASH0" telemetry-transformation-rules get "$ORIGIN" --dataset "$DATASET" -o json

echo "=== Telemetry transformation rule apply idempotency test PASSED ==="
