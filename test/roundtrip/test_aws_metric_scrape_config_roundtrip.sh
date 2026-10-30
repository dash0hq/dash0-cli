#!/usr/bin/env bash
set -euo pipefail

# Exercises the aws-metric-scrape-configs commands and `apply` against a real
# AWS integration: update, get, re-apply of the export (no diff), and delete.
#
# Each integration has exactly one scrape config, so the test cannot create its
# own. It borrows the integration named by DASH0_AWS_INTEGRATION_ID, snapshots
# its config first, and restores it on exit. A failed restore fails the test.
# The restore puts back the spec only: the CLI never sends dash0.com/origin, so
# a config the test had to clear gets a new origin from the server.

export DASH0_AGENT_MODE=0

if [ -z "${DASH0_AWS_INTEGRATION_ID:-}" ]; then
  # CI sets the secret, so a skip there means the test silently stopped running.
  if [ -n "${CI:-}" ]; then
    echo "=== FAIL: DASH0_AWS_INTEGRATION_ID is not set in CI ==="
    exit 1
  fi
  echo "=== SKIP: DASH0_AWS_INTEGRATION_ID is not set ==="
  echo "Set it to the id of an AWS integration whose scrape config this test may replace and restore."
  exit 0
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
TMPDIR="$(mktemp -d)"
ID="$DASH0_AWS_INTEGRATION_ID"
SNAPSHOT="${TMPDIR}/snapshot.yaml"
FIXTURE="${TMPDIR}/scrape-config.yaml"

spec_of() {
  "$DASH0" -X aws-metric-scrape-configs get "$ID" -o json | jq -S '.spec'
}

restore() {
  local status=$?
  echo "--- Restore: putting back the config snapshotted before the test ---"
  if [ "$(yq '.spec.namespaces | length' "$SNAPSHOT")" = "0" ] && [ "$(yq '.metadata.annotations == null' "$SNAPSHOT")" = "true" ]; then
    "$DASH0" -X aws-metric-scrape-configs delete "$ID" --force > /dev/null || status=1
  else
    "$DASH0" -X aws-metric-scrape-configs update "$ID" -f "$SNAPSHOT" > /dev/null || status=1
  fi
  if [ "$(spec_of)" != "$(yq -o json '.spec' "$SNAPSHOT" | jq -S '.')" ]; then
    echo "FAIL: restored spec differs from the snapshot; the original is in $SNAPSHOT"
    exit 1
  fi
  echo "Restored."
  rm -rf "$TMPDIR"
  exit "$status"
}

echo "=== AWS metric scrape config round-trip test ==="
echo "Integration: $ID"

echo "--- Step 0: Snapshot the current config ---"
if ! "$DASH0" -X aws-metric-scrape-configs get "$ID" -o yaml > "$SNAPSHOT"; then
  echo "FAIL: could not snapshot the config of integration $ID"
  exit 1
fi
trap restore EXIT

sed -e "s|__ID__|${ID}|" "${FIXTURES}/aws-metric-scrape-config.yaml" > "$FIXTURE"

echo "--- Step 1: Replace the config from the fixture ---"
"$DASH0" -X aws-metric-scrape-configs update -f "$FIXTURE"
EXPECTED='["AWS/Lambda","AWS/SQS"]'
ACTUAL=$(spec_of | jq -c '[.namespaces[].namespace]')
if [ "$ACTUAL" != "$EXPECTED" ]; then
  echo "FAIL: expected namespaces $EXPECTED, got $ACTUAL"
  exit 1
fi

echo "--- Step 2: Get as a table ---"
GET_OUTPUT=$("$DASH0" -X aws-metric-scrape-configs get "$ID")
echo "$GET_OUTPUT"
if ! echo "$GET_OUTPUT" | grep -q "AWS/SQS (2 metrics)"; then
  echo "FAIL: get output does not list AWS/SQS with 2 metrics"
  exit 1
fi

echo "--- Step 3: Re-apply the exported YAML with apply -f ---"
EXPORT="${TMPDIR}/exported.yaml"
"$DASH0" -X aws-metric-scrape-configs get "$ID" -o yaml > "$EXPORT"
APPLY_OUTPUT=$("$DASH0" apply -f "$EXPORT" 2>&1)
echo "$APPLY_OUTPUT"
if ! echo "$APPLY_OUTPUT" | grep -q "no changes"; then
  echo "FAIL: re-applying the exported config reported changes"
  exit 1
fi
if echo "$APPLY_OUTPUT" | grep -q "warning"; then
  echo "FAIL: re-applying the exported config printed a warning"
  exit 1
fi

echo "--- Step 4: Clear the config ---"
"$DASH0" -X aws-metric-scrape-configs delete "$ID" --force
if [ "$(spec_of | jq -c '.namespaces')" != "[]" ]; then
  echo "FAIL: config still has namespaces after delete"
  exit 1
fi

echo "=== AWS metric scrape config round-trip test passed ==="
