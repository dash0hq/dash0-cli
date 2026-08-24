#!/usr/bin/env bash
set -euo pipefail

# Regression test for https://github.com/dash0hq/dash0-cli/issues/261:
# concurrent `dash0` CLI invocations creating/updating dataset-scoped assets
# in the same dataset used to race the dataset's optimistic-concurrency
# "dataset version" and fail with an unretried 409 for all but one writer.
# The fix retries a 409 dataset version conflict with backoff instead of
# surfacing it immediately (see internal/client/retry_conflict.go).
#
# Unlike the Terraform provider's version of this bug, the dash0 CLI has no
# intra-process concurrency in its asset-write paths — `apply` applies
# documents from a single sequential loop — so this test reproduces the race
# the way it actually occurs for the CLI: multiple separate `dash0` process
# invocations writing the same dataset at once (e.g. a CI matrix, or a
# script/agent running several `create`/`apply` commands concurrently).
#
# Steps:
#   1. Launch FILTER_COUNT concurrent `dash0 spam-filters create` processes
#      against the same dataset, each with a unique origin.
#   2. Assert every process exited 0 — before the fix, all but (usually)
#      one exited non-zero with an unretried 409.
#   3. Verify all FILTER_COUNT filters exist via the CLI.
#   4. Clean up by deleting each one.

export DASH0_AGENT_MODE=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASH0="${SCRIPT_DIR}/../../build/dash0"
FIXTURES="${SCRIPT_DIR}/fixtures"
FIXTURE="${FIXTURES}/spam-filter.yaml"
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# Comfortably enough concurrent writers to make the race the common case
# rather than a lucky non-collision.
FILTER_COUNT=8

echo "=== Spam filter concurrent create test ==="
echo "Filter count: ${FILTER_COUNT}"

declare -a ORIGINS
for i in $(seq 1 "$FILTER_COUNT"); do
  origin="concurrent-create-$(printf '%02d' "$i")-$(uuidgen | tr '[:upper:]' '[:lower:]')"
  ORIGINS+=("$origin")
  yaml_file="${TMPDIR}/filter-${i}.yaml"
  ORIGIN="$origin" yq '.metadata.labels."dash0.com/origin" = env(ORIGIN)' "$FIXTURE" > "$yaml_file"
done

echo "--- Step 1: Launching ${FILTER_COUNT} concurrent 'spam-filters create' processes ---"
declare -a PIDS
for i in $(seq 1 "$FILTER_COUNT"); do
  "$DASH0" -X spam-filters create -f "${TMPDIR}/filter-${i}.yaml" \
    > "${TMPDIR}/out-${i}.log" 2>&1 &
  PIDS+=($!)
done

echo "--- Step 2: Waiting for all processes and checking exit codes ---"
FAILED=0
for i in $(seq 1 "$FILTER_COUNT"); do
  pid="${PIDS[$((i - 1))]}"
  if ! wait "$pid"; then
    echo "FAIL: concurrent create #${i} (origin ${ORIGINS[$((i - 1))]}) exited non-zero:"
    cat "${TMPDIR}/out-${i}.log"
    FAILED=$((FAILED + 1))
  else
    echo "  #${i} (origin ${ORIGINS[$((i - 1))]}): OK"
  fi
done

if [ "$FAILED" -gt 0 ]; then
  echo "FAIL: ${FAILED}/${FILTER_COUNT} concurrent creates failed — likely the unretried 409 dataset version conflict from issue #261."
  # Best-effort cleanup of whichever ones did succeed.
  for origin in "${ORIGINS[@]}"; do
    "$DASH0" -X spam-filters delete "$origin" --force > /dev/null 2>&1 || true
  done
  exit 1
fi

echo "All ${FILTER_COUNT} concurrent creates converged in a single run."

echo "--- Step 3: Verifying all ${FILTER_COUNT} filters exist via the CLI ---"
LIST_JSON=$("$DASH0" -X spam-filters list --all -o json)
for origin in "${ORIGINS[@]}"; do
  count=$(echo "$LIST_JSON" | jq --arg o "$origin" '[.[] | select(.metadata.labels["dash0.com/origin"] == $o)] | length')
  if [ "$count" != "1" ]; then
    echo "FAIL: expected exactly 1 record with origin '$origin', got $count"
    exit 1
  fi
done
echo "All ${FILTER_COUNT} filters verified."

echo "--- Step 4: Cleaning up ---"
CLEANUP_FAILED=0
for origin in "${ORIGINS[@]}"; do
  if ! "$DASH0" -X spam-filters delete "$origin" --force > /dev/null; then
    echo "FAIL: cleanup delete failed for origin '$origin'"
    CLEANUP_FAILED=1
  fi
done
if [ "$CLEANUP_FAILED" -ne 0 ]; then
  exit 1
fi

echo "=== Spam filter concurrent create test PASSED ==="
