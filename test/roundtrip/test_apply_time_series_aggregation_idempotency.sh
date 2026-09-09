#!/usr/bin/env bash
set -euo pipefail

# Exercises `dash0 apply` for time series aggregations. Confirms that:
#   - apply creates the aggregation on the first run;
#   - a second apply of the same file reports "no changes" and creates no
#     duplicate — this is the assertion that would fail if marshalForDiff
#     stopped stripping dash0.com/version, which the server increments on
#     every PUT;
#   - a document without dash0.com/origin fails during validation, before any
#     API call;
#   - applying the same document to a second dataset fails with the
#     cross-dataset explanation rather than a bare 400.
#
# Asserting the second run's *output*, not only the record count, is what
# makes this test capable of failing when the diff logic regresses: a
# count-only check passes even when every reapply renders a spurious diff.
#
# The origin is unique per run. Origins are unique per organization and apply
# upserts by PUT, so a fixed origin would overwrite a real aggregation.

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

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

ORIGIN="apply-tsa-$(uuidgen | tr '[:upper:]' '[:lower:]')"
YAML_FILE="${TMPDIR}/aggregation.yaml"

echo "=== Time series aggregation apply idempotency ==="
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

echo "--- Step 1: First apply (expect: created) ---"
APPLY1=$("$DASH0" apply -f "$YAML_FILE")
echo "$APPLY1"
if ! echo "$APPLY1" | grep -q "created"; then
  echo "FAIL: expected 'created' in first apply output"
  exit 1
fi

echo "--- Step 2: Second apply (expect: no changes, no duplicate) ---"
if ! APPLY2=$("$DASH0" apply -f "$YAML_FILE" 2>&1); then
  echo "FAIL: second apply errored out (expected idempotent upsert):"
  echo "$APPLY2"
  exit 1
fi
echo "$APPLY2"
if echo "$APPLY2" | grep -q "created"; then
  echo "FAIL: unexpected 'created' on second apply — a duplicate was created"
  exit 1
fi
if ! echo "$APPLY2" | grep -q "no changes"; then
  echo "FAIL: expected 'no changes' on the second apply of identical content."
  echo "This usually means the diff no longer strips the server-managed labels"
  echo "(dash0.com/version is incremented on every PUT)."
  exit 1
fi

echo "--- Step 3: Exactly one record with this origin ---"
COUNT=$("$DASH0" tsa list --all -o json | jq --arg o "$ORIGIN" '[.[] | select(.metadata.labels["dash0.com/origin"] == $o)] | length')
if [ "$COUNT" != "1" ]; then
  echo "FAIL: expected exactly 1 record with origin '$ORIGIN', got $COUNT"
  exit 1
fi

echo "--- Step 4: A document without an origin fails validation ---"
NO_ORIGIN_FILE="${TMPDIR}/no-origin.yaml"
yq 'del(.metadata.labels)' "$YAML_FILE" > "$NO_ORIGIN_FILE"
if BAD_OUT=$("$DASH0" apply -f "$NO_ORIGIN_FILE" --dry-run 2>&1); then
  echo "FAIL: expected apply --dry-run to fail without an origin, got:"
  echo "$BAD_OUT"
  exit 1
fi
echo "$BAD_OUT"
if ! echo "$BAD_OUT" | grep -q "dash0.com/origin"; then
  echo "FAIL: error message did not name the missing origin label"
  exit 1
fi

echo "--- Step 5: The same document in another dataset is rejected ---"
# Origins are unique per organization while each aggregation belongs to one
# dataset, so this is the error a user hits the first time they point one
# asset directory at a second dataset.
OTHER_DATASET="${DASH0_TSA_OTHER_DATASET:-default}"
CURRENT_DATASET=$("$DASH0" config show -o json | jq -r '.dataset.value // "default"')
if [ "$OTHER_DATASET" = "$CURRENT_DATASET" ]; then
  echo "SKIP: the active dataset is already '$OTHER_DATASET'; set DASH0_TSA_OTHER_DATASET to a different one to exercise this step"
else
  if CROSS_OUT=$("$DASH0" apply -f "$YAML_FILE" --dataset "$OTHER_DATASET" 2>&1); then
    echo "FAIL: expected the cross-dataset apply to fail, got:"
    echo "$CROSS_OUT"
    exit 1
  fi
  echo "$CROSS_OUT"
  if ! echo "$CROSS_OUT" | grep -q "unique per organization"; then
    echo "FAIL: the cross-dataset error did not explain the organization-wide origin namespace"
    exit 1
  fi
fi

echo "--- Step 6: Delete ---"
if ! "$DASH0" tsa delete "$ORIGIN" --force > /dev/null; then
  echo "FAIL: cleanup delete failed"
  exit 1
fi

echo "=== Time series aggregation apply idempotency test PASSED ==="
