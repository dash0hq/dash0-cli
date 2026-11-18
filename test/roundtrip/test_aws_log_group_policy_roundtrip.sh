#!/usr/bin/env bash
set -euo pipefail

# Exercises the aws-log-group-policies commands and `apply` against a real AWS
# integration: update, get, re-apply of the export (no diff), and delete.
#
# Each integration has at most one policy, so the test borrows the integration
# named by DASH0_AWS_INTEGRATION_ID, snapshots its policy first, and restores it
# on exit. The fixture matches a tag value no log group carries, so the test
# never starts or stops forwarding, and every delete keeps forwarding.

export DASH0_AGENT_MODE=0

if [ -z "${DASH0_AWS_INTEGRATION_ID:-}" ]; then
  if [ -n "${CI:-}" ]; then
    echo "=== FAIL: DASH0_AWS_INTEGRATION_ID is not set in CI ==="
    exit 1
  fi
  echo "=== SKIP: DASH0_AWS_INTEGRATION_ID is not set ==="
  echo "Set it to the id of an AWS integration whose log-group policy this test may replace and restore."
  exit 0
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
ID="$DASH0_AWS_INTEGRATION_ID"
SNAPSHOT="${TMPDIR}/snapshot.yaml"
FIXTURE="${TMPDIR}/policy.yaml"
MARKER="never-matches-$(date +%s)-$$"

spec_of() {
  "$DASH0" -X aws-log-group-policies get "$ID" -o json | jq -S '.spec'
}

# Prints active, deleted, or none.
snapshot_state() {
  if [ ! -s "$SNAPSHOT" ]; then
    echo none
  elif [ "$(yq '.metadata.annotations["dash0.com/deleted-at"] == null' "$SNAPSHOT")" = "true" ]; then
    echo active
  else
    echo deleted
  fi
}

restore() {
  local status=$?
  echo "--- Restore: putting back the policy snapshotted before the test ---"
  if [ "$(snapshot_state)" = "active" ]; then
    "$DASH0" -X aws-log-group-policies update "$ID" -f "$SNAPSHOT" > /dev/null || status=1
    if [ "$(spec_of)" != "$(yq -o json '.spec' "$SNAPSHOT" | jq -S '.')" ]; then
      echo "FAIL: restored spec differs from the snapshot; the original is in $SNAPSHOT"
      exit 1
    fi
  else
    "$DASH0" -X aws-log-group-policies delete "$ID" --force > /dev/null 2>&1 || status=1
  fi
  echo "Restored."
  rm -rf "$TMPDIR"
  exit "$status"
}

echo "=== AWS log-group policy round-trip test ==="
echo "Integration: $ID"

echo "--- Step 0: Snapshot the current policy ---"
if ! "$DASH0" -X aws-log-group-policies get "$ID" -o yaml > "$SNAPSHOT" 2> "${TMPDIR}/get.err"; then
  if ! grep -q "not found" "${TMPDIR}/get.err"; then
    cat "${TMPDIR}/get.err"
    echo "FAIL: could not snapshot the policy of integration $ID"
    exit 1
  fi
  : > "$SNAPSHOT"
fi
echo "Snapshot state: $(snapshot_state)"
trap restore EXIT

sed -e "s|__ID__|${ID}|" -e "s|__MARKER__|${MARKER}|" "${FIXTURES}/aws-log-group-policy.yaml" > "$FIXTURE"

echo "--- Step 1: Replace the policy from the fixture ---"
if ! UPDATE_OUTPUT=$("$DASH0" -X aws-log-group-policies update -f "$FIXTURE" 2>&1); then
  echo "$UPDATE_OUTPUT"
  # The API refuses policies until the integration's CloudFormation stack
  # reports log-group support, which no CLI change can provide.
  if echo "$UPDATE_OUTPUT" | grep -q "has not reported log-group instrumentation support"; then
    trap - EXIT
    rm -rf "$TMPDIR"
    echo "=== SKIP: integration $ID has not reported log-group instrumentation support ==="
    echo "Point DASH0_AWS_INTEGRATION_ID at an integration whose stack enables log-group instrumentation to run this test."
    exit 0
  fi
  exit 1
fi
echo "$UPDATE_OUTPUT"
ACTUAL=$(spec_of | jq -r '.rules[0].conditions[0].value')
if [ "$ACTUAL" != "$MARKER" ]; then
  echo "FAIL: expected the first condition value $MARKER, got $ACTUAL"
  exit 1
fi

echo "--- Step 2: Get as a table ---"
GET_OUTPUT=$("$DASH0" -X aws-log-group-policies get "$ID")
echo "$GET_OUTPUT"
if ! echo "$GET_OUTPUT" | grep -q "dash0.cloud.tags.env is_one_of \[never-a, never-b\]"; then
  echo "FAIL: get output does not list the is_one_of condition"
  exit 1
fi

echo "--- Step 3: Re-apply the exported YAML with apply -f ---"
EXPORT="${TMPDIR}/exported.yaml"
"$DASH0" -X aws-log-group-policies get "$ID" -o yaml > "$EXPORT"
APPLY_OUTPUT=$("$DASH0" apply -f "$EXPORT" 2>&1)
echo "$APPLY_OUTPUT"
if ! echo "$APPLY_OUTPUT" | grep -q "no changes"; then
  echo "FAIL: re-applying the exported policy reported changes"
  exit 1
fi
if echo "$APPLY_OUTPUT" | grep -q "warning"; then
  echo "FAIL: re-applying the exported policy printed a warning"
  exit 1
fi

echo "--- Step 4: Delete the policy, keeping forwarding ---"
"$DASH0" -X aws-log-group-policies delete "$ID" --force
DISPOSITION=$("$DASH0" -X aws-log-group-policies get "$ID" -o json | jq -r '.metadata.annotations["dash0.com/deletion-disposition"]')
if [ "$DISPOSITION" != "keep" ]; then
  echo "FAIL: expected deletion disposition keep, got $DISPOSITION"
  exit 1
fi

echo "=== AWS log-group policy round-trip test passed ==="
