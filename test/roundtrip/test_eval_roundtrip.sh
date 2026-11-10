#!/usr/bin/env bash
set -euo pipefail

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
SUFFIX="$(uuidgen | tr '[:upper:]' '[:lower:]' | cut -c1-8)"
NAME="cli-rt-${SUFFIX}"
ORIGIN="dash0-cli_rt-${SUFFIX}"
FIXTURE="${TMPDIR}/eval.yaml"
CREATED=0

cleanup() {
  if [ "$CREATED" = "1" ]; then
    echo "--- Cleanup: deleting eval $ORIGIN ---"
    "$DASH0" -X evals delete "$ORIGIN" --force > /dev/null 2>&1 || true
  fi
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

sed -e "s|__ORIGIN__|${ORIGIN}|" -e "s|__NAME__|${NAME}|g" -e "s|__RATE__|0.1|" "${FIXTURES}/eval.yaml" > "$FIXTURE"

count_by_origin() {
  "$DASH0" -X evals list --all -o json | jq --arg o "$ORIGIN" '[.[] | select(.metadata.labels["dash0.com/origin"] == $o)] | length'
}

echo "=== Eval round-trip test ==="
echo "Origin: $ORIGIN"

# Step 1: Create from fixture
echo "--- Step 1: Create eval from fixture ---"
if ! CREATE_OUTPUT=$("$DASH0" -X evals create -f "$FIXTURE"); then
  echo "FAIL: evals create failed"
  exit 1
fi
CREATED=1
echo "$CREATE_OUTPUT"
if ! echo "$CREATE_OUTPUT" | grep -q "\"$NAME\" created"; then
  echo "FAIL: create output does not report eval '$NAME' as created"
  exit 1
fi

# Step 2: List and find the created eval
echo "--- Step 2: List evals and find created eval ---"
if [ "$(count_by_origin)" != "1" ]; then
  echo "FAIL: Could not find created eval '$ORIGIN' in list"
  exit 1
fi

# Step 3: Get by origin
echo "--- Step 3: Get eval by origin ---"
GET_JSON=$("$DASH0" -X evals get "$ORIGIN" -o json)
KIND=$(echo "$GET_JSON" | jq -r '.spec.judge.output.kind')
if [ "$KIND" != "boolean" ]; then
  echo "FAIL: expected judge output kind 'boolean', got '$KIND'"
  exit 1
fi

# Step 4: Export, edit, and update
echo "--- Step 4: Export, edit, and update ---"
EXPORT_FILE="${TMPDIR}/exported.yaml"
"$DASH0" -X evals get "$ORIGIN" -o yaml > "$EXPORT_FILE"
yq -i '.spec.sample.rate = 0.2' "$EXPORT_FILE"
if ! UPDATE_OUTPUT=$("$DASH0" -X evals update -f "$EXPORT_FILE"); then
  echo "FAIL: evals update failed"
  exit 1
fi
echo "$UPDATE_OUTPUT"
RATE=$("$DASH0" -X evals get "$ORIGIN" -o json | jq -r '.spec.sample.rate')
if [ "$RATE" != "0.2" ]; then
  echo "FAIL: expected sample rate 0.2 after update, got '$RATE'"
  exit 1
fi

# Step 5: Re-applying the export is a no-op
echo "--- Step 5: Apply the export (expect: no changes) ---"
APPLY_OUTPUT=$("$DASH0" -X apply -f "$EXPORT_FILE")
echo "$APPLY_OUTPUT"
if ! echo "$APPLY_OUTPUT" | grep -q 'no changes'; then
  echo "FAIL: applying the unchanged export did not report 'no changes'"
  exit 1
fi

# Step 6: Changing the judge output kind is rejected with an explanation
echo "--- Step 6: Change judge output kind (expect: explained error) ---"
yq -i '.spec.judge.output = {"kind": "numeric", "spec": {"threshold": 0.5}}' "$EXPORT_FILE"
if KIND_OUTPUT=$("$DASH0" -X evals update -f "$EXPORT_FILE" 2>&1); then
  echo "FAIL: changing the judge output kind was accepted"
  exit 1
fi
if ! echo "$KIND_OUTPUT" | grep -q 'spec.judge.output.kind cannot change'; then
  echo "FAIL: the output kind error is not explained: $KIND_OUTPUT"
  exit 1
fi

# Step 7: Delete
echo "--- Step 7: Delete eval ---"
if ! "$DASH0" -X evals delete "$ORIGIN" --force; then
  echo "FAIL: evals delete failed"
  exit 1
fi

# Step 8: Verify deletion: the list leaves it out, get shows it as deleted
echo "--- Step 8: Verify deletion ---"
if [ "$(count_by_origin)" != "0" ]; then
  echo "FAIL: eval '$ORIGIN' still listed after deletion"
  exit 1
fi
if ! "$DASH0" -X evals get "$ORIGIN" | grep -q '^Deleted:'; then
  echo "FAIL: get on the deleted eval does not show it as deleted"
  exit 1
fi

# Step 9: Deleting again with --force succeeds (idempotent)
echo "--- Step 9: Delete again with --force (expect: success) ---"
if ! "$DASH0" -X evals delete "$ORIGIN" --force; then
  echo "FAIL: second evals delete --force failed"
  exit 1
fi
CREATED=0

echo "=== Eval round-trip test PASSED ==="
